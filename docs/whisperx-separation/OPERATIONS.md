# Independent WhisperX operations

## Release status and compatibility

This is the implemented operator runbook for the split app and private worker API v1. **The split has not been deployed or published by this work.** [Stage G](STAGE_G.md) passed exact CPU corpus comparison and real CPU/CUDA recovery/replacement tests, but strict CUDA comparison failed on 43 word-confidence scores (maximum difference 0.002). Final coherent production media recovery, remaining failure-matrix checks and the 24-hour production observation window remain open. These documents do not clear those gates.

| Component | Implemented/tested baseline |
| --- | --- |
| Lean app | 1.9.4; public app API/schema preserved; HTTP transcription backend in Compose |
| Independent worker | 1.0.0, wire major 1; separate version file `docker/whisperx/VERSION` |
| Engine | WhisperX 3.8.6, CTranslate2 4.8.1, faster-whisper 1.2.1, pyannote.audio 4.0.7 |
| CPU profile | `baseline-wx3.8.6-cpu-v1`; Torch/torchaudio 2.8.0+cpu; TorchCodec 0.7.0; int8 |
| CUDA profile | `baseline-wx3.8.6-cu128-v1`; Torch/torchaudio 2.8.0+cu128; TorchCodec 0.7.0+cu128; float16 |
| Container runtime | Python 3.12.13, Linux/amd64; PowerShell 7 and Compose >= 2.24 for wrappers; Docker/Compose 29.8.1/5.5.1 observed in Stage G |
| Persistent protocol | Worker schema 1, `briefcast-whisperx-chunks-v1`, pipeline `briefcast-script-v1`; app remote envelope version 1 |

The archived monolith also reports app version 1.9.4: version labels alone do not identify lean versus monolith images. Use immutable image IDs/registry digests and source/variant manifests. [Stage F](STAGE_F.md) records verified local app/worker archives and checksums; [migration inventory](migration-inventory.md) records exact local IDs. No ARM worker or alternative machine/OS has been accepted. CUDA was exercised on an RTX 5070 Ti; this is not validation of every GPU/driver combination. The baseline optional CUDA TorchCodec decoder lacks `libnppicc.so.12`; positive gated diarization/file decoding remains unverified. Default ASR/VAD uses FFmpeg-decoded waveforms. Do not add dependencies or enable diarization as part of a routine split rollout.

## Implemented contract

Wire shapes: [openapi.yaml](openapi.yaml); lifecycle/hashing: [CONTRACT.md](CONTRACT.md). The app owns its DB, scheduling, file validation, canonical text, exports and summaries. The worker owns accepted execution, process groups, checkpoints, journal and durable results. It has no app DB/LLM credentials, config-tree mount, Docker socket or host-published port.

| Operation | Behavior |
| --- | --- |
| `PUT /v1/jobs/{job_id}` | Validate rooted media/options/hash, then durably queue before 201; matching replay 200, changed identity 409, capacity 429 or drain 503 without acceptance |
| `GET /v1/jobs/{job_id}` | Persisted state, monotonic revision/progress and effective runtime; percent 100 alone does not mean an app DB commit |
| `GET /v1/jobs/{job_id}/result` | Succeeded envelope with verified canonical transcript SHA256; private media/result identity bound to the submission |
| `POST /v1/jobs/{job_id}/cancel` | Idempotent cancellation; running job becomes terminal only after process-group cleanup |
| `POST /v1/jobs/{job_id}/ack` | Exact succeeded result digest; idempotent after the app's conditional DB commit |
| `GET /v1/capabilities` | Authenticated API/profile/package/options/checkpoint/capacity and worker-instance identity |
| `POST /v1/admin/drain`, `/resume` | Persisted acceptance control; accepted work and result/ACK access continue during drain |

All `/v1` operations require the shared bearer file. Health is unauthenticated but remains on the private network. Requests contain relative media references, never URLs/scripts/credentials or worker state paths. HTTP mode preserves audio across remote infrastructure errors. Lost acceptance responses reuse the durable UUID/request; accepted jobs are reconnected, not blindly re-submitted. Changed generations/fingerprints or uncertain lost worker state fail closed. Unknown checkpoint formats stop automatic work. Manual repair is the intentional path for a new attempt, after inspecting the previous identity/outcome.

Unacknowledged successful artifacts have no time expiry. The worker runs garbage collection hourly: acknowledged large artifacts may be pruned after seven days while compact identity/digest tombstones and referenced checkpoints remain. Offline `whisperx-worker gc --state <state-directory>` requires exclusive ownership and a stopped worker. Never delete journal rows or referenced job directories manually to free space.

