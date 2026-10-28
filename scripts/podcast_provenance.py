"""Capture factual, portable WhisperX production records (no runtime dependencies)."""

import hashlib
import json
from datetime import UTC, datetime
from importlib.metadata import PackageNotFoundError, version
from pathlib import Path
from uuid import NAMESPACE_URL, uuid4, uuid5


def completed_at():
    return datetime.now(UTC).isoformat().replace("+00:00", "Z")


def file_sha256(path):
    digest = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def installed_version(package, module=None):
    try:
        return version(package)
    except PackageNotFoundError:
        fallback = str(getattr(module, "__version__", "")).strip()
        if fallback and fallback != "unknown":
            return fallback
        raise RuntimeError(f"Cannot capture installed {package} version") from None


def resource_id(kind, identity):
    return uuid5(NAMESPACE_URL, f"briefcast/podcast-markdown/{kind}/{identity}").urn


def model_identity(reference):
    """A local model is identified by its files, never by an absolute host path."""
    path = Path(reference)
    if not path.exists():
        return reference, None
    files = sorted(path.rglob("*")) if path.is_dir() else [path]
    digest = hashlib.sha256()
    for file in files:
        if not file.is_file():
            continue
        relative = file.relative_to(path).as_posix() if path.is_dir() else file.name
        digest.update(json.dumps([relative, file_sha256(file)]).encode("utf-8"))
    return "local-model:" + path.name, digest.hexdigest()


def text_snapshot(kind, text, coverage="full"):
    digest = hashlib.sha256(text.encode("utf-8")).hexdigest()
    return {
        "kind": kind,
        "id": resource_id(kind, digest),
        "revision": 1,
        "uri": f"inputs/{digest}.txt",
        "sha256": digest,
        "coverage": coverage,
        "text": text,
    }


def segment_snapshot(segments):
    return text_snapshot(
        "transcript",
        json.dumps(segments, ensure_ascii=False, sort_keys=True, separators=(",", ":")),
    )


def processing_event(operation, timestamp, inputs, output, tool, tool_version, settings=None):
    return {
        "event_id": uuid4().urn,
        "operation": operation,
        "completed_at": timestamp,
        "tool": tool,
        "tool_version": tool_version,
        "inputs": inputs,
        "output": {key: output[key] for key in ("id", "revision", "sha256")},
        "settings": settings or {},
    }
