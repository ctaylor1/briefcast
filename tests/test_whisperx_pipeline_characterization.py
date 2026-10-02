"""Exercise the real pipeline control flow without model or audio dependencies."""

import importlib.util
import json
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


@pytest.fixture
def pipeline(monkeypatch, tmp_path):
    # Audio decoding is replaced below. Importing the script must not require
    # installing NumPy, Torch or WhisperX into the repository tooling environment.
    monkeypatch.setitem(sys.modules, "numpy", ModuleType("numpy"))
    source = Path(__file__).resolve().parents[1] / "scripts" / "whisperx_transcribe.py"
    spec = importlib.util.spec_from_file_location("characterized_whisperx", source)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    monkeypatch.setattr(module, "setup_logging", lambda **kwargs: None)
    monkeypatch.setattr(module, "configure_third_party_logging", lambda: None)

    audio = tmp_path / "episode.mp3"
    audio.write_bytes(b"synthetic audio")
    segments = tmp_path / "segments.json"
    progress = tmp_path / "progress.json"
    monkeypatch.setattr(sys, "argv", [str(source), str(audio)])
    for key in ("WHISPERX_HF_TOKEN", "WHISPERX_RESUME_FILE", "WHISPERX_RESUME_JSON"):
        monkeypatch.delenv(key, raising=False)
    config = module.default_config()
    config.update(device="cpu", compute_type="int8", batch_size=4, align=False, diarization=False)
    monkeypatch.setenv("WHISPERX_CONFIG_JSON", json.dumps(config))
    monkeypatch.setenv("WHISPERX_SEGMENTS_FILE", str(segments))
    monkeypatch.setenv("WHISPERX_PROGRESS_FILE", str(progress))

    class Model:
        calls = 0
        fail_at = None

        def transcribe(self, audio, batch_size):
            self.calls += 1
            if self.calls == self.fail_at:
                raise RuntimeError("synthetic inference failure")
            return {
                "language": "en",
                "segments": [
                    {
                        "start": 1,
                        "end": 2,
                        "text": "Chunk.",
                        "words": [
                            {"word": "Chunk", "start": 1, "end": 1.5},
                            {"word": "."},
                        ],
                    }
                ],
            }

    model = Model()
    whisperx = SimpleNamespace(
        __version__="characterized-whisperx",
        load_model=lambda *args, **kwargs: model,
        load_audio=lambda path: [0],
        load_align_model=lambda **kwargs: (object(), {}),
        align=lambda segments, *args, **kwargs: {"language": "en", "segments": segments},
    )
    torch = SimpleNamespace(
        __version__="characterized-torch", cuda=SimpleNamespace(is_available=lambda: False)
    )
    monkeypatch.setattr(module, "load_runtime_modules", lambda: (torch, whisperx))
    monkeypatch.setattr(module, "probe_audio_duration_seconds", lambda path: 300)
    starts = []

    def load_chunk(path, start, duration):
        starts.append((start, duration))
        return [0]

    monkeypatch.setattr(module, "load_audio_chunk", load_chunk)
    return SimpleNamespace(
        module=module,
        model=model,
        whisperx=whisperx,
        config=config,
        segments=segments,
        progress=progress,
        starts=starts,
    )


def test_multichunk_failure_resumes_completed_chunks_and_offsets_words(
    pipeline, monkeypatch, capsys
):
    pipeline.model.fail_at = 2
    assert pipeline.module.main() == 1
    failure = json.loads(capsys.readouterr().out)
    assert failure["error"] == "whisperx_failed"
    assert failure["error_type"] == "RuntimeError"
    saved = json.loads(pipeline.segments.read_text())
    progress = json.loads(pipeline.progress.read_text())
    assert saved["completed_seconds"] == 120
    assert len(saved["segments"]) == 1
    assert progress["stage"] == "failed"
    assert progress["checkpoint"]["completed_seconds"] == 120

    monkeypatch.setenv("WHISPERX_RESUME_JSON", json.dumps(progress["checkpoint"]))
    pipeline.model.fail_at = None
    pipeline.starts.clear()
    assert pipeline.module.main() == 0
    result = json.loads(capsys.readouterr().out)
    assert pipeline.starts == [(120, 120), (240, 60)]
    assert [segment["start"] for segment in result["segments"]] == [1, 121, 241]
    assert [segment["end"] for segment in result["segments"]] == [2, 122, 242]
    assert result["segments"][2]["words"][0]["start"] == 241
    assert result["segments"][2]["words"][1] == {"word": "."}
    assert all("words" not in segment for segment in result["segments_pre_align"])
    assert result["metadata"]["resumed_from_checkpoint"] is True
    assert result["metadata"]["checkpoint_segment_count"] == 1
    assert not pipeline.segments.exists()
    assert json.loads(pipeline.progress.read_text())["stage"] == "complete"


