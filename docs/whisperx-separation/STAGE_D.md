# Stage D: durable WhisperX worker

Prepared 2026-10-02 on `feat/whisperx-service`, starting from `42c825a66f15acc066c8f75f288af779bb77f948`. This implements the private worker. Briefcast still uses the local backend. Application integration is Stage E; production packaging and rollout are Stages F/G.

## Implementation and boundaries

| Location | Responsibility |
| --- | --- |
| `cmd/whisperx-worker` | Independent executable/version, HTTP server, runtime probing, graceful shutdown, online drain/resume/status and offline GC. |
| `internal/transcription/canonical.go`, `protocol.go` | Strict v1 types/validation, canonical JSON, request/options/result fingerprints, normalized transcript validation and sanitized errors. |
| `internal/whisperxworker/store.go`, platform files | Worker-only SQLite journal, persistent instance identity, schema version, exclusive ownership lease, atomic acceptance/status/receipt transactions. |
| `internal/whisperxworker/worker.go`, `media.go`, `artifacts.go` | Bounded dispatch, rooted media fingerprints, recovery, immutable checkpoint snapshots, durable result verification, cancellation, receipt and retention. |
| `internal/whisperxworker/http.go` | Authenticated job/capability/admin API, public health, control-body bounds, typed HTTP failures. |
| `internal/whisperxworker/python.go`, guardian files | Deployment-owned Python invocation, restricted environment, runtime caches, bounded output/diagnostics, serialized progress, Linux descendant cleanup. |
| `scripts/whisperx_transcribe.py` | Existing inference pipeline with explicit worker mode for durable files, bound checkpoints, chunk-boundary quiescence and deferred checkpoint cleanup. |

The worker imports no application database, controller or service package and receives no application DB/LLM configuration. It uses existing Go dependencies; manifests and lockfiles are unchanged. Public application APIs, application schema, frontend and Compose files are unchanged. The authorized additions are the private worker HTTP API, worker SQLite schema, executable flags and internal Python worker protocol.

Local mode retains its existing inference/checkpoint behavior. The local adapter explicitly overrides `WHISPERX_WORKER_MODE=0`, including when its parent environment contains worker settings. Existing Stage B characterizations remain unchanged; their application persistence risks remain Stage E work.

