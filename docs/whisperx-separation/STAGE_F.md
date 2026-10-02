# Stage F — independent images and release tooling

## Scope and source

Implemented independent app/CPU/CUDA packaging, explicit Compose modes, shared selection for command wrappers, independently versioned worker releases, immutable offline artifacts, and packaging CI. No production deployment, publication, application API/schema change, root tooling lock change, or engine upgrade was performed.

Runtime images below were built from committed source `7d8545f6c0a63ecbb182f318c5deb0b4b63b02bb`, following packaging commit `8c51abd97b0f27afad6fdfa382f8688e9c54632f`. Subsequent evidence/check changes document the observed optional CUDA decoder limitation and bound Python scratch storage. The pipeline remains unchanged from Stage E.

## Packaging and configuration changes

- Root Dockerfile contains the app, frontend and feedparser/mutagen helpers. It omits the WhisperX script, Torch, WhisperX, CTranslate2 and NVIDIA packages. App privilege behavior is retained; the worker runs as configurable UID/GID, default 1000:1000.
- `docker/whisperx/Dockerfile` builds an independent static Go worker with baseline WhisperX 3.8.6, CTranslate2 4.8.1, faster-whisper 1.2.1 and pyannote.audio 4.0.7. CPU uses Torch/torchaudio 2.8.0+cpu; CUDA retains 2.8.0+cu128. CPU TorchCodec is 0.7.0 because no 0.7.0+cpu artifact exists; CUDA retains 0.7.0+cu128.
- Separate Python 3.12 runtime locks include the transitive dependency versions and hashes. Source-only ANTLR/sgmllib3k releases build without isolation using a separately hashed setuptools/wheel lock. Root Python 3.14 development tooling is independent. Wheel artifacts resolved and installed successfully; no baseline engine version was substituted.
- Python, Go and Node bases are pinned by digest. The Debian 13 runtime preserves FFmpeg 7.1.5; all installed Debian versions are locked and inventoried. The pinned base has newer Debian patch packages than the original monolith: this is not a claim of identical system binaries. Missing archive/mirror versions fail the build rather than upgrading silently.
- Images include source/version/package/system/pipeline manifests; worker manifests specify API v1, `briefcast-script-v1` and `briefcast-whisperx-chunks-v1`. SBOM and SLSA provenance attestations are present in the exported archives.
- Normal Compose has an explicit required app image and no implicit GPU/build layer. `docker/compose-modes.json` selects disabled, CPU, or CPU+CUDA files. GPU reservation belongs only to the worker. Clearly named legacy files preserve the former definitions.
- The optional advanced env file participates in interpolation after the primary env file. Diarization/concurrency/item limits are configurable; the example preserves diarization false/concurrency one and disables transcription until explicitly enabled. Worker environment excludes app DB/LLM settings and uses private token-file mounts.
- Worker media is read-only; state/cache require pre-provisioned directories. Root filesystem is read-only, capabilities are dropped, no worker host port is published, and both `/tmp` and the Python runner's `/cache/tmp` are capped at 512 MiB each. The network permits optional model egress; app startup has no worker readiness dependency.
- `just`, deploy, verify and rollback use the same ordered files. File paths are read as literal environment/parameter values and passed in argument arrays. Source-build overlays are explicit. Deployment checks archive identity/checksum, records the actual running container image, drains an existing worker, refuses active/queued work, and recreates only the selected service with `--no-build --no-deps`.
- App readiness uses the running container's basic-auth configuration without exposing credentials. Worker image-only rollback preserves the app container; state snapshot/restore and resume remain operator recovery gates. Destructive reset helpers are disabled because the former monolith reset can erase media referenced by durable jobs.
- Local version tags are protected from accidental reuse. The app and worker have separate version files and workflows; worker publishing does not bump/rebuild the app. CI checks Compose matrices, source-build selection, helper deployments and each image variant.

## Verified runtime artifacts

These are local rehearsal artifacts, not published registry digests or production images. Manifests pin Docker image IDs and archive SHA256 values. The directory pointer is private at `%TEMP%\briefcast-stage-f-directory.txt`; final archives/manifests are under its `verified-artifacts` subdirectory. Initial CLI-build artifacts are kept separately under `artifacts`.

| Image | Version | Immutable local image ID | Uncompressed bytes |
| --- | --- | --- | ---: |
| `briefcast:stage-f-verified` | 1.9.4 | `sha256:eadbf9df0f3418328d5824739b08e63abe9b4e1f43d0b5743ef4242acea6e2dc` | 271556357 |
| `briefcast-whisperx-cpu:stage-f-verified` | 1.0.0 | `sha256:b032e151f70d8b1f1236a50864405677cb74da8aec938c90a89e81bd670bace0` | 4113417626 |
| `briefcast-whisperx-cu128:stage-f-verified` | 1.0.0 | `sha256:64be5c42e52b03f6371a6183acd08ec2925ef312ca5b06f6f00aa30dde520fba` | 13144140052 |

| Archive | SHA256 |
| --- | --- |
| `briefcast_v1.9.4.tar` | `33140c8b2f9137547c2936dbb3dbe459664aeb3b85c4244ec63316fc02fe6317` |
| `briefcast-whisperx-cpu_v1.0.0.tar` | `afdb42ad9dafad6de7cef1b945f49cd730694e29bc9796620d97d3c316d2966d` |
| `briefcast-whisperx-cu128_v1.0.0.tar` | `bcc8862d29241fb905ac0a33e29227e53569964552537d5485b9540209558de3` |

