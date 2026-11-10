"""feedparser_parse.py must hand feedparser the payload as content, never as a path."""

import json
import os
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

STUB = """
import io, json, os

class _Result:
    feed = {}
    entries = []
    bozo = False
    version = "stub"

def parse(source):
    with open(os.environ["STUB_RECORD"], "w") as f:
        json.dump({"file_like": hasattr(source, "read"), "type": type(source).__name__,
                   "content": source.read().decode() if hasattr(source, "read") else None}, f)
    return _Result()
"""


def test_payload_is_parsed_as_content_not_opened_as_a_path(tmp_path):
    stub_dir = tmp_path / "stub"
    stub_dir.mkdir()
    (stub_dir / "feedparser.py").write_text(STUB)
    record = tmp_path / "record.json"
    env = {
        "PATH": os.environ.get("PATH", ""),
        "PYTHONPATH": os.pathsep.join([str(stub_dir), str(ROOT / "src")]),
        "STUB_RECORD": str(record),
        "LOG_OUTPUT": "stderr",
    }
    if "SYSTEMROOT" in os.environ:
        env["SYSTEMROOT"] = os.environ["SYSTEMROOT"]
    path_shaped = b"/etc/hostname"
    result = subprocess.run(
        [sys.executable, str(ROOT / "scripts" / "feedparser_parse.py")],
        input=path_shaped,
        capture_output=True,
        env=env,
        timeout=60,
    )
    assert result.returncode == 0, result.stderr.decode(errors="replace")
    seen = json.loads(record.read_text())
    assert seen["file_like"] is True, seen
    assert seen["content"] == path_shaped.decode()
    assert json.loads(result.stdout)["version"] == "stub"
