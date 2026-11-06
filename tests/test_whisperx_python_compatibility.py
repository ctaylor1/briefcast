"""The deployed monolith runs Python 3.12, independently of dev tooling."""

import ast
from pathlib import Path

import pytest


@pytest.mark.parametrize("script", ["whisperx_transcribe.py", "podcast_provenance.py"])
def test_whisperx_scripts_parse_on_deployed_python(script):
    source = Path(__file__).resolve().parents[1] / "scripts" / script
    ast.parse(source.read_text(encoding="utf-8"), filename=str(source), feature_version=(3, 12))
