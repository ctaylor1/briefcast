# Private worker contract v1 rules

`openapi.yaml` defines the wire shapes. This file fixes cross-request behavior that JSON Schema cannot express. API v1 is implemented by `cmd/whisperx-worker`, `internal/transcription`, `internal/whisperxworker` and the app HTTP adapter. [Operations](OPERATIONS.md) documents runtime settings, storage and recovery commands; [Stage G](STAGE_G.md) distinguishes tested behavior from outstanding production acceptance. The protocol/fixtures were frozen in Stage A and remain version 1.

## Identity, acceptance and state

- The app creates UUIDs for deployment, episode, generation and job. It persists canonical UTF-8 PUT request JSON, job UUID, options snapshot and source fingerprint in its versioned checkpoint envelope before making the first request. `request_digest` is lower-case SHA-256 of the request body serialized as canonical JSON per RFC 8785. A lost response is retried with that UUID and the same canonical request. JSON whitespace or object key order does not change identity; a different parsed request under an existing UUID is a conflict. The server rejects duplicate object keys, nonfinite numbers, trailing JSON, unknown fields and unsupported options before acceptance.
- `options_sha256` is lower-case SHA-256 of the request's `options` object serialized as canonical JSON per RFC 8785. This fingerprint identifies option compatibility independently of whitespace and request identity. `input.sha256` is a streaming hash of the audio bytes, not of the relative path.
- The worker validates authentication, contract major, option/profile support, input path and declared size/hash, resume compatibility, execution prerequisites and capacity before committing a submission. It atomically commits the UUID, request digest, validated request and initial `queued` status. Only then may it return 201. A matching replay returns 200 and the current persisted status. A different digest returns 409. A full queue or drain returns 429/503 without acceptance. On uncertain transport failure, the app asks for the same UUID before creating any new attempt.
- State transitions are `queued -> running -> succeeded | failed | cancelled`, with `running -> cancelling -> cancelled` after the process group is reaped. A recovered `running` attempt may return to `queued` after verified checkpoints. Terminal states are immutable. Cancellation and success race through one durable transaction. `revision` starts at 1 and increases for every persisted state/progress change. Percent and completed seconds cannot decrease within one attempt; a new attempt has a new UUID. Status `percent: 100` alone is not application success.
- A succeeded job has a readable, verified result artifact before the state transaction commits. A result body is an envelope whose `transcript` is the normalized result. `result_sha256` is the lower-case SHA-256 of the **transcript object's** canonical UTF-8 JSON per RFC 8785, excluding the outer envelope. This makes the Stage A result fixture executable without a self-referential digest. The worker checks the digest on retrieval; the app checks it before committing. An ACK requires a succeeded state and exact digest and is idempotent. The app ACKs only after the conditional transcript DB commit. Unacknowledged results have no time-based expiry. Large acknowledged artifacts may be pruned after seven days, leaving compact identity and digest tombstones.
- Drain and resume state is persisted. Drain rejects new PUTs but allows accepted work and retrieval/ACK to complete. Health liveness is cheap. Readiness verifies storage/path/runtime prerequisites without loading a model; normal queue saturation is reported in capabilities and may still be ready. No worker endpoint is publicly exposed by the default Compose setup.

## Configuration and compatibility

The app maps `WhisperXConfig` fields in `service/whisperx.go` exactly as follows. All request fields are required, including those that are inactive when a feature is off, so accepted configuration can be reproduced. Do not infer production defaults from the Python script or `.env.example` alone. At cutover, record the actual effective values privately.

