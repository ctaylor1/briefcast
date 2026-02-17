#!/usr/bin/env python3
import json
import logging
import os
import sys
from contextlib import redirect_stdout
from datetime import datetime, timezone
from pathlib import Path

ROOT_DIR = Path(__file__).resolve().parents[1]
SRC_DIR = ROOT_DIR / "src"
if str(SRC_DIR) not in sys.path:
    sys.path.insert(0, str(SRC_DIR))

from briefcast_tools import log_extra, setup_logging

logger = logging.getLogger(__name__)


def default_config():
    return {
        "model": "medium.en",
        "language": "en",
        "device": "auto",
        "compute_type": "auto",
        "batch_size": 0,
        "asr_options": {
            "beam_size": 5,
            "patience": 1,
            "condition_on_previous_text": True,
            "initial_prompt": "Podcast interview. Speakers are Host and Guest. Use punctuation and capitalization.",
        },
        "vad_options": {
            "chunk_size": 45,
            "vad_onset": 0.50,
            "vad_offset": 0.50,
        },
        "vad_method": "pyannote",
        "align": True,
        "diarization": True,
        "diarization_model": "pyannote/speaker-diarization-3.1",
        "min_speakers": 2,
        "max_speakers": 2,
    }


def merge_config(base, override):
    for key, value in override.items():
        if isinstance(value, dict) and isinstance(base.get(key), dict):
            base[key] = merge_config(base[key], value)
        else:
            base[key] = value
    return base


def emit_json(payload):
    json.dump(payload, sys.stdout, ensure_ascii=False)


def load_config():
    raw = os.environ.get("WHISPERX_CONFIG_JSON", "").strip()
    base = default_config()
    if not raw:
        return base
    try:
        override = json.loads(raw)
    except json.JSONDecodeError:
        logger.warning("invalid whisperx config json; using defaults")
        return base
    if isinstance(override, dict):
        return merge_config(base, override)
    return base


def choose_device(config, torch):
    device = str(config.get("device", "auto")).strip().lower()
    if device in ("cuda", "cpu"):
        return device
    return "cuda" if torch.cuda.is_available() else "cpu"


def choose_compute_type(config, device):
    compute_type = str(config.get("compute_type", "auto")).strip().lower()
    if compute_type and compute_type != "auto":
        return compute_type
    return "float16" if device == "cuda" else "int8"


def choose_batch_size(config, device):
    try:
        configured = int(config.get("batch_size", 0))
    except (TypeError, ValueError):
        logger.warning(
            "invalid batch size in whisperx config; using default",
            extra=log_extra({"batch_size": config.get("batch_size")}),
        )
        configured = 0
    if configured > 0:
        return configured
    return 16 if device == "cuda" else 4


def main():
    setup_logging(service_name="briefcast-whisperx")

    if len(sys.argv) < 2:
        logger.error("missing audio path argument")
        emit_json({"error": "missing audio path"})
        return 2

    audio_file = sys.argv[1]
    if not os.path.exists(audio_file):
        logger.error("audio file not found", extra=log_extra({"audio_file": audio_file}))
        emit_json({"error": "audio file not found"})
        return 2

    try:
        import torch
        import whisperx
    except Exception:
        logger.exception("missing whisperx dependencies")
        emit_json({"error": "missing whisperx dependencies"})
        return 2

    config = load_config()
    device = choose_device(config, torch)
    compute_type = choose_compute_type(config, device)
    batch_size = choose_batch_size(config, device)

    asr_options = config.get("asr_options", {}) or {}
    vad_options = config.get("vad_options", {}) or {}
    vad_method = config.get("vad_method", "pyannote")
    model_name = config.get("model", "medium.en")
    language = config.get("language", "en")
    align = bool(config.get("align", True))
    diarization = bool(config.get("diarization", True))
    diarization_model = config.get("diarization_model", "pyannote/speaker-diarization-3.1")
    min_speakers = config.get("min_speakers", 2)
    max_speakers = config.get("max_speakers", 2)

    hf_token = os.environ.get("WHISPERX_HF_TOKEN", "").strip()
    logger.info(
        "starting whisperx transcription",
        extra=log_extra(
            {
                "audio_file": audio_file,
                "model": model_name,
                "language": language,
                "device": device,
                "compute_type": compute_type,
                "batch_size": batch_size,
                "align": align,
                "diarization": diarization,
                "has_hf_token": bool(hf_token),
            }
        ),
    )

    try:
        with redirect_stdout(sys.stderr):
            model = whisperx.load_model(
                model_name,
                device,
                compute_type=compute_type,
                language=language,
                asr_options=asr_options,
                vad_options=vad_options,
                vad_method=vad_method,
            )

            audio = whisperx.load_audio(audio_file)
            result = model.transcribe(audio, batch_size=batch_size)

            if align:
                model_a, metadata = whisperx.load_align_model(
                    language_code=result.get("language", language), device=device
                )
                result = whisperx.align(
                    result.get("segments", []),
                    model_a,
                    metadata,
                    audio,
                    device,
                    return_char_alignments=False,
                )

            diarize_used = False
            diarize_error = ""
            if diarization:
                if not hf_token:
                    diarize_error = "missing_hf_token"
                else:
                    from whisperx.diarize import DiarizationPipeline, assign_word_speakers

                    diarize_model = DiarizationPipeline(
                        model_name=diarization_model,
                        token=hf_token,
                        device=device,
                    )
                    diarize_df = diarize_model(
                        audio_file, min_speakers=min_speakers, max_speakers=max_speakers
                    )
                    result = assign_word_speakers(diarize_df, result)
                    diarize_used = True

        payload = {
            "provider": "whisperx",
            "model": model_name,
            "language": result.get("language", language),
            "device": device,
            "compute_type": compute_type,
            "batch_size": batch_size,
            "asr_options": asr_options,
            "vad_options": vad_options,
            "vad_method": vad_method,
            "aligned": align,
            "diarization": {
                "enabled": diarization,
                "used": diarize_used,
                "model": diarization_model,
                "min_speakers": min_speakers,
                "max_speakers": max_speakers,
                "error": diarize_error,
            },
            "segments": result.get("segments", []),
            "metadata": {
                "generated_at": datetime.now(timezone.utc).isoformat(),
                "whisperx_version": getattr(whisperx, "__version__", "unknown"),
                "torch_version": getattr(torch, "__version__", "unknown"),
            },
        }
        emit_json(payload)
        logger.info(
            "whisperx transcription complete",
            extra=log_extra(
                {
                    "audio_file": audio_file,
                    "segment_count": len(result.get("segments", [])),
                    "diarization_used": diarize_used,
                    "diarization_error": diarize_error,
                }
            ),
        )
        return 0
    except Exception:
        logger.exception("whisperx transcription failed", extra=log_extra({"audio_file": audio_file}))
        emit_json({"error": "whisperx_failed"})
        return 1


if __name__ == "__main__":
    sys.exit(main())
