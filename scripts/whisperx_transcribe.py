#!/usr/bin/env python3
import json
import logging
import os
import sys
from contextlib import redirect_stdout
from datetime import UTC, datetime
from pathlib import Path

ROOT_DIR = Path(__file__).resolve().parents[1]
SRC_DIR = ROOT_DIR / "src"
if str(SRC_DIR) not in sys.path:
    sys.path.insert(0, str(SRC_DIR))

from briefcast_tools import log_extra, setup_logging  # noqa: E402

logger = logging.getLogger(__name__)


def default_config():
    return {
        "model": "medium.en",
        "language": "en",
        "device": "auto",
        "compute_type": "auto",
        "batch_size": 0,
        "chunk_seconds": 120,
        "asr_options": {
            "beam_size": 5,
            "patience": 1,
            "condition_on_previous_text": True,
            "initial_prompt": (
                "Podcast interview. Speakers are Host and Guest. "
                "Use punctuation and capitalization."
            ),
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
    sys.stdout.write("\n")
    sys.stdout.flush()


def compact_error(exc, max_len=240):
    detail = str(exc).strip()
    if len(detail) <= max_len:
        return detail
    return detail[:max_len] + "...(truncated)"


def parse_float(value, default):
    try:
        return float(value)
    except (TypeError, ValueError):
        return default


def parse_chunk_seconds(value):
    parsed = parse_float(value, 120.0)
    if parsed <= 0:
        return 120.0
    return parsed


def progress_file_path():
    return os.environ.get("WHISPERX_PROGRESS_FILE", "").strip()


def transcription_progress_percent(completed_seconds, total_seconds):
    if total_seconds <= 0:
        return 75
    ratio = completed_seconds / total_seconds
    if ratio < 0:
        ratio = 0
    if ratio > 1:
        ratio = 1
    return int(round(20 + ratio * 60))


def make_checkpoint(segments, completed_seconds, total_seconds, language):
    return {
        "segments": segments,
        "completed_seconds": float(max(completed_seconds, 0.0)),
        "total_seconds": float(max(total_seconds, 0.0)),
        "language": language or "",
    }


def emit_progress(
    stage,
    percent,
    completed_seconds=0.0,
    total_seconds=0.0,
    checkpoint=None,
    error="",
):
    path = progress_file_path()
    if not path:
        return
    payload = {
        "stage": stage,
        "percent": max(0, min(100, int(percent))),
        "completed_seconds": float(max(completed_seconds, 0.0)),
        "total_seconds": float(max(total_seconds, 0.0)),
        "updated_at": datetime.now(UTC).isoformat(),
    }
    if checkpoint is not None:
        payload["checkpoint"] = checkpoint
    if error:
        payload["error"] = error

    tmp_path = f"{path}.tmp"
    try:
        with open(tmp_path, "w", encoding="utf-8") as handle:
            json.dump(payload, handle, ensure_ascii=False)
        os.replace(tmp_path, path)
    except OSError as exc:
        logger.warning(
            "failed to persist whisperx progress update",
            extra=log_extra({"stage": stage, "error": compact_error(exc)}),
        )


def shift_segment_timing(segment, offset_seconds):
    if not isinstance(segment, dict):
        return segment

    shifted = dict(segment)
    for key in ("start", "end"):
        value = shifted.get(key)
        if isinstance(value, (int, float)):
            shifted[key] = float(value) + offset_seconds

    words = shifted.get("words")
    if isinstance(words, list):
        shifted_words = []
        for word in words:
            if not isinstance(word, dict):
                shifted_words.append(word)
                continue
            shifted_word = dict(word)
            for key in ("start", "end"):
                value = shifted_word.get(key)
                if isinstance(value, (int, float)):
                    shifted_word[key] = float(value) + offset_seconds
            shifted_words.append(shifted_word)
        shifted["words"] = shifted_words

    return shifted


def load_resume_checkpoint():
    resume_file = os.environ.get("WHISPERX_RESUME_FILE", "").strip()
    raw = ""
    if resume_file:
        try:
            raw = Path(resume_file).read_text(encoding="utf-8").strip()
        except OSError as exc:
            logger.warning(
                "failed to read whisperx resume file",
                extra=log_extra({"path": resume_file, "error": compact_error(exc)}),
            )

    if not raw:
        raw = os.environ.get("WHISPERX_RESUME_JSON", "").strip()
    if not raw:
        return [], 0.0, ""

    try:
        payload = json.loads(raw)
    except json.JSONDecodeError:
        logger.warning("invalid whisperx resume payload; starting from beginning")
        return [], 0.0, ""

    if not isinstance(payload, dict):
        return [], 0.0, ""

    segments = payload.get("segments", [])
    if not isinstance(segments, list):
        segments = []
    safe_segments = [segment for segment in segments if isinstance(segment, dict)]

    completed_seconds = parse_float(payload.get("completed_seconds", 0.0), 0.0)
    if completed_seconds < 0:
        completed_seconds = 0.0
    language = str(payload.get("language", "")).strip()

    return safe_segments, completed_seconds, language


def load_runtime_modules():
    import torch
    import whisperx

    return torch, whisperx


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


def configure_third_party_logging():
    """Clamp noisy dependency loggers so app DEBUG doesn't flood container logs."""
    level_name = os.environ.get("WHISPERX_THIRD_PARTY_LOG_LEVEL", "WARNING").strip().upper()
    level = getattr(logging, level_name, logging.WARNING)
    for logger_name in (
        "filelock",
        "urllib3",
        "fsspec",
        "lightning",
        "pyannote",
        "speechbrain",
        "httpx",
    ):
        logging.getLogger(logger_name).setLevel(level)


def run_healthcheck():
    try:
        torch, whisperx = load_runtime_modules()
    except Exception as exc:
        logger.exception("whisperx healthcheck failed")
        emit_json(
            {
                "ok": False,
                "error": "missing whisperx dependencies",
                "error_type": exc.__class__.__name__,
                "detail": compact_error(exc),
            }
        )
        return 2

    payload = {
        "ok": True,
        "python_version": sys.version.split()[0],
        "whisperx_version": getattr(whisperx, "__version__", "unknown"),
        "torch_version": getattr(torch, "__version__", "unknown"),
    }
    emit_json(payload)
    logger.info("whisperx healthcheck passed", extra=log_extra(payload))
    return 0


def main():
    setup_logging(service_name="briefcast-whisperx")
    configure_third_party_logging()

    if len(sys.argv) >= 2 and sys.argv[1] in ("--healthcheck", "--preflight"):
        return run_healthcheck()

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
        torch, whisperx = load_runtime_modules()
    except Exception as exc:
        logger.exception("missing whisperx dependencies")
        emit_json(
            {
                "error": "missing whisperx dependencies",
                "error_type": exc.__class__.__name__,
                "detail": compact_error(exc),
            }
        )
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
    align_requested = bool(config.get("align", True))
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
                "align": align_requested,
                "diarization": diarization,
                "has_hf_token": bool(hf_token),
            }
        ),
    )

    completed_seconds = 0.0
    total_seconds = 0.0
    checkpoint_language = language
    merged_segments = []

    try:
        emit_progress("starting", 1)
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
                active_vad_method = vad_method
        except Exception:
            # pyannote VAD can fail in constrained/offline environments; fallback to silero.
            if str(vad_method).strip().lower() != "pyannote":
                raise
            logger.warning(
                "pyannote VAD model load failed; retrying with silero",
                extra=log_extra({"audio_file": audio_file, "vad_method": vad_method}),
            )
            with redirect_stdout(sys.stderr):
                model = whisperx.load_model(
                    model_name,
                    device,
                    compute_type=compute_type,
                    language=language,
                    asr_options=asr_options,
                    vad_options=vad_options,
                    vad_method="silero",
                )
            active_vad_method = "silero"
        emit_progress("model_loaded", 10)

        with redirect_stdout(sys.stderr):
            audio = whisperx.load_audio(audio_file)
        if hasattr(audio, "__len__"):
            total_seconds = float(len(audio)) / 16000.0

        resume_segments, resume_completed_seconds, resume_language = load_resume_checkpoint()
        merged_segments = list(resume_segments)
        checkpoint_language = resume_language or language
        completed_seconds = max(resume_completed_seconds, 0.0)
        if total_seconds > 0 and completed_seconds > total_seconds:
            completed_seconds = total_seconds

        if completed_seconds > 0:
            logger.info(
                "resuming whisperx transcription from checkpoint",
                extra=log_extra(
                    {
                        "audio_file": audio_file,
                        "completed_seconds": completed_seconds,
                        "total_seconds": total_seconds,
                        "checkpoint_segments": len(merged_segments),
                    }
                ),
            )
            emit_progress(
                "resuming",
                transcription_progress_percent(completed_seconds, total_seconds),
                completed_seconds=completed_seconds,
                total_seconds=total_seconds,
                checkpoint=make_checkpoint(
                    merged_segments, completed_seconds, total_seconds, checkpoint_language
                ),
            )
        else:
            emit_progress("audio_loaded", 15, total_seconds=total_seconds)

        chunk_seconds = parse_chunk_seconds(config.get("chunk_seconds", 120))
        with redirect_stdout(sys.stderr):
            while total_seconds <= 0 or completed_seconds < total_seconds:
                start_seconds = completed_seconds
                end_seconds = start_seconds + chunk_seconds
                if total_seconds > 0:
                    end_seconds = min(total_seconds, end_seconds)
                start_index = int(start_seconds * 16000.0)
                end_index = int(end_seconds * 16000.0)
                if end_index <= start_index:
                    break
                chunk_audio = audio[start_index:end_index]
                if hasattr(chunk_audio, "__len__") and len(chunk_audio) == 0:
                    break

                chunk_result = model.transcribe(chunk_audio, batch_size=batch_size)
                chunk_segments = chunk_result.get("segments", []) or []
                shifted_segments = [
                    shift_segment_timing(segment, start_seconds) for segment in chunk_segments
                ]
                merged_segments.extend(shifted_segments)

                chunk_language = str(chunk_result.get("language", "")).strip()
                if chunk_language:
                    checkpoint_language = chunk_language

                completed_seconds = end_seconds
                emit_progress(
                    "transcribing",
                    transcription_progress_percent(completed_seconds, total_seconds),
                    completed_seconds=completed_seconds,
                    total_seconds=total_seconds,
                    checkpoint=make_checkpoint(
                        merged_segments,
                        completed_seconds,
                        total_seconds,
                        checkpoint_language,
                    ),
                )

                if total_seconds <= 0:
                    break

        result = {
            "language": checkpoint_language or language,
            "segments": merged_segments,
        }

        align_used = False
        align_error = ""
        if align_requested:
            emit_progress(
                "aligning",
                85,
                completed_seconds=completed_seconds,
                total_seconds=total_seconds,
                checkpoint=make_checkpoint(
                    result.get("segments", []),
                    completed_seconds,
                    total_seconds,
                    result.get("language", language),
                ),
            )
            try:
                with redirect_stdout(sys.stderr):
                    model_a, metadata = whisperx.load_align_model(
                        language_code=result.get("language", language),
                        device=device,
                    )
                    result = whisperx.align(
                        result.get("segments", []),
                        model_a,
                        metadata,
                        audio,
                        device,
                        return_char_alignments=False,
                    )
                align_used = True
            except Exception as exc:
                align_error = f"align_failed:{exc.__class__.__name__}"
                logger.warning(
                    "alignment failed; continuing with base transcript segments",
                    extra=log_extra({"audio_file": audio_file, "error": align_error}),
                )

        diarize_used = False
        diarize_error = ""
        if diarization:
            emit_progress(
                "diarizing",
                92,
                completed_seconds=completed_seconds,
                total_seconds=total_seconds,
                checkpoint=make_checkpoint(
                    result.get("segments", []),
                    completed_seconds,
                    total_seconds,
                    result.get("language", language),
                ),
            )
            if not hf_token:
                diarize_error = "missing_hf_token"
            else:
                try:
                    from whisperx.diarize import DiarizationPipeline, assign_word_speakers

                    with redirect_stdout(sys.stderr):
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
                except Exception as exc:
                    diarize_error = f"diarization_failed:{exc.__class__.__name__}"
                    logger.warning(
                        "diarization failed; continuing without speaker labels",
                        extra=log_extra({"audio_file": audio_file, "error": diarize_error}),
                    )

        if total_seconds > 0:
            completed_seconds = total_seconds

        emit_progress(
            "complete",
            100,
            completed_seconds=completed_seconds,
            total_seconds=total_seconds,
            checkpoint=make_checkpoint(
                result.get("segments", []),
                completed_seconds,
                total_seconds,
                result.get("language", language),
            ),
        )

        payload = {
            "provider": "whisperx",
            "model": model_name,
            "language": result.get("language", language),
            "device": device,
            "compute_type": compute_type,
            "batch_size": batch_size,
            "asr_options": asr_options,
            "vad_options": vad_options,
            "vad_method": active_vad_method,
            "aligned": align_used,
            "alignment": {
                "requested": align_requested,
                "used": align_used,
                "error": align_error,
            },
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
                "generated_at": datetime.now(UTC).isoformat(),
                "whisperx_version": getattr(whisperx, "__version__", "unknown"),
                "torch_version": getattr(torch, "__version__", "unknown"),
                "resumed_from_checkpoint": resume_completed_seconds > 0,
                "checkpoint_segment_count": len(resume_segments),
            },
        }
        emit_json(payload)
        logger.info(
            "whisperx transcription complete",
            extra=log_extra(
                {
                    "audio_file": audio_file,
                    "segment_count": len(result.get("segments", [])),
                    "align_used": align_used,
                    "align_error": align_error,
                    "diarization_used": diarize_used,
                    "diarization_error": diarize_error,
                    "resumed": resume_completed_seconds > 0,
                }
            ),
        )
        return 0
    except Exception as exc:
        logger.exception(
            "whisperx transcription failed", extra=log_extra({"audio_file": audio_file})
        )
        emit_progress(
            "failed",
            transcription_progress_percent(completed_seconds, total_seconds),
            completed_seconds=completed_seconds,
            total_seconds=total_seconds,
            checkpoint=make_checkpoint(
                merged_segments, completed_seconds, total_seconds, checkpoint_language
            ),
            error=f"{exc.__class__.__name__}:{compact_error(exc)}",
        )
        emit_json(
            {
                "error": "whisperx_failed",
                "error_type": exc.__class__.__name__,
                "detail": compact_error(exc),
            }
        )
        return 1


if __name__ == "__main__":
    sys.exit(main())