| Existing source | Request field | Constraint or owner |
| --- | --- | --- |
| `WHISPERX_MODEL`, `WHISPERX_LANGUAGE`, `WHISPERX_CHUNK_SECONDS` | `options.model`, `language`, `chunk_seconds` | Worker-advertised model; 1–3600 second chunks |
| `WHISPERX_ALIGN`, `WHISPERX_DIARIZATION` | `options.align`, `diarization` | Best-effort stages retain explicit outcome fields |
| `WHISPERX_DIARIZATION_MODEL`, `WHISPERX_MIN_SPEAKERS`, `WHISPERX_MAX_SPEAKERS` | Corresponding `options` fields | Supported model; speaker range 1–32 and min <= max |
| `WHISPERX_VAD_METHOD` | `options.vad_method` | `pyannote` or `silero`; actual method recorded in result |
| `WHISPERX_BEAM_SIZE`, `WHISPERX_PATIENCE`, `WHISPERX_CONDITION_ON_PREVIOUS_TEXT`, `WHISPERX_INITIAL_PROMPT` | `options.asr_options` | Beam 1–64; patience 0–10; prompt <= 4096 characters |
| `WHISPERX_VAD_CHUNK_SIZE`, `WHISPERX_VAD_ONSET`, `WHISPERX_VAD_OFFSET` | `options.vad_options` | Chunk 1–3600; onset/offset 0–1 |
| `WHISPERX_DEVICE`, `WHISPERX_COMPUTE_TYPE`, `WHISPERX_BATCH_SIZE` | `effective_config` response | Worker deployment-owned; reject an incompatible app expectation, never silently substitute |
| `WHISPERX_TIMEOUT_SECONDS` | `effective_config.execution_timeout_seconds` | Worker-owned cumulative execution budget, initially 21,600 seconds; queue wait is separate |
| `WHISPERX_MAX_CONCURRENCY`, `WHISPERX_MAX_ITEMS`, retry delay/backoff, lock and polling settings | No request field | App scheduling or worker capacity; not inference options |
| Python/script paths, HF token/home, logging and telemetry settings | No request field | Deployment-owned; tokens and executable paths are forbidden in requests |

The worker returns supported models, diarization models, VAD methods and checkpoint formats in capabilities. Client preflight checks wire major 1, profile, options and device/compute/batch expectations before submission. Supported model lists and `model_ready` do not prove every model weight is cached or gated access will succeed: current readiness checks runtime/storage prerequisites without loading models. A real canary verifies those dependencies. `engine_profile` is immutable for an accepted job. Shipped profiles are `baseline-wx3.8.6-cpu-v1` and `baseline-wx3.8.6-cu128-v1`, with pinned combinations in `docker/whisperx/compatibility.json`. Fixture profile strings remain synthetic examples and are not deployment defaults.

Checkpoint format `briefcast-whisperx-chunks-v1` records input SHA-256, options SHA-256, pipeline version and completed chunk/segment metadata. `resume_from_job_id` must name a terminal failed/cancelled job in the same deployment, episode and generation, with matching input and compatible options/profile/checkpoint format. The worker rejects an incompatible reference with `CHECKPOINT_INCOMPATIBLE`; it never skips audio solely because a seconds counter exists. Legacy sidecars can be imported only by the trusted rollout path after validating their rooted location and provenance; an API request cannot name an absolute sidecar path.

The app's existing `PodcastItem.TranscriptCheckpointJSON` text column holds a versioned internal envelope in HTTP mode. Its minimum shape is:

```json
{
  "version": 1,
  "backend": "http",
  "deployment_id": "11111111-1111-4111-8111-111111111111",
  "episode_id": "22222222-2222-4222-8222-222222222222",
  "generation_id": "33333333-3333-4333-8333-333333333333",
  "job_id": "44444444-4444-4444-8444-444444444444",
  "attempt": 1,
  "worker_instance_id": null,
  "engine_profile": "baseline-whisperx-3.8.6-cuda128",
  "input_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "options_sha256": "60baeabe17a6baa942c7b52f59aaf6ea87ed413aba156038ed6fdc68a8b2c83e",
  "request_digest": "1e2a33ce2babc8b68df214870738c2c68b1cc17f67bf411bc5ca0704c1b8ba23",
  "request_json": "<persisted canonical UTF-8 PUT body, represented as a JSON string>",
  "last_revision": 0,
  "receipt_result_sha256": null,
  "legacy_import": null
}
```

