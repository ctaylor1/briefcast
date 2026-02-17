#!/usr/bin/env python3
import json
import logging
import sys
import time
from datetime import datetime, timezone
from pathlib import Path

import feedparser

ROOT_DIR = Path(__file__).resolve().parents[1]
SRC_DIR = ROOT_DIR / "src"
if str(SRC_DIR) not in sys.path:
    sys.path.insert(0, str(SRC_DIR))

from briefcast_tools import log_extra, setup_logging

logger = logging.getLogger(__name__)


def _to_iso(value):
    if isinstance(value, time.struct_time):
        dt = datetime(
            value.tm_year,
            value.tm_mon,
            value.tm_mday,
            value.tm_hour,
            value.tm_min,
            value.tm_sec,
            tzinfo=timezone.utc,
        )
        return dt.isoformat()
    return value


def _convert(value):
    if isinstance(value, dict):
        return {str(k): _convert(v) for k, v in value.items()}
    if isinstance(value, list):
        return [_convert(v) for v in value]
    if isinstance(value, time.struct_time):
        return _to_iso(value)
    if isinstance(value, bytes):
        return value.decode("utf-8", "replace")
    return value


def main():
    setup_logging(service_name="briefcast-feedparser")

    try:
        raw = sys.stdin.buffer.read()
        logger.debug("parsing feed payload", extra=log_extra({"input_bytes": len(raw)}))
        parsed = feedparser.parse(raw)
        payload = {
            "feed": _convert(parsed.feed),
            "entries": _convert(parsed.entries),
            "bozo": bool(getattr(parsed, "bozo", False)),
            "version": getattr(parsed, "version", ""),
        }
        if getattr(parsed, "bozo", False):
            bozo_error = str(getattr(parsed, "bozo_exception", ""))
            payload["bozo_exception"] = bozo_error
            logger.warning(
                "feed marked as bozo by feedparser",
                extra=log_extra({"bozo_exception": bozo_error}),
            )

        json.dump(payload, sys.stdout, ensure_ascii=False)
        return 0
    except Exception:
        logger.exception("feed parsing failed")
        return 1


if __name__ == "__main__":
    sys.exit(main())