`CONTRACT.md` clarifies that `result_sha256` hashes the canonical **transcript object**, excluding the outer envelope. This matches the frozen Stage A fixture and avoids a self-referential hash. No fixture digest changed. Canonicalization implements the number, string and property-order rules of [RFC 8785](https://www.rfc-editor.org/rfc/rfc8785) using the standard library; no external implementation was copied or vendored. Invalid UTF-8, duplicate keys, lone surrogates, non-finite numbers, excessive nesting and invalid request fields are rejected.

## Durability and failure behavior

- SQLite uses WAL, FULL synchronization and a single connection. A submission returns 201 only after its identity, digest and validated request are committed. Identical concurrent/replayed requests return the same attempt; mismatches return 409. Queue saturation returns 429 with retry guidance. Drain survives restart and still permits accepted work and matching replays.
- Media references are relative to a rooted read-only media directory. Regular-file, size and streaming SHA-256 checks run before acceptance, execution and result commitment. Traversal, drive/UNC paths, escaping symlinks and special files are rejected. Worker failures never delete source audio.
- Checkpoints bind input/options/profile/pipeline/package versions to durably stored segments. Only verified checkpoints resume. Corruption visibly restarts from zero with a reason; incompatible requested resume is rejected. Legacy unverified sidecars are not imported through the worker API.
- Result artifacts are canonicalized, normalized and verified, then flushed, renamed and directory-synchronized before success commits. Retrieval verifies the result digest again. Storage faults stop intake and cancel active execution; corrupt successful results fail readiness and startup. A persisted result with a failed success transaction cannot produce false success.
- Execution budget is cumulative across recovery. Interrupted running attempts recover under the same UUID; repeated crashes without progress terminate visibly. Cancellation becomes terminal only after child cleanup. Graceful stop requests a completed-chunk checkpoint before forced cleanup.
- A Linux guardian inherits the state lease and observes parent liveness. Parent death, cancellation or signals kill and reap Python and its process-group descendants before ownership can transfer. Linux/amd64 is the execution target; Windows supports the journal/API tests and rejects Python execution.
- Unacknowledged results have no time expiry. Exact-digest ACK is idempotent. Seven-day acknowledged-artifact GC retains compact identity/digest tombstones and checkpoint snapshots referenced by another attempt. Failed/cancelled resume sources remain available.

The state directory is private and has one worker owner. For backup, quiesce and stop the worker and copy the complete journal/artifact tree, or use a consistent SQLite backup with its referenced artifacts. Never hot-copy only the SQLite main file while WAL is active. Candidate images need separate writable caches/state during rehearsal.

## Operator interface

`serve` requires `--token-file` and an immutable `--profile`. The token is read from a file, is at least 32 characters without whitespace, and is compared through a constant-time hash check. Optional Hugging Face credentials use `--hf-token-file`. Tokens and raw stderr are not returned in API errors.

Defaults: private listener `:8090`, `/state`, read-only `/media`, `/cache`, model `medium.en`, CPU/int8, batch 16, one running plus four waiting jobs, cumulative execution timeout 21600 seconds, 1 MiB control bodies, 64 MiB result/checkpoint artifacts, 512 MiB free-state-space floor and 180 seconds stop grace. Models, diarization models, interpreter, script and execution resources are deployment-owned flags. Reusing a profile with changed pipeline hash, versions or execution settings fails startup; choose a new profile for a reviewed runtime change. `workerVersion` is independent and can be set with `-ldflags '-X main.workerVersion=<version>'`.

```text
whisperx-worker serve --token-file /run/service-token --profile <immutable-profile>
whisperx-worker status --url http://whisperx:8090 --token-file /run/service-token
whisperx-worker drain --url http://whisperx:8090 --token-file /run/service-token
whisperx-worker resume --url http://whisperx:8090 --token-file /run/service-token
whisperx-worker gc --state /state
whisperx-worker version
```

Offline `gc` requires a stopped worker and acquires the same ownership lease. Serve also checks eligible GC hourly. Maintenance HTTP calls have bounded responses/timeouts, prohibit redirects and do not inherit proxy configuration. Health distinguishes cheap liveness from storage/media/runtime readiness without loading a model. CUDA readiness probes device availability. Structured logs expose job IDs and lifecycle/fault events; capabilities expose capacity/profile/version and maintenance responses expose unacknowledged-result counts. Private per-job stderr is bounded to 64 KiB. Output, progress and result sizes are enforced independently.

## Automated verification

Go tests unset `BRIEFCAST_INTEGRATION_DB_URL` and `BRIEFCAST_WHISPERX_REAL`. Tests use temporary state/media, synthetic audio, deterministic child stubs and injected storage failures. No production database was queried. Frontend files are untouched and their tests were not repeated.

| Command | Result |
| --- | --- |
| `go test ./internal/transcription ./service -run 'Test(TranscriptionBackend\|LocalTranscriptionBackend\|Characterize_(WhisperX\|Transcript)\|IntegrationFeedDownloadWhisperX)' -count=1` | Passed before production edits; service 19.284 seconds. |
| `go test ./internal/transcription ./internal/whisperxworker ./cmd/whisperx-worker -count=1` | Passed repeatedly; final focused run after GC reference tests passed, worker 1.615 seconds. |
| `go test ./...` | Passed twice; latest service run 94.523 seconds. Final worker-only GC refinement was subsequently verified by the focused and Linux race suites. |
| `go vet ./internal/transcription ./internal/whisperxworker ./cmd/whisperx-worker ./service` | Passed on final source. |
| `uv run --no-sync pytest` | 19 passed: 15 existing plus four worker pipeline tests. |
| `uv run --no-sync ruff check .` | Passed. |
| `uv run --no-sync ruff format --check .` | Passed with repository exclusions. |
| `uv run --no-sync mypy src` | Passed. |
| `uv run --no-sync ruff check scripts/whisperx_transcribe.py tests/test_whisperx_worker_pipeline.py` | Passed, explicitly covering the normally excluded script. |
| `uv run --no-sync ruff format --check scripts/whisperx_transcribe.py tests/test_whisperx_worker_pipeline.py` | Test file passed; script reports two baseline exception-clause changes. `ruff format --diff --config pyproject.toml <temporary-baseline-script>` confirmed the identical pre-existing changes at the starting SHA. Preserved existing runtime-compatible syntax. |
| `go list -deps ./cmd/whisperx-worker` | Confirmed no application database/service/controller dependency. |
| `git diff --check`, `git diff --cached --check` | Passed. |

New tests cover concurrent acceptance, authentication, capacity/drain/replay, result/ACK restart, checkpoint matching/corruption, malicious media, input mutation, journal/result persistence faults, corrupt successful results, incompatible profile reuse, journal identity mismatch, ownership/budget recovery and referenced-checkpoint retention. Linux tests cover valid/malformed child output, progress failure, output/diagnostic bounds, cancellation, descendant reaping, FIFO rejection and actual SIGKILL of a worker helper followed by recovery of the accepted job. Python tests execute the existing script control flow for checkpoint retention/bindings, persistence failures and chunk-boundary stop.

### Linux and race commands

The Windows host has CGO disabled. Linux process semantics were tested in Docker, using the exact existing baseline image, no network and read-only source mounts. The following commands were run from PowerShell; quoted test flags prevent PowerShell from splitting dotted arguments.

```powershell
$env:GOOS = 'linux'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
go test -c -o bin/whisperx-worker-tests-stage-d ./internal/whisperxworker
docker run --rm --init --network none --mount type=bind,source=C:\DevRepo\Projects\briefcast\bin,target=/test-bin,readonly --mount type=bind,source=C:\DevRepo\Projects\briefcast,target=/source,readonly --workdir /source/internal/whisperxworker --entrypoint /test-bin/whisperx-worker-tests-stage-d sha256:a7542b94500a5ac7e00271451ef2a51db233bd5e2bec92c615b240302b06d052 '-test.v' '-test.timeout=90s'
docker run --rm --network none --mount type=bind,source=C:\DevRepo\Projects\briefcast,target=/source,readonly --mount type=bind,source=C:\Users\Taylor\go\pkg\mod,target=/gomod,readonly --mount type=bind,source=C:\DevRepo\Projects\briefcast\bin,target=/out --workdir /source --env GOMODCACHE=/gomod --env GOTOOLCHAIN=local --env CGO_ENABLED=1 golang:1.26 go test -race -c -o /out/whisperx-worker-race-tests-stage-d ./internal/whisperxworker
docker run --rm --init --network none --mount type=bind,source=C:\DevRepo\Projects\briefcast\bin,target=/test-bin,readonly --mount type=bind,source=C:\DevRepo\Projects\briefcast,target=/source,readonly --workdir /source/internal/whisperxworker --entrypoint /test-bin/whisperx-worker-race-tests-stage-d sha256:a7542b94500a5ac7e00271451ef2a51db233bd5e2bec92c615b240302b06d052 '-test.v' '-test.timeout=120s'
```

The non-race Linux suite passed. The race suite passed twice, including the final source and GC test (last run about 5.9 seconds). Existing module caches were mounted read-only; no dependencies were installed. Generated test binaries remain ignored under `bin/`.

## Real CPU rehearsal

Two real synthetic-speech jobs succeeded using the installed baseline engine: WhisperX 3.8.6, Torch/torchaudio 2.8.0+cu128, CTranslate2 4.8.1, faster-whisper 1.2.1 and pyannote.audio 4.0.7. Execution used Linux/amd64 CPU, medium.en, int8, batch 16, 120-second chunks, alignment enabled and diarization disabled. Alignment was requested and used, with no alignment error. Both transcripts matched the spoken sentence. The CUDA-enabled packages executed on CPU; this is not GPU validation.

The input was generated locally: “This is an isolated transcription test. Briefcast keeps durable worker results until the application confirms receipt. The quick brown fox jumps over the lazy dog.” Model caches were copied into an isolated writable cache; production sources were only read. Network was disabled, Hugging Face offline flags enabled, root filesystem read-only, media read-only, capabilities dropped, privileges restricted and the worker ran as UID/GID 1000. No GPU or host port was assigned. Token/state/cache were outside the build context; the worker had no application credentials or production mounts.

| Artifact | Evidence |
| --- | --- |
| Exact baseline image | `sha256:a7542b94500a5ac7e00271451ef2a51db233bd5e2bec92c615b240302b06d052` |
| First real job | `2a784b68-3bd7-40f7-a2e9-ff4c83d22ec5`; transcript digest `b274f639b53c96da8ebe16714d3b0b76a0cbecef321e106c2c764226ed8b9d5d` |
| Second real job | `a49ee94c-867a-4f58-8af8-a1d84fc9d00a`; transcript digest `c087aa3799b452f8eeed21e3fa6894c3cb738df3302921ccb58a0fce3971560c` |
| Image executing second job | `sha256:e9cfdd7b10f14b729e059200a76d6f755a91bfdf85b0af7fc6d01068d6a08c38` |
| Final rehearsal image | `briefcast-whisperx:stage-d-rehearsal`, `sha256:e36f18ddbe9610e734994879a43c975d4621019b17e93d6d18d70024c7faea84` |
| Final Linux binary SHA-256 | `82530f14f1c4f35099ee4db0d8f8cfe241aa3b86e1970c3529654df9bb5c378a` |
| Pipeline SHA-256 | `7aa01ea856e94438e728f944b8e71aee79a6843470afc67a0a3334e5d2c14520` |
| Persistent worker instance | `75f64ce8-5875-43f2-bf50-2003e22b8985` |
| Profile | `baseline-whisperx-3.8.6-cpu-stage-d` |

The final image incorporates the later journal/GC safeguards. It recovered both actual results with byte-for-byte equality, passed ready/status/drain/resume, and was stopped. Matching PUT replay and repeated exact ACK were also verified during the real-job rehearsal. The image was built offline from a separate local tag verified against the immutable baseline; it is a rehearsal artifact retaining baseline monolith layers, not the final independently packaged worker image. Stage F must replace this with pinned dedicated CPU/CUDA builds. Build command: `docker build --network none --pull=false -t briefcast-whisperx:stage-d-rehearsal <isolated-build-context>`. The context held only the worker binary, scripts and Python source. Private rehearsal files remain in the local temporary Stage D directory for review; their credentials are not tracked.

## Remaining gates and rollback

Stage D source is complete. Stage E must implement application HTTP submission identity, safe conditional transcript commits, retries, receipt reconciliation and legacy checkpoint handling. Stage F must pin runtime/model revisions, produce dedicated images and deployment configuration. Stage G must validate real multi-chunk/multiple-speaker/silence/malformed audio, CUDA, independent replacement/rollback, consistent backup/restore and the production observation window. Short synthetic CPU speech and deterministic crash tests do not satisfy those broader release gates.

No production deployment, database migration, push or runtime upgrade occurred. The complete production recovery package and restore rehearsal remain prerequisites for cutover; this report does not claim they exist. All test workers are stopped. The branch remains local. Rollback of this phase is to stop its isolated worker, retain its state/cache if needed and revert the Stage D source commit. The application still uses local execution and has no worker-state dependency or migration to reverse.
