"""Secrets stay in private captures; restoration never targets the live server."""

import importlib.util
import io
import json
import tarfile
from pathlib import Path
from types import SimpleNamespace

import pytest

SOURCE = Path(__file__).resolve().parents[1] / "scripts/briefcast_recovery.py"
spec = importlib.util.spec_from_file_location("briefcast_recovery", SOURCE)
recovery = importlib.util.module_from_spec(spec)
spec.loader.exec_module(recovery)


def test_capture_uses_private_password_file_and_never_secret_arguments(
    tmp_path, monkeypatch, capsys
):
    destination = tmp_path / "private"
    calls = []
    image = "sha256:" + "a" * 64
    raw = [
        {
            "Config": {
                "Env": [
                    "DATABASE_URL=postgres://operator:private-secret@host.docker.internal/briefcast?sslmode=disable"
                ]
            }
        }
    ]

    def command(*args, **kwargs):
        args = list(map(str, args))
        calls.append(args)
        if args[:2] == ["docker", "inspect"]:
            return image if "--format" in args else json.dumps(raw)
        if "{{.Size}}" in args:
            return "1"
        if "{{.Id}}" in args:
            return image
        if "server_version_num" in args[-1]:
            return "170000"
        if "pg_dump" in args[-1]:
            (destination / "database.dump").write_bytes(b"custom archive fixture")
        if "rev-parse" in args:
            return "b" * 40
        if "bundle" in args and "create" in args:
            (destination / "source.bundle").write_bytes(b"bundle fixture")
        return ""

    monkeypatch.setattr(recovery, "command", command)
    monkeypatch.setattr(recovery.shutil, "disk_usage", lambda path: SimpleNamespace(free=2**40))

    def copy_archive(*args, **kwargs):
        with tarfile.open(fileobj=kwargs["stdout"], mode="w") as tar:
            member = tarfile.TarInfo("config")
            member.type = tarfile.DIRTYPE
            tar.addfile(member)
        return SimpleNamespace(returncode=0, stderr=b"")

    monkeypatch.setattr(recovery.subprocess, "run", copy_archive)
    recovery.capture(
        SimpleNamespace(
            destination=destination,
            container="fixture",
            postgres_image="fixture-postgres",
            config_file=[],
            skip_image=True,
        )
    )
    assert "private-secret" not in capsys.readouterr().out
    assert all("private-secret" not in " ".join(call) for call in calls)
    assert "private-secret" in (destination / "pgpass.private").read_text()
    assert "PGPASSWORD" not in (destination / "postgres.private.env").read_text()
    manifest = json.loads((destination / "manifest.json").read_text())
    assert not manifest["coherent_media_checkpoint"]
    assert "image_archive_sha256" not in manifest


def test_database_restore_uses_unique_container_and_no_network(tmp_path, monkeypatch, capsys):
    archive = tmp_path / "database.dump"
    archive.write_bytes(b"fixture")
    manifest = {
        "database_sha256": recovery.checksum(archive),
        "postgres_image": "sha256:" + "a" * 64,
    }
    (tmp_path / "manifest.json").write_text(json.dumps(manifest))
    calls = []

    def command(*args, **kwargs):
        args = list(map(str, args))
        calls.append(args)
        if args[:2] == ["docker", "run"]:
            assert "none" in args and "--network" in args and "--publish" not in args
            return "isolated-container-id"
        if "SELECT tablename" in args[-1]:
            return "podcasts\npodcast_items\nsettings"
        if args[-1].startswith("SELECT count"):
            return "42"
        return ""

    monkeypatch.setattr(recovery, "command", command)
    monkeypatch.setattr(
        recovery.subprocess, "run", lambda *args, **kwargs: SimpleNamespace(returncode=0)
    )
    recovery.verify_database(SimpleNamespace(destination=tmp_path))
    assert calls[-1] == ["docker", "stop", "isolated-container-id"]
    restoration = json.loads((tmp_path / "database-restore.json").read_text())
    assert restoration["tables"]["podcast_items"] == 42
    assert "42" not in capsys.readouterr().out


def test_database_checksum_rejected_before_any_docker_operation(tmp_path, monkeypatch):
    (tmp_path / "database.dump").write_bytes(b"corrupt")
    (tmp_path / "manifest.json").write_text(json.dumps({"database_sha256": "a" * 64}))
    monkeypatch.setattr(recovery, "command", lambda *args, **kwargs: pytest.fail("Docker called"))
    with pytest.raises(ValueError):
        recovery.verify_database(SimpleNamespace(destination=tmp_path))


def test_recovery_directory_cannot_overwrite_repository_or_existing_backup(tmp_path):
    for destination in [recovery.ROOT, recovery.ROOT / "capture", tmp_path]:
        with pytest.raises(ValueError):
            recovery.private_directory(destination)


def test_portable_configuration_restore_dereferences_internal_cache_links(tmp_path):
    archive = tmp_path / "config.tar"
    with tarfile.open(archive, "w") as tar:
        regular = tarfile.TarInfo("config/nested/blob")
        regular.size = 5
        tar.addfile(regular, io.BytesIO(b"model"))
        link = tarfile.TarInfo("config/nested/snapshot/file")
        link.type = tarfile.SYMTYPE
        link.linkname = "../blob"
        tar.addfile(link)
    destination = tmp_path / "restored"
    recovery.unpack_config(archive, destination)
    assert (destination / "nested/snapshot/file").read_bytes() == b"model"
    assert not (destination / "nested/snapshot/file").is_symlink()


def test_configuration_restore_refuses_links_outside_archive(tmp_path):
    archive = tmp_path / "config.tar"
    with tarfile.open(archive, "w") as tar:
        link = tarfile.TarInfo("config/link")
        link.type = tarfile.SYMTYPE
        link.linkname = "../../outside"
        tar.addfile(link)
    with pytest.raises(ValueError):
        recovery.unpack_config(archive, tmp_path / "restored")
    assert not (tmp_path / "outside").exists()


def test_cleanup_attempts_all_commands_and_preserves_original_failure(tmp_path, monkeypatch):
    calls = []

    def fail(*args, **kwargs):
        calls.append(args)
        raise RuntimeError("cleanup failure")

    monkeypatch.setattr(recovery, "command", fail)
    with pytest.raises(ValueError, match="original failure"):
        try:
            raise ValueError("original failure")
        finally:
            recovery.cleanup_commands([("first",), ("second",)], tmp_path)
    assert calls == [("first",), ("second",)]
    assert (tmp_path / "cleanup-incomplete.txt").exists()
    with pytest.raises(RuntimeError, match="cleanup incomplete"):
        recovery.cleanup_commands([("third",)], tmp_path)
