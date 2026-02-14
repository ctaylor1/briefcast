#!/usr/bin/env python3
import json
import sys

from mutagen.id3 import ID3, ID3NoHeaderError


def normalize_text(value):
    if value is None:
        return ""
    if isinstance(value, list):
        return [normalize_text(v) for v in value]
    return str(value)


def iter_subframes(frame):
    subframes = getattr(frame, "sub_frames", None)
    if subframes:
        if hasattr(subframes, "values"):
            for subframe in subframes.values():
                yield subframe
        else:
            for subframe in subframes:
                yield subframe
    subframes = getattr(frame, "subframes", None)
    if subframes:
        if hasattr(subframes, "values"):
            for subframe in subframes.values():
                yield subframe
        else:
            for subframe in subframes:
                yield subframe


def extract_chapters(id3):
    chapters = []
    for key, frame in id3.items():
        if not key.startswith("CHAP"):
            continue
        chapter = {
            "id": getattr(frame, "element_id", key),
            "start_time_ms": getattr(frame, "start_time", None),
            "end_time_ms": getattr(frame, "end_time", None),
            "start_offset": getattr(frame, "start_offset", None),
            "end_offset": getattr(frame, "end_offset", None),
            "title": "",
        }
        for subframe in iter_subframes(frame):
            if subframe.FrameID in ("TIT2", "TIT3", "TIT1") and getattr(subframe, "text", None):
                chapter["title"] = normalize_text(subframe.text)
                break
        chapters.append(chapter)
    return chapters


def extract_tags(id3):
    tags = {}
    for key, frame in id3.items():
        if key.startswith("CHAP") or key.startswith("CTOC"):
            continue
        value = None
        if hasattr(frame, "text"):
            value = normalize_text(frame.text)
        elif hasattr(frame, "url"):
            value = normalize_text(frame.url)
        else:
            value = normalize_text(str(frame))

        existing = tags.get(frame.FrameID, [])
        if isinstance(value, list):
            existing.extend(value)
        else:
            existing.append(value)
        tags[frame.FrameID] = existing
    return tags


def main():
    if len(sys.argv) < 2:
        print(json.dumps({"tags": {}, "chapters": []}))
        return
    path = sys.argv[1]
    try:
        id3 = ID3(path)
    except ID3NoHeaderError:
        print(json.dumps({"tags": {}, "chapters": []}))
        return
    except Exception:
        print(json.dumps({"tags": {}, "chapters": []}))
        return

    payload = {
        "tags": extract_tags(id3),
        "chapters": extract_chapters(id3),
    }
    json.dump(payload, sys.stdout, ensure_ascii=False)


if __name__ == "__main__":
    main()
