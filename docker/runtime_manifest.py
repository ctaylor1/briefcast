"""Build-time inventory; never reads environment values or credentials."""

import hashlib
import importlib.metadata
import json
import platform
import subprocess
import sys
from pathlib import Path

kind, version, revision = sys.argv[1:]
pipeline = Path("/worker/scripts/whisperx_transcribe.py")
manifest = {
    "kind": kind,
    "version": version,
    "source_revision": revision,
    "python": platform.python_version(),
    "architecture": platform.machine(),
    "packages": {d.metadata["Name"]: d.version for d in importlib.metadata.distributions()},
    "system_packages": subprocess.check_output(
        ["dpkg-query", "-W", "-f=${Package}=${Version}\\n"], text=True
    ).splitlines(),
}
if pipeline.exists():
    manifest.update(
        api_major=1,
        pipeline_version="briefcast-script-v1",
        checkpoint_format="briefcast-whisperx-chunks-v1",
        pipeline_sha256=hashlib.sha256(pipeline.read_bytes()).hexdigest(),
    )
print(json.dumps(manifest, indent=2, sort_keys=True))
