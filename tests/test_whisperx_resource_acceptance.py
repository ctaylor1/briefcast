"""Fail closed if a resource run is unbounded or its evidence is inconclusive."""

import copy
import importlib.util
import json
import sys
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))
try:
    import whisperx_resource_acceptance as resource
finally:
    sys.path.pop(0)
spec = importlib.util.spec_from_file_location(
    "resource_child", ROOT / "tests/fixtures/whisperx/resource_child.py"
)
child = importlib.util.module_from_spec(spec)
spec.loader.exec_module(child)


def envelope(tmp_path):
    mounts = {"/state": (tmp_path / "state", False), "/media": (tmp_path / "media", True)}
    tmpfs = {"/tmp": "size=8388608,uid=1000,gid=1000,mode=0700"}
    info = {
        "Image": "sha256:fixture",
        "Config": {
            "Labels": {resource.LABEL: "fixture"},
            "User": "1000:1000",
            "Env": ["PATH=/bin"],
        },
        "Mounts": [
            {"Destination": target, "Source": str(source), "RW": not readonly, "Type": "bind"}
            for target, (source, readonly) in mounts.items()
        ],
        "HostConfig": {
            "NetworkMode": "none",
            "Memory": resource.MEMORY,
            "MemorySwap": resource.MEMORY,
            "NanoCpus": 1_000_000_000,
            "PidsLimit": 64,
            "ReadonlyRootfs": True,
            "Init": True,
            "Privileged": False,
            "CapAdd": None,
            "CapDrop": ["ALL"],
            "SecurityOpt": ["no-new-privileges"],
            "Devices": [],
            "DeviceRequests": None,
            "PortBindings": {},
            "RestartPolicy": {"Name": "no"},
            "Tmpfs": tmpfs,
        },
    }
    return info, mounts, tmpfs


@pytest.mark.parametrize(
    "field,value",
    [
        ("NetworkMode", "host"),
        ("Memory", 0),
        ("MemorySwap", -1),
        ("NanoCpus", 0),
        ("PidsLimit", 0),
        ("ReadonlyRootfs", False),
        ("Init", False),
        ("Privileged", True),
        ("CapAdd", ["SYS_ADMIN"]),
        ("CapDrop", []),
        ("SecurityOpt", []),
        ("Devices", [{"PathOnHost": "/dev/sda"}]),
        ("DeviceRequests", [{"Count": -1}]),
        ("PortBindings", {"8090/tcp": [{"HostPort": "8090"}]}),
        ("RestartPolicy", {"Name": "always"}),
        ("Tmpfs", {}),
    ],
)
def test_rejects_unbounded_or_external_container(tmp_path, field, value):
    info, mounts, tmpfs = envelope(tmp_path)
    resource.assert_envelope(info, "fixture", "sha256:fixture", mounts, tmpfs, ["PATH=/bin"])
    info["HostConfig"][field] = value
    with pytest.raises(ValueError):
        resource.assert_envelope(info, "fixture", "sha256:fixture", mounts, tmpfs, ["PATH=/bin"])


@pytest.mark.parametrize(
    "mutation", ["label", "image", "user", "secret", "source", "writable_media", "volume"]
)
def test_rejects_unowned_or_wrong_mounts(tmp_path, mutation):
    info, mounts, tmpfs = envelope(tmp_path)
    if mutation == "label":
        info["Config"]["Labels"][resource.LABEL] = "production"
    elif mutation == "image":
        info["Image"] = "unpinned"
    elif mutation == "user":
        info["Config"]["User"] = "0:0"
    elif mutation == "secret":
        info["Config"]["Env"].append("DATABASE_URL=private")
    elif mutation == "source":
        info["Mounts"][0]["Source"] = str(tmp_path / "production")
    elif mutation == "writable_media":
        info["Mounts"][1]["RW"] = True
    else:
        info["Mounts"].append({"Type": "volume", "Destination": "/config"})
    with pytest.raises(ValueError):
        resource.assert_envelope(info, "fixture", "sha256:fixture", mounts, tmpfs, ["PATH=/bin"])


