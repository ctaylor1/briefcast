"""Real cgroup/tmpfs faults with a controlled child and the shipped worker."""

import argparse
import json
import re
import secrets
import shutil
import subprocess
import sys
import uuid
from pathlib import Path

from whisperx_acceptance import ROOT, Harness, isolated_environment, sha256

MEMORY = 128 * 1024 * 1024
SCRATCH = 8 * 1024 * 1024
STATE = 16 * 1024 * 1024
LABEL = "org.briefcast.resource-test"


def mount_source(value):
    value = value.replace("\\", "/")
    for prefix in ("/run/desktop/mnt/host/", "/host_mnt/"):
        if value.startswith(prefix):
            value = value[len(prefix) :]
            value = value[0] + ":" + value[1:]
            break
    # Docker Desktop drive paths are case insensitive; Linux paths are not.
    return value.lower() if re.match(r"^[a-zA-Z]:/", value) else value


def assert_envelope(info, project, image, mounts, tmpfs, image_env):
    host, config = info["HostConfig"], info["Config"]
    actual_mounts = {
        item["Destination"]: (mount_source(item["Source"]), not item["RW"])
        for item in info["Mounts"]
        if item["Type"] == "bind"
    }
    expected_mounts = {
        target: (mount_source(str(source)), readonly)
        for target, (source, readonly) in mounts.items()
    }
    if (
        config.get("Labels", {}).get(LABEL) != project
        or info["Image"] != image
        or config["User"] != "1000:1000"
        or config["Env"] != image_env
        or host["NetworkMode"] != "none"
        or host["Memory"] != MEMORY
        or host["MemorySwap"] != MEMORY
        or host["NanoCpus"] != 1_000_000_000
        or host["PidsLimit"] != 64
        or not host["ReadonlyRootfs"]
        or not host["Init"]
        or host["Privileged"]
        or host["CapAdd"]
        or host["CapDrop"] != ["ALL"]
        or host["SecurityOpt"] != ["no-new-privileges"]
        or host["Devices"]
        or host["DeviceRequests"]
        or host["PortBindings"]
        or host["RestartPolicy"]["Name"] != "no"
        or host["Tmpfs"] != tmpfs
        or actual_mounts != expected_mounts
        or any(item["Type"] not in {"bind", "tmpfs"} for item in info["Mounts"])
    ):
        raise ValueError("resource container escaped its bounded isolation envelope")


def assert_oom(before, after):
    if (
        after["max_bytes"] != MEMORY
        or after["swap_bytes"] != 0
        or any(after["events"][key] <= before["events"][key] for key in ("max", "oom", "oom_kill"))
    ):
        raise ValueError("no verified cgroup memory-limit OOM kill")


def assert_enospc(proof):
    if proof.get("errno") != 28 or proof.get("available_bytes") != 0:
        raise ValueError("no verified bounded filesystem ENOSPC")


