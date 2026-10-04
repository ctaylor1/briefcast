# Stage G — isolated acceptance and recovery tooling

## Status and source

Stage G tooling and the rehearsals below are implemented. **Stage G acceptance and production cutover remain gated:** strict CUDA output comparison failed, and a final coherent production database/config/media checkpoint has not been taken. The production observation window has not begun. Production was not deployed, stopped, restarted, migrated or upgraded.

Branch: `feat/whisperx-service`; starting HEAD: `ffd5aba96465cd6c2ae82c261e3f27173e32252a`. Ordered local commits:

1. `1970205` — private recovery and offline monolith rollback tooling.
2. `491ddbe` — isolated real-audio acceptance and crash recovery.
3. The commit containing this report — deployment recovery integration, release gates and evidence.

The final source SHA is pinned in the private cutover record and a verified Git bundle made after committing. These commits are unpushed; no PR, registry publication or GitHub workflow execution was performed. Runtime images are the previously verified Stage F builds from `7d8545f6c0a63ecbb182f318c5deb0b4b63b02bb`; Stage G adds operator tools/tests without changing the pipeline or public app HTTP API/schema. No dependencies or lockfiles changed.

## Implementation

- `scripts/whisperx_acceptance.py` creates new private run directories, unique Compose projects, copied audio/cache, fresh tokens and isolated SQLite. It strips operator environment overrides and rejects production database/mount/port/credential/dispatch settings before starting. It checks authenticated jobs, repeated submission/conflict/cancellation, real inference, checkpoint restarts, malformed-media preservation, drain, unavailable worker, full-state restore, replacement/rollback, durable results and ACK. Optional app restart exercises the real HTTP adapter and persistence/export path.
- `WorkerRecovery.ps1` and `whisperx_recovery.py` require the worker ownership lock, schema/integrity checks, drained terminal jobs and checkpoint/result references. They archive the **whole stopped state tree**, including journal and referenced artifacts, with per-file hashes and an immutable image ID. Restore validates archive paths/types/checksums and writes only a new directory. It does not overwrite a candidate-era journal. Reboot/API replay verifies restored results against their original envelopes and digests.
- `briefcast_recovery.py` captures the running monolith image, PostgreSQL custom dump, binary configuration tar, private inspect/env/config copies, container-layer diff and source bundle. Passwords use a restricted `.pgpass`, not CLI arguments or printed logs. It verifies image reload and version smoke, restores PostgreSQL into a new server without source access, and boots the archived app on a private internal network with restored config and copied synthetic assets. It serves the restored podcast catalog and settings. Original PostgreSQL roles/ACLs and production media are not restored by this rehearsal.
- `cmd/transcription-rollback` is an offline operator command, dry-run by default. It does not initialize/migrate the app, unlock leases, start scheduling or contact a worker. Apply requires `--services-stopped` and an exclusive private backup file; the backup is synced before any row update. Only recognized unfinished remote envelopes with empty transcript/canonical output are reset, in one transaction with conditional row fences. Unknown/corrupt/committed envelopes abort. Audio, completed outputs, summaries, bookmark/play edits and legacy checkpoints are preserved.
- `RELEASE.ps1` now exposes worker `drain`, `status` and `resume`. Existing-worker deploy stops only the drained worker, requires a full verified snapshot, saves rollback identity before loading the candidate and leaves acceptance drained. Rollback verifies the original snapshot and also preserves candidate-era state before selecting the old image. Incompatible state/config requires reviewed reconciliation and optional restore into a new directory; it is not silently overwritten.
- `PrepareCutover.ps1` produces a private, checksum-checked record with image/backup/source identities, explicit Compose arguments, command arrays, image overrides and rollback actions. Archive, archived-app database rehearsal, config, candidate source bundle, coherent assets and representative quality are separate gates. It performs no deployment. Private candidate settings preserve the inspected DB, internal paths, host port and effective baseline settings; proposed durable worker directories have not been provisioned.
- `PrepareAcceptanceCorpus.ps1` prepares a hash-pinned public corpus; `PrepareAcceptanceCache.ps1` prepares a new baseline ASR/VAD/alignment/NLTK cache. `TestWorkerPython.ps1` runs explicit script lint and tests under the shipped Python 3.12 with existing pytest tools mounted read-only. `scripts/ruff.toml` prevents root Python 3.14 formatting from producing syntax incompatible with that runtime.
- `Compose.ps1` now binds Docker's `-d` explicitly: PowerShell previously consumed it as common `-Debug`, making startup attach and hang. A regression test and actual detached harness starts passed.
- CI includes focused Linux race and recovery tests. The CPU worker release job runs a bounded real inference/app recovery/replacement check. CUDA publication requires a trusted default-branch operator record for the exact source, hardware, pinned model and successful quality/recovery; image-content/import checks alone do not authorize publication. New `just` targets invoke the implemented tools.

