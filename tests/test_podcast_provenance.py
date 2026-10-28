import importlib.util
from pathlib import Path
from unittest.mock import patch

SCRIPT = Path(__file__).resolve().parents[1] / "scripts" / "podcast_provenance.py"
SPEC = importlib.util.spec_from_file_location("podcast_provenance", SCRIPT)
assert SPEC and SPEC.loader
provenance = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(provenance)


def test_exact_snapshot_hash_and_stable_identity(tmp_path):
    text = "A qualification.\nSecond line: café.\n"
    snapshot = provenance.text_snapshot("transcript", text)
    path = tmp_path / "snapshot.txt"
    path.write_bytes(text.encode("utf-8"))
    assert snapshot["sha256"] == provenance.file_sha256(path)
    assert snapshot["id"] == provenance.text_snapshot("transcript", text)["id"]
    assert snapshot["id"] != provenance.text_snapshot("transcript", text + "edited")["id"]


def test_processing_events_preserve_separate_stages():
    first = provenance.segment_snapshot([{"start": 1.0, "text": "A claim"}])
    second = provenance.segment_snapshot([{"start": 1.1, "text": "A claim"}])
    when = "2026-10-27T12:00:00Z"
    event = provenance.processing_event("alignment", when, [first], second, "WhisperX", "fixture-1")
    assert event["operation"] == "alignment"
    assert event["inputs"][0]["sha256"] == first["sha256"]
    assert event["output"]["sha256"] == second["sha256"]
    assert event["completed_at"] == when


def test_installed_version_never_invents_unknown():
    with patch.object(provenance, "version", side_effect=provenance.PackageNotFoundError):
        try:
            provenance.installed_version("missing")
        except RuntimeError as error:
            assert "Cannot capture" in str(error)
        else:
            raise AssertionError("missing software version was fabricated")


def test_local_model_uses_portable_identity_and_real_file_digest(tmp_path):
    model = tmp_path / "local-asr"
    model.mkdir()
    (model / "model.bin").write_bytes(b"fixture weights")
    identifier, revision = provenance.model_identity(str(model))
    assert identifier == "local-model:local-asr"
    assert len(revision) == 64
    (model / "model.bin").write_bytes(b"changed fixture weights")
    assert provenance.model_identity(str(model))[1] != revision
