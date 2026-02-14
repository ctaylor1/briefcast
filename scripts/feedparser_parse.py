#!/usr/bin/env python3
import json
import sys
import time
from datetime import datetime, timezone

import feedparser


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
    raw = sys.stdin.buffer.read()
    parsed = feedparser.parse(raw)
    payload = {
        "feed": _convert(parsed.feed),
        "entries": _convert(parsed.entries),
        "bozo": bool(getattr(parsed, "bozo", False)),
        "version": getattr(parsed, "version", ""),
    }
    if getattr(parsed, "bozo", False):
        payload["bozo_exception"] = str(getattr(parsed, "bozo_exception", ""))

    json.dump(payload, sys.stdout, ensure_ascii=False)


if __name__ == "__main__":
    main()
