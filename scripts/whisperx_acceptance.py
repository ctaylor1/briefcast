"""Isolated real-engine/container acceptance; never inherits deployment configuration."""

import argparse
import copy
import hashlib
import json
import os
import secrets
import shutil
import subprocess
import time
import uuid
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def isolated_environment():
    allowed = {
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
    return {key: value for key, value in os.environ.items() if key.upper() in allowed}


def sha256(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def normalized(transcript):
    # Timestamp/identity/package provenance varies between attempts. All actual
    # output fields, stage errors, text, speakers and numeric timestamps remain.
    return {key: value for key, value in transcript.items() if key != "metadata"}


def assert_isolated(config, run_dir, project, allow_dispatch=False, allow_proxy=False):
    services = config["services"]
    allowed = {"briefcast", "whisperx", "postgres"}
    if allow_proxy:
        allowed.add("acceptance-proxy")
    if set(services) - allowed:
        raise ValueError("unexpected acceptance services")
    app = services["briefcast"]
    env = app["environment"]
    if (
        env["DATABASE_URL"] != "sqlite:///config/acceptance.db"
        or str(env["LLM_ENABLED"]).lower() != "false"
        or str(env["WHISPERX_ENABLED"]).lower() != ("true" if allow_dispatch else "false")
        or env.get("PASSWORD", "")
        or app["container_name"] != project + "-app"
        or (allow_proxy and not allow_dispatch)
        or (
            allow_dispatch
            and env.get("WHISPERX_SERVICE_URL")
            != ("http://acceptance-proxy:8091" if allow_proxy else "http://whisperx:8090")
        )
    ):
        raise ValueError("acceptance must use synthetic state and disabled scheduling/LLM")
    for name in ("briefcast", "whisperx"):
        for mount in services[name]["volumes"]:
            source = Path(mount["source"]).resolve()
            if mount["type"] != "bind" or run_dir not in source.parents:
                raise ValueError("acceptance mount escapes its new private run directory")
    if services["whisperx"].get("ports") or services["whisperx"].get("env_file"):
        raise ValueError("worker must have no host API port or app configuration")
    if allow_proxy:
        proxy = services["acceptance-proxy"]
        mounts = proxy.get("volumes", [])
        if (
            proxy.get("ports")
            or proxy.get("environment")
            or proxy.get("env_file")
            or proxy.get("privileged")
            or proxy.get("network_mode")
            or not proxy.get("read_only")
            or len(mounts) != 1
            or mounts[0]["type"] != "bind"
            or Path(mounts[0]["source"]).resolve() != run_dir / "gate"
            or mounts[0]["target"] != "/gate"
        ):
            raise ValueError("proxy must have only its isolated gate mount and private network")
    for port in app["ports"]:
        if port.get("host_ip") != "127.0.0.1" or str(port["published"]) != "0":
            raise ValueError("app acceptance port must be random and localhost only")


class Harness:
    def __init__(self, args):
        self.args = args
        self.env = isolated_environment()
        self.run_dir = args.run_dir.resolve()
        if self.run_dir == ROOT or ROOT in self.run_dir.parents or self.run_dir.exists():
            raise ValueError("run directory must be NEW and outside the repository")
        self.run_dir.mkdir(mode=0o700)
        self.project = "briefcast-acceptance-" + uuid.uuid4().hex[:12]
        self.started = False
        self.evidence = {"schema": 1, "project": self.project, "device": args.runtime, "checks": []}
        for name in ("config", "media/audio", "logs", "state", "recovery"):
            (self.run_dir / name).mkdir(mode=0o700, parents=True)
        # Candidates never share a writable cache with the baseline/production.
        if args.cold_cache_only:
            (self.run_dir / "cache").mkdir(mode=0o700)
        else:
            # HF snapshots contain Linux symlinks. Windows file traversal can
            # silently omit Docker Desktop's reparse entries; resolve them in
            # the same Linux filesystem view used for inference. The source
            # is read-only and the destination belongs to this NEW run only.
            (self.run_dir / "cache").mkdir(mode=0o700)
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
                f"type=bind,source={args.cache.resolve()},target=/source,readonly",
                "--mount",
                f"type=bind,source={self.run_dir / 'cache'},target=/cache",
                "--mount",
                f"type=bind,source={ROOT / 'scripts/whisperx_acceptance_cache.py'},"
                "target=/copy-cache.py,readonly",
                self.image_id(args.worker_image),
                "/copy-cache.py",
                "/source",
                "/cache",
                timeout=max(300, args.timeout),
            )
        (self.run_dir / "token").write_text(secrets.token_hex(32))
        (self.run_dir / "hf-token").write_text("")
        for path in (self.run_dir / "token", self.run_dir / "hf-token"):
            path.chmod(0o600)
        self.media = []
        for index, audio in enumerate(args.audio):
            destination = self.run_dir / "media/audio" / (str(index) + audio.suffix.lower())
            shutil.copyfile(audio, destination)
            self.media.append(destination)
        malformed = self.run_dir / "media/audio/malformed.wav"
        malformed.write_bytes(b"not a decodable audio file")
        self.malformed = malformed
        self.profile = "baseline-wx3.8.6-" + ("cu128" if args.runtime == "cuda" else "cpu") + "-v1"
        self.env_file = self.run_dir / "acceptance.env"
        values = {
            "BRIEFCAST_IMAGE": self.image_id(args.app_image),
            "WHISPERX_IMAGE": self.image_id(args.worker_image),
            "BRIEFCAST_CONTAINER_NAME": self.project + "-app",
            "DATABASE_URL": "sqlite:///config/acceptance.db",
            "DB_DRIVER": "sqlite",
            "LLM_ENABLED": "false",
            "WHISPERX_ENABLED": "true" if args.app_restarts else "false",
            "WHISPERX_RUNTIME": args.runtime,
            "WHISPERX_ENGINE_PROFILE": self.profile,
            "WHISPERX_ENV_FILE": str(self.run_dir / "missing.env"),
            "HOST_PORT": "127.0.0.1:0",
            "WHISPERX_OFFLINE": "true",
            "WHISPERX_DEVICE": args.runtime,
            "WHISPERX_COMPUTE_TYPE": "float16" if args.runtime == "cuda" else "int8",
            "WHISPERX_BATCH_SIZE": "16",
            # Committed receipts are reconciled by the normal scheduler even
            # when manual repair finds no unfinished transcript rows.
            "WHISPERX_CHECK_FREQUENCY": "1",
        }
        for key, suffix in {
            "HOST_CONFIG_DIR": "config",
            "HOST_ASSETS_DIR": "media",
            "HOST_LOGS_DIR": "logs",
            "HOST_WHISPERX_STATE_DIR": "state",
            "HOST_WHISPERX_CACHE_DIR": "cache",
            "HOST_WHISPERX_TOKEN_FILE": "token",
            "HOST_WHISPERX_HF_TOKEN_FILE": "hf-token",
        }.items():
            values[key] = (self.run_dir / suffix).as_posix()
        self.env_file.write_text("".join(f"{key}={value}\n" for key, value in values.items()))
        self.proxy_file = None
        if args.app_crash_boundaries:
            gate_dir = self.run_dir / "gate"
            gate_dir.mkdir(mode=0o700)
            (gate_dir / "mode").write_text("off")
            shutil.copyfile(ROOT / "scripts/whisperx_acceptance_proxy.py", gate_dir / "proxy.py")
            self.proxy_file = self.run_dir / "proxy-compose.json"
            self.proxy_file.write_text(
                json.dumps(
                    {
                        "services": {
                            "briefcast": {
                                "environment": {
                                    "WHISPERX_SERVICE_URL": "http://acceptance-proxy:8091"
                                }
                            },
                            "acceptance-proxy": {
                                "image": values["BRIEFCAST_IMAGE"],
                                "entrypoint": ["python"],
                                "command": ["/gate/proxy.py", "--control-dir", "/gate"],
                                "init": True,
                                "read_only": True,
                                "cap_drop": ["ALL"],
                                "security_opt": ["no-new-privileges:true"],
                                "volumes": [
                                    {
                                        "type": "bind",
                                        "source": gate_dir.as_posix(),
                                        "target": "/gate",
                                    }
                                ],
                            },
                        }
                    }
                )
            )
        self.evidence["images"] = {
            key: values[key] for key in ("BRIEFCAST_IMAGE", "WHISPERX_IMAGE")
        }
        if os.name != "nt":
            # Provision only the newly created isolated tree for Compose's
            # fixed worker UID. Never chown the source cache or media library.
            provision = (
                "import os; from pathlib import Path; root=Path('/acceptance'); "
                "os.chown(root,1000,1000); "
                "[os.chown(p,1000,1000) for p in root.rglob('*')]"
            )
            self.command(
                "docker",
                "run",
                "--rm",
                "--network",
                "none",
                "--user",
                "0:0",
                "--entrypoint",
                "python",
                "--mount",
                f"type=bind,source={self.run_dir},target=/acceptance",
                values["WHISPERX_IMAGE"],
                "-c",
                provision,
            )

    def command(self, *args, input=None, timeout=300):
        result = subprocess.run(
            list(map(str, args)),
            cwd=ROOT,
            env=self.env,
            input=input,
            text=True,
            capture_output=True,
            timeout=timeout,
        )
        if result.returncode:
            # Raw engine output may contain private speech. Keep it outside Git.
            (self.run_dir / "last-error.log").write_text(result.stdout + result.stderr)
            raise RuntimeError("acceptance command failed; see private last-error.log")
        return result.stdout.strip()

    def compose(self, *args):
        return self.command(
            "pwsh",
            "-NoLogo",
            "-NoProfile",
            "-File",
            ROOT / "scripts/Compose.ps1",
            "-EnvFile",
            self.env_file,
            "-Runtime",
            self.args.runtime,
            "-ProjectName",
            self.project,
            *(["--file", self.proxy_file] if self.proxy_file else []),
            *args,
        )

    def image_id(self, image):
        return self.command("docker", "image", "inspect", "--format", "{{.Id}}", image)

    def container(self, service):
        container = self.compose("ps", "-q", service)
        if not container or "\n" in container:
            raise ValueError("expected one isolated container")
        label = self.command(
            "docker",
            "inspect",
            "--format",
            '{{index .Config.Labels "com.docker.compose.project"}}',
            container,
        )
        if label != self.project:
            raise ValueError("container does not belong to acceptance project")
        return container

    def api(self, method, path, body=None, authenticated=True, expected=200, service="whisperx"):
        # Read the bearer token inside the worker; never put it on a command line.
        port = 8090 if service == "whisperx" else 8080
        code = (
            "import json,urllib.request,urllib.error\n"
            f"r=urllib.request.Request('http://127.0.0.1:{port}'+"
            + repr(path)
            + ",method="
            + repr(method)
            + ",data="
            + (repr(json.dumps(body).encode()) if body is not None else "None")
            + ")\n"
            "r.add_header('Content-Type','application/json')\n"
        )
        if authenticated and service == "whisperx":
            code += (
                "token=open('/run/secrets/whisperx-service-token').read().strip()\n"
                "r.add_header('Authorization','Bearer '+token)\n"
            )
        code += (
            "try:\n response=urllib.request.urlopen(r,timeout=10)\n"
            "except urllib.error.HTTPError as e:\n response=e\n"
            "print(json.dumps({'status':response.status,'body':json.loads(response.read())}))\n"
        )
        result = json.loads(
            self.command("docker", "exec", "-i", self.container(service), "python", "-", input=code)
        )
        if result["status"] != expected:
            raise ValueError(f"unexpected API status: {result['status']} (expected {expected})")
        return result["body"]

    def check(self, name):
        self.evidence["checks"].append(name)
        self.save()
        print("PASS " + name, flush=True)

    def save(self):
        (self.run_dir / "evidence.json").write_text(json.dumps(self.evidence, indent=2) + "\n")

    def ready(self, drained=False):
        deadline = time.monotonic() + 150
        while time.monotonic() < deadline:
            try:
                self.api(
                    "GET", "/health/ready", authenticated=False, expected=503 if drained else 200
                )
                caps = self.api("GET", "/v1/capabilities")
                assert 1 in caps["contract_versions"] and caps["engine_profile"] == self.profile
                return caps
            except (RuntimeError, ValueError, AssertionError):
                time.sleep(2)
        raise TimeoutError("isolated worker readiness timed out")

    def request(self, audio):
        request = json.loads((ROOT / "docs/whisperx-separation/fixtures/put-job.json").read_text())
        for field in ("deployment_id", "episode_id", "generation_id"):
            request[field] = str(uuid.uuid4())
        request["engine_profile"] = self.profile
        request["input"] = {
            "relative_path": audio.relative_to(self.run_dir / "media").as_posix(),
            "size_bytes": audio.stat().st_size,
            "sha256": sha256(audio),
        }
        return request

    def wait(self, job):
        deadline = time.monotonic() + self.args.timeout
        while time.monotonic() < deadline:
            status = self.api("GET", "/v1/jobs/" + job)
            if status["state"] in ("succeeded", "failed", "cancelled"):
                return status
            time.sleep(2)
        raise TimeoutError("real inference exceeded acceptance deadline")

    def app_identity(self):
        container = self.container("briefcast")
        image = self.command("docker", "inspect", "--format", "{{.Image}}", container)
        return {"container": container, "image": image}

    def app_ready(self):
        code = "import urllib.request; assert urllib.request.urlopen('http://127.0.0.1:8080/',timeout=5).status==200"
        self.command("docker", "exec", self.container("briefcast"), "python", "-c", code)

    def app_sql(self, query, parameters=()):
        # Test fixtures only, on the app side of the DB ownership boundary.
        code = (
            "import sqlite3,json\n"
            "db=sqlite3.connect('/config/acceptance.db',timeout=10)\n"
            "cursor=db.execute(" + repr(query) + "," + repr(parameters) + ")\n"
            "print(json.dumps(cursor.fetchall()))\n"
            "db.commit()\n"
        )
        return json.loads(
            self.command(
                "docker", "exec", "-i", self.container("briefcast"), "python", "-", input=code
            )
        )

    def app_restarts(self, boundary="running"):
        if boundary != "running":
            self.set_gate(boundary)
        podcast, episode = str(uuid.uuid4()), str(uuid.uuid4())
        audio = self.media[0]
        source_hash = sha256(audio)
        self.app_sql(
            "UPDATE settings SET auto_download=0,retention_keep_all=1,summarization_enabled=0"
        )
        self.app_sql(
            "INSERT INTO podcasts(id,title,url,is_paused,retention_keep_all) VALUES (?,?,?,?,?)",
            (podcast, "Isolated acceptance", "http://invalid.test/fixture", 1, 1),
        )
        self.app_sql(
            "INSERT INTO podcast_items(id,podcast_id,title,guid,file_url,download_path,"
            "download_status,download_date,pub_date,transcript_status,transcript_json,"
            "canonical_transcript,transcript_checkpoint_json,llm_summary_status) "
            "VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
            (
                episode,
                podcast,
                "Isolated speech " + episode,
                episode,
                "http://invalid.test/speech",
                "/assets/" + audio.relative_to(self.run_dir / "media").as_posix(),
                2,
                "2026-10-02 00:00:00+00:00",
                "2026-10-02 00:00:00+00:00",
                "pending_whisperx",
                "",
                "",
                "",
                "pending",
            ),
        )
        self.api(
            "POST",
            "/settings/repair-work?limit=1",
            service="briefcast",
            authenticated=False,
            expected=202,
        )
        deadline = time.monotonic() + self.args.timeout
        job = None
        while time.monotonic() < deadline:
            raw = self.app_sql(
                "SELECT transcript_checkpoint_json FROM podcast_items WHERE id=?", (episode,)
            )[0][0]
            if raw:
                envelope = json.loads(raw)
                try:
                    status = self.api("GET", "/v1/jobs/" + envelope["job_id"])
                    marker = self.run_dir / "gate/reached.json"
                    reached = (boundary == "running" and status["state"] == "running") or (
                        boundary != "running"
                        and marker.exists()
                        and json.loads(marker.read_text())
                        == {
                            "boundary": boundary,
                            "path": "/v1/jobs/" + envelope["job_id"] + "/" + boundary,
                        }
                    )
                    if reached:
                        job = envelope["job_id"]
                        break
                except ValueError:
                    pass  # Durable envelope may precede worker acceptance.
            time.sleep(1)
        if job is None:
            raise TimeoutError("app did not reach the requested crash boundary")
        before = self.app_sql(
            "SELECT transcript_status,transcript_json,transcript_checkpoint_json,llm_summary "
            "FROM podcast_items WHERE id=?",
            (episode,),
        )[0]
        receipt = json.loads(before[2]).get("receipt_result_sha256")
        if boundary == "ack":
            if before[0] != "available" or not before[1] or not receipt or before[3]:
                raise ValueError("ACK gate did not follow a durable transcript commit")
        elif before[1] or receipt or before[3]:
            raise ValueError("result gate must precede transcript commit and summary")
        bookmark = "2026-10-02 12:34:56+00:00"
        self.app_sql(
            "UPDATE podcast_items SET is_played=1,bookmark_date=? WHERE id=?", (bookmark, episode)
        )
        app = self.container("briefcast")
        self.command("docker", "kill", "--signal", "KILL", app)
        if boundary != "running":
            # Stop blocked proxy threads before opening the gate. A pending
            # ACK must never be forwarded on behalf of the killed app.
            self.compose("stop", "acceptance-proxy")
            self.set_gate("off")
            self.compose("up", "-d", "--no-build", "--no-deps", "acceptance-proxy")
        if self.wait(job)["state"] != "succeeded":
            raise ValueError("worker did not finish after app crash")
        result = self.api("GET", "/v1/jobs/" + job + "/result")
        self.check("worker retains durable success while app is stopped")
        self.command("docker", "start", app)
        for _ in range(60):
            try:
                self.app_ready()
                break
            except RuntimeError:
                time.sleep(1)
        else:
            raise TimeoutError("app did not restart")
        self.api(
            "POST",
            "/settings/repair-work?limit=1",
            service="briefcast",
            authenticated=False,
            expected=202,
        )
        deadline = time.monotonic() + self.args.timeout
        while time.monotonic() < deadline:
            row = self.app_sql(
                "SELECT transcript_status,transcript_checkpoint_json,canonical_transcript,"
                "is_played,bookmark_date,llm_summary,transcript_json "
                "FROM podcast_items WHERE id=?",
                (episode,),
            )[0]
            if row[0] == "available" and not row[1]:
                if not row[2] or row[3] != 1 or row[4] != bookmark or row[5]:
                    raise ValueError("app persistence lost edits or summarized prematurely")
                if json.loads(row[6])["metadata"]["job_id"] != job:
                    raise ValueError("app submitted a duplicate attempt after restart")
                break
            time.sleep(1)
        else:
            raise TimeoutError("app did not commit/reconcile durable result")
        if self.api("GET", "/v1/jobs/" + job + "/result") != result or sha256(audio) != source_hash:
            raise ValueError("result or audio changed during app recovery")
        exports = list((self.run_dir / "media/transcripts").rglob("*" + episode + ".txt"))
        if len(exports) != 1 or exports[0].read_text(encoding="utf-8") != row[2]:
            raise ValueError("app did not export this episode's committed transcript")
        self.check(
            "app restart at " + boundary + " reconnects same job, commits canonical/export, "
            "preserves bookmark/play and ACKs"
        )
        self.evidence.setdefault("app_crashes", []).append(
            {
                "boundary": boundary,
                "job_id": job,
                "episode_id": episode,
                "receipt_before_kill": receipt,
                "result_sha256": result["result_sha256"],
            }
        )
        self.save()

    def set_gate(self, mode):
        control = self.run_dir / "gate"
        marker = control / "reached.json"
        if marker.exists():
            marker.unlink()
        temporary = control / "mode.tmp"
        temporary.write_text(mode)
        temporary.replace(control / "mode")

    def baseline(self, audio, request):
        config = {
            **request["options"],
            "device": self.args.runtime,
            "compute_type": "float16" if self.args.runtime == "cuda" else "int8",
            "batch_size": 16,
        }
        arguments = [
            "docker",
            "run",
            "--rm",
            "--network",
            "none",
            "--read-only",
            "--tmpfs",
            "/tmp:size=536870912",
            "--entrypoint",
            "python",
        ]
        if self.args.runtime == "cuda":
            arguments += ["--gpus", "all"]
        for source, target, readonly in [
            (self.run_dir / "media", "/media", True),
            (self.run_dir / "cache", "/cache", False),
        ]:
            arguments += [
                "--mount",
                f"type=bind,source={source},target={target}" + (",readonly" if readonly else ""),
            ]
        for key, value in {
            "WHISPERX_CONFIG_JSON": json.dumps(config),
            "HF_HUB_OFFLINE": "1",
            "TRANSFORMERS_OFFLINE": "1",
            "HF_HOME": "/cache/huggingface",
            "TORCH_HOME": "/cache/torch",
            "NLTK_DATA": "/cache/nltk",
            "HOME": "/cache/home",
            "XDG_CACHE_HOME": "/cache/home/.cache",
        }.items():
            arguments += ["--env", key + "=" + value]
        raw = self.command(
            *arguments,
            self.image_id(self.args.baseline_image),
            self.args.baseline_script,
            "/media/" + request["input"]["relative_path"],
            timeout=self.args.timeout,
        )
        return json.loads(raw)

    def run(self):
        config = json.loads(self.compose("config", "--format", "json"))
        assert_isolated(
            config,
            self.run_dir,
            self.project,
            allow_dispatch=self.args.app_restarts,
            allow_proxy=self.args.app_crash_boundaries,
        )
        self.check("rendered Compose isolation guards")
        self.started = True  # Cleanup even if startup partially fails.
        services = ["briefcast", "whisperx"]
        if self.args.app_crash_boundaries:
            services.append("acceptance-proxy")
        self.compose("up", "-d", "--no-build", "--no-deps", *services)
        self.ready()
        self.app_ready()
        app_before = self.app_identity()
        self.api("GET", "/v1/capabilities", authenticated=False, expected=401)
        self.api("GET", "/v1/jobs/" + str(uuid.uuid4()), expected=404)
        self.check("live authenticated worker and lean app")
        if self.args.cold_cache_only:
            # Disconnect ONLY the labelled fixture worker. Missing weights
            # cannot accidentally download; loopback API probes still work.
            self.command(
                "docker",
                "network",
                "disconnect",
                self.project + "_default",
                self.container("whisperx"),
            )
            audio, job = self.media[0], str(uuid.uuid4())
            request = self.request(audio)
            self.api("PUT", "/v1/jobs/" + job, request, expected=201)
            status = self.wait(job)
            if status["state"] != "failed" or sha256(audio) != request["input"]["sha256"]:
                raise ValueError("offline cold cache must fail visibly and preserve audio")
            self.evidence["cold_cache_error_code"] = status["error"]["code"]
            self.check("offline cold cache produces terminal failure and preserves audio")
            self.evidence["complete"] = True
            self.save()
            return
        results = []
        for index, audio in enumerate(self.media):
            request, job = self.request(audio), str(uuid.uuid4())
            start = time.monotonic()
            self.api("PUT", "/v1/jobs/" + job, request, expected=201)
            self.api("PUT", "/v1/jobs/" + job, request)
            if index == 0:
                queued = str(uuid.uuid4())
                self.api("PUT", "/v1/jobs/" + queued, self.request(audio), expected=201)
                self.api("POST", "/v1/jobs/" + queued + "/cancel")
                self.api("POST", "/v1/jobs/" + queued + "/cancel")
                if self.wait(queued)["state"] != "cancelled":
                    raise ValueError("queued cancellation did not persist")
                self.check("queued cancellation and duplicate cancel")
            conflicting = copy.deepcopy(request)
            conflicting["attempt"] += 1
            self.api("PUT", "/v1/jobs/" + job, conflicting, expected=409)
            if (self.args.restart_at_checkpoint or self.args.kill_at_checkpoint) and index == 0:
                deadline = time.monotonic() + self.args.timeout
                while time.monotonic() < deadline:
                    status = self.api("GET", "/v1/jobs/" + job)
                    if (
                        status["checkpoint"]["completed_seconds"] > 0
                        and status["state"] == "running"
                    ):
                        if self.args.kill_at_checkpoint:
                            self.command(
                                "docker", "kill", "--signal", "KILL", self.container("whisperx")
                            )
                            self.compose("up", "-d", "--no-build", "--no-deps", "whisperx")
                        else:
                            self.compose("restart", "whisperx")
                        self.ready()
                        break
                    if status["state"] != "running" and status["state"] != "queued":
                        raise ValueError("audio did not expose a resumable running chunk boundary")
                    time.sleep(0.5)
                else:
                    raise TimeoutError("no checkpoint observed")
            status = self.wait(job)
            if status["state"] != "succeeded":
                raise ValueError("real engine did not succeed")
            result = self.api("GET", "/v1/jobs/" + job + "/result")
            if (self.args.restart_at_checkpoint or self.args.kill_at_checkpoint) and index == 0:
                if not result["transcript"]["metadata"]["resumed_from_checkpoint"]:
                    raise ValueError("worker did not resume committed chunks")
                stop_mode = "SIGKILL" if self.args.kill_at_checkpoint else "graceful restart"
                self.check("real checkpoint resume after " + stop_mode)
            (self.run_dir / f"result-{index}.json").write_text(json.dumps(result))
            if sha256(audio) != request["input"]["sha256"]:
                raise ValueError("valid source media changed")
            if self.args.baseline_image:
                baseline = self.baseline(audio, request)
                (self.run_dir / f"baseline-{index}.json").write_text(json.dumps(baseline))
                if normalized(baseline) != normalized(result["transcript"]):
                    raise ValueError("baseline normalized output differs (no implicit tolerance)")
                self.check(f"exact baseline output audio {index}")
            results.append((job, result))
            self.evidence.setdefault("audio", []).append(
                {
                    "index": index,
                    "sha256": sha256(audio),
                    "seconds": status["total_seconds"],
                    "elapsed_seconds": round(time.monotonic() - start, 3),
                }
            )
            self.check(f"real inference, duplicate/conflict and media preservation audio {index}")
        malformed_request = self.request(self.malformed)
        bad_job = str(uuid.uuid4())
        self.api("PUT", "/v1/jobs/" + bad_job, malformed_request, expected=201)
        if self.wait(bad_job)["state"] != "failed" or not self.malformed.exists():
            raise ValueError("malformed audio must fail visibly and remain intact")
        self.check("malformed audio failure preserves source")
        self.api("POST", "/v1/admin/drain")
        rejected = str(uuid.uuid4())
        self.api("PUT", "/v1/jobs/" + rejected, self.request(self.media[0]), expected=503)
        self.compose("stop", "whisperx")
        self.app_ready()
        self.check("app remains available with stopped worker")
        worker_id = self.evidence["images"]["WHISPERX_IMAGE"]
        snapshot = self.run_dir / "recovery/pre-replacement"
        for operation, state in [
            ("snapshot", self.run_dir / "state"),
            ("restore", self.run_dir / "restored-state"),
        ]:
            self.command(
                "pwsh",
                "-NoLogo",
                "-NoProfile",
                "-File",
                ROOT / "scripts/WorkerRecovery.ps1",
                "-Operation",
                operation,
                "-SnapshotDir",
                snapshot,
                "-StateDir",
                state,
                "-Image",
                worker_id,
            )
        # Same isolated Compose selection, restored journal and compatible second image.
        self.env["HOST_WHISPERX_STATE_DIR"] = (self.run_dir / "restored-state").as_posix()
        replacement = self.image_id(self.args.replacement_image)
        if replacement == worker_id:
            raise ValueError("replacement evidence requires a distinct immutable image")
        for image_id in (replacement, worker_id):
            self.env["WHISPERX_IMAGE"] = image_id
            self.compose("up", "-d", "--no-build", "--no-deps", "--force-recreate", "whisperx")
            caps = self.ready(drained=True)
            if caps["capacity"]["accepting"] or self.app_identity() != app_before:
                raise ValueError("replacement changed acceptance or app identity")
            for job, result in results:
                if self.api("GET", "/v1/jobs/" + job + "/result") != result:
                    raise ValueError("durable result changed across restore/replacement")
            self.compose("stop", "whisperx")
        self.check(
            "state restore, independent compatible replacement and rollback; app identity unchanged"
        )
        self.compose("up", "-d", "--no-build", "--no-deps", "whisperx")
        self.ready(drained=True)
        self.api("POST", "/v1/admin/resume")
        for job, result in results:
            self.api("POST", "/v1/jobs/" + job + "/ack", {"result_sha256": result["result_sha256"]})
            self.api("POST", "/v1/jobs/" + job + "/ack", {"result_sha256": result["result_sha256"]})
        self.check("explicit resume and idempotent receipt")
        if self.args.app_restarts:
            self.app_restarts()
        if self.args.app_crash_boundaries:
            for boundary in ("result", "ack"):
                self.app_restarts(boundary)
        if self.app_identity() != app_before:
            raise ValueError("app image/container changed during crash recovery")
        self.evidence["app_identity"] = app_before
        self.evidence["replacement_image"] = replacement
        self.evidence["complete"] = True
        self.save()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("app-image", "worker-image", "replacement-image", "cache", "run-dir"):
        parser.add_argument(
            "--" + name, required=True, type=Path if name in ("cache", "run-dir") else str
        )
    parser.add_argument("--audio", type=Path, action="append", required=True)
    parser.add_argument("--runtime", choices=("cpu", "cuda"), default="cpu")
    parser.add_argument("--baseline-image")
    parser.add_argument("--baseline-script", default="/api/scripts/whisperx_transcribe.py")
    parser.add_argument("--restart-at-checkpoint", action="store_true")
    parser.add_argument("--kill-at-checkpoint", action="store_true")
    parser.add_argument("--cold-cache-only", action="store_true")
    parser.add_argument("--app-restarts", action="store_true")
    parser.add_argument(
        "--app-crash-boundaries",
        action="store_true",
        help="also SIGKILL the app after success and after commit, before ACK",
    )
    parser.add_argument("--timeout", type=int, default=1800)
    args = parser.parse_args()
    if args.app_crash_boundaries:
        if args.cold_cache_only:
            parser.error("app crash boundaries require a warm cache")
        args.app_restarts = True
    harness = Harness(args)
    try:
        harness.run()
    finally:
        if harness.started:
            # No down, volume removal, live data cleanup or image pruning.
            services = ["briefcast", "whisperx"]
            if args.app_crash_boundaries:
                services.append("acceptance-proxy")
            harness.compose("stop", *services)


if __name__ == "__main__":
    main()
