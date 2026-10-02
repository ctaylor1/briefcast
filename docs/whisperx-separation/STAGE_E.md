# Stage E: application HTTP transcription backend

Prepared 2026-10-02 on `feat/whisperx-service`, starting from `57475b115269cc51a94c352887f03bdde208ce24`. This connects the application to the Stage D worker. Local execution remains the transitional default. No deployment, application schema migration, dependency change or public application API change is included.

## Implementation

| Location | Responsibility |
| --- | --- |
| `service/transcription_http.go` | Backend selection, deployment identity, worker-specific private-network transport, capabilities, polling, typed sanitized errors, result verification and ACK. |
| `service/transcription_remote_state.go` | Versioned checkpoint envelope, canonical request snapshot, rooted input hashing, legacy handling and exact-date job-lease fencing. |
| `service/transcription_remote.go` | Paginated queue eligibility, mandatory identity persistence, conditional transcript writes, retry bookkeeping, receipt reconciliation and safe downstream writes. |
| `service/whisperx.go` | New configuration and explicit HTTP dispatch through the existing execution seam. |
| `service/transcription_local.go` | Rejects remote/versioned checkpoints before launching Python; rollback must reconcile those rows explicitly. |

The existing `PodcastItem.TranscriptCheckpointJSON` text column contains the HTTP envelope. It stores deployment/episode/generation/job identity, attempt, original canonical request and fingerprints, profile, execution expectations, worker instance and revision, terminal/checkpoint information, reconnect bookkeeping and any pending receipt. No database columns are added. A new generation is created when a new episode submission has no existing remote envelope. Confirmed failed/cancelled attempts use a new job UUID and incremented attempt within their generation; uncertain acceptance, outages, app restarts and DB failures retain the original job UUID and request bytes.

The prepared worker instance ID is an additional envelope field. It protects the window before the first accepted status is received: a lost PUT response followed by worker state loss cannot silently recreate possibly accepted work on a new instance. An accepted job that becomes unknown requires operator reconciliation. Restored worker state preserves its instance ID. Environment changes cannot alter an in-flight options snapshot. Profile and explicit device/compute/batch/budget expectations are checked; there is no automatic local fallback.

Before the first job GET/PUT, the application must successfully commit the envelope. On success it validates the normalized transcript, canonical transcript digest, job/request identity, input/options/profile/pipeline/API provenance, inference options and outcomes. When the current capability profile matches, dependency provenance must match too. It rehashes its local audio before committing. Only transcript-owned columns are updated, conditioned on the current checkpoint, status, source identity and lease. Progress uses monotonic revisions/percent and remains below 100 until the transcript commits; succeeded inference is shown as `saving_result` during that interval.

The job-lock row ID can be reused by later runs. HTTP mode therefore fences by the exact acquired/renewed date and duration as well as the row ID. Heartbeat failure, expiry or ownership loss cancels the app wait, stops dispatch and prevents further persistence. Renewal and release cannot modify a successor's lease. Episode deletion, checkpoint clearing/regeneration, changed source URL/GUID/path, or input byte changes prevent stale transcript writes. One app instance per database remains the supported deployment; this does not replace blanket startup unlocking with a multi-instance coordination scheme.

## Results, receipts and downstream work

The successful transcript transaction retains its exact result digest in the checkpoint. Only then does the application send ACK. Failed transcript commits retain the worker job identity/result and do not ACK, export or summarize. Re-running after a DB failure retrieves the same worker result rather than repeating inference.

A lost ACK response leaves the completed row's receipt available for reconciliation. Each scheduler invocation reconciles up to 100 due generated HTTP receipts and repeats the idempotent ACK; successful ACK clears only the matching receipt. Completed legacy rows are filtered before the batch limit. Failed ACKs and corrupt/mismatched receipts defer reconciliation with existing retry-delay policy and retain their data for inspection. Available transcript status remains unchanged. Receipt validation compares the actual committed transcript digest before ACK. Terminal receipts are processed even when no inference items are due.

Exports and summarization follow successful database persistence. An ACK transport failure does not undo a committed transcript or force inference again. HTTP orchestration reloads the episode before downstream work and writes only summary-owned columns, with generation/payload and lease checks around the external request. Play/bookmark/title edits made during inference or summarization survive. Existing summary transport, model/prompt selection and export formats are reused. External summary requests are not part of a distributed transaction, and exactly-once LLM billing is not claimed. Export/summary failures continue through existing repair mechanisms.

