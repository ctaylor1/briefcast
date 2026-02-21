"""Shared logging configuration for Briefcast Python modules and scripts."""

from __future__ import annotations

import json
import logging
import os
import re
import sys
from collections.abc import Mapping
from datetime import UTC, datetime
from logging.handlers import RotatingFileHandler
from pathlib import Path
from typing import Literal

LOG_LEVEL_ENV = "LOG_LEVEL"
LOG_FORMAT_ENV = "LOG_FORMAT"
LOG_OUTPUT_ENV = "LOG_OUTPUT"
LOG_RUN_TIMESTAMP_ENV = "LOG_RUN_TIMESTAMP"

_DEFAULT_LOG_LEVEL = "INFO"
_DEFAULT_LOG_FORMAT: Literal["text", "json"] = "text"
_DEFAULT_LOG_OUTPUT = "stderr"
_REDACTED = "***REDACTED***"
_SENSITIVE_KEYWORDS = (
    "api_key",
    "apikey",
    "auth",
    "authorization",
    "cookie",
    "hf_token",
    "password",
    "passwd",
    "refresh_token",
    "secret",
    "set_cookie",
    "token",
)
_NON_ALNUM = re.compile(r"[^a-z0-9]+")
_NON_TS = re.compile(r"[^A-Za-z0-9_-]+")
_configured = False
_run_timestamp: str | None = None


def setup_logging(service_name: str = "briefcast-python", *, force: bool = False) -> None:
    """Configure root logging once using LOG_LEVEL/LOG_FORMAT/LOG_OUTPUT env vars."""
    global _configured
    global _run_timestamp
    if _configured and not force:
        return
    if force:
        _run_timestamp = None

    root_logger = logging.getLogger()
    log_level = _parse_log_level(os.getenv(LOG_LEVEL_ENV, _DEFAULT_LOG_LEVEL))
    log_format = _parse_log_format(os.getenv(LOG_FORMAT_ENV, _DEFAULT_LOG_FORMAT))
    handlers = _build_handlers(service_name=service_name, log_format=log_format)

    for handler in root_logger.handlers:
        handler.close()
    root_logger.handlers.clear()
    for handler in handlers:
        root_logger.addHandler(handler)
    root_logger.setLevel(log_level)
    _configured = True


def log_extra(context: Mapping[str, object] | None = None) -> dict[str, object]:
    """Build sanitized logging extra context."""
    if context is None:
        return {}
    return {"context": redact_sensitive(dict(context))}


def redact_sensitive(value: object) -> object:
    """Recursively redact common secret-bearing keys in mappings."""
    if isinstance(value, Mapping):
        redacted: dict[str, object] = {}
        for raw_key, raw_value in value.items():
            key = str(raw_key)
            if _is_sensitive_key(key):
                redacted[key] = _REDACTED
            else:
                redacted[key] = redact_sensitive(raw_value)
        return redacted
    if isinstance(value, list):
        return [redact_sensitive(item) for item in value]
    if isinstance(value, tuple):
        return [redact_sensitive(item) for item in value]
    if isinstance(value, set):
        return [redact_sensitive(item) for item in sorted(value, key=str)]
    return value


def _parse_log_level(raw_level: str) -> int:
    level_name = raw_level.strip().upper()
    candidate = getattr(logging, level_name, None)
    if isinstance(candidate, int):
        return candidate
    return logging.INFO


def _parse_log_format(raw_format: str) -> Literal["text", "json"]:
    value = raw_format.strip().lower()
    if value == "json":
        return "json"
    return "text"


def _build_handlers(
    *, service_name: str, log_format: Literal["text", "json"]
) -> list[logging.Handler]:
    formatter: logging.Formatter
    if log_format == "json":
        formatter = _JSONFormatter(service_name=service_name)
    else:
        formatter = _TextFormatter(service_name=service_name)

    handlers: list[logging.Handler] = []
    for token in _parse_log_outputs(os.getenv(LOG_OUTPUT_ENV, _DEFAULT_LOG_OUTPUT)):
        handler = _build_handler(token)
        if handler is None:
            continue
        handler.setFormatter(formatter)
        handlers.append(handler)

    if handlers:
        return handlers

    fallback = logging.StreamHandler(stream=sys.stderr)
    fallback.setFormatter(formatter)
    return [fallback]