@pytest.mark.parametrize("token_present", [False, True])
def test_best_effort_vad_alignment_and_diarization_failures_are_reported(
    pipeline, monkeypatch, capsys, token_present
):
    pipeline.config.update(align=True, diarization=True)
    monkeypatch.setenv("WHISPERX_CONFIG_JSON", json.dumps(pipeline.config))
    methods = []

    def load_model(*args, **kwargs):
        methods.append(kwargs["vad_method"])
        if kwargs["vad_method"] == "pyannote":
            raise RuntimeError("synthetic VAD failure")
        return pipeline.model

    def fail_alignment(**kwargs):
        raise ValueError("synthetic alignment failure")

    monkeypatch.setattr(pipeline.whisperx, "load_model", load_model)
    monkeypatch.setattr(pipeline.whisperx, "load_align_model", fail_alignment)
    if token_present:
        monkeypatch.setenv("WHISPERX_HF_TOKEN", "synthetic-test-token")
        diarize = ModuleType("whisperx.diarize")

        def fail_diarization(**kwargs):
            raise RuntimeError("synthetic diarization failure")

        diarize.DiarizationPipeline = fail_diarization
        diarize.assign_word_speakers = lambda *args: pytest.fail("unexpected speaker assignment")
        monkeypatch.setitem(sys.modules, "whisperx.diarize", diarize)

    assert pipeline.module.main() == 0
    result = json.loads(capsys.readouterr().out)
    assert methods == ["pyannote", "silero"]
    assert result["provider"] == "whisperx"
    assert result["vad_method"] == "silero"
    assert result["asr_options"] == pipeline.config["asr_options"]
    assert result["vad_options"] == pipeline.config["vad_options"]
    assert result["aligned"] is False
    assert result["alignment"] == {
        "requested": True,
        "used": False,
        "error": "align_failed:ValueError",
    }
    assert result["diarization"]["enabled"] is True
    assert result["diarization"]["used"] is False
    assert result["diarization"]["error"] == (
        "diarization_failed:RuntimeError" if token_present else "missing_hf_token"
    )
    assert all("speaker" not in segment for segment in result["segments"])
    assert result["metadata"]["whisperx_version"] == "characterized-whisperx"
    assert "synthetic-test-token" not in json.dumps(result)


def test_successful_alignment_and_diarization_preserve_pre_alignment_text(
    pipeline, monkeypatch, capsys
):
    pipeline.config.update(align=True, diarization=True)
    monkeypatch.setenv("WHISPERX_CONFIG_JSON", json.dumps(pipeline.config))
    monkeypatch.setenv("WHISPERX_HF_TOKEN", "synthetic-test-token")

    def align(segments, *args, **kwargs):
        return {
            "language": "en",
            "segments": [
                {"start": 1, "end": 2, "text": "Aligned text.", "words": [{"word": "untimed"}]}
            ],
        }

    monkeypatch.setattr(pipeline.whisperx, "align", align)
    diarize = ModuleType("whisperx.diarize")
    calls = []

    def make_diarizer(**kwargs):
        calls.append(kwargs)
        return lambda audio, **kwargs: "synthetic speaker intervals"

    def assign(intervals, result):
        assert intervals == "synthetic speaker intervals"
        result["segments"][0]["speaker"] = "SPEAKER_00"
        return result

    diarize.DiarizationPipeline = make_diarizer
    diarize.assign_word_speakers = assign
    monkeypatch.setitem(sys.modules, "whisperx.diarize", diarize)
    assert pipeline.module.main() == 0
    result = json.loads(capsys.readouterr().out)
    assert result["aligned"] is True
    assert result["alignment"] == {"requested": True, "used": True, "error": ""}
    assert result["diarization"]["used"] is True
    assert result["diarization"]["error"] == ""
    assert [segment["text"] for segment in result["segments_pre_align"]] == ["Chunk."] * 3
    assert result["segments"][0]["text"] == "Aligned text."
    assert result["segments"][0]["speaker"] == "SPEAKER_00"
    assert result["segments"][0]["words"] == [{"word": "untimed"}]
    assert calls[0]["model_name"] == pipeline.config["diarization_model"]
    assert calls[0]["device"] == "cpu"


@pytest.mark.parametrize("raw", ["not JSON", "[]", "null", '"checkpoint"'])
def test_malformed_or_nonobject_legacy_checkpoint_restarts_from_zero(pipeline, monkeypatch, raw):
    monkeypatch.setenv("WHISPERX_RESUME_JSON", raw)
    assert pipeline.module.load_resume_checkpoint() == ([], 0.0, "", "")


def test_legacy_checkpoint_loads_sidecar_and_filters_nonsegment_values(pipeline, monkeypatch):
    pipeline.segments.write_text(
        json.dumps(
            {
                "segments": [{"start": 1, "text": "Earlier."}, None],
                "completed_seconds": 120,
                "language": "en",
            }
        )
    )
    monkeypatch.setenv(
        "WHISPERX_RESUME_JSON", json.dumps({"segments_file": str(pipeline.segments)})
    )
    assert pipeline.module.load_resume_checkpoint() == (
        [{"start": 1, "text": "Earlier."}],
        120.0,
        "en",
        str(pipeline.segments),
    )


def test_legacy_inline_segments_take_precedence_over_sidecar(pipeline, monkeypatch):
    pipeline.segments.write_text(json.dumps({"segments": [{"text": "Old sidecar."}]}))
    monkeypatch.setenv(
        "WHISPERX_RESUME_JSON",
        json.dumps(
            {
                "segments": [{"text": "Inline."}],
                "completed_seconds": 120,
                "language": "en",
                "segments_file": str(pipeline.segments),
            }
        ),
    )
    assert pipeline.module.load_resume_checkpoint() == (
        [{"text": "Inline."}],
        120.0,
        "en",
        str(pipeline.segments),
    )


def test_characterize_missing_legacy_sidecar_keeps_unsafe_resume_offset(pipeline, monkeypatch):
    # Known defect, recorded in STAGE_B.md: new HTTP jobs must validate segment
    # data before resuming. This assertion describes the existing local reader.
    monkeypatch.setenv(
        "WHISPERX_RESUME_JSON",
        json.dumps({"completed_seconds": 120, "segments_file": str(pipeline.segments)}),
    )
    assert pipeline.module.load_resume_checkpoint() == ([], 120.0, "", str(pipeline.segments))
