#Requires -Version 7.0
[CmdletBinding()]
param([Parameter(Mandatory)][string]$Image)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
Push-Location $root
try {
    # Pytest is an existing development dependency. Mount its pure Python tools
    # read-only, then execute tests with the shipped worker's Python 3.12.
    $site = (& uv run python -c 'import pytest; from pathlib import Path; print(Path(pytest.__file__).parent.parent)').Trim()
    if ($LASTEXITCODE -ne 0) { throw 'Existing development environment is required.' }
    & uv run ruff check scripts/podcast_provenance.py scripts/whisperx_transcribe.py scripts/whisperx_recovery.py scripts/whisperx_acceptance.py scripts/whisperx_acceptance_proxy.py scripts/whisperx_acceptance_cache.py scripts/whisperx_resource_acceptance.py scripts/briefcast_recovery.py
    if ($LASTEXITCODE -ne 0) { throw 'Worker script lint failed.' }
    & uv run ruff format --check scripts/podcast_provenance.py scripts/whisperx_transcribe.py scripts/whisperx_recovery.py scripts/whisperx_acceptance.py scripts/whisperx_acceptance_proxy.py scripts/whisperx_acceptance_cache.py scripts/whisperx_resource_acceptance.py scripts/briefcast_recovery.py
    if ($LASTEXITCODE -ne 0) { throw 'Worker script formatting failed.' }
    & docker run --rm --network none --read-only --tmpfs /tmp:size=536870912 --entrypoint python `
        --mount "type=bind,source=$root,target=/repo,readonly" --mount "type=bind,source=$site,target=/test-tools,readonly" `
        --env PYTHONPATH=/test-tools:/repo/src:/repo/tests --env PYTHONDONTWRITEBYTECODE=1 $Image `
        -m pytest -c /repo/pyproject.toml /repo/tests/test_whisperx_pipeline_characterization.py `
        /repo/tests/test_whisperx_worker_pipeline.py /repo/tests/test_podcast_provenance.py /repo/tests/test_whisperx_provenance.py /repo/tests/test_whisperx_recovery.py /repo/tests/test_whisperx_acceptance.py /repo/tests/test_whisperx_acceptance_proxy.py /repo/tests/test_whisperx_acceptance_cache.py /repo/tests/test_whisperx_resource_acceptance.py /repo/tests/test_briefcast_recovery.py
    if ($LASTEXITCODE -ne 0) { throw 'Worker-runtime Python tests failed.' }
} finally { Pop-Location }