The `request_json` value above is explanatory; this minimum-shape example is not a complete writable checkpoint. The implementation also stores `expected_effective_config`, a checkpoint summary and, when applicable, prepared worker identity, terminal/retry/reconnect metadata and a restart reason (`service/transcription_remote_state.go`). Operators must use the shared decoder/rollback helper rather than hand-edit envelopes. A later accepted status fills `worker_instance_id` and advances `last_revision`; the receipt digest remains until ACK succeeds. An unversioned legacy checkpoint is parsed only as the current local format (`completed_seconds`, `total_seconds`, `language`, `segments_file` and optional inline `segments`). Unknown envelope versions stop automatic dispatch. A legacy absolute `segments_file` is resolved beneath the configured asset root during trusted import and never sent as a request option. The app's stable deployment ID is `/config/whisperx-deployment-id` and must travel with its database on relocation.

## Resource, result and error rules

- Client connect timeout: 5 seconds. Per-request deadline: 30 seconds. Poll at 2 seconds with jitter and backoff capped at 15 seconds after transient failures. Control bodies are limited to 1 MiB. The default result limit is 64 MiB and can be raised by tested configuration. Large results use bounded temporary storage. Timeouts distinguish queue wait, HTTP transport and the cumulative inference budget.
- Default worker capacity is one running job and four waiting jobs. Saturation returns 429 with `Retry-After`; it is not a failed accepted attempt. Storage or path failure must not return 201 or a false succeeded status. After three consecutive crash recoveries without checkpoint progress, fail visibly.
- Media paths use `/` separators relative to the app asset root. Reject absolute, drive, UNC, traversal, NUL, special-file and symlink-escape paths. The worker streams the file to verify size and SHA-256 before execution and again before committing success. It never fetches a URL. Result JSON requires expected provider/profile, arrays of segment/word objects, finite nonnegative times and `end >= start` where both exist, bounded text and collections, and real alignment/diarization outcomes. Empty segments are valid silence; an error object is not a transcript. Optional missing word times and speakers remain missing. Both sides validate; no fabricated fields.
- Public errors carry a sanitized `code`, `message` and `retryable`; stderr and tokens stay in private diagnostic state. Defined codes are `WORKER_UNAVAILABLE`, `BUSY`, `MODEL_UNAVAILABLE`, `MODEL_ACCESS_DENIED`, `INVALID_CONFIG`, `INPUT_MISSING`, `INPUT_CHANGED`, `INPUT_UNREADABLE`, `STATE_UNAVAILABLE`, `CHECKPOINT_INCOMPATIBLE`, `TIMEOUT`, `CANCELLED`, `INTERNAL`. HTTP 401/403 indicate authorization failure, 404 only an actually unknown UUID, 409 a request/state/digest conflict, and 410 only a pruned acknowledged result. Worker/mount/model/network errors never trigger app media deletion. A worker `INPUT_MISSING` does not prove the app's local audio is missing.
- The app conditionally writes only transcript-owned columns after matching generation and source fingerprint, then exports/summarizes according to existing sequencing. It reconciles committed but unacknowledged results after restart. An HTTP outage leaves the app usable and preserves local audio. One app and one worker instance per state directory are supported in this release.

## Fixture scope

`fixtures/` shows one valid submission, queued and succeeded status, result, ACK and a structured error. UUIDs, transcript and versions are synthetic. Digests are calculated from the parsed fixture request/result as RFC 8785 canonical JSON, regardless of file line endings. Executable fixture/hash tests are in `internal/transcription/protocol_test.go`; worker and service suites cover negative requests, replay conflict, paths/hashes, malformed output and terminal races. See [Stage G evidence](STAGE_G.md) for real CPU/CUDA recovery and the remaining matrix gaps.
