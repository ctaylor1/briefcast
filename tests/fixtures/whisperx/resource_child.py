"""Controlled child for bounded container resource faults; never loads ML models."""

import errno
import json
import os
import subprocess
import sys
import time
from pathlib import Path


def identity(pid):
    fields = Path(f"/proc/{pid}/stat").read_text().rsplit(")", 1)[1].split()
    return {"pid": pid, "start_time": fields[19]}


def fill(path, limit=32 * 1024 * 1024):
    """Bound the write even if the caller accidentally selected a larger disk."""
    block = b"x" * (512 * 1024)
    try:
        with path.open("wb", buffering=0) as stream:
            for _ in range(limit // len(block)):
                stream.write(block)
            os.fsync(stream.fileno())
    except OSError as error:
        if error.errno != errno.ENOSPC:
            raise
        return {
            "errno": error.errno,
            "available_bytes": os.statvfs(path.parent).f_bavail * os.statvfs(path.parent).f_frsize,
        }
    raise RuntimeError("bounded fixture did not exhaust its filesystem")


def success(config, golden):
    transcript = json.loads(golden.read_text())["transcript"]
    for key in (
        "model",
        "language",
        "device",
        "compute_type",
        "batch_size",
        "asr_options",
        "vad_options",
        "vad_method",
    ):
        transcript[key] = config[key]
    transcript["aligned"] = config["align"]
    transcript["alignment"] = {"requested": config["align"], "used": config["align"], "error": ""}
    transcript["diarization"] = {
        "enabled": False,
        "used": False,
        "model": config["diarization_model"],
        "min_speakers": config["min_speakers"],
        "max_speakers": config["max_speakers"],
        "error": "",
    }
    return transcript


def main():
    mode = Path("/fixture/mode").read_text().strip()
    if mode == "success":
        print(
            json.dumps(
                success(
                    json.loads(os.environ["WHISPERX_CONFIG_JSON"]), Path("/fixture/result.json")
                )
            )
        )
        return
    if mode not in {"oom", "scratch"}:
        raise ValueError("unknown fixture mode")
    job_dir = Path(os.environ["WHISPERX_PROGRESS_FILE"]).parent
    child = subprocess.Popen([sys.executable, "-c", "import time; time.sleep(120)"])
    evidence = {"mode": mode, "processes": [identity(os.getpid()), identity(child.pid)]}
    (job_dir / "resource-fault.json").write_text(json.dumps(evidence))
    if mode == "scratch":
        evidence.update(fill(Path("/cache/tmp/fill.bin")))
        (job_dir / "resource-fault.json").write_text(json.dumps(evidence))
        raise RuntimeError("controlled scratch ENOSPC")
    # Favor the fixture parent over the journal owner/guardian and descendant.
    # Increasing our own score requires no extra capability; never disable OOM.
    Path("/proc/self/oom_score_adj").write_text("1000")
    blocks = []
    for _ in range(256):
        blocks.append(bytearray(1024 * 1024))
        time.sleep(0.005)
    raise RuntimeError("bounded fixture was not killed by OOM")


if __name__ == "__main__":
    main()
