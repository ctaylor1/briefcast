"""The container harness must reject production configuration before starting."""

import copy
import importlib.util
from pathlib import Path

import pytest

SOURCE = Path(__file__).resolve().parents[1] / "scripts/whisperx_acceptance.py"
spec = importlib.util.spec_from_file_location("whisperx_acceptance", SOURCE)
acceptance = importlib.util.module_from_spec(spec)
spec.loader.exec_module(acceptance)


def fixture_config(tmp_path):
    mount = {"type": "bind", "source": str(tmp_path / "private-state")}
    return {
        "services": {
            "briefcast": {
                "environment": {
                    "DATABASE_URL": "sqlite:///config/acceptance.db",
                    "LLM_ENABLED": "false",
                    "WHISPERX_ENABLED": "false",
                },
                "container_name": "fixture-app",
                "volumes": [mount],
                "ports": [{"host_ip": "127.0.0.1", "published": "0"}],
            },
            "whisperx": {"volumes": [mount]},
        }
    }


@pytest.mark.parametrize(
    "fault", ["database", "mount", "llm", "port", "name", "token", "worker-port"]
)
def test_production_configuration_rejected(tmp_path, fault):
    config = copy.deepcopy(fixture_config(tmp_path))
    app, worker = config["services"]["briefcast"], config["services"]["whisperx"]
    if fault == "database":
        app["environment"]["DATABASE_URL"] = "postgres://production/briefcast"
    elif fault == "mount":
        app["volumes"][0]["source"] = str(tmp_path.parent / "production")
    elif fault == "llm":
        app["environment"]["LLM_ENABLED"] = "true"
    elif fault == "port":
        app["ports"][0]["host_ip"] = "0.0.0.0"
    elif fault == "name":
        app["container_name"] = "briefcast"
    elif fault == "token":
        app["environment"]["PASSWORD"] = "private"
    else:
        worker["ports"] = [{"published": "8090"}]
    with pytest.raises(ValueError):
        acceptance.assert_isolated(config, tmp_path, "fixture")


def test_isolated_config_and_environment(tmp_path, monkeypatch):
    acceptance.assert_isolated(fixture_config(tmp_path), tmp_path, "fixture")
    monkeypatch.setenv("DATABASE_URL", "private")
    monkeypatch.setenv("WHISPERX_IMAGE", "production")
    assert "DATABASE_URL" not in acceptance.isolated_environment()
    assert "WHISPERX_IMAGE" not in acceptance.isolated_environment()


def test_dispatch_requires_explicit_test_mode_and_private_worker_endpoint(tmp_path):
    config = fixture_config(tmp_path)
    env = config["services"]["briefcast"]["environment"]
    env["WHISPERX_ENABLED"] = "true"
    env["WHISPERX_SERVICE_URL"] = "http://whisperx:8090"
    with pytest.raises(ValueError):
        acceptance.assert_isolated(config, tmp_path, "fixture")
    acceptance.assert_isolated(config, tmp_path, "fixture", allow_dispatch=True)
    env["WHISPERX_SERVICE_URL"] = "http://production-worker:8090"
    with pytest.raises(ValueError):
        acceptance.assert_isolated(config, tmp_path, "fixture", allow_dispatch=True)


def test_normalization_preserves_errors_words_speakers_and_timestamps():
    transcript = {
        "metadata": {"job_id": "different"},
        "segments": [
            {"start": 1.1, "end": 2.2, "speaker": "SPEAKER_00", "words": [{"word": "speech"}]}
        ],
        "alignment": {"error": "failed"},
    }
    assert acceptance.normalized(transcript) == {
        key: value for key, value in transcript.items() if key != "metadata"
    }


@pytest.mark.parametrize("fault", ["mode", "endpoint", "mount", "ports", "environment", "writable"])
def test_crash_proxy_cannot_escape_fixture(tmp_path, fault):
    config = fixture_config(tmp_path)
    env = config["services"]["briefcast"]["environment"]
    env.update(WHISPERX_ENABLED="true", WHISPERX_SERVICE_URL="http://acceptance-proxy:8091")
    proxy = {
        "read_only": True,
        "volumes": [{"type": "bind", "source": str(tmp_path / "gate"), "target": "/gate"}],
    }
    config["services"]["acceptance-proxy"] = proxy
    acceptance.assert_isolated(config, tmp_path, "fixture", allow_dispatch=True, allow_proxy=True)
    if fault == "mode":
        with pytest.raises(ValueError):
            acceptance.assert_isolated(config, tmp_path, "fixture", allow_dispatch=True)
        return
    if fault == "endpoint":
        env["WHISPERX_SERVICE_URL"] = "http://production:8090"
    elif fault == "mount":
        proxy["volumes"][0]["source"] = str(tmp_path / "config")
    elif fault == "ports":
        proxy["ports"] = [{"published": "8091"}]
    elif fault == "environment":
        proxy["environment"] = {"DATABASE_URL": "private"}
    else:
        proxy["read_only"] = False
    with pytest.raises(ValueError):
        acceptance.assert_isolated(
            config, tmp_path, "fixture", allow_dispatch=True, allow_proxy=True
        )