## Configuration and storage

| Setting | Owner / default |
| --- | --- |
| `WHISPERX_RUNTIME` | Primary dotenv/wrapper mode: `disabled`, `cpu` or `cuda`; selects files, not a container env switch |
| `WHISPERX_ENABLED`, `WHISPERX_BACKEND` | App dispatch; example disabled; split Compose forces backend `http` |
| `WHISPERX_SERVICE_URL`, `SERVICE_TOKEN_FILE`, `ENGINE_PROFILE` | App HTTP endpoint `http://whisperx:8090`, shared secret mount and selected profile (full names start with `WHISPERX_`) |
| `WHISPERX_MODEL`, language/chunk/alignment/diarization/decoder/VAD settings | App request options; preserve inspected baseline, not merely example-file defaults |
| `WHISPERX_MAX_CONCURRENCY`, `MAX_ITEMS`, intervals/retry/lease settings | App scheduling; baseline concurrency one and diarization false; advanced dotenv can override item limits |
| `WHISPERX_WORKER_BATCH_SIZE`, `MAX_RUNNING`, `MAX_WAITING` | Worker: 16, 1, 4; CUDA device/float16 and CPU device/int8 are selected by Compose |
| `WHISPERX_WORKER_TIMEOUT_SECONDS` | Worker cumulative inference budget 21600 seconds; distinct from HTTP/queue wait |
| `WHISPERX_SERVICE_RESULT_LIMIT_BYTES` / `WHISPERX_WORKER_RESULT_LIMIT_BYTES` | App/worker result bounds, both default 67108864 bytes; configure consistently |
| `WHISPERX_WORKER_MIN_FREE_BYTES` | Worker state acceptance floor 536870912 bytes; provision additional space for models/backups/results |
| `WHISPERX_OFFLINE` | Worker model egress flags; true by default, requires copied warmed caches |
| `WHISPERX_UID`, `WHISPERX_GID` | Worker Unix identity, default 1000:1000; recovery wrapper currently fixed at 1000:1000 |

Full persistent path/permission inventory is in [migration-inventory.md](migration-inventory.md). App `/config` includes `whisperx-deployment-id` and travels with the DB; worker `/state` includes `journal.sqlite`, `owner.lock` and all job references/results. They identify different things and are both retained. State/cache are writable; media and token mounts are read-only; worker root is read-only. `/tmp` and `/cache/tmp` each have a 512 MiB tmpfs cap and are scratch, not recovery state.

Model paths are `/cache/huggingface` (`HF_HOME`, hub subdirectory), `/cache/torch` (`TORCH_HOME`), `/cache/nltk` (`NLTK_DATA`) and `/cache/home` (`HOME`, library caches). Model revision: `a29b04bd15381511a9af671baec01072039215e3`; alignment SHA256 is recorded in Stage G. Copy baseline caches into the new worker cache without deleting originals. A cache copied from `/config/.cache/huggingface` must land under `/cache/huggingface`, not under an extra nested cache root. Preserve needed Torch/NLTK/runtime-layer caches too. Allowlisted model names alone do not guarantee local weights or gated access.

Service token: at least 32 non-whitespace characters, shared read-only at `/run/secrets/whisperx-service-token` in both services. HF file: worker-only `/run/secrets/whisperx-hf-token`, may be empty when gated access is unused. Pre-provision files/directories and verify container reads/writes using the selected identity. On Linux use restricted parent directories and file permissions granting only the required readers; a 0600 file owned by a different UID will fail for the worker. On Windows restrict host ACLs to operator/service accounts and verify Docker Desktop bind behavior. Do not log token values or place backups/secrets in the build context. Tokens are read at process startup; coordinate both service restarts when rotating the shared token, preserving jobs/state.

## Explicit deployment selection

`docker/compose-modes.json` is the shared file selector:

| Runtime | Ordered files |
| --- | --- |
| disabled | `docker-compose.yml` |
| cpu | Base, `docker-compose.whisperx.yml` |
| cuda | Base, worker layer, `docker-compose.whisperx.gpu.yml` |

Only the worker gets GPU reservations. Source builds explicitly add `docker-compose.build.yml` and, for workers, `docker-compose.whisperx.build.yml`. There is no implicit override. Use `Compose.ps1`/`RELEASE.ps1` for the same selection. Plain `docker compose` must include both ordered file and env lists; a single base file omits the worker.

