"""Private monolith capture and isolated PostgreSQL dump verification.

This is an initial logical backup, not a database/media consistency checkpoint.
The app must be stopped separately for the final reviewed cutover snapshot.
"""

import argparse
import getpass
import hashlib
import json
import os
import posixpath
import secrets
import shutil
import subprocess
import sys
import tarfile
import uuid
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def command(*args, input=None, log_dir=None):
    result = subprocess.run(
        list(map(str, args)), input=input, capture_output=True, text=True, timeout=3600
    )
    if result.returncode:
        if log_dir:
            (log_dir / "private-error.log").write_text(result.stderr)
        raise RuntimeError("Recovery command failed; inspect private diagnostics")
    return result.stdout.strip()


def checksum(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def cleanup_commands(commands, destination):
    """Attempt every cleanup; preserve the original failure when one is active."""
    failed = False
    original = sys.exception()
    for arguments in commands:
        try:
            command(*arguments, log_dir=destination)
        except (OSError, RuntimeError, subprocess.TimeoutExpired):
            failed = True
    if failed:
        (destination / "cleanup-incomplete.txt").write_text(
            "Inspect this rehearsal's recorded container IDs and network; cleanup was incomplete.\n"
        )
        if original is None:
            raise RuntimeError("Isolated recovery cleanup incomplete; inspect restricted artifacts")


def private_directory(path):
    path = path.resolve()
    if path.exists() or path == ROOT or ROOT in path.parents:
        raise ValueError("recovery destination must be NEW and outside the repository")
    path.mkdir(mode=0o700)
    if os.name == "nt":
        command("icacls", path, "/inheritance:r", "/grant:r", getpass.getuser() + ":(OI)(CI)F")
    return path


def capture(args):
    destination = private_directory(args.destination)
    image = command("docker", "inspect", "--format", "{{.Image}}", args.container)
    if not image.startswith("sha256:"):
        raise ValueError("cannot resolve running image")
    image_bytes = int(command("docker", "image", "inspect", "--format", "{{.Size}}", image))
    if shutil.disk_usage(destination).free < image_bytes * 2 + 512 * 1024 * 1024:
        raise ValueError("insufficient recovery space for image and rehearsal copies")
    record = {"schema": 1, "image_id": image, "coherent_media_checkpoint": False}
    # Capture secrets directly into restricted storage, never print raw inspect
    # environments, database URLs, dotenv contents or psql connection errors.
    raw = command("docker", "inspect", args.container, log_dir=destination)
    (destination / "container.private.json").write_text(raw)
    (destination / "runtime-diff.private.txt").write_text(command("docker", "diff", args.container))
    config = json.loads(raw)[0]
    environment = dict(item.split("=", 1) for item in config["Config"]["Env"] if "=" in item)
    url = environment.get("DATABASE_URL", "")
    from urllib.parse import parse_qs, unquote, urlsplit

    parsed = urlsplit(url)
    if parsed.scheme not in ("postgres", "postgresql"):
        raise ValueError("this recovery operation requires the actual PostgreSQL deployment")
    options = parse_qs(parsed.query)
    pg = {
        "PGHOST": parsed.hostname,
        "PGPORT": str(parsed.port or 5432),
        "PGUSER": unquote(parsed.username or ""),
        "PGDATABASE": unquote(parsed.path.lstrip("/")),
        "PGSSLMODE": options.get("sslmode", ["prefer"])[0],
    }
    if any(not value or "\n" in value or "\r" in value for value in pg.values()):
        raise ValueError("unsupported database connection configuration")
    if set(options) - {"sslmode"}:
        raise ValueError("additional database connection options need an explicit capture recipe")
    (destination / "postgres.private.env").write_text(
        "".join(f"{key}={value}\n" for key, value in pg.items())
    )
    fields = [
        pg["PGHOST"],
        pg["PGPORT"],
        pg["PGDATABASE"],
        pg["PGUSER"],
        unquote(parsed.password or ""),
    ]
    if any("\n" in value or "\r" in value for value in fields):
        raise ValueError("unsupported password-file format")
    (destination / "pgpass.private").write_text(
        ":".join(value.replace("\\", "\\\\").replace(":", "\\:") for value in fields) + "\n"
    )
    # Capture the selected client by ID too; no mutable tag in recovery evidence.
    pg_image = command("docker", "image", "inspect", "--format", "{{.Id}}", args.postgres_image)
    record["postgres_image"] = pg_image
    client = [
        "docker",
        "run",
        "--rm",
        "--add-host",
        "host.docker.internal:host-gateway",
        "--env-file",
        destination / "postgres.private.env",
        "--mount",
        f"type=bind,source={destination},target=/private",
        "--entrypoint",
        "sh",
        pg_image,
        "-ec",
    ]
    setup = "install -m 600 /private/pgpass.private /tmp/pgpass; export PGPASSFILE=/tmp/pgpass; "
    version = command(*client, setup + "psql -XAtqc 'SHOW server_version_num'", log_dir=destination)
    record["postgres_server_version_num"] = int(version)
    command(
        *client,
        setup + "pg_dump --format=custom --file=/private/database.dump",
        log_dir=destination,
    )
    record["database_sha256"] = checksum(destination / "database.dump")
    (destination / "manifest.json").write_text(json.dumps(record, indent=2) + "\n")
    finish_capture(args, record)


def finish_capture(args, record=None):
    destination = args.destination.resolve()
    if record is None:
        record = json.loads((destination / "manifest.json").read_text())
    image = record["image_id"]
    if checksum(destination / "database.dump") != record["database_sha256"]:
        raise ValueError("existing database capture checksum mismatch")
    if command("docker", "inspect", "--format", "{{.Image}}", args.container) != image:
        raise ValueError("runtime changed since initial capture")
    # Preserve Linux symlinks without requesting Windows symlink privileges.
    # Binary output goes directly to a Python file handle, not shell redirection.
    config_tar = destination / "config.tar"
    if not config_tar.exists():
        with config_tar.open("xb") as output:
            result = subprocess.run(
                ["docker", "cp", args.container + ":/config", "-"],
                stdout=output,
                stderr=subprocess.PIPE,
                timeout=3600,
            )
        if result.returncode:
            (destination / "private-error.log").write_bytes(result.stderr)
            raise RuntimeError("configuration archive capture failed")
    with tarfile.open(config_tar) as tar:
        record["config_archive_members"] = len(tar.getmembers())
    record["config_archive_sha256"] = checksum(config_tar)
    for index, file in enumerate(args.config_file):
        (destination / f"configuration-{index}.private").write_bytes(file.read_bytes())
    if not (destination / "source.bundle").exists():
        command(
            "git",
            "-C",
            ROOT,
            "bundle",
            "create",
            destination / "source.bundle",
            "--all",
            log_dir=destination,
        )
    command(
        "git", "-C", ROOT, "bundle", "verify", destination / "source.bundle", log_dir=destination
    )
    record["source_revision"] = command("git", "-C", ROOT, "rev-parse", "HEAD")
    record["source_dirty"] = bool(command("git", "-C", ROOT, "status", "--porcelain"))
    record["source_bundle_sha256"] = checksum(destination / "source.bundle")
    if not args.skip_image and not record.get("image_archive_sha256"):
        if (destination / "monolith.tar").exists():
            raise ValueError(
                "incomplete image archive exists; preserve it and select a reviewed new capture"
            )
        command(
            "docker",
            "image",
            "save",
            "--output",
            destination / "monolith.tar",
            image,
            log_dir=destination,
        )
        record["image_archive_sha256"] = checksum(destination / "monolith.tar")
        command(
            "docker", "image", "load", "--input", destination / "monolith.tar", log_dir=destination
        )
        if command("docker", "image", "inspect", "--format", "{{.Id}}", image) != image:
            raise ValueError("image reload identity mismatch")
        command(
            "docker",
            "run",
            "--rm",
            "--network",
            "none",
            "--entrypoint",
            "/api/app",
            image,
            "--version",
            log_dir=destination,
        )
    (destination / "manifest.json").write_text(json.dumps(record, indent=2) + "\n")
    print(
        "Initial private capture complete; media consistency, cache capture "
        "and restore verification remain required."
    )


def verify_database(args):
    destination = args.destination.resolve()
    manifest = json.loads((destination / "manifest.json").read_text())
    if checksum(destination / "database.dump") != manifest["database_sha256"]:
        raise ValueError("database archive checksum mismatch")
    name = "briefcast-restore-" + uuid.uuid4().hex
    secret = secrets.token_hex(32)
    env_file = destination / (name + ".private.env")
    env_file.write_text("POSTGRES_PASSWORD=" + secret + "\nPOSTGRES_DB=briefcast_restore\n")
    container = None
    try:
        container = command(
            "docker",
            "run",
            "-d",
            "--name",
            name,
            "--label",
            "briefcast.recovery.kind=database-restore",
            "--label",
            "briefcast.recovery.dump=" + manifest["database_sha256"],
            "--network",
            "none",
            "--env-file",
            env_file,
            "--mount",
            f"type=bind,source={destination},target=/backup,readonly",
            manifest["postgres_image"],
            log_dir=destination,
        )
        import time

        for _ in range(60):
            result = subprocess.run(
                ["docker", "exec", container, "pg_isready", "-U", "postgres"], capture_output=True
            )
            if result.returncode == 0:
                break
            time.sleep(1)
        else:
            raise TimeoutError("isolated PostgreSQL readiness failed")
        command(
            "docker",
            "exec",
            container,
            "pg_restore",
            "--exit-on-error",
            "--no-owner",
            "--no-privileges",
            "-U",
            "postgres",
            "-d",
            "briefcast_restore",
            "/backup/database.dump",
            log_dir=destination,
        )
        # Exact counts from the restored dump, not approximate live statistics.
        query = "SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename"
        tables = command(
            "docker",
            "exec",
            container,
            "psql",
            "-XAt",
            "-U",
            "postgres",
            "-d",
            "briefcast_restore",
            "-c",
            query,
            log_dir=destination,
        ).splitlines()
        counts = {}
        for table in tables:
            identifier = '"' + table.replace('"', '""') + '"'
            counts[table] = int(
                command(
                    "docker",
                    "exec",
                    container,
                    "psql",
                    "-XAt",
                    "-U",
                    "postgres",
                    "-d",
                    "briefcast_restore",
                    "-c",
                    "SELECT count(*) FROM " + identifier,
                    log_dir=destination,
                )
            )
        if not {"podcasts", "podcast_items", "settings"}.issubset(counts):
            raise ValueError("expected application tables missing from restored database")
        restoration = {
            "schema": 1,
            "dump_sha256": manifest["database_sha256"],
            "container_id": container,
            "complete": True,
            "tables": counts,
            "ownership": "isolated postgres role; original roles/ACLs not restored",
        }
        (destination / "database-restore.json").write_text(json.dumps(restoration, indent=2) + "\n")
        print(
            "Isolated PostgreSQL restore and exact table counts verified; "
            "representative records/app boot remain a separate gate."
        )
    finally:
        if container:
            # Only the unique ID returned by this invocation; no database volume
            # pruning or cleanup of any pre-existing container.
            cleanup_commands([("docker", "stop", container)], destination)


def unpack_config(archive, destination):
    """Portable restore: dereference only links to members inside this archive."""
    destination.mkdir(mode=0o700)
    seen = set()
    with tarfile.open(archive) as tar:
        for member in tar:
            parts = member.name.split("/")
            if (
                parts[0] != "config"
                or ".." in parts
                or "\\" in member.name
                or ":" in member.name
                or member.name in seen
            ):
                raise ValueError("unsafe configuration archive")
            seen.add(member.name)
            target = destination.joinpath(*parts[1:])
            if member.isdir():
                target.mkdir(mode=0o700, parents=True, exist_ok=True)
                continue
            if member.issym() or member.islnk():
                link = posixpath.normpath(
                    posixpath.join(posixpath.dirname(member.name), member.linkname)
                    if member.issym()
                    else member.linkname
                )
                if not link.startswith("config/"):
                    raise ValueError("configuration link escapes archive")
            elif not member.isfile():
                raise ValueError("unsupported configuration artifact")
            target.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
            with tar.extractfile(member) as source, target.open("xb") as output:
                shutil.copyfileobj(source, output)


def verify_app(args):
    destination = args.destination.resolve()
    manifest = json.loads((destination / "manifest.json").read_text())
    restoration = json.loads((destination / "database-restore.json").read_text())
    if (
        not restoration["complete"]
        or restoration["dump_sha256"] != manifest["database_sha256"]
        or checksum(destination / "config.tar") != manifest["config_archive_sha256"]
    ):
        raise ValueError("app rehearsal requires matching verified recovery artifacts")
    database = restoration["container_id"]
    label = command(
        "docker",
        "inspect",
        "--format",
        '{{index .Config.Labels "briefcast.recovery.dump"}}',
        database,
    )
    if label != manifest["database_sha256"]:
        raise ValueError("database container is not this isolated restoration")
    if command("docker", "inspect", "--format", "{{.State.Running}}", database) != "false":
        raise ValueError("stop the isolated restore server before boot rehearsal")
    mounts = json.loads(command("docker", "inspect", "--format", "{{json .Mounts}}", database))
    data = [
        mount
        for mount in mounts
        if mount["Destination"] == "/var/lib/postgresql/data" and mount["Type"] == "volume"
    ]
    if len(data) != 1:
        raise ValueError("expected private restored PostgreSQL data volume")
    volume = data[0]["Name"]
    if command("docker", "ps", "-q", "--filter", "volume=" + volume):
        raise ValueError("restored database volume is already in use")
    # Reuse only the secret generated for the restored server, never source
    # production settings. Find its file by the container's unique recorded name.
    name = command("docker", "inspect", "--format", "{{.Name}}", database).lstrip("/")
    if not name.startswith("briefcast-restore-") or "/" in name or "\\" in name:
        raise ValueError("unexpected restore container identity")
    values = dict(
        line.split("=", 1)
        for line in (destination / (name + ".private.env")).read_text().splitlines()
    )
    secret = values["POSTGRES_PASSWORD"]
    config = destination / ("config-restored-" + uuid.uuid4().hex)
    unpack_config(destination / "config.tar", config)
    assets = destination / "rehearsal-assets"
    assets.mkdir(mode=0o700, exist_ok=True)
    shutil.copyfile(ROOT / "tests/fixtures/whisperx/speech.flac", assets / "speech.flac")
    app_env = destination / (name + "-app.private.env")
    app_env.write_text(
        "DB_DRIVER=postgres\nDATABASE_URL=postgres://postgres:"
        + secret
        + "@restored-postgres:5432/briefcast_restore?sslmode=disable\n"
        "CONFIG=/config\nDATA=/assets\nWHISPERX_ENABLED=false\nLLM_ENABLED=false\nPASSWORD=\nLOG_OUTPUT=stdout\n"
    )
    network, app, server = None, None, None
    try:
        network = command(
            "docker", "network", "create", "--internal", name + "-network-" + uuid.uuid4().hex[:8]
        )
        # Reuse this VERIFIED private volume with only one active postgres.
        # Docker cannot attach a container created in `none` network mode to
        # a bridge; no data recopy, source-server access or volume deletion.
        server = command(
            "docker",
            "run",
            "-d",
            "--name",
            name + "-boot-" + uuid.uuid4().hex[:8],
            "--network",
            network,
            "--network-alias",
            "restored-postgres",
            "--env-file",
            destination / (name + ".private.env"),
            "--mount",
            f"type=volume,source={volume},target=/var/lib/postgresql/data",
            manifest["postgres_image"],
            log_dir=destination,
        )
        import time

        for _ in range(60):
            result = subprocess.run(
                ["docker", "exec", server, "pg_isready", "-U", "postgres"], capture_output=True
            )
            if result.returncode == 0:
                break
            time.sleep(1)
        else:
            raise TimeoutError("isolated database boot readiness failed")
        app = command(
            "docker",
            "run",
            "-d",
            "--name",
            name + "-app-" + uuid.uuid4().hex[:12],
            "--network",
            network,
            "--env-file",
            app_env,
            "--mount",
            f"type=bind,source={config},target=/config",
            "--mount",
            f"type=bind,source={assets},target=/assets",
            "--entrypoint",
            "/api/app",
            manifest["image_id"],
            log_dir=destination,
        )
        probe = "import urllib.request; assert urllib.request.urlopen('http://127.0.0.1:8080/',timeout=5).status==200"
        for _ in range(60):
            try:
                command("docker", "exec", app, "python", "-c", probe, log_dir=destination)
                break
            except RuntimeError:
                time.sleep(1)
        else:
            raise TimeoutError("archived monolith boot failed in isolation")
        probe = (
            "import json,urllib.request; "
            "catalog=json.load(urllib.request.urlopen("
            "'http://127.0.0.1:8080/podcasts',timeout=10)); "
            "assert catalog; "
            "assert urllib.request.urlopen('http://127.0.0.1:8080/settings',timeout=10).status==200"
        )
        command("docker", "exec", app, "python", "-c", probe, log_dir=destination)
        restoration["archived_app_boot"] = True
        restoration["catalog_and_settings_served"] = True
        restoration["media"] = "copied synthetic fixture only; production media unverified"
        (destination / "database-restore.json").write_text(json.dumps(restoration, indent=2) + "\n")
        print(
            "Archived app booted with restored database/config and copied fixture assets; "
            "production media recovery remains unverified."
        )
    finally:
        cleanup = []
        if app:
            cleanup.append(("docker", "stop", app))
        if server:
            cleanup.append(("docker", "stop", server))
        if network:
            if app:
                cleanup.append(("docker", "network", "disconnect", network, app))
            if server:
                cleanup.append(("docker", "network", "disconnect", network, server))
            cleanup.append(("docker", "network", "rm", network))
        cleanup_commands(cleanup, destination)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "operation", choices=("capture", "finish-capture", "verify-database", "verify-app")
    )
    parser.add_argument("--destination", type=Path, required=True)
    parser.add_argument("--container", default="briefcast")
    parser.add_argument("--postgres-image", default="postgres:17-alpine")
    parser.add_argument("--config-file", type=Path, action="append", default=[])
    parser.add_argument(
        "--skip-image",
        action="store_true",
        help="partial capture only; does not meet image recovery gate",
    )
    args = parser.parse_args()
    if args.operation == "capture":
        capture(args)
    elif args.operation == "finish-capture":
        finish_capture(args)
    elif args.operation == "verify-app":
        verify_app(args)
    else:
        verify_database(args)


if __name__ == "__main__":
    try:
        main()
    except (
        OSError,
        ValueError,
        RuntimeError,
        KeyError,
        subprocess.TimeoutExpired,
        tarfile.TarError,
    ):
        raise SystemExit(
            "Recovery incomplete. Inspect restricted artifacts; "
            "no production restore was attempted."
        ) from None