Recovery/rollback changes are data-integrity sensitive. Verification includes corrupted archive/path/ownership guards, private credential arguments, transactional failure injection, actual isolated PostgreSQL restore and archived-app boot, and real worker journal restoration/replay. No backup, stopped rehearsal container or data volume was pruned.

## Runtime and corpus

Host: Windows Docker Desktop, Linux/amd64 containers, Docker 29.8.1, Compose 5.5.1, NVIDIA GeForce RTX 5070 Ti (16303 MiB). Production had only the app process when GPU rehearsals began. GPU memory readings include desktop use; timings below are scenario durations, not inference benchmarks.

| Runtime | Immutable local image ID |
| --- | --- |
| Archived monolith | `sha256:a7542b94500a5ac7e00271451ef2a51db233bd5e2bec92c615b240302b06d052` |
| Lean app 1.9.4 | `sha256:eadbf9df0f3418328d5824739b08e63abe9b4e1f43d0b5743ef4242acea6e2dc` |
| CPU worker 1.0.0/API v1 | `sha256:b032e151f70d8b1f1236a50864405677cb74da8aec938c90a89e81bd670bace0` |
| CUDA worker 1.0.0/API v1 | `sha256:64be5c42e52b03f6371a6183acd08ec2925ef312ca5b06f6f00aa30dde520fba` |
| Distinct compatible CPU replacement | `sha256:3a9be122083abe640c9e7bb63cb890d0f753b9d6b198163bd0154a644db57aae` |
| Distinct compatible CUDA replacement | `sha256:9a27b4ed2f321af04d58a77c94877677a9030caf030f58c1a068b609bc5d30db` |

Replacement builds add only a label to the verified worker. This proves independent compatible image replacement, **not an upstream engine/version upgrade**. Baseline engine: WhisperX 3.8.6, Torch 2.8.0+cpu/+cu128, CTranslate2 4.8.1, faster-whisper 1.2.1, pyannote.audio 4.0.7. Pipeline SHA256 remains `7aa01ea856e94438e728f944b8e71aee79a6843470afc67a0a3334e5d2c14520`.

Options: `medium.en`, English, 120-second chunks, alignment enabled, diarization disabled, batch 16; CPU int8, CUDA float16. All ASR/VAD/options match the frozen request fixture. Warm caches are private copies of the preserved Stage D cache: model revision `a29b04bd15381511a9af671baec01072039215e3`; alignment SHA256 `488fd4f16de84438ffc945334278c1b9fb9b7159a806c1080b16111a958c945d`. Empty HF token files were used; no gated diarization credentials were required or exposed.

