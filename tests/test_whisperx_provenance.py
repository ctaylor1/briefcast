"""Exercise the actual script orchestration with lightweight runtime doubles."""

import importlib.util
import json
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


@pytest.mark.parametrize("resume_seconds", [0.0, 0.5, 1.0])
def test_whisperx_records_real_stages_and_exact_inputs(monkeypatch, tmp_path, resume_seconds):
    scripts = Path(__file__).resolve().parents[1] / "scripts"
    monkeypatch.syspath_prepend(str(scripts))
    monkeypatch.setitem(sys.modules, "numpy", ModuleType("numpy"))
    spec = importlib.util.spec_from_file_location(
        "whisperx_provenance_fixture", scripts / "whisperx_transcribe.py"
    )
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)

    class Audio:
        def __len__(self):
            return 16000

        def tobytes(self):
            return b"exact decoded PCM fixture"

    class Model:
        def transcribe(self, audio, batch_size):
            assert audio.tobytes() == b"exact decoded PCM fixture"
            return {
                "language": "en",
                "segments": [{"start": 0.1, "end": 0.3, "text": "A qualified claim."}],
            }

    def align(segments, *_args, **_kwargs):
        return {"segments": [dict(segment, start=segment["start"] + 0.01) for segment in segments]}

    runtime = SimpleNamespace(
        load_model=lambda *_args, **_kwargs: Model(),
        load_audio=lambda *_args: Audio(),
        load_align_model=lambda **_kwargs: (object(), {}),
        align=align,
    )
    config = module.default_config()
    config.update({"align": True, "diarization": False, "device": "cpu"})
    output = []
    audio = tmp_path / "audio.mp3"
    audio.write_bytes(b"exact original compressed audio fixture")
    monkeypatch.setattr(module, "setup_logging", lambda **_kwargs: None)
    monkeypatch.setattr(
        module,
        "load_runtime_modules",
        lambda: (SimpleNamespace(cuda=SimpleNamespace(is_available=lambda: False)), runtime),
    )
    monkeypatch.setattr(module, "load_config", lambda: config)
    monkeypatch.setattr(module, "installed_version", lambda package, *_args: f"fixture-{package}-1")
    monkeypatch.setattr(module, "probe_audio_duration_seconds", lambda _path: 1.0)
    monkeypatch.setattr(module, "load_audio_chunk", lambda *_args: Audio())
    monkeypatch.setattr(module, "emit_progress", lambda *_args, **_kwargs: None)
    monkeypatch.setattr(module, "emit_json", output.append)
    monkeypatch.setattr(
        module.subprocess,
        "run",
        lambda *_args, **_kwargs: SimpleNamespace(stdout="ffmpeg version fixture-1\n"),
    )
    monkeypatch.setattr(
        module,
        "load_resume_checkpoint",
        lambda: (
            [{"start": 0.0, "end": 0.4, "text": "Previous."}] if resume_seconds else [],
            resume_seconds,
            "en",
            "",
        ),
    )
    monkeypatch.setattr(sys, "argv", [str(scripts / "whisperx_transcribe.py"), str(audio)])
    assert module.main() == 0
    payload = output[-1]
    assert payload["metadata"]["whisperx_version"] == "fixture-whisperx-1"
    assert payload["metadata"]["faster_whisper_version"] == "fixture-faster-whisper-1"
    provenance = payload["provenance"]
    assert payload["source_media"]["sha256"] == module.file_sha256(audio)
    events = provenance["processing_events"]
    if resume_seconds == 1.0:
        assert payload["metadata"]["transcribed_at"] is None
        assert payload["metadata"]["asr_performed"] is False
        assert payload["model"] == ""
        assert [event["operation"] for event in events] == ["checkpoint_import", "alignment"]
        assert provenance["inputs"][0]["kind"] == "transcript"
        return
    assert payload["metadata"]["transcribed_at"].endswith("Z")
    assert provenance["inputs"][0]["sha256"] == module.file_sha256(audio)
    assert [event["operation"] for event in events] == [
        "audio_preprocessing",
        "transcription",
        "alignment",
    ]
    assert events[1]["completed_at"] == payload["metadata"]["transcribed_at"]
    assert events[2]["inputs"][0]["sha256"] == events[1]["output"]["sha256"]
    assert events[1]["inputs"][0]["sha256"] == events[0]["output"]["sha256"]
    assert str(tmp_path) not in json.dumps(payload)
    if resume_seconds:
        assert provenance["inputs"][0]["coverage"] == "partial"
        assert provenance["inherited_provenance_missing"] == ["resumed_asr_history"]
        assert events[1]["inputs"][1]["coverage"] == "unknown"
