"""Packaging gates use synthetic config and mock deployments, never production state."""

import hashlib
import json
import os
import shutil
import subprocess
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[1]
PWSH = shutil.which("pwsh")
pytestmark = pytest.mark.skipif(not PWSH, reason="PowerShell 7 is required for release tooling")


def run_ps(*args, env=None):
    result = subprocess.run(
        [PWSH, "-NoLogo", "-NoProfile", *map(str, args)],
        cwd=ROOT,
        env=env,
        capture_output=True,
        text=True,
    )
    assert result.returncode == 0, result.stderr
    return result


@pytest.mark.parametrize("runtime", ["disabled", "cpu", "cuda"])
@pytest.mark.parametrize("advanced", [False, True])
@pytest.mark.parametrize("source_build", [False, True])
def test_compose_matrix(tmp_path, runtime, advanced, source_build):
    if not shutil.which("docker"):
        pytest.skip("Docker Compose is required for the packaging matrix")
    advanced_file = tmp_path / "advanced.env"
    if advanced:
        advanced_file.write_text("WHISPERX_DIARIZATION=true\nWHISPERX_MAX_CONCURRENCY=3\n")
    env_file = tmp_path / "fixture $literal config.env"
    env_file.write_text(
        f"WHISPERX_RUNTIME={runtime}\nWHISPERX_ENV_FILE={advanced_file.as_posix()}\n"
        "DATABASE_URL=sqlite:///config/fixture.db\nLLM_ENABLED=false\n"
        "BRIEFCAST_IMAGE=fixture/app@sha256:" + "a" * 64 + "\n"
        "WHISPERX_IMAGE=fixture/worker@sha256:" + "b" * 64 + "\n"
        f"HOST_WHISPERX_TOKEN_FILE={tmp_path.as_posix()}/token\n"
        f"HOST_WHISPERX_HF_TOKEN_FILE={tmp_path.as_posix()}/hf-token\n"
        "WHISPERX_DIARIZATION=false\nWHISPERX_MAX_CONCURRENCY=1\n"
    )
    # Do not inherit operator settings; only synthetic test configuration is rendered.
    env = {
        k: v
        for k, v in os.environ.items()
        if k.upper()
        in {
            "PATH",
            "SYSTEMROOT",
            "WINDIR",
            "TEMP",
            "TMP",
            "USERPROFILE",
            "HOME",
            "APPDATA",
            "LOCALAPPDATA",
            "PROGRAMFILES",
            "PROGRAMDATA",
            "PATHEXT",
            "COMSPEC",
        }
    }
    source_args = ["-SourceBuild"] if source_build else []
    env["ENV_FILE"] = str(env_file)
    output = run_ps(
        "-File",
        ROOT / "scripts/Compose.ps1",
        "-Runtime",
        runtime,
        *source_args,
        "--profile",
        "postgres",
        "config",
        "--format",
        "json",
        env=env,
    )
    config = json.loads(output.stdout)
    app = config["services"]["briefcast"]
    assert ("build" in app) == source_build
    assert "gpus" not in app and "deploy" not in app
    assert app["environment"]["WHISPERX_BACKEND"] == "http"
    assert app["environment"]["WHISPERX_DIARIZATION"] == str(advanced).lower()
    assert app["environment"]["WHISPERX_MAX_CONCURRENCY"] == ("3" if advanced else "1")
    assert config["services"]["postgres"]["profiles"] == ["postgres"]
    if runtime == "disabled":
        assert "whisperx" not in config["services"]
        assert app["environment"]["WHISPERX_ENABLED"] == "false"
        return
    worker = config["services"]["whisperx"]
    assert ("build" in worker) == source_build
    assert "ports" not in worker and "env_file" not in worker
    if source_build:
        assert worker["build"]["args"]["RUNTIME"] == ("cu128" if runtime == "cuda" else "cpu")
    assert worker["read_only"] and worker["user"] == "1000:1000"
    assert any(entry.startswith("/cache/tmp:size=536870912") for entry in worker["tmpfs"])
    assert set(worker["environment"]) == {
        "HF_HUB_OFFLINE",
        "TRANSFORMERS_OFFLINE",
        "HF_DATASETS_OFFLINE",
    }
    mounts = {m["target"]: m for m in worker["volumes"]}
    assert set(mounts) == {
        "/media",
        "/state",
        "/cache",
        "/run/secrets/whisperx-service-token",
        "/run/secrets/whisperx-hf-token",
    }
    assert mounts["/media"]["read_only"]
    assert not mounts["/state"].get("read_only", False)
    assert "--device=" + ("cuda" if runtime == "cuda" else "cpu") in worker["command"]
    assert ("deploy" in worker) == (runtime == "cuda")
    assert not config["networks"]["default"].get("internal", False)


