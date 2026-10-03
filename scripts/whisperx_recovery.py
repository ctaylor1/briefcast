"""Offline worker-state snapshots. Run in the Linux worker image, never on live state."""

import argparse
import contextlib
import hashlib
import json
import os
import re
import shutil
import sqlite3
import tarfile
import uuid
from pathlib import Path, PurePosixPath


def digest(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def relative(name):
    path = PurePosixPath(name)
    if (
        not name
        or path.is_absolute()
        or "\\" in name
        or ":" in name
        or any(part in ("", ".", "..") for part in name.split("/"))
    ):
        raise ValueError("unsafe snapshot member")
    return path


@contextlib.contextmanager
def ownership(state):
    # Same flock as the supervisor and its descendants. Stopping the HTTP
    # process alone is not sufficient if an inference child still owns state.
    import fcntl

    with (state / "owner.lock").open("a+b") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        yield


def inspect_state(state):
    journal = state / "journal.sqlite"
    if not journal.is_file() or journal.is_symlink():
        raise ValueError("missing regular worker journal")
    with sqlite3.connect(journal.as_uri() + "?mode=ro", uri=True) as db:
        if db.execute("PRAGMA integrity_check").fetchall() != [("ok",)]:
            raise ValueError("worker journal integrity failed")
        metadata = dict(db.execute("SELECT key,value FROM metadata"))
        if metadata.get("schema_version") != "1" or metadata.get("accepting") != "0":
            raise ValueError("snapshot requires supported, drained state")
        uuid.UUID(metadata["instance_id"])
        counts = {}
        for job_id, status, raw in db.execute("SELECT id,state,data FROM jobs"):
            uuid.UUID(job_id)
            if status not in ("succeeded", "failed", "cancelled"):
                raise ValueError("snapshot requires no queued/running jobs")
            counts[status] = counts.get(status, 0) + 1
            record = json.loads(raw)
            if record["status"]["job_id"] != job_id or record["status"]["state"] != status:
                raise ValueError("journal job identity mismatch")
            checkpoint = record.get("checkpoint")
            if checkpoint:
                path = state / relative(checkpoint["path"])
                if path.is_symlink() or digest(path) != checkpoint["sha256"]:
                    raise ValueError("checkpoint integrity failed")
            if status == "succeeded" and not record.get("pruned", False):
                path = state / "jobs" / job_id / "result.json"
                if path.is_symlink():
                    raise ValueError("unsafe result")
                result = json.loads(path.read_bytes())
                if (
                    result["contract_version"] != 1
                    or result["job_id"] != job_id
                    or result["result_sha256"] != record["result_digest"]
                    or result["request_digest"] != record["status"]["request_digest"]
                ):
                    raise ValueError("result identity mismatch")
    return {"schema": 1, "instance_id": metadata["instance_id"], "jobs": counts}


def inventory(state):
    files = {}
    for path in sorted(state.rglob("*")):
        if path.is_symlink() or not (path.is_file() or path.is_dir()):
            raise ValueError("state contains symlink or special file")
        if path.is_file():
            name = path.relative_to(state).as_posix()
            relative(name)
            files[name] = {"size": path.stat().st_size, "sha256": digest(path)}
    return files


def snapshot(state, destination, image_id):
    state, destination = state.resolve(), destination.resolve()
    if destination == state or state in destination.parents:
        raise ValueError("snapshot must be outside state")
    if not re.fullmatch(r"sha256:[a-f0-9]{64}", image_id):
        raise ValueError("immutable source image ID required")
    with ownership(state):
        identity = inspect_state(state)
        files = inventory(state)
        required = sum(item["size"] for item in files.values()) + 16 * 1024 * 1024
        if shutil.disk_usage(destination.parent).free < required:
            raise ValueError("insufficient snapshot space")
        destination.mkdir(mode=0o700)  # Never overwrite a prior recovery point.
        archive = destination / "state.tar"
        with tarfile.open(archive, "w") as tar:
            for name in files:
                tar.add(state / name, arcname=name, recursive=False)
        with archive.open("r+b") as stream:
            os.fsync(stream.fileno())
        manifest = {
            **identity,
            "image_id": image_id,
            "archive_sha256": digest(archive),
            "files": files,
        }
        with (destination / "manifest.json").open("x") as stream:
            stream.write(json.dumps(manifest, indent=2) + "\n")
            stream.flush()
            os.fsync(stream.fileno())
        if os.name != "nt":
            directory = os.open(destination, os.O_RDONLY | os.O_DIRECTORY)
            try:
                os.fsync(directory)
            finally:
                os.close(directory)
        verify(destination)
    return identity


def verify(snapshot_dir):
    manifest = json.loads((snapshot_dir / "manifest.json").read_bytes())
    archive = snapshot_dir / "state.tar"
    if manifest["schema"] != 1 or digest(archive) != manifest["archive_sha256"]:
        raise ValueError("snapshot checksum/schema mismatch")
    seen = set()
    with tarfile.open(archive, "r:") as tar:
        for member in tar:
            relative(member.name)
            if not member.isfile() or member.name in seen:
                raise ValueError("snapshot contains nonregular or duplicate member")
            seen.add(member.name)
            expected = manifest["files"][member.name]
            if member.size != expected["size"]:
                raise ValueError("snapshot size mismatch")
            with tar.extractfile(member) as stream:
                if hashlib.file_digest(stream, "sha256").hexdigest() != expected["sha256"]:
                    raise ValueError("snapshot member checksum mismatch")
    if seen != set(manifest["files"]):
        raise ValueError("snapshot inventory mismatch")
    return manifest


def restore(snapshot_dir, destination):
    manifest = verify(snapshot_dir)
    required = sum(item["size"] for item in manifest["files"].values()) + 16 * 1024 * 1024
    if shutil.disk_usage(destination.parent).free < required:
        raise ValueError("insufficient restore space")
    # New directory only: no replacement, deletion or rollback of newer jobs.
    destination.mkdir(mode=0o700)
    with tarfile.open(snapshot_dir / "state.tar", "r:") as tar:
        for member in tar:
            target = destination / relative(member.name)
            target.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
            with tar.extractfile(member) as source, target.open("xb") as output:
                shutil.copyfileobj(source, output)
                output.flush()
                os.fsync(output.fileno())
            os.chmod(target, 0o600)
    with ownership(destination):
        identity = inspect_state(destination.resolve())
        if (
            identity["instance_id"] != manifest["instance_id"]
            or identity["jobs"] != manifest["jobs"]
        ):
            raise ValueError("restored state identity mismatch")
    return identity


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("operation", choices=("snapshot", "verify", "restore"))
    parser.add_argument("--state", type=Path)
    parser.add_argument("--snapshot", type=Path, required=True)
    parser.add_argument("--image-id")
    args = parser.parse_args()
    if args.operation == "snapshot":
        if args.state is None or args.image_id is None:
            parser.error("snapshot requires --state and --image-id")
        result = snapshot(args.state, args.snapshot, args.image_id)
    elif args.operation == "restore":
        if args.state is None:
            parser.error("restore requires a new --state directory")
        result = restore(args.snapshot, args.state)
    else:
        result = verify(args.snapshot)
        result = {key: result[key] for key in ("schema", "instance_id", "jobs")}
    print(json.dumps(result))


if __name__ == "__main__":
    # Recovery paths can contain private identifiers. Avoid traceback/logging
    # arbitrary file content on operator/CI errors; retain artifacts privately.
    try:
        main()
    except (OSError, ValueError, KeyError, sqlite3.Error, tarfile.TarError):
        raise SystemExit(
            "Recovery failed; verify quiescence, permissions and snapshot integrity."
        ) from None