def test_oom_requires_limit_hit_and_new_kernel_kill():
    before = {
        "max_bytes": resource.MEMORY,
        "swap_bytes": 0,
        "events": {"max": 0, "oom": 0, "oom_kill": 0},
    }
    after = copy.deepcopy(before)
    after["events"] = {"max": 10, "oom": 2, "oom_kill": 1}
    resource.assert_oom(before, after)
    for key in after["events"]:
        invalid = copy.deepcopy(after)
        invalid["events"][key] = 0
        with pytest.raises(ValueError):
            resource.assert_oom(before, invalid)
    after["swap_bytes"] = resource.MEMORY
    with pytest.raises(ValueError):
        resource.assert_oom(before, after)


def test_disk_evidence_requires_actual_enospc_and_zero_free_space():
    resource.assert_enospc({"errno": 28, "available_bytes": 0})
    for invalid in ({"errno": 13, "available_bytes": 0}, {"errno": 28, "available_bytes": 4096}):
        with pytest.raises(ValueError):
            resource.assert_enospc(invalid)


def test_filler_never_fills_an_unbounded_host_filesystem(tmp_path):
    destination = tmp_path / "filler"
    with pytest.raises(RuntimeError, match="did not exhaust"):
        child.fill(destination, limit=1024 * 1024)
    assert destination.stat().st_size == 1024 * 1024


def test_mount_checks_preserve_linux_case_and_normalize_desktop_paths():
    assert resource.mount_source("/private/State") != resource.mount_source("/private/state")
    assert resource.mount_source("C:\\Private\\State") == resource.mount_source(
        "/run/desktop/mnt/host/c/private/state"
    )


def test_container_is_inspected_before_start(tmp_path):
    harness = resource.ResourceHarness.__new__(resource.ResourceHarness)
    harness.run_dir, harness.project, harness.image = tmp_path, "fixture", "sha256:fixture"
    harness.image_env, harness.owned = ["PATH=/bin"], []
    harness.profile = "fixture"
    commands = []

    def command(*args):
        commands.append(args)
        return "container-id"

    harness.command = command
    info, _, _ = envelope(tmp_path)
    info["HostConfig"]["NetworkMode"] = "host"
    harness.inspect_owned = lambda _: info
    with pytest.raises(ValueError, match="isolation envelope"):
        harness.start("oom")
    assert len(commands) == 1 and commands[0][:2] == ("docker", "create")
    assert harness.owned == ["container-id"]


def test_cleanup_attempts_every_owned_container_and_preserves_fault(tmp_path):
    harness = resource.ResourceHarness.__new__(resource.ResourceHarness)
    harness.run_dir, harness.evidence = tmp_path, {"passed": False}
    harness.owned = ["first", "second"]
    stopped = []

    def failed(_):
        raise ValueError("original fault")

    def stop(container):
        stopped.append(container)
        if container == "first":
            raise RuntimeError("cleanup failed")

    harness.failed_child, harness.stop = failed, stop
    with pytest.raises(ValueError, match="original fault"):
        harness.run()
    assert stopped == harness.owned
    assert json.loads((tmp_path / "evidence.json").read_text())["cleanup_passed"] is False


def test_cleanup_refuses_unowned_container():
    harness = resource.ResourceHarness.__new__(resource.ResourceHarness)
    harness.project = "fixture"
    commands = []

    def command(*args):
        commands.append(args)
        return json.dumps([{"Config": {"Labels": {resource.LABEL: "production"}}}])

    harness.command = command
    with pytest.raises(ValueError, match="outside this run"):
        harness.stop("production-id")
    assert commands == [("docker", "inspect", "production-id")]


def test_success_fixture_uses_worker_runtime_configuration():
    config = json.loads((ROOT / "docs/whisperx-separation/fixtures/put-job.json").read_text())[
        "options"
    ]
    config.update(device="cpu", compute_type="int8", batch_size=16, diarization_model="fixture")
    result = child.success(config, ROOT / "docs/whisperx-separation/fixtures/job-result.json")
    assert result["device"] == "cpu" and result["compute_type"] == "int8"
    assert result["diarization"]["model"] == "fixture"
    assert result["segments"][0]["text"] == "Hello world."