The transitional local orchestration and its Stage B characterizations are preserved. The known local full-record persistence/early-sidecar-cleanup/downstream-on-save-failure behaviors are not advertised as safe remote behavior. Dedicated HTTP orchestration avoids those paths. Local mode refuses a remote/versioned checkpoint to prevent accidental duplicate local inference; Stage G rollback tooling must inventory and reconcile those envelopes before returning pending work to the monolith.

## Retry, cancellation and legacy handling

Worker transport/HTTP failures are typed independently of inference failure. HTTP 401/403, identity/digest conflicts, malformed or oversized responses and incompatible configuration remain visible and require correction/explicit repair. Capacity and infrastructure failures retain the UUID, preserve audio and do not increment inference retry count. Reconnect delay backs off to at most 15 seconds and respects bounded `Retry-After`; scheduling frequency still determines when the next invocation runs. Polling uses the configured interval plus jitter. Only a confirmed terminal worker failure increments inference retry count and uses the existing retry/backoff policy.

Automatic queue selection filters permanent failures before applying `WHISPERX_MAX_ITEMS`. Keyset pagination preserves processing/pending/failed priority and retry/download ordering, so a blocked first page cannot starve later due entries. Existing `ForceFailedTranscriptRetriesDue` explicitly rearms failed entries. A terminal attempt is deliberately replaced after that action; compatible worker checkpoints can resume. A summary marked compatible with zero completed seconds is not sufficient evidence of an available resume artifact.

Worker/mount/model/network errors never authorize source audio removal. Only locally verified audio absence resets download fields. Rooted regular-file validation and context-aware streaming hashes reject traversal, outside paths, special files, changes during hashing and cancelled reads. Application cancellation stops its HTTP wait and releases its lease but does not send remote cancellation for accepted work; the worker continues and its result is retrieved later.

Unversioned legacy checkpoints remain readable. They lack verified input/options provenance, so this ordinary scheduler preserves their descriptor and existing sidecar and explicitly starts from zero. It never sends absolute sidecar paths or trusts a completed-seconds counter as a resume offset. Trusted legacy import with rollout provenance remains Stage G work. Unknown/corrupt envelope versions block dispatch and preserve their original data.

## Configuration

| Variable | Default and validation | Owner |
| --- | --- | --- |
| `WHISPERX_ENABLED` | Existing false default; disabled transcription skips backend construction and network calls. | App |
| `WHISPERX_BACKEND` | `local`; supported values `local`, `http`, with no fallback for unknown values. | App |
| `WHISPERX_SERVICE_URL` | `http://whisperx:8090`; operator-controlled HTTP/HTTPS origin only, no credentials, path, query or fragment. | App |
| `WHISPERX_SERVICE_TOKEN_FILE` | Required in HTTP mode; regular file, at most 4096 bytes, token at least 32 characters without whitespace. Secret value is never logged. | App/worker shared secret |
| `WHISPERX_ENGINE_PROFILE` | Empty selects the advertised profile for new work, then persists it; explicit value must match. Existing jobs retain their accepted profile. | App expectation |
| `WHISPERX_SERVICE_RESULT_LIMIT_BYTES` | 67108864; zero selects the default; negative or greater than 1 GiB is rejected. Bounds temporary result storage and subsequent parsing. | App |
| `CONFIG` | Stable application config directory; HTTP fallback `/config`. Contains `whisperx-deployment-id`. | App |
| `DATA` | Application media root; HTTP fallback `/assets`. Worker mounts the same media tree read-only and uses relative references. | App/worker mounts |
| Existing model/ASR/VAD/alignment/diarization fields | Mapped to frozen v1 options. Python/script, HF credentials and executable/cache paths are not sent. | App options / worker deployment |
| Existing device/compute/batch/timeout fields | Explicit expectations are checked against effective execution; `auto` or batch zero allows worker selection. | Worker resources / app expectations |
| Existing concurrency/max-items/retry/lock/poll fields | Retained; HTTP polling defaults to 2 seconds, with jitter and 15-second cap. | App scheduling |

The deployment ID is a durable random UUID published atomically without replacing an existing creator. Corrupt identity fails closed. Copy `CONFIG/whisperx-deployment-id` with the application database and worker state during relocation; never regenerate it just because the container changes. Keep token files outside tracked source and build contexts.

HTTP mode uses a dedicated transport with 5-second connect/TLS timeouts, a 30-second request deadline, bounded headers, 1 MiB control responses, no proxy inheritance, no redirects and no compression. The private endpoint exception does not change public feed/media URL protections. Result bodies first stream to a bounded temporary file, then undergo bounded canonical JSON validation. Remote messages, bodies and transport errors containing URLs/headers are not logged or persisted as diagnostics. App startup does not probe the worker: readiness is checked by the scheduled transcription invocation. An outage leaves the library available and the queue intact; later scheduling recovers without an app restart or local runtime.