class ResourceHarness(Harness):
    # Reuse only private command, loopback API and frozen request helpers.
    # No app, Compose, models, production environment or cache is selected.
    def __init__(self, args):
        self.args, self.env = args, isolated_environment()
        self.run_dir = args.run_dir.resolve()
        if self.run_dir.exists() or self.run_dir == ROOT or ROOT in self.run_dir.parents:
            raise ValueError("run directory must be NEW and outside the repository")
        self.run_dir.mkdir(mode=0o700)
        self.project = "briefcast-resource-" + uuid.uuid4().hex[:12]
        self.profile = "resource-fixture-cpu-v1"
        self.owned = []
        self.current = None
        self.image = self.image_id(args.worker_image)
        image_info = json.loads(self.command("docker", "image", "inspect", self.image))[0]
        self.image_env = image_info["Config"]["Env"]
        self.evidence = {
            "schema": 1,
            "project": self.project,
            "image": self.image,
            "profile": self.profile,
            "real_ml": False,
            "passed": False,
            "fixture_sha256": sha256(ROOT / "tests/fixtures/whisperx/resource_child.py"),
            "harness_sha256": sha256(Path(__file__)),
            "checks": [],
            "scenarios": {},
        }
        if self.command("docker", "info", "--format", "{{.CgroupVersion}}") != "2":
            raise ValueError("resource acceptance requires cgroup v2")
        (self.run_dir / "media/audio").mkdir(parents=True, mode=0o700)
        self.audio = self.run_dir / "media/audio/speech.flac"
        shutil.copyfile(ROOT / "tests/fixtures/whisperx/speech.flac", self.audio)
        self.source_digest = sha256(self.audio)
        self.evidence["source_sha256"] = self.source_digest
        (self.run_dir / "token").write_text(secrets.token_hex(32))
        (self.run_dir / "token").chmod(0o600)
        for scenario in ("oom", "scratch", "state"):
            for directory in ("state", "cache", "fixture"):
                (self.run_dir / scenario / directory).mkdir(mode=0o700, parents=True)
            fixture = self.run_dir / scenario / "fixture"
            shutil.copyfile(
                ROOT / "tests/fixtures/whisperx/resource_child.py", fixture / "child.py"
            )
            shutil.copyfile(
                ROOT / "docs/whisperx-separation/fixtures/job-result.json", fixture / "result.json"
            )
            (fixture / "mode").write_text("success" if scenario == "state" else scenario)
        # Only this new private tree; the source repository remains read-only.
        provision = """import os
from pathlib import Path
root = Path('/run-data')
for scenario in ('oom', 'scratch', 'state'):
 for name in ('state', 'cache'):
  os.chown(root/scenario/name, 1000, 1000)
 for p in [root/scenario/'fixture', *(root/scenario/'fixture').rglob('*')]:
  p.chmod(0o755 if p.is_dir() else 0o644)
for p in [root/'media', *(root/'media').rglob('*')]:
 p.chmod(0o755 if p.is_dir() else 0o644)
os.chown(root/'token', 1000, 1000)
(root/'token').chmod(0o400)
"""
        self.command(
            "docker",
            "run",
            "--rm",
            "--network",
            "none",
            "--read-only",
            "--user",
            "0:0",
            "--entrypoint",
            "python",
            "--mount",
            f"type=bind,source={self.run_dir},target=/run-data",
            self.image,
            "-c",
            provision,
        )

    def inspect_owned(self, container):
        info = json.loads(self.command("docker", "inspect", container))[0]
        if info["Config"].get("Labels", {}).get(LABEL) != self.project:
            raise ValueError("refusing operation on a container outside this run")
        return info

    def container(self, service="whisperx"):
        if service != "whisperx" or self.current is None:
            raise ValueError("resource harness has no app or external endpoint")
        self.inspect_owned(self.current)
        return self.current

    def execute(self, code):
        return json.loads(
            self.command("docker", "exec", "-i", self.container(), "python", "-", input=code)
        )

    def start(self, scenario):
        base = self.run_dir / scenario
        mounts = {
            "/media": (self.run_dir / "media", True),
            "/cache": (base / "cache", False),
            "/fixture": (base / "fixture", True),
            "/run/secrets/whisperx-service-token": (self.run_dir / "token", True),
        }
        tmpfs = {
            "/tmp": f"size={SCRATCH},uid=1000,gid=1000,mode=0700",
            "/cache/tmp": f"size={SCRATCH},uid=1000,gid=1000,mode=0700",
        }
        if scenario == "state":
            tmpfs["/state"] = f"size={STATE},uid=1000,gid=1000,mode=0700"
        else:
            mounts["/state"] = (base / "state", False)
        command = [
            "docker",
            "create",
            "--name",
            self.project + "-" + scenario,
            "--label",
            LABEL + "=" + self.project,
            "--network",
            "none",
            "--memory",
            str(MEMORY),
            "--memory-swap",
            str(MEMORY),
            "--cpus",
            "1",
            "--pids-limit",
            "64",
            "--init",
            "--read-only",
            "--user",
            "1000:1000",
            "--cap-drop",
            "ALL",
            "--security-opt",
            "no-new-privileges",
            "--restart",
            "no",
        ]
        for target, (source, readonly) in mounts.items():
            command += [
                "--mount",
                f"type=bind,source={source},target={target}" + (",readonly" if readonly else ""),
            ]
        for target, options in tmpfs.items():
            command += ["--tmpfs", target + ":" + options]
        command += [
            self.image,
            "serve",
            "--listen=127.0.0.1:8090",
            "--token-file=/run/secrets/whisperx-service-token",
            "--profile=" + self.profile,
            "--script=/fixture/child.py",
            "--device=cpu",
            "--compute-type=int8",
            "--execution-timeout=60",
            "--stop-grace=3s",
            "--min-free-bytes=1048576",
        ]
        self.current = self.command(*command)
        self.owned.append(self.current)
        assert_envelope(
            self.inspect_owned(self.current),
            self.project,
            self.image,
            mounts,
            tmpfs,
            self.image_env,
        )
        self.command("docker", "start", self.current)
        caps = self.ready()
        self.evidence["scenarios"][scenario] = {
            "container": self.current,
            "worker_version": caps["worker_version"],
            "package_versions": caps["package_versions"],
        }
        self.check(scenario + ": inspected isolation and hard resource bounds before start")

    def memory(self):
        return self.execute(
            "import json; from pathlib import Path; p=Path('/sys/fs/cgroup')\n"
            "events=dict((k,int(v)) for k,v in (line.split() for line in "
            "(p/'memory.events.local').read_text().splitlines()))\n"
            "print(json.dumps({'max_bytes':int((p/'memory.max').read_text()),"
            "'swap_bytes':int((p/'memory.swap.max').read_text()),'events':events}))"
        )

    def preserved(self):
        if sha256(self.audio) != self.source_digest:
            raise ValueError("source media changed")
        proof = self.execute(
            "import hashlib,json; from pathlib import Path; p=Path('/media/audio/speech.flac')\n"
            "print(json.dumps(hashlib.file_digest(p.open('rb'),'sha256').hexdigest()))"
        )
        if proof != self.source_digest:
            raise ValueError("container source media changed")

    def success_job(self, job, request):
        self.api("PUT", "/v1/jobs/" + job, request, expected=201)
        status = self.wait(job)
        if status["state"] != "succeeded":
            raise ValueError("post-fault fixture did not succeed")
        result = self.api("GET", "/v1/jobs/" + job + "/result")
        if result["job_id"] != job or result["transcript"]["segments"][0]["text"] != "Hello world.":
            raise ValueError("post-fault result identity/content mismatch")
        receipt = {"result_sha256": result["result_sha256"]}
        ack = self.api("POST", "/v1/jobs/" + job + "/ack", receipt)
        if self.api("POST", "/v1/jobs/" + job + "/ack", receipt) != ack:
            raise ValueError("ACK was not idempotent")
        return result, ack

    def failed_child(self, scenario):
        self.start(scenario)
        before = self.memory()
        job, request = str(uuid.uuid4()), self.request(self.audio)
        self.api("PUT", "/v1/jobs/" + job, request, expected=201)
        status = self.wait(job)
        if (
            status["state"] != "failed"
            or status.get("error", {}).get("code") != "INTERNAL"
            or status["error"].get("retryable") is not True
        ):
            raise ValueError("fault did not produce a typed retryable terminal failure")
        self.api("GET", "/v1/jobs/" + job + "/result", expected=409)
        self.api("POST", "/v1/jobs/" + job + "/ack", {"result_sha256": "a" * 64}, expected=409)
        # Read evidence through the same Linux mount view as the supervisor.
        proof = self.execute(
            "import json; from pathlib import Path\n"
            "paths=list(Path('/state').rglob('resource-fault.json')); assert len(paths)==1\n"
            "proof=json.loads(paths[0].read_text()); proof['remaining_processes']=[]\n"
            "for identity in proof['processes']:\n"
            " p=Path('/proc')/str(identity['pid'])/'stat'\n"
            " if p.exists() and p.read_text().rsplit(')',1)[1].split()[19]"
            "==identity['start_time']: proof['remaining_processes'].append(identity)\n"
            "print(json.dumps(proof))"
        )
        if proof["mode"] != scenario or proof["remaining_processes"]:
            raise ValueError("fixture child or descendant survived terminal failure")
        if scenario == "oom":
            after = self.memory()
            assert_oom(before, after)
            self.evidence["scenarios"][scenario]["memory_before"] = before
            self.evidence["scenarios"][scenario]["memory_after"] = after
        else:
            assert_enospc(proof)
            self.execute(
                "import json; from pathlib import Path; Path('/cache/tmp/fill.bin').unlink()\n"
                "print(json.dumps(True))"
            )
        self.preserved()
        self.evidence["scenarios"][scenario]["fault"] = proof
        self.evidence["scenarios"][scenario]["job_id"] = job
        self.evidence["scenarios"][scenario]["error"] = status["error"]
        self.check(
            scenario
            + ": verified fault, typed failure, result/ACK refusal, reaping and source digest"
        )
        self.command("docker", "restart", self.container())
        self.ready()
        if self.api("GET", "/v1/jobs/" + job) != status:
            raise ValueError("failed outcome changed after restart")
        (self.run_dir / scenario / "fixture/mode").write_text("success")
        if self.api("PUT", "/v1/jobs/" + job, request) != status:
            raise ValueError("same UUID re-executed after failure")
        new_job = str(uuid.uuid4())
        result, ack = self.success_job(new_job, self.request(self.audio))
        self.command("docker", "restart", self.container())
        self.ready()
        if (
            self.api("GET", "/v1/jobs/" + new_job + "/result") != result
            or self.api("GET", "/v1/jobs/" + job) != status
        ):
            raise ValueError("durable outcomes changed after second restart")
        self.api("GET", "/v1/jobs/" + job + "/result", expected=409)
        if (
            self.api(
                "POST",
                "/v1/jobs/" + new_job + "/ack",
                {"result_sha256": result["result_sha256"]},
            )
            != ack
        ):
            raise ValueError("ACK receipt changed after restart")
        self.preserved()
        self.check(
            scenario
            + ": durable failure, UUID idempotency, new attempt and ACK recovered across restart"
        )
        self.stop(self.current)

    def full_state(self):
        self.start("state")
        # The 16 MiB state tmpfs is deliberately ephemeral. No durability claim.
        code = (
            "import sys,json; from pathlib import Path; sys.path.insert(0,'/fixture')\n"
            "from child import fill; print(json.dumps(fill(Path('/state/fill.bin'))))"
        )
        proof = self.execute(code)
        assert_enospc(proof)
        self.api("GET", "/health/ready", authenticated=False, expected=503)
        job, request = str(uuid.uuid4()), self.request(self.audio)
        rejected = self.api("PUT", "/v1/jobs/" + job, request, expected=503)
        if (
            rejected["error"]["code"] != "STATE_UNAVAILABLE"
            or rejected["error"]["retryable"] is not True
        ):
            raise ValueError("full state did not refuse acceptance with typed error")
        self.api("GET", "/v1/jobs/" + job, expected=404)
        self.api("GET", "/v1/jobs/" + job + "/result", expected=404)
        self.api("POST", "/v1/jobs/" + job + "/ack", {"result_sha256": "a" * 64}, expected=404)
        self.preserved()
        self.evidence["scenarios"]["state"]["fault"] = proof
        self.evidence["scenarios"]["state"]["job_id"] = job
        self.evidence["scenarios"]["state"]["error"] = rejected["error"]
        self.check(
            "state: ENOSPC, unavailable readiness, refused acceptance and source digest retained"
        )
        self.execute(
            "import json; from pathlib import Path; Path('/state/fill.bin').unlink()\n"
            "print(json.dumps(True))"
        )
        self.ready()
        self.success_job(job, request)
        self.preserved()
        self.check(
            "state: removing only fixture filler restores acceptance, result and idempotent ACK"
        )
        self.stop(self.current)

    def stop(self, container):
        info = self.inspect_owned(container)
        if info["State"]["Running"]:
            self.command("docker", "stop", "--time", "10", container)
        if self.inspect_owned(container)["State"]["Running"]:
            raise ValueError("owned test container still running")

    def run(self):
        try:
            self.failed_child("oom")
            self.failed_child("scratch")
            self.full_state()
            self.evidence["passed"] = True
        finally:
            errors = []
            for container in self.owned:
                try:
                    self.stop(container)
                except (RuntimeError, ValueError, OSError, subprocess.SubprocessError) as error:
                    errors.append(str(error))
            self.evidence["cleanup_passed"] = not errors
            if errors:
                self.evidence["passed"] = False
            self.save()
            if errors and sys.exception() is None:
                raise RuntimeError("owned test container cleanup failed")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--worker-image", required=True)
    parser.add_argument("--run-dir", type=Path, required=True)
    parser.add_argument("--timeout", type=int, default=90)
    args = parser.parse_args()
    if not 10 <= args.timeout <= 300:
        parser.error("timeout must be between 10 and 300 seconds")
    harness = ResourceHarness(args)
    harness.run()
    print("Resource acceptance passed; private evidence.json retained", flush=True)


if __name__ == "__main__":
    main()