Selection uses explicit `-Runtime`, otherwise process/primary dotenv `WHISPERX_RUNTIME`. The primary env file is required. `WHISPERX_ENV_FILE` selects optional advanced dotenv, loaded after primary for interpolation and app environment. Process values have highest precedence. Runtime selection is not taken solely from the advanced file. Worker env is explicitly mapped and does not inherit the app dotenv wholesale. Use a controlled shell; do not print full rendered config or raw environment arrays into public logs.

### Fresh isolated development setup

1. Create a new private lab root **outside this repository**, with fresh config/media/log/state/cache directories and token files. Use a new database, container name, project and loopback host port; keep LLM disabled. Never point a test app at production (startup unlocks jobs).
2. Copy `.env.example` to a private lab env file. Set absolute `HOST_*` paths to that new root, `BRIEFCAST_CONTAINER_NAME=briefcast-split-dev`, `HOST_PORT=127.0.0.1:18080`, `DATABASE_URL=sqlite:///config/briefcast.db`, and `WHISPERX_ENABLED=false`. Set `WHISPERX_ENV_FILE` to a lab-specific absent file, or a private copy of `whisperx.env.example`, to avoid accidentally loading the repository operator file.
3. Build the app dev tag, set `BRIEFCAST_IMAGE=briefcast:local`, then render/start only the app. For CPU/CUDA, build the appropriate worker tag, set its image/profile/runtime and mounts, copy/preload the tested cache, then explicitly enable dispatch. `cuda` is a Compose mode; the Dockerfile build argument/tag variant is `cu128`.

Examples are for that new lab; assign `$LabEnv` to its absolute env path:

```powershell
just docker-build image=briefcast:local
pwsh -File scripts/Compose.ps1 -EnvFile $LabEnv -Runtime disabled -ProjectName briefcast-split-dev config --quiet
pwsh -File scripts/Compose.ps1 -EnvFile $LabEnv -Runtime disabled -ProjectName briefcast-split-dev up -d --no-deps briefcast
just docker-build-whisperx image=briefcast-whisperx-cpu:local runtime=cpu
# CUDA alternative, not an app rebuild:
just docker-build-whisperx image=briefcast-whisperx-cu128:local runtime=cu128
# After setting the chosen worker image/mounts/options in the private lab env:
pwsh -File scripts/Compose.ps1 -EnvFile $LabEnv -Runtime cpu -ProjectName briefcast-split-dev config --quiet
pwsh -File scripts/Compose.ps1 -EnvFile $LabEnv -Runtime cpu -ProjectName briefcast-split-dev up -d --no-build --no-deps briefcast whisperx
```

Use `-Runtime cuda` consistently for a CUDA lab. The named lab is independent of production. Image references must exist locally; these examples do not pull an imaginary published split release. CPU/CUDA require existing media/state/cache/token mounts. `PrepareAcceptanceCache.ps1 -Image $WorkerImage -CacheDir $NewCacheDir` downloads baseline ASR/VAD/alignment/NLTK into a new outside-repository directory; it is a connected preparation step, not a production-cache refresh or gated-diarization preload. Stage G used preserved caches and did not locally exercise this fresh network download helper.

## Health and maintenance

For the remainder, assign `$DeploymentEnv`, `$Runtime`, `$ProjectName`, `$ArtifactsDir` and `$RecoveryRoot` from the reviewed private deployment record. Use the **same** values on every command; `$RecoveryRoot` must already be private, outside the repository and writable by the recovery identity.

```powershell
pwsh -File scripts/Compose.ps1 -EnvFile $DeploymentEnv -Runtime $Runtime -ProjectName $ProjectName ps
pwsh -File RELEASE.ps1 status -Target worker -Runtime $Runtime -EnvFile $DeploymentEnv -ProjectName $ProjectName
pwsh -File RELEASE.ps1 drain -Target worker -Runtime $Runtime -EnvFile $DeploymentEnv -ProjectName $ProjectName
pwsh -File scripts/Compose.ps1 -EnvFile $DeploymentEnv -Runtime $Runtime -ProjectName $ProjectName exec -T whisperx python -c "import urllib.request; assert urllib.request.urlopen('http://127.0.0.1:8090/health/live',timeout=5).status==200"
# Resume only after compatibility and recovery checks:
pwsh -File RELEASE.ps1 resume -Target worker -Runtime $Runtime -EnvFile $DeploymentEnv -ProjectName $ProjectName
```