The representative corpus uses Ubuntu Podcast S14E30, with attribution to the Ubuntu UK Local Community Team and presenters. The official [license statement](https://ubuntupodcast.org/about/) covers audio under CC BY-SA 3.0. The [source MP3](https://static.ubuntupodcast.org/ubuntupodcast/s14/e30/ubuntupodcast_s14e30.mp3) is hash-pinned before decoding. Audio/transcripts remain outside Git; preserve attribution/share-alike terms when redistributing derivatives.

| Corpus | Duration / transformation | SHA256 |
| --- | --- | --- |
| Original podcast MP3 | Source | `063cfe433f1e636b3785a06fcf709a1c4a2416658808fa54d2a598900f1281f4` |
| Multiple human voices | 300 seconds from offset 60, mono 16 kHz WAV | `08e1349ef8ace32568667e38dee62fba46a30f47c8f8f29ca02507ca50c34131` |
| Normal short excerpt | 40 seconds from offset 60, FLAC | `c827f8d068f9828176020d325f67a76c34303b2e77a3830d78785d0e2f6e7004` |
| Generated silence | 10 seconds, WAV | `933d309674b5ead9b5bee9cdd8bff71a69294d12e91db63cee67e6b78ebddd57` |
| Repeated synthetic speech | 271.80263 seconds, WAV | `f2e5b824258628d1eea85e96824a4f6b8f3a05fbd7492eb51b6ace2a2eca68cc` |

The committed FLAC fixture is self-synthesized speech (241914 bytes), not production media. Malformed input is generated separately. Positive speaker-label diarization remains unverified; multiple voices with diarization disabled does not prove that optional feature.

## Observed real-container results

Private runs are under the directory referenced by `%TEMP%\briefcast-stage-g-directory.txt`. Each successful run retains `evidence.json`, configuration, results and recovery artifacts. Exact comparisons remove only operational `metadata`; schema, alignment/diarization outcomes and errors, text, word confidence scores, speakers and numeric timestamps remain exact. No floating-point tolerance was introduced after a failure.

| Run | Result |
| --- | --- |
| `cpu-2` | PASS: 271.8-second real synthetic speech; graceful restart after durable running chunk; exact archived-baseline comparison; source preserved; restore/replacement/rollback/ACK |
| `corpus-cpu` | PASS: all three representative cases exact against archived baseline; 300-second speech resumed after graceful restart; queue cancel/conflict, malformed input, durable replay and independent replacement/rollback |
| `cuda` | **FAIL strict quality comparison:** 43 word confidence scores differ, maximum absolute difference 0.002. Text, timestamps and structure match. Private comparison summary/results retained |
| `cuda-recovery` | PASS: real CUDA synthetic speech resumed after worker SIGKILL at a durable checkpoint; full-state restore, replacement/rollback and idempotent receipt |
| `corpus-cuda` | PASS recovery/format checks: 300-second human speech with worker SIGKILL/resume, FLAC and silence; malformed source retained; app available with worker stopped; restore/replacement/rollback. No exact baseline comparison was requested in this separate recovery run |
| `app-recovery` | PASS real CUDA/HTTP adapter: app SIGKILL while accepted job runs; worker finishes and result stays available; restarted app reconnects to the same job, commits transcript/canonical output/export, ACKs and retains concurrent bookmark/play edits; no early summary |
| `cold-cache` | PASS: offline, disconnected isolated worker with empty cache reports terminal `INTERNAL`, source unchanged. Runtime preflight initially ready does not mean every model weight is cached |

The 300-second CPU scenario took 182.849 seconds including candidate restart and baseline execution; the CUDA recovery scenario took 80.611 seconds. These are not comparable performance measurements. All rehearsal services are stopped.

The CPU corpus app container `9aa0442d928286644771ec114e287ca52712607f8d6ff5f75e620ae77b9db5dd` and CUDA corpus app container `0daa79242b9945e74819da1753fed52f68614260fdfc7f4990c8537f6b10b64e` retained the lean app image and identical container identity throughout worker replacement/rollback. Both restored three successful results, one cancelled job and one malformed failed job. State archive SHA256: CPU `7a17757ee6508e2e299b79c14ad366d835eb2f4b364b46c6b2fe650dca0c6dd1`; CUDA `01554e48caabc3f145e6218bc1a9906610e78967e69ac5ad056168137d458816`.

## Deterministic failure coverage

Existing Stage D/E contract tests remain part of the Stage G gate. Their controlled runners/endpoints cover failures that are not safely or reliably injected into real ML containers:

| Matrix area | Evidence / boundary |
| --- | --- |
| Acceptance uncertainty / at-most-one active job | `TestAuthenticatedContractAndIdempotentConcurrentAcceptance`, `TestRemoteLostAcceptanceResponseReusesSubmission`, `TestRemoteInterruptedResultTransferReconnectsWithoutInferenceRetry` |
| DB commit / lost receipt / stale data | `TestRemoteCommitFailureRetainsResultAndSkipsDownstream`, `TestRemoteReceiptReconcilesAfterLostAckResponse`, `TestRemoteGenerationOrInputChangeRejectsResult`, concurrent edit and summary-write tests |
| Storage / checkpoint / result corruption | `TestStorageFailuresCannotClaimAcceptanceOrSuccess`, `TestResultCommitFailureRecoversVerifiedCheckpoint`, `TestCorruptCheckpointRecoveryRestartsAtZeroWithReason`, `TestCorruptResultFailsReadinessAndStartup`, recovery archive tests |
| Process shutdown / ownership / limits | Linux supervisor cancellation/guardian-crash/descendant-reaping/output-bound tests; `TestStateOwnershipAndRecoveryBudget`; actual graceful restart and SIGKILL above |
| Security / typed HTTP errors / media | Malicious path/FIFO/symlink/request-bound tests, `TestRemoteHTTPFailuresPreserveAudioAndDoNotConsumeInferenceRetries`, configuration/isolation guards, actual 401/404/409/503 and malformed media |
| Queue / retries / replacement identity | Remote pagination/permanent-failure/manual-repair/retry tests, lost-worker-state and prepared-identity replacement tests, actual compatible-image rollback |
| Full split rollback | Three `TestMonolithRollback*` cases: dry-run/backup/output/manual-field preservation, unknown-envelope rejection, injected SQL failure with transactional rollback |

Direct real-container OOM/disk-exhaustion injection, app kills at every post-commit/ACK boundary, a changed upstream engine/state schema, positive gated diarization and production playback/media recovery were not demonstrated. Do not describe the entire acceptance matrix as passed. Existing feed/download/feed-transcript/image/summary/repair/frontend behavior is covered by the repository suites; the isolated harness keeps LLM calls disabled and does not claim a real production summary-provider canary.

## Baseline recovery evidence

Private `baseline-recovery` contains restricted raw configuration/inspect/password material, which must not be uploaded or committed. The recovery manifest and image load/version smoke were verified:

| Artifact | Bytes | SHA256 |
| --- | ---: | --- |
| Exact monolith image archive | 4585897472 | `97a4d7f7d041fa4503bdcf5647ec095c6794dc3cb31bb5245bdf461ac72c03a5` |
| PostgreSQL custom dump | 2744555799 | `5003e3789b975dd8d739c443c8f7c5bb433560179f48b6ffc73c0a37556c2a86` |
| Configuration/cache tar | 1821339648 | `9332b4c215c503d4bc3dd5d0294b488089e54c1b3df7f9ebc38dbbf7346962f6` |

Source PostgreSQL is 16.11; dump client/isolated restored server use the local PostgreSQL 17 image `sha256:c7526c0f6c3f30260a563d7bcf8ad778effac59a44f8ffa86678c35418338609`. Restore uses a new isolated role, `--no-owner --no-privileges`, no source-server connection and no host port. Observed restored counts: 14 podcasts, 7296 podcast items, 118710 show-note links, 59 migrations, 4 job locks and 2 settings; tags, podcast tags and prompt versions are empty. The archived app booted and served catalog/settings using the restored configuration and copied synthetic asset. Linux cache symlinks were portably dereferenced from the archive into regular files for Windows rehearsal.

The initial source bundle verifies and preserves the Stage F starting HEAD; its manifest correctly records the working tree was dirty during initial capture. A separate final Stage G bundle preserves committed implementation. Neither bundle captures private settings/uncommitted files. Image archives do not capture mounted media, database, live processes or all writable-layer changes; the private diff is an operator review input.

**This is not a final consistency checkpoint.** Production media was not copied, snapshotted or altered. Original roles/ACLs were not restored. The final maintenance checkpoint must protect database/config/media together and account for writes after the initial dump. Full rollback must retain completed current outputs and separately preserve legacy sidecars before the old local script may overwrite them. No production rollback row resets were performed.

## Commands and verification

Executed focused/full source gates:

```powershell
$env:UV_NO_SYNC='1'
$env:CGO_ENABLED='0'
Remove-Item Env:BRIEFCAST_INTEGRATION_DB_URL,Env:BRIEFCAST_WHISPERX_REAL -ErrorAction SilentlyContinue
go test ./internal/transcription/... ./internal/whisperxworker/... ./service/... -count=1
go test ./service -run TestMonolithRollback -count=1
go test ./...
just test-full
uv run ruff check .
uv run ruff format --check .
uv run mypy src
uv run pytest
pwsh -NoProfile -File scripts/TestWorkerPython.ps1 -Image briefcast-whisperx-cpu:stage-f-verified
git diff --check
```

All passed. `just test-full` included Go, isolated feed/download/WhisperX integration, frontend typecheck/build and 18 frontend unit tests. Final host pytest: **64 passed, 1 skipped** (Linux ownership lock on Windows). Final shipped worker Python 3.12 gate: **43 passed**, including the real Linux lock test and explicit script lint/format. PowerShell parsing and `just --list` passed. Linux CGO race suite also passed:

```powershell
docker run --rm --network none --mount "type=bind,source=$PWD,target=/repo,readonly" --mount "type=bind,source=$GoModuleCache,target=/modules,readonly" --mount "type=bind,source=$G/go-cache,target=/go-cache" --env GOMODCACHE=/modules --env GOCACHE=/go-cache --env CGO_ENABLED=1 --workdir /repo golang:1.26 go test -race ./internal/transcription/... ./internal/whisperxworker/... ./service/...
```

`$GoModuleCache` was the existing host module cache and `$G` the private Stage G root. A subsequent focused Linux `go test -race ./service -run TestMonolithRollback -count=1` passed after adding the rollback tests. No dependency synchronization/install was needed.

Actual real-corpus commands used the same arguments as these reusable commands. Set `$G` from the private directory pointer and `$Cache` to the preserved isolated cache; **each new `--run-dir` must not exist**:

```powershell
pwsh -File scripts/PrepareAcceptanceCorpus.ps1 -Image briefcast-whisperx-cpu:stage-f-verified -CorpusDir "$G/prepared-corpus"
uv run python scripts/whisperx_acceptance.py --runtime cpu --app-image briefcast:stage-f-verified --worker-image briefcast-whisperx-cpu:stage-f-verified --replacement-image briefcast-whisperx-cpu:stage-g-replacement --baseline-image sha256:a7542b94500a5ac7e00271451ef2a51db233bd5e2bec92c615b240302b06d052 --cache $Cache --audio "$G/prepared-corpus/ubuntu-300s.wav" --audio "$G/prepared-corpus/ubuntu-excerpt.flac" --audio "$G/prepared-corpus/silence.wav" --run-dir "$G/corpus-cpu" --restart-at-checkpoint --timeout 900
just test-whisperx-container-cuda --app-image briefcast:stage-f-verified --worker-image briefcast-whisperx-cu128:stage-f-verified --replacement-image briefcast-whisperx-cu128:stage-g-replacement --cache $Cache --audio "$G/prepared-corpus/ubuntu-300s.wav" --audio "$G/prepared-corpus/ubuntu-excerpt.flac" --audio "$G/prepared-corpus/silence.wav" --run-dir "$G/corpus-cuda" --kill-at-checkpoint --timeout 900
```

Synthetic CPU/CUDA runs used the same harness with `--audio "$G/long-speech.wav"`; the failed CUDA comparison included `--baseline-image` and `--restart-at-checkpoint`. Separate CUDA recovery used `--kill-at-checkpoint`; app recovery used `--app-restarts`. Cold-cache failure used an empty new cache and `--cold-cache-only`. `WorkerRecovery.ps1` snapshot/restore were invoked by every successful warm harness, and verified by replay from both old and replacement images. Reused cache preparation/download helper syntax is checked, but its fresh network preload was not executed locally; warm/offline inference and explicit offline cache failure were executed.

Private baseline recovery commands executed:

```powershell
uv run python scripts/briefcast_recovery.py capture --container briefcast --destination "$G/baseline-recovery" --postgres-image postgres:17-alpine --config-file .env --config-file docker-compose.legacy.yml --config-file docker-compose.legacy-gpu.yml
uv run python scripts/briefcast_recovery.py finish-capture --container briefcast --destination "$G/baseline-recovery"
uv run python scripts/briefcast_recovery.py verify-database --destination "$G/baseline-recovery"
uv run python scripts/briefcast_recovery.py verify-app --destination "$G/baseline-recovery"
```

Initial Windows directory-copy/symlink and Docker `none`-network attach failures were fixed with binary tar capture and a private boot server using only the verified restored volume. Repeated `verify-app` now passed including catalog/settings probes. Original diagnostics are retained privately. Cleanup attempts every own resource and preserves the original error; stopped volumes/containers remain available for review. The capture separately copied `.env` and both legacy Compose definitions; effective advanced settings are in restricted container inspection. Copy the original advanced dotenv file at the final maintenance checkpoint as well. Legacy Compose still references repository `.env` in its service `env_file`; restoring only interpolation with `--env-file` is insufficient.

## Cutover package and remaining gates

Subsequent implementation records: [app crash recovery](ACCEPTANCE_FOLLOWUP.md) (2026-10-03) and [bounded container resource failures](RESOURCE_ACCEPTANCE.md) (2026-10-04). Those reports qualify the historical gaps below with actual outcomes and remaining limits; neither clears the CUDA quality or production recovery/cutover gates.

`baseline-recovery/cutover` contains private baseline queue counts, effective candidate settings, a quality/recovery record, verified final source bundle and `cutover-review.json` produced by `PrepareCutover.ps1`. Source, archive hashes and explicit ordered Compose commands are concrete; the record remains **not ready for maintenance review** while asset/quality gates are false. Worker paths in that file are proposals, not created production mounts. Baseline database address and app `/assets`/`/config` paths are preserved. Source/image provenance are recorded separately.

Before cutover:

1. Review the retained strict GPU score differences. Any justified GPU tolerance must be declared and reviewed for a new comparison run; this failed run must remain failed. Add remaining controlled failure checks where required by the acceptance matrix.
2. Take and verify the final coherent database/config/media maintenance checkpoint, restore representative production media in isolation, and review writable-layer/runtime and DB ownership recovery assumptions.
3. Provision durable worker state/cache/token paths with tested ownership, preserve originals and verify capabilities/cache with baseline settings. Optional gated diarization remains unproven; the known baseline CUDA TorchCodec missing `libnppicc.so.12` was not fixed by changing dependencies.
4. Review the concrete package, stop the old app before any candidate production DB access, run one complete transcript/export/summary-provider canary, then record the long episode, scheduling interval and 24-hour observation window.

The production container ID still matched initial private capture; image remained the archived monolith and `StartedAt` remained `2026-10-02T20:40:04.862330437Z`. Final running-container inventory contained only production `briefcast`. Single-app/single-worker operation, same-host mounted media and deferred host migration remain unchanged. Stage H operational documentation is subsequent work; acceptance gaps above must not be treated as passed merely because the tools exist.