All three archive load/identity checks passed. All three archives contain SPDX SBOM and SLSA provenance predicates, independently checked by reading the OCI archive attestations.

## Runtime evidence and baseline limitation

All final image content checks passed: every locked Python distribution matches its required version, all worker system packages match the system lock, core runtime imports succeed, and the app retains feedparser/mutagen functionality without the transcription stack.

The initial strict CUDA smoke check failed on direct TorchCodec import: `libnppicc.so.12` is missing. Running the same import against the preserved monolith image confirmed the identical failure. This is an optional CUDA file-decoder limitation; adding NPP would change the baseline. The content check now reports this exact known failure and rejects other import failures. Actual WhisperX processing below uses FFmpeg-decoded waveforms and successfully uses pyannote VAD; no VAD fallback was needed. Diarization/file decoding and GPU execution need separate Stage G acceptance.

Both final worker images executed a real synthetic medium.en/int8/batch-16 transcription **on CPU**, with alignment enabled and diarization disabled, on Linux amd64 in Windows Docker Desktop. They ran as UID 1000 with read-only root/media, dropped capabilities, no network or host port, isolated journal/token, and the preserved isolated rehearsal caches. Both transcripts matched the Stage D baseline text; pyannote VAD and alignment were used. Result retrieval from the CPU journal succeeded after container restart. An additional container verified `/cache/tmp` was writable at UID 1000, had an actual 536870912-byte tmpfs limit, and could replay the persisted result. Rehearsal containers are stopped.

The lean app started in isolation, created/read its SQLite database using its CGO build and served HTTP. The readiness probe also succeeded with basic auth enabled, without printing its generated test credential.

Preserved model evidence:

- `Systran/faster-whisper-medium.en` cached revision: `a29b04bd15381511a9af671baec01072039215e3`.
- Alignment cache `torch/hub/checkpoints/wav2vec2_fairseq_base_ls960_asr_ls960.pth`: 377664473 bytes, SHA256 `488fd4f16de84438ffc945334278c1b9fb9b7159a806c1080b16111a958c945d`.
- Default offline mode reuses those caches rather than refreshing mutable references. Complete VAD/diarization/cold-cache inventory remains Stage G work.

## Validation commands and results

- `go test ./...` with `CGO_ENABLED=0`, integration DB/real-engine opt-ins unset: passed; service suite 150.720 seconds. Host tests use the existing pure-Go SQLite fallback. Linux final images separately exercised the app CGO database and worker SQLite/Python execution.
- `uv run pytest`: 35 tests passed, including 16 packaging cases. Tests use synthetic env files and mock deployment/publication commands. Matrix covers disabled/CPU/CUDA, optional advanced file present/absent, explicit source-build overlays, literal paths containing spaces/`$`, targeted deploy/rollback, checksum rejection, actual-container rollback identity and worker-only version publishing.
- `uv run ruff check .`, `uv run ruff format --check .`, `uv run mypy src`: passed. Hooks also ran these checks and pytest. `UV_NO_SYNC=1` prevented host dependency installs.
- `docker buildx build --load --platform linux/amd64 --sbom=true --provenance=mode=max --build-arg SOURCE_REVISION=7d8545f6c0a63ecbb182f318c5deb0b4b63b02bb --build-arg APP_VERSION=1.9.4 -t briefcast:stage-f-verified .`: passed; includes the frontend production build.
- Corresponding worker builds used `-f docker/whisperx/Dockerfile`, `WORKER_VERSION=1.0.0`, and `RUNTIME=cpu`/`cu128`: passed. `pwsh -File scripts/CheckImage.ps1 -Image <final-tag> -Kind app|cpu|cu128`: passed with the explicit baseline decoder limitation above.
- `pwsh -File RELEASE.ps1 build -Target worker -Runtime cpu -Version 1.0.0 -BuildDir <private>/artifacts`: passed. The initial CUDA helper build hit the over-strict TorchCodec check; the corrected check and final CUDA build/content/processing checks passed. Final source-pinned archives were exported into a separate directory to retain the initial version-tagged candidate artifacts.
- `docker image save`, SHA256 checks, `docker image load`, image-ID comparisons: passed for app, CPU and CUDA. Raw archives, synthetic requests/results, logs and caches stay outside Git and the build context.
- Host Compose 5.5.1 matrix passed. The official Compose 2.24.0 Linux binary (SHA256 `43fb39c0bc24ac796123935032eddcfb57a7acd9216badeb10714c986ea1e090`) was separately downloaded with checksum verification and rendered all three modes with a missing optional advanced file. The [documented optional env-file minimum](https://docs.docker.com/compose/how-tos/environment-variables/set-environment-variables/) is therefore exercised locally.
- `just --list`, `just --dry-run up`, `git diff --check`: passed. GitHub workflows were updated but not remotely executed or published.

## Production and next gate

The running production container still uses `sha256:a7542b94500a5ac7e00271451ef2a51db233bd5e2bec92c615b240302b06d052`; its recorded start time remains `2026-10-02T20:40:04.862330437Z`. No production state/configuration/mount was used by a candidate app or worker.

Stage G must complete real multi-chunk/multiple-speaker/silence/unusual-format/malformed-audio comparisons, actual CUDA execution, cold/warm starts, Linux host ownership checks, outage/crash/ACK reconciliation, independent worker replacement/state-compatible rollback, verified database/media recovery and observation gates. Production cutover and host migration remain unauthorized by this evidence. Image-only rollback checks do not replace a quiescent worker-state backup/restore rehearsal.