def _parse_log_outputs(raw_outputs: str) -> list[str]:
    outputs = [token.strip() for token in raw_outputs.split(",") if token.strip()]
    if outputs:
        return outputs
    return [_DEFAULT_LOG_OUTPUT]


def _build_handler(token: str) -> logging.Handler | None:
    value = token.strip()
    if not value:
        return None

    lowered = value.lower()
    if lowered == "stdout":
        return logging.StreamHandler(stream=sys.stdout)
    if lowered == "stderr":
        return logging.StreamHandler(stream=sys.stderr)

    path_value = value
    if lowered.startswith("file:"):
        path_value = value[len("file:") :]
    path_value = _expand_log_path(path_value)
    if not path_value:
        return None

    log_path = Path(path_value)
    log_path.parent.mkdir(parents=True, exist_ok=True)
    max_bytes = max(_parse_int_env("LOG_FILE_MAX_SIZE_MB", 50), 1) * 1024 * 1024
    backup_count = max(_parse_int_env("LOG_FILE_MAX_BACKUPS", 7), 0)

    return RotatingFileHandler(
        log_path,
        maxBytes=max_bytes,
        backupCount=backup_count,
        encoding="utf-8",
    )


def _expand_log_path(path: str) -> str:
    resolved = path.strip()
    if not resolved:
        return ""

    timestamp = _resolve_run_timestamp()
    return (
        resolved.replace("{startup_ts}", timestamp)
        .replace("{timestamp}", timestamp)
        .replace("{run_ts}", timestamp)
    )


def _resolve_run_timestamp() -> str:
    global _run_timestamp
    if _run_timestamp is not None:
        return _run_timestamp

    raw = os.getenv(LOG_RUN_TIMESTAMP_ENV, "").strip()
    if not raw:
        raw = datetime.now(tz=UTC).strftime("%Y%m%d-%H%M%S")

    sanitized = _NON_TS.sub("", raw)
    if not sanitized:
        sanitized = datetime.now(tz=UTC).strftime("%Y%m%d-%H%M%S")

    _run_timestamp = sanitized
    os.environ[LOG_RUN_TIMESTAMP_ENV] = sanitized
    return sanitized


def _parse_int_env(name: str, fallback: int) -> int:
    raw = os.getenv(name, "").strip()
    if not raw:
        return fallback
    try:
        return int(raw)
    except ValueError:
        return fallback


def _is_sensitive_key(key: str) -> bool:
    normalized = _NON_ALNUM.sub("_", key.strip().lower()).strip("_")
    if not normalized:
        return False
    for keyword in _SENSITIVE_KEYWORDS:
        if keyword == normalized or keyword in normalized:
            return True
    return False


def _context_from_record(record: logging.LogRecord) -> object | None:
    context = record.__dict__.get("context")
    if context is None:
        return None
    return redact_sensitive(context)


class _TextFormatter(logging.Formatter):
    def __init__(self, *, service_name: str):
        super().__init__()
        self._service_name = service_name

    def format(self, record: logging.LogRecord) -> str:
        timestamp = datetime.fromtimestamp(record.created, tz=UTC).isoformat()
        message = (
            f"{timestamp} {record.levelname} {self._service_name} "
            f"{record.name} {record.getMessage()}"
        )

        context = _context_from_record(record)
        if context is not None:
            message += " context=" + json.dumps(context, ensure_ascii=False, default=str)

        if record.exc_info:
            message += " exception=" + self.formatException(record.exc_info)
        return message


class _JSONFormatter(logging.Formatter):
    def __init__(self, *, service_name: str):
        super().__init__()
        self._service_name = service_name

    def format(self, record: logging.LogRecord) -> str:
        payload: dict[str, object] = {
            "ts": datetime.fromtimestamp(record.created, tz=UTC).isoformat(),
            "level": record.levelname,
            "service": self._service_name,
            "logger": record.name,
            "message": record.getMessage(),
        }
        context = _context_from_record(record)
        if context is not None:
            payload["context"] = context
        if record.exc_info:
            payload["exception"] = self.formatException(record.exc_info)
        return json.dumps(payload, ensure_ascii=False, default=str)