## Verification

All application tests use temporary SQLite databases/media and an authenticated Stage D worker with a deterministic runner. `BRIEFCAST_INTEGRATION_DB_URL` and `BRIEFCAST_WHISPERX_REAL` were removed from Go test environments. No production database, media mutations, model downloads or deployment were used. Python source, dependency manifests, worker implementation, Compose, frontend and release scripts are unchanged.

New tests cover durable pre-submission identity and DB failure, lost PUT/ACK responses, stable deployment identity, environment edits during reconnect, receipt reconciliation, concurrent library/summary edits, stale generations, changed bytes, episode deletion, missing/stolen leases, status revision regression, malformed/provenance/digest/oversized results, redirects, HTTP authorization/capacity/server errors, accepted-job disappearance, worker instance loss before acceptance confirmation, blocked-page pagination, confirmed retry/manual repair, verified checkpoint resume, legacy preservation, unknown versions, rooted media and app cancellation/outage recovery without Python.

The first focused run found an incorrect hard-deletion test assertion; it was corrected to accept the repository's actual row removal. Retry testing also exposed that a compatible summary can exist without a persisted checkpoint; the adapter now requires positive verified completed seconds before naming a resume source. Those failure cases pass after correction.

| Command | Result |
| --- | --- |
| `go test ./service -run '^TestRemote(Deployment\|Configuration)' -count=1` | Initially failed as expected because the new implementation did not exist; passed after implementation. |
| `go test ./service -run '^Test(Remote\|LocalBackendRejectsRemote)' -count=1` | Passed repeatedly; latest full focused host run 28.980 seconds, before the final transfer-classification refinement. |
| `go test ./service -run '^TestRemote(InterruptedResult\|StructuredPermanent\|CorruptReceipt\|CompletedLegacy)' -count=1` | Passed on final source, 3.442 seconds. |
| `go test ./...` | Passed; service 112.432 seconds, controllers 24.648 seconds. Final HTTP receipt/transfer refinements were subsequently checked by focused tests and the Linux race suite. |
| `go vet ./service ./internal/transcription ./internal/whisperxworker` | Passed on final source. |
| `uv run --no-sync pytest` | 19 passed. |
| `uv run --no-sync ruff check .` | Passed. |
| `uv run --no-sync ruff format --check .` | Passed. |
| `uv run --no-sync mypy src` | Passed. |
| `git diff --check`, `git diff --cached --check` | Passed. |

The host has CGO disabled. Linux race validation used existing local `golang:1.26` and baseline images with networking disabled and source/module caches mounted read-only. No dependencies were installed. An initial cross-compiled service race binary passed the remote and Stage C seam cases in the exact Stage D baseline image. The following final source run passed all selected cases without race reports (service 30.217 seconds), including the final receipt and interrupted-transfer regressions:

```powershell
docker run --rm --init --network none --mount type=bind,source=C:\DevRepo\Projects\briefcast,target=/source,readonly --mount type=bind,source=C:\Users\Taylor\go\pkg\mod,target=/gomod,readonly --workdir /source --env GOMODCACHE=/gomod --env GOTOOLCHAIN=local --env CGO_ENABLED=1 --env BRIEFCAST_INTEGRATION_DB_URL= --env BRIEFCAST_WHISPERX_REAL= golang:1.26 go test -race ./service '-run=^Test(Remote|LocalBackendRejectsRemote|TranscriptionBackend)' '-timeout=120s' -count=1 -v
```

Stage D's real CPU inference evidence remains in [STAGE_D.md](STAGE_D.md); this phase verifies the application/worker contract with deterministic inference, and does not claim a new real CPU/CUDA or production acceptance run. Frontend tests were not repeated because frontend files are unchanged. The commit hooks run Ruff format/lint, mypy and pytest with `UV_NO_SYNC=1` to avoid installing dependencies.

## Rollback and remaining work

The source change can be reverted while local remains configured and no remote rows have been created. Once HTTP jobs exist, first drain/inventory the worker and preserve database, deployment ID, worker state and legacy files; reconcile pending remote envelopes before enabling old local code. Do not discard completed transcripts or outstanding receipts, and do not clear all checkpoint columns indiscriminately. No schema rollback is required.

Stage F is independent application/worker image packaging and deployment configuration. Stage G includes trusted legacy import, the full real-audio/CUDA acceptance matrix, explicit worker replacement/rollback and production recovery/restore evidence. No production cutover is authorized by these test results. This branch remains local and unpushed.