Liveness is cheap. Readiness verifies state/runtime/path prerequisites and acceptance; it returns 503 while drained, so Compose can show a deliberately drained worker as unhealthy. Capability `model_ready` currently reflects supervisor availability, not loading every model or testing gated credentials. A real isolated canary is necessary. Saturation is separate capacity information. Drain rejects new PUTs, lets accepted jobs finish and preserves result/ACK access. Wait for queued/running counts to reach zero and app receipts to settle; drain is not cancellation. The app stays usable with unavailable/drained worker and retains its own transcription queue.

HTTP connect timeout is 5 seconds, request deadline 30 seconds, default polling 2 seconds with capped reconnect/backoff; control body bound is 1 MiB. The worker process stop grace is 180 seconds and Compose stop grace 210 seconds. A restart can resume verified committed chunks; missing/incompatible checkpoints visibly restart/fail rather than skip audio. Monitor worker/app structured errors, failed/retried/old-processing jobs, queue capacity, unacknowledged results, free state/cache disk, GPU availability and downstream exports/summaries. Private per-job diagnostics can include audio paths/text; keep them out of public reports.

## Independent replacement

Only after split acceptance, select a worker candidate in a separate reviewed change. Preserve app image/container identity; record source, API/profile/runtime/model/checkpoint/state compatibility and test evidence before deployment. Engine changes require a new justified profile and corpus comparison, not just a new image tag. Stage G's second label-only build proves compatible image replacement, not an upstream version upgrade.

```powershell
pwsh -File RELEASE.ps1 build -Target worker -Runtime $Runtime -Version $CandidateVersion -EnvFile $DeploymentEnv -BuildDir $ArtifactsDir
# Run isolated acceptance and retain the immutable manifest before replacing:
pwsh -File RELEASE.ps1 deploy -Target worker -Runtime $Runtime -Version $CandidateVersion -EnvFile $DeploymentEnv -ProjectName $ProjectName -BuildDir $ArtifactsDir -RecoveryDir $RecoveryRoot
```

Build refuses an existing version tag. Existing-worker deploy checks the candidate archive checksum before mutations, captures the actual old image ID, drains/refuses queued/running work, stops only worker and snapshots complete state. It saves `rollback-$ProjectName-worker-$Runtime.json` before loading candidate images, then targeted `up --no-build --no-deps --force-recreate whisperx`; app/PostgreSQL are not recreated. Copy settings/version/rollback records privately; future helper calls do not automatically persist image overrides into the operator dotenv, so update its image reference to the reviewed immutable ID after success.

Acceptance stays drained. Deploy's internal verification uses liveness plus capabilities in this state; standalone `RELEASE.ps1 verify` expects accepting readiness and can fail while drained. Check saved results/state identity and candidate compatibility while drained; run candidate canaries in isolation first. New canary PUTs are rejected until resume. Use existing dispatch controls to allow one eligible canary after explicit resume, verify transcript/export/summary and app identity, then restore recorded normal settings. Stop acceptance again if it fails. No general helper automatically proves the canary or observes 24 hours.

CI runs deterministic/race/recovery tests; CPU release has bounded real inference/app recovery. CUDA publication requires a trusted operator record for the exact source/hardware/pinned model and successful quality/recovery. The failed Stage G CUDA comparison cannot satisfy that record. Publication and deployment are separate; nothing here authorizes `ship`/registry push.

## Rollback

### Worker-only rollback

```powershell
pwsh -File RELEASE.ps1 rollback -Target worker -Runtime $Runtime -Version $CandidateVersion -EnvFile $DeploymentEnv -ProjectName $ProjectName -BuildDir $ArtifactsDir -RecoveryDir $RecoveryRoot
pwsh -File RELEASE.ps1 status -Target worker -Runtime $Runtime -EnvFile $DeploymentEnv -ProjectName $ProjectName
```

Use the original deployment selection and saved rollback record. Rollback verifies the pre-replacement snapshot, drains a running candidate, stops it and preserves a second candidate-era state snapshot. It targets only the old immutable worker image and **keeps the current journal**. It does not restore old cache/options/env or prove a newer journal is readable by old code. Restore original worker settings/profile/cache selection, check liveness/capabilities/results and reconcile job envelopes before explicit resume. Standalone image verification must use the appropriate old artifact/version and accepting readiness, not the candidate artifact.

