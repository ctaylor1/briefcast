"""Worker-only durability changes around the existing inference pipeline."""

import json

import pytest

import test_whisperx_pipeline_characterization as characterization

pipeline = characterization.pipeline


@pytest.fixture
def worker_mode(pipeline, monkeypatch):
    context = {
        "format": "briefcast-whisperx-chunks-v1",
        "input_sha256": "a" * 64,
        "options_sha256": "b" * 64,
        "pipeline_version": "briefcast-script-v1",
        "engine_profile": "test-profile",
        "deployment_id": "test-deployment",
        "episode_id": "test-episode",
        "generation_id": "test-generation",
    }
    monkeypatch.setenv("WHISPERX_WORKER_MODE", "1")
    monkeypatch.setenv("WHISPERX_WORKER_CONTEXT", json.dumps(context))
    return context


def test_worker_success_retains_fingerprinted_checkpoint(pipeline, worker_mode, capsys):
    assert pipeline.module.main() == 0
    result = json.loads(capsys.readouterr().out)
    assert result["provider"] == "whisperx"
    saved = json.loads(pipeline.segments.read_text())
    assert saved["worker"] == worker_mode
    assert saved["completed_seconds"] == 300
    assert len(saved["segments"]) == 3
    assert result["metadata"]["transcribed_at"].endswith("Z")
    assert result["provenance"]["inputs"][0]["sha256"] == pipeline.module.file_sha256(
        pipeline.segments.parent / "episode.mp3"
    )


def test_worker_complete_checkpoint_retains_request_model_without_new_asr(
    pipeline, worker_mode, monkeypatch, capsys
):
    monkeypatch.setattr(
        pipeline.module,
        "load_resume_checkpoint",
        lambda: ([{"start": 0, "end": 300, "text": "Completed earlier."}], 300, "en", ""),
    )
    assert pipeline.module.main() == 0
    result = json.loads(capsys.readouterr().out)
    assert pipeline.model.calls == 0
    assert result["model"] == pipeline.config["model"]
    assert result["metadata"]["asr_performed"] is False
    assert result["metadata"]["transcribed_at"] is None
    assert "transcription_model" not in result["provenance"]


def test_worker_checkpoint_failure_cannot_publish_success(
    pipeline, worker_mode, monkeypatch, capsys
):
    original = pipeline.module.write_json_file

    def fail_segments(path, payload):
        if path == str(pipeline.segments):
            raise OSError("synthetic disk full")
        original(path, payload)

    monkeypatch.setattr(pipeline.module, "write_json_file", fail_segments)
    assert pipeline.module.main() == 1
    assert json.loads(capsys.readouterr().out)["error_type"] == "OSError"
    assert not pipeline.segments.exists()


def test_worker_progress_failure_does_not_return_success(pipeline, worker_mode, monkeypatch):
    def fail_write(path, payload):
        raise OSError("synthetic progress persistence failure")

    monkeypatch.setattr(pipeline.module, "write_json_file", fail_write)
    with pytest.raises(OSError):
        pipeline.module.main()


def test_worker_stop_quiesces_at_durable_chunk_boundary(
    pipeline, worker_mode, monkeypatch, tmp_path, capsys
):
    stop = tmp_path / "stop"
    stop.write_text("stop")
    monkeypatch.setenv("WHISPERX_STOP_FILE", str(stop))
    assert pipeline.module.main() == 1
    assert json.loads(capsys.readouterr().out)["error_type"] == "WorkerQuiesced"
    saved = json.loads(pipeline.segments.read_text())
    assert saved["completed_seconds"] == 120
    assert pipeline.starts == [(0.0, 120.0)]
