# Independent runtime packaging

App: root `Dockerfile`, Python 3.12 feedparser/mutagen lock in `docker/app/requirements.lock`. Worker: this Dockerfile, API v1, independent `VERSION`, Linux amd64, baseline WhisperX 3.8.6. `compatibility.json` records the engine combination and known cached model revision. Every image contains `/opt/briefcast/runtime.json` with source/version, package/system inventory and worker pipeline hash. Published CI builds emit SBOM and provenance attestations.

## Locks and builds

`baseline-packages.json` records distribution names/versions from the preserved baseline image, without credentials or private installation URLs. Constraints retain those versions. The CPU variant replaces the Torch/Torchaudio/TorchVision CUDA wheels with CPU wheels, uses TorchCodec 0.7.0 (the CPU index has no `0.7.0+cpu`), and omits NVIDIA runtime packages. Triton remains because an installed dependency requires it. CUDA uses the exact installed cu128 combination.

Reproduce lock generation with the existing `uv` tool, independently of root Python 3.14 tooling:

```bash
uv pip compile docker/whisperx/requirements.in --python-version 3.12 --python-platform x86_64-unknown-linux-gnu --generate-hashes --index https://pypi.org/simple --index https://download.pytorch.org/whl/cpu --index-strategy unsafe-best-match --constraint docker/whisperx/constraints-cpu.txt --output-file docker/whisperx/requirements-cpu.lock --no-header
# Repeat with cu128 for both index and file names.
uv pip compile docker/app/requirements.in --python-version 3.12 --python-platform x86_64-unknown-linux-gnu --generate-hashes --output-file docker/app/requirements.lock --no-header
```

Do not remove constraints or upgrade packages as part of separation. Hash checking and `pip check` fail the build on unavailable/incompatible artifacts. ANTLR's baseline release and sgmllib3k have source distributions; they build without isolation using the hashed build-tool lock in `docker/build-requirements.lock`. The pinned Python base is Debian 13, matching the baseline. FFmpeg is pinned to baseline 7.1.5; every Debian package is pinned in `system-packages.lock` and inventoried in the runtime manifest. Missing mirror versions fail the build and require reviewed archival/snapshot recovery, not substitution.

The baseline CUDA TorchCodec wheel cannot import its optional file decoder without `libnppicc.so.12`; the preserved monolith image has the same failure. The content check reports that exact known limitation and rejects other failures. WhisperX uses FFmpeg-decoded waveforms for the default ASR/VAD path. Gated diarization/file decoding needs separate acceptance before enabling it; this phase does not add NPP or change the baseline engine.

```bash
pwsh -File RELEASE.ps1 build -Target app
pwsh -File RELEASE.ps1 build -Target worker -Runtime cpu
pwsh -File RELEASE.ps1 build -Target worker -Runtime cuda
```

Build produces separate archives, SHA256 files and JSON identity manifests. Local deployments use immutable image IDs; published deployments set `BRIEFCAST_IMAGE`/`WHISPERX_IMAGE` to the registry digest from the workflow artifact. App releases do not build workers. `worker-release.yml` reads only the worker version and never bumps/rebuilds the app. Ordinary Compose has no `build:`; `just rebuild` explicitly adds source-build files.

## Explicit Compose modes

Requires PowerShell 7 and [Docker Compose >= 2.24](https://docs.docker.com/compose/how-tos/environment-variables/set-environment-variables/) for optional env files. The ordered files are declared once in `docker/compose-modes.json` and shared by `just`, release helpers and CI tests:

| Runtime | Files after `docker-compose.yml` |
| --- | --- |
| disabled | none |
| cpu | `docker-compose.whisperx.yml` |
| cuda | CPU layer, then `docker-compose.whisperx.gpu.yml` |

Set `WHISPERX_RUNTIME` in the primary env file, or pass `-Runtime` to the scripts. The optional advanced env file is loaded after the primary file for both interpolation and app environment. Process environment overrides still take precedence. Plain Docker commands must specify the same files and env files explicitly. No implicit override is present.

Worker image/token/HF-token mounts are required when selecting CPU/CUDA. State, cache and media directories must already exist; bind mounts refuse to create missing host paths. Provision ownership for the configured UID/GID (default 1000:1000). Worker media is read-only, state/cache writable, root filesystem read-only, `/tmp` and Python's `/cache/tmp` each limited to 512 MiB of temporary memory, capabilities dropped, and no host port is published. The default Compose network permits model egress when `WHISPERX_OFFLINE=false`; it does not block app startup on worker readiness.

| Configuration | Owner/default |
| --- | --- |
| `WHISPERX_ENABLED`, diarization, retry/lock/model/decoder settings | App; disabled/diarization false/concurrency 1 in example |
| `WHISPERX_IMAGE` | Worker; required immutable release reference |
| `HOST_WHISPERX_TOKEN_FILE` | Shared private bearer token, read-only file mount |
| `HOST_WHISPERX_HF_TOKEN_FILE` | Worker only; empty file allowed without gated models |
| `HOST_WHISPERX_STATE_DIR`, `HOST_WHISPERX_CACHE_DIR` | Worker; `./worker-state`, `./worker-cache` |
| `WHISPERX_UID`, `WHISPERX_GID` | Worker identity; 1000:1000 |
| `WHISPERX_ENGINE_PROFILE` | Immutable worker profile; baseline CPU/cu128 defaults |
| `WHISPERX_OFFLINE` | Worker; true, requires preserved caches |
| `WHISPERX_WORKER_BATCH_SIZE`, `MAX_RUNNING`, `MAX_WAITING` | Worker; 16, 1, 4 |
| `WHISPERX_WORKER_TIMEOUT_SECONDS` | Worker cumulative execution budget; 21600 |
| `WHISPERX_WORKER_RESULT_LIMIT_BYTES`, `MIN_FREE_BYTES` | Worker; 67108864, 536870912 |
| `WHISPERX_WORKER_MODELS`, `DIARIZATION_MODELS` | Allow lists; medium.en, pyannote/speaker-diarization-3.1 |

Do not reuse writable production caches/state for a candidate. Preserve the baseline model caches; offline mode avoids refreshing mutable model references. VAD/alignment/diarization revision inventory and cold/warm real audio comparisons remain Stage G acceptance gates. No production cutover until recovery/rollback gates pass.

Targeted deploy verifies archive checksums before Docker mutations, captures the actual running container image, drains an existing worker and refuses replacement while work is queued/running. It recreates only the selected service with `--no-build --no-deps`. Worker acceptance stays drained until an operator verifies compatibility and resumes. Image-only rollback does not restore journal state: retain and rehearse a quiescent state snapshot before replacement, as required by the execution plan. Destructive reset is disabled in the split release helper.
