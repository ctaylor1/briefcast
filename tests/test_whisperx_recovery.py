"""Recovery archive integrity and non-destructive restoration checks."""

import contextlib
import hashlib
import importlib.util
import io
import json
import os
import sqlite3
import tarfile
import uuid
from pathlib import Path

import pytest

SOURCE = Path(__file__).resolve().parents[1] / "scripts/whisperx_recovery.py"
spec = importlib.util.spec_from_file_location("whisperx_recovery", SOURCE)
recovery = importlib.util.module_from_spec(spec)
spec.loader.exec_module(recovery)


@pytest.fixture
def state(tmp_path, monkeypatch):
    state = tmp_path / "state"
    state.mkdir()
    with sqlite3.connect(state / "journal.sqlite") as db:
        db.execute("CREATE TABLE metadata(key TEXT PRIMARY KEY, value TEXT)")
        db.execute("CREATE TABLE jobs(id TEXT PRIMARY KEY,state TEXT,data BLOB)")
        db.executemany(
            "INSERT INTO metadata VALUES (?,?)",
            [("schema_version", "1"), ("accepting", "0"), ("instance_id", str(uuid.uuid4()))],
        )
    if os.name == "nt":
        # Linux flock is checked in the worker-runtime job, not emulated here.
        monkeypatch.setattr(recovery, "ownership", lambda state: contextlib.nullcontext())
    return state


def test_snapshot_restore_preserves_full_state_and_refuses_overwrite(state, tmp_path):
    job = state / "jobs" / str(uuid.uuid4())
    job.mkdir(parents=True)
    (job / "diagnostic.log").write_bytes(b"private retained artifact")
    snapshot = tmp_path / "snapshot"
    expected = recovery.snapshot(state, snapshot, "sha256:" + "a" * 64)
    target = tmp_path / "restored"
    assert recovery.restore(snapshot, target) == expected
    assert (target / job.relative_to(state) / "diagnostic.log").read_bytes() == (
        job / "diagnostic.log"
    ).read_bytes()
    with pytest.raises(FileExistsError):
        recovery.restore(snapshot, state)
    with pytest.raises(FileExistsError):
        recovery.snapshot(state, snapshot, "sha256:" + "a" * 64)


@pytest.mark.parametrize("field,value", [("schema_version", "2"), ("accepting", "1")])
def test_unknown_or_accepting_state_is_rejected(state, tmp_path, field, value):
    with sqlite3.connect(state / "journal.sqlite") as db:
        db.execute("UPDATE metadata SET value=? WHERE key=?", (value, field))
    with pytest.raises(ValueError):
        recovery.snapshot(state, tmp_path / "snapshot", "sha256:" + "a" * 64)
    assert not (tmp_path / "snapshot").exists()


def test_running_job_and_missing_result_are_rejected(state, tmp_path):
    job = str(uuid.uuid4())
    record = {"status": {"job_id": job, "state": "running"}}
    with sqlite3.connect(state / "journal.sqlite") as db:
        db.execute("INSERT INTO jobs VALUES (?,?,?)", (job, "running", json.dumps(record)))
    with pytest.raises(ValueError):
        recovery.snapshot(state, tmp_path / "snapshot", "sha256:" + "a" * 64)
    record["status"]["state"] = "succeeded"
    with sqlite3.connect(state / "journal.sqlite") as db:
        db.execute("UPDATE jobs SET state='succeeded',data=?", (json.dumps(record),))
    with pytest.raises(FileNotFoundError):
        recovery.snapshot(state, tmp_path / "snapshot", "sha256:" + "a" * 64)


def test_corrupt_snapshot_rejected_before_destination_created(state, tmp_path):
    snapshot = tmp_path / "snapshot"
    recovery.snapshot(state, snapshot, "sha256:" + "a" * 64)
    with (snapshot / "state.tar").open("ab") as stream:
        stream.write(b"corruption")
    with pytest.raises(ValueError):
        recovery.restore(snapshot, tmp_path / "restored")
    assert not (tmp_path / "restored").exists()


@pytest.mark.parametrize("name", ["../outside", "/absolute", "C:/drive", "a\\b", "a/../b"])
def test_traversal_even_in_rehashed_archive_is_rejected(tmp_path, name):
    snapshot = tmp_path / "snapshot"
    snapshot.mkdir()
    archive = snapshot / "state.tar"
    data = b"danger"
    with tarfile.open(archive, "w") as tar:
        member = tarfile.TarInfo(name)
        member.size = len(data)
        tar.addfile(member, io.BytesIO(data))
    manifest = {
        "schema": 1,
        "archive_sha256": recovery.digest(archive),
        "files": {name: {"size": len(data), "sha256": hashlib.sha256(data).hexdigest()}},
    }
    (snapshot / "manifest.json").write_text(json.dumps(manifest))
    with pytest.raises(ValueError):
        recovery.restore(snapshot, tmp_path / "restored")
    assert not (tmp_path / "restored").exists()


@pytest.mark.skipif(os.name == "nt", reason="real supervisor flock runs in Linux worker runtime")
def test_snapshot_cannot_take_live_state_ownership(state, tmp_path):
    with recovery.ownership(state), pytest.raises(BlockingIOError):
        recovery.snapshot(state, tmp_path / "snapshot", "sha256:" + "a" * 64)