@pytest.mark.parametrize("bad_checksum", [False, True])
def test_worker_deploy_is_targeted_and_checks_archive(tmp_path, bad_checksum):
    archive = tmp_path / "worker.tar"
    archive.write_bytes(b"synthetic archive; mock Docker only")
    old_id, new_id = "sha256:" + "a" * 64, "sha256:" + "b" * 64
    manifest = {
        "version": "1.0.0",
        "variant": "cpu",
        "service": "whisperx",
        "image_id": new_id,
        "archive": archive.name,
        "archive_sha256": "invalid"
        if bad_checksum
        else hashlib.sha256(archive.read_bytes()).hexdigest(),
    }
    (tmp_path / "briefcast-whisperx-cpu_v1.0.0.json").write_text(json.dumps(manifest))
    env_file = tmp_path / "fixture.env"
    env_file.write_text("WHISPERX_RUNTIME=cpu\n")
    log = tmp_path / "calls.jsonl"
    harness = tmp_path / "harness.ps1"
    # Mock the CLI inside PowerShell, so no deployment command can reach Docker.
    harness.write_text(
        "param($Root,$EnvFile,$BuildDir,$Log,$OldId,$NewId)\n"
        "$global:MockLog=$Log; $global:MockOld=$OldId; $global:MockNew=$NewId\n"
        "function global:docker {\n"
        "  $a=@($args); ConvertTo-Json -Compress -InputObject $a | "
        "Add-Content -LiteralPath $global:MockLog\n"
        "  $global:LASTEXITCODE=0\n"
        "  if (($a -join ' ') -eq 'compose version --short') { '2.24.0'; return }\n"
        "  if ($a -contains 'ps') { 'worker-container'; return }\n"
        "  if ($a[0] -eq 'inspect') { $global:MockOld; return }\n"
        "  if ($a -contains 'drain') { '{\"running\":0,\"queued\":0}'; return }\n"
        "  if ($a -contains 'inspect') { $global:MockNew; return }\n"
        "}\n"
        "& (Join-Path $Root 'scripts/ReleasePackaging.ps1') deploy -Target worker -Runtime cpu "
        "-Version 1.0.0 -EnvFile $EnvFile -BuildDir $BuildDir -SkipVerify\n"
        "& (Join-Path $Root 'scripts/ReleasePackaging.ps1') rollback -Target worker -Runtime cpu "
        "-Version 1.0.0 -EnvFile $EnvFile -BuildDir $BuildDir\n"
    )
    result = subprocess.run(
        [
            PWSH,
            "-NoLogo",
            "-NoProfile",
            "-File",
            str(harness),
            str(ROOT),
            str(env_file),
            str(tmp_path),
            str(log),
            old_id,
            new_id,
        ],
        capture_output=True,
        text=True,
    )
    if bad_checksum:
        assert result.returncode != 0
        assert not log.exists()
        return
    assert result.returncode == 0, result.stderr
    calls = [json.loads(line) for line in log.read_text().splitlines()]
    ups = [c for c in calls if "up" in c]
    assert len(ups) == 2
    assert all(
        c[-6:] == ["up", "-d", "--no-build", "--no-deps", "--force-recreate", "whisperx"]
        for c in ups
    )
    assert next(i for i, c in enumerate(calls) if c[0] == "inspect") < next(
        i for i, c in enumerate(calls) if "load" in c
    )
    assert not any("down" in c or "build" in c for c in calls)
    assert not any(c[-1] == "briefcast" for c in calls)
    rollback = json.loads((tmp_path / "rollback-worker-cpu.json").read_text(encoding="utf-8-sig"))
    assert rollback["image_id"] == old_id


def test_no_implicit_gpu_override_and_all_locks_are_hashed():
    assert not (ROOT / "docker-compose.override.yml").exists()
    for path in [
        ROOT / "docker/app/requirements.lock",
        *sorted((ROOT / "docker/whisperx").glob("requirements-*.lock")),
    ]:
        records = path.read_text().split("\n    # via")
        for record in records:
            if "==" in record:
                assert "--hash=sha256:" in record


def test_worker_publish_updates_only_worker_version(tmp_path):
    for file in ["scripts/ReleasePackaging.ps1", "scripts/Packaging.psm1", "release.config.json"]:
        destination = tmp_path / file
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(ROOT / file, destination)
    version = tmp_path / "docker/whisperx/VERSION"
    version.parent.mkdir(parents=True)
    version.write_text("1.0.0\n")
    app = tmp_path / "pyproject.toml"
    app.write_text('[project]\nversion = "9.9.9"\n')
    harness = tmp_path / "publish.ps1"
    log = tmp_path / "git.jsonl"
    harness.write_text(
        "param($Root,$Log)\n$global:GitLog=$Log\n"
        "function global:git {\n"
        " ConvertTo-Json -Compress -InputObject @($args) | "
        "Add-Content -LiteralPath $global:GitLog\n"
        " $global:LASTEXITCODE=0\n}\n"
        "& (Join-Path $Root 'scripts/ReleasePackaging.ps1') publish -Target worker "
        "-Runtime cpu -Bump patch -NoPush\n"
    )
    run_ps("-File", harness, tmp_path, log)
    assert version.read_text().strip() == "1.0.1"
    assert app.read_text() == '[project]\nversion = "9.9.9"\n'
    calls = [json.loads(line) for line in log.read_text().splitlines()]
    add = next(c for c in calls if "add" in c)
    assert add[2:] == ["add", "--", "docker/whisperx/VERSION"]
    assert any("worker-v1.0.1" in c for c in calls)
    assert not any("push" in c for c in calls)