If old code rejects current state, keep both journals, stop worker, use [WorkerRecovery restoration](../backup-and-restore.md#worker-snapshots-and-restoration) into a new directory, reconcile app envelopes against missing/newer jobs and select that directory deliberately. A UUID missing after rollback is not permission to discard its outcome or silently re-execute it. Preserve completed app outputs and candidate receipts; manual requeue requires a new identity after proving old execution cannot still run.

### Full split rollback to the monolith

1. Drain, settle/inventory accepted work, then stop both split services using the same Compose selection. Preserve current DB/media, completed transcripts/summaries, both worker journals and legacy sidecars. Do not use an old DB snapshot as routine code rollback.
2. Load reviewed **private database settings into the operator command environment**, with logs protected. The offline helper uses `DATABASE_URL`/driver configuration; it does not select a database from `RELEASE.ps1 -EnvFile`, app-specific Compose interpolation or automatic dotenv loading. The normal app entrypoint autoloads repository `.env`; this offline CLI does not. Explicitly verify the selected isolated DB when rehearsing.
3. Run dry-run and review counts/recognized envelopes. Apply mutates only unfinished remote transcription queue fields after a durable private backup; both services must already be stopped. Unknown/corrupt/committed envelopes abort and require investigation.

```powershell
go run ./cmd/transcription-rollback
# Only after reviewed inventory and stopped services; new private backup filename:
go run ./cmd/transcription-rollback --apply --services-stopped --backup $NewPrivateEnvelopeBackup
```

4. Restore the exact archived monolith image and original effective config/legacy base+GPU files, preserving app `/config` and `/assets` paths. Legacy service `env_file` still references repository `.env` and the advanced dotenv; restoring only `--env-file` interpolation is insufficient. Select both legacy files explicitly and use `--no-build --no-deps` for the app. The private cutover record contains command arrays/image overrides for this operation; review them against restored env/mounts before execution. Old local mode cannot consume worker-private `/state` paths; use only proven-compatible legacy sidecars, otherwise restart unfinished audio from zero.
5. Verify isolated archived-app/DB compatibility before production rollback, then queue/audio/playback/transcript/export/summary behavior. If data corruption actually requires database restore, separately reconcile all writes after the snapshot. Keep recovery artifacts until acceptance and retention policy permit removal.

## First cutover and future machines

`PrepareCutover.ps1` prepares a private review record and deploys nothing. Inputs are private candidate env, checksum-verified app/worker archives, baseline recovery directory, trusted quality evidence and a verified source bundle. It checks archive/config/DB rehearsal/source gates separately from consistent assets and representative quality. A true preparatory gate is not the production observation window. Preserve complete outcomes and manual gate evidence; do not fabricate flags.

```powershell
pwsh -File scripts/PrepareCutover.ps1 -EnvFile $DeploymentEnv -ArtifactsDir $ArtifactsDir -RecoveryDir $BaselineRecoveryDir -OutputFile $NewPrivateCutoverRecord -Runtime $Runtime -CorpusEvidence $QualityEvidence -SourceBundle $VerifiedSourceBundle
```

Follow the ordered maintenance/canary/observation steps in [execution plan sections 8–10](EXECUTION_PLAN.md#8-production-cutover-on-this-machine). The initial split uses the pinned baseline engine. No candidate app may access production until the old app is stopped and final consistency recovery is verified. Host migration is deferred; the [inventory](migration-inventory.md) lists unresolved target hardware, connectivity, storage/permissions and restore requirements.

## Validation commands

```powershell
just test-whisperx-contract
just test-whisperx-unit
just test-whisperx-python -Image $WorkerImage
just test-whisperx-container-cpu --app-image $AppImage --worker-image $CPUImage --replacement-image $CPUReplacement --baseline-image $BaselineImage --cache $IsolatedWarmCache --audio $LongAudio --run-dir $NewCPURunDir --restart-at-checkpoint
just test-whisperx-container-cuda --app-image $AppImage --worker-image $CUDAImage --replacement-image $CUDAReplacement --cache $IsolatedWarmCache --audio $LongAudio --run-dir $NewCUDARunDir --kill-at-checkpoint --app-restarts
```

All harness run dirs must be new, private and outside the repository; it copies media/cache and rejects production configuration. CPU/CUDA replacement IDs must be distinct compatible images. `--baseline-image` enables strict comparison (including word scores); include it in a new CUDA quality run with predeclared/reviewed tolerance policy if a later change adds one. Without it, a successful recovery run does not prove output equality. Repeat `--audio` for FLAC/silence/representative episodes. Use `--cold-cache-only` for the isolated offline failure scenario. See [Stage G](STAGE_G.md) for exact prior commands, fixture hashes, runtime evidence and limitations; its historical results are not tests rerun by Stage H documentation work.
