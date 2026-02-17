"""Shared logging configuration for Briefcast Python modules and scripts."""

from __future__ import annotations

import json
import logging
import os
import re
import sys
from collections.abc import Mapping
from datetime import UTC, datetime
from typing import Literal

LOG_LEVEL_ENV = "LOG_LEVEL"
LOG_FORMAT_ENV = "LOG_FORMAT"

_DEFAULT_LOG_LEVEL = "INFO"
_DEFAULT_LOG_FORMAT: Literal["text", "json"] = "text"
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
_configured = False


def setup_logging(service_name: str = "briefcast-python", *, force: bool = False) -> None:
    """Configure root logging once using LOG_LEVEL and LOG_FORMAT env vars."""
    global _configured
    if _configured and not force:
        return

    root_logger = logging.getLogger()
    log_level = _parse_log_level(os.getenv(LOG_LEVEL_ENV, _DEFAULT_LOG_LEVEL))
    log_format = _parse_log_format(os.getenv(LOG_FORMAT_ENV, _DEFAULT_LOG_FORMAT))

    handler = logging.StreamHandler(stream=sys.stderr)
    formatter: logging.Formatter
    if log_format == "json":
        formatter = _JSONFormatter(service_name=service_name)
    else:
        formatter = _TextFormatter(service_name=service_name)
    handler.setFormatter(formatter)

    root_logger.handlers.clear()
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
