# WhisperX service separation specification

Prepared on 2026-10-02 for coding agents implementing the separation on the current Docker host. Read this specification together with [the execution plan](EXECUTION_PLAN.md). These documents describe future implementation work; their creation does not mean the images, backups, branches, or implementation already exist.

Implementation status is now recorded in the stage reports. For the implemented API and operational commands, read [CONTRACT.md](CONTRACT.md) and [OPERATIONS.md](OPERATIONS.md). [Stage G](STAGE_G.md) records outstanding acceptance gates; this design document does not certify production readiness.

The recommended design is a separately versioned Briefcast transcription worker, maintained in this repository, with a stable private HTTP contract. Initially reuse Briefcast's existing Python transcription pipeline and the installed WhisperX dependency versions. Keep the application responsible for podcast data and workflow decisions. Upgrade WhisperX independently only after compatibility tests pass. Host migration is a later project.

## 1 Goals and boundaries

The implementation must deliver:

- A normal Briefcast image without WhisperX, Torch, CUDA, or other transcription model dependencies. Retain Python and the feedparser and mutagen helpers used by other features; do not assume the app can become a Go-only image.
- A separate CPU or NVIDIA CUDA worker image with its own release version, dependency lock, persistent model cache, and durable job state.
- Preservation of transcript content structure, progress, chunk resume, alignment and diarization behavior, queue eligibility, retry timing, exports, canonical transcripts, and summarization sequencing.
- Independent worker upgrades and rollback without rebuilding or replacing the app image when the service contract remains compatible.
- Explicit, repeatable Compose commands, an isolated rehearsal environment, and a verified recovery package for the current installation.
- No production database schema changes in the initial split. A versioned internal checkpoint representation in the existing text column is an intentional internal data-contract change and must be tested against old data and rollback.

Exclude Kubernetes, Redis, Kafka, S3, multiple application replicas, remote-host audio upload, new public frontend features, broad cleanup, database replacement, and the machine migration itself. Do not change the transcription model, enable diarization in production, or tune accuracy/performance while proving separation. Improving model residency and GPU throughput is later work.

The user requested a plan for implementation, not live deployment in this documentation turn. During execution, follow AGENTS.md, preserve unrelated changes, and implement the authorized separation in small commits. New external dependencies or a different architecture need an explicit decision; the default below uses Go standard-library HTTP and dependencies already in the repository.

## 2 Observed baseline

Reconfirm all runtime observations immediately before execution.

| Item | Observed value |
| --- | --- |
| Source HEAD | `62ecec0`, Add OpenRouter model support and summary job tracking |
| Branch status before writing these documents | Clean `main`, one commit ahead of the locally recorded `origin/main`; remote freshness was not verified |
| Running image | `briefcast:1.9.4` |
| Running image ID | `sha256:a7542b94500a5ac7e00271451ef2a51db233bd5e2bec92c615b240302b06d052` |
| Platform and image size | Linux/amd64, approximately 13.3 GB |
| Installed packages | WhisperX 3.8.6; Torch and torchaudio 2.8.0+cu128; CTranslate2 4.8.1; faster-whisper 1.2.1; pyannote.audio 4.0.7 |
| Effective transcription settings | CUDA, float16, batch size 16, chunk size 120 seconds, concurrency 1, diarization false |
| Media | `E:/Podcasts` mounted at `/assets` |
| Configuration | Repository `config` mounted at `/config`; Hugging Face cache exists below it |
| Database | PostgreSQL through `host.docker.internal`; not the SQLite file present in `config` |
| Compose files used by running container | Base file plus `docker-compose.override.yml` |

Do not infer that HEAD produced the running image. Record both independently. `docker diff` showed runtime caches, including NLTK data outside `/config`; capture and classify the full diff before claiming an image archive reproduces the complete runtime. The running container was restarted during the interval between the two assessments; use current inspection, not an assumed uptime.

Relevant implementation locations:

| Location and symbol | Relevance |
| --- | --- |
| `service/whisperx.go`, `TranscribePendingEpisodes`, `RunWhisperXWithProgress` | Queue, locks, subprocess, progress, error handling, result persistence |
| `scripts/whisperx_transcribe.py`, `main`, checkpoint helpers | Chunking, alignment, diarization, JSON result, sidecar resume |
| `db/podcast.go`, `PodcastItem` | Transcript status, retry state, progress, checkpoint JSON, lineage |
| `service/repair_work.go`, transcript repair functions | User-triggered repair and failed-job retry behavior |
| `service/canonical_transcript.go`, `service/export.go`, `service/summarize.go` | Downstream consumers whose behavior must be preserved |
| `service/outbound_url_policy.go` | Private service networking must not weaken feed URL protections |
| `main.go`, `intiCron` | Scheduler starts automatically and calls `UnlockAllJobs` |
| `Dockerfile`, Compose files, `justfile`, release scripts and workflow | Coupled dependency builds and inconsistent GPU selection |
| `service/fileService.go`, `CreateBackup` | Only archives `/config/briefcast.db`; does not protect the live PostgreSQL database |

## 3 Wrapper research and decision

These are the strongest relevant candidates found in this assessment, not an exhaustive popularity ranking. GitHub stars are an adoption signal, not proof of reliability or Docker download volume. Counts and last-push dates were retrieved from GitHub repository metadata on 2026-10-02. Documentation and selected source were inspected; no third-party wrapper image was run.

| Candidate | Adoption and activity observed | Fit and decision |
| --- | --- | --- |
| [ahmetoner/whisper-asr-webservice](https://github.com/ahmetoner/whisper-asr-webservice) | 3,349 stars, 586 forks; pushed Oct 1; latest release reported v1.10.0 | Best general-purpose candidate. MIT wrapper; CPU/GPU images and WhisperX backend. Inspected `/asr` executes transcription before returning output. No durable job submission, cancellation, or Briefcast-compatible chunk-resume contract in the inspected API. Do not substitute it directly. |
| [jim60105/docker-whisperX](https://github.com/jim60105/docker-whisperX) | 456 stars, 46 forks; pushed Sep 27 | Strong packaging candidate. Community CLI images, scheduled builds, models and cache conventions. It is not an HTTP job service. Potential future base image only after digest, dependency, entrypoint, Python, CPU/GPU, and script compatibility checks; it does not replace our supervisor. |
| [murtaza-nasir/whisperx-asr-service](https://github.com/murtaza-nasir/whisperx-asr-service) | 106 stars, 25 forks; pushed Sep 30; latest release reported v0.4.2 | API and multiple GPU image variants. Inspected README describes alpha limitations, no authentication, and simple/Ray modes. No verified compatibility with Briefcast checkpoints. Repository metadata did not classify the license, but the inspected LICENSE includes MIT terms. Reject as the default until contract tests and current release behavior establish a better fit. |
| [Nyralei/whisperx-api-server](https://github.com/Nyralei/whisperx-api-server) | 92 stars, 26 forks; pushed Sep 24; latest release reported v1.4.0.1 | Closest lifecycle feature set: progress, authentication, CPU/CUDA and distributed workers. Its inspected status tracker is process-local; distributed durability adds Kafka/S3. No verified chunk-checkpoint compatibility. GPL-3.0 wrapper; do not copy its source into this MIT repository without a separate licensing decision. Reject the added infrastructure for this scope. |

Source detail: [Ahmetoner routes](https://github.com/ahmetoner/whisper-asr-webservice/blob/main/app/webservice.py), [endpoint documentation](https://github.com/ahmetoner/whisper-asr-webservice/blob/main/docs/endpoints.md), [Jim Dockerfile](https://github.com/jim60105/docker-whisperX/blob/master/Dockerfile), [Murtaza README](https://github.com/murtaza-nasir/whisperx-asr-service/blob/main/README.md), [Murtaza LICENSE](https://github.com/murtaza-nasir/whisperx-asr-service/blob/main/LICENSE), [Nyralei status storage](https://github.com/Nyralei/whisperx-api-server/blob/main/src/whisperx_api_server/request_status.py). Resolve reviewed files to commit SHAs in the implementation ADR because main-branch links can change.

The decision is a Briefcast-owned worker. This is an inference from the contract gaps, not a claim that the other projects are poor transcription engines. Wrapping a synchronous third-party endpoint in a queue still cannot recover its internal chunk checkpoints, and splitting audio outside it can change alignment and speaker assignments. Building a small supervisor around our existing script preserves more tested behavior with fewer moving parts.

Before coding, record this decision in `docs/adr/whisperx-worker.md`, refresh the candidates' release metadata, and check whether a newer release actually closes the gaps. Do not spend an open-ended research phase implementing competing adapters. An alternative must pass the same acceptance suite and preserve durable resume, error semantics, output fields, CPU/CUDA operation, and independent upgrades without extra infrastructure. Otherwise proceed with the specified worker.

As of this assessment, [GitHub's latest WhisperX release](https://github.com/m-bain/whisperX/releases/tag/v3.8.6) and [PyPI](https://pypi.org/project/whisperx/) both report 3.8.6, already installed. Upstream main declares 3.8.7rc1, which is a prerelease development version. The inspected upstream Python constraint is >=3.10 and <3.14. Keep the worker on the known Python 3.12 baseline initially; the repository's Python 3.14 tooling project is not its runtime dependency lock.

## 4 Service ownership and implementation shape

Use two services on the same host and Compose network:

```text
Browser -> Briefcast -> application PostgreSQL or SQLite
               |
               | authenticated HTTP jobs, progress, results
               v
       Briefcast WhisperX worker -> Python transcription child
               |                         |
               +---- /state               +---- /media read only
               +---- /cache
```

The app owns episode selection, download validation, application job locks, retries between attempts, application database changes, canonical transcript construction, exports, and summary scheduling. It holds no Torch/CUDA runtime and does not launch local transcription in the final lean image.

The worker owns accepted execution attempts, bounded queueing, model access, child-process lifecycle, progress capture, checkpoint files, persisted results, cancellation, and recovery of interrupted attempts. It has no application database credentials, LLM key, app password, feed fetching, export logic, or Docker socket.

Implement the supervisor in Go using `net/http`, the current Go module, and a dedicated local SQLite store through the SQLite dependency already used by this repository. Do not import `service` or initialize the global app `db.DB` in the worker. Keep worker models and migrations private to its state store. Reuse the Python script and its logging helper; do not rewrite the model pipeline into Go or introduce FastAPI solely for routing.

Suggested boundaries, with exact names adjustable to existing conventions:

- `internal/transcription/`: shared wire types, validation, version negotiation, normalized result and error definitions; no app database imports.
- `service/transcription_backend.go`, `transcription_local.go`, `transcription_http.go`: backend seam and app adapter. Keep local mode during transition and characterization; final lean deployment requires HTTP mode when transcription is enabled.
- `cmd/whisperx-worker/`: worker command, configuration, health/administration subcommands.
- `internal/whisperxworker/`: HTTP handlers, independent state store, scheduler and subprocess runner.
- `docker/whisperx/`: Dockerfile, separate CPU/CUDA locks, compatibility manifest and build instructions.
- Existing `scripts/whisperx_transcribe.py`: shared during transition, shipped only in the worker image in the final normal deployment. Keep feedparser/mutagen scripts in the app image.

Do not add a generic plugin framework. One backend interface, one production HTTP adapter, and one temporary local adapter are sufficient. The app's orchestration must call backend-specific preflight: HTTP mode must never require a local Python executable or run `py_compile` on a missing script.

## 5 HTTP contract

Commit an OpenAPI document and JSON fixtures before parallel client/server implementation. Wire contract version 1 is independent from the worker package version and the WhisperX version. Breaking wire changes require a new major contract. Unknown optional response fields are tolerated; unsupported request fields and options are rejected rather than silently dropped.

All job endpoints require a bearer token mounted from a secret file. Health responses contain no secrets or filesystem paths. Tokens are never included in job bodies or stored results.

| Endpoint | Required behavior |
| --- | --- |
| `GET /health/live` | Cheap process health; never loads models or downloads resources |
| `GET /health/ready` | Store and paths usable, supervisor accepting work, declared runtime prerequisites valid; 503 while draining or broken; saturation remains a capacity condition |
| `GET /v1/capabilities` | Contract versions, worker instance ID, worker and pipeline versions, dependency versions, supported options/checkpoint formats, device, model readiness, capacity |
| `PUT /v1/jobs/{job_id}` | Client-generated UUID; atomically persist submission before 201. Identical replay returns existing state with 200. Same ID with different request digest returns 409. Capacity exhaustion returns 429 and Retry-After without acceptance |
| `GET /v1/jobs/{job_id}` | Persisted state, progress revision, stage/percent, checkpoint summary, sanitized structured error and provenance; 404 only if genuinely unknown |
| `GET /v1/jobs/{job_id}/result` | 200 only for a durably succeeded job; 409 before success; 410 for an acknowledged result whose large artifact was pruned; returns normalized transcript JSON and an integrity digest |
| `POST /v1/jobs/{job_id}/cancel` | Idempotent request; cancellation is not terminal until the child process group has stopped. Already terminal returns its current state |
| `POST /v1/jobs/{job_id}/ack` | Idempotent receipt after the app commits the transcript. Accept only a succeeded job and matching result digest |
| `POST /v1/admin/drain` and `/v1/admin/resume` | Authenticated maintenance operations. Drain rejects new submissions, lets accepted jobs finish, and reports remaining jobs. Must survive worker restart |

The API is asynchronous. Never hold a transcription HTTP request open for hours. Do not introduce callbacks or a browser connection to the worker. The app polls with bounded requests.

Example request, with illustrative values only:

```json
{
  "contract_version": 1,
  "deployment_id": "deployment-uuid",
  "episode_id": "episode-uuid",
  "generation_id": "generation-uuid",
  "attempt": 1,
  "input": {
    "relative_path": "audio/Example/episode.mp3",
    "size_bytes": 123456,
    "sha256": "64-hex-characters"
  },
  "engine_profile": "baseline-whisperx-3.8.6-cuda128",
  "options": {
    "model": "medium.en",
    "language": "en",
    "chunk_seconds": 120,
    "align": true,
    "diarization": false,
    "asr_options": {},
    "vad_options": {}
  },
  "resume_from_job_id": null
}
```

The production schema must enumerate and constrain all existing script options: model, language, chunk length, beam size, patience, condition-on-previous-text, prompt, VAD chunk/onset/offset/method, alignment, diarization model and speaker limits. Preserve the effective values captured at rollout. Device, compute type, worker concurrency, and batch-size limits are deployment-owned; the accepted effective configuration is returned and recorded. Resume requires the same relevant input, pipeline and option fingerprints. HF tokens and arbitrary executable/script paths are not options.

A status includes `job_id`, `request_digest`, `worker_instance_id`, `state`, `revision`, `stage`, `percent`, `completed_seconds`, `total_seconds`, timestamps, attempt information, checkpoint compatibility and error. Revision increases monotonically. Progress is monotonic within an attempt; a new attempt may reset it explicitly. Report successful completion to the user only after the app database commit, even when inference itself is finished.

Preserve the current result fields: provider, model, language, device, compute_type, batch_size, ASR/VAD options, actual VAD method, aligned/alignment outcome, diarization outcome, segments_pre_align, segments, and metadata. Add worker/API/pipeline versions, immutable engine profile, input/config hashes, job ID and package versions to metadata. Never fabricate missing words, timestamps, speaker labels, or a successful alignment. Preserve existing best-effort alignment/diarization fallbacks and expose their failures as metadata and logs; unsupported configuration must fail preflight.

Validate the result schema before committing success: expected provider/profile, segment/word types, finite nonnegative timing values with end >= start where timestamps exist, and bounded text/collection sizes. Empty segments are valid for silence; a syntactically valid JSON error object is not a transcript. Preserve optional or absent word timings exactly rather than inventing values. Both worker and client validate the contract, and use package metadata rather than an unreliable module `__version__` attribute for installed dependency versions.

Default client limits: 5-second connect timeout; 30-second per-request deadline; 2-second polling with jitter and a cap of 15 seconds during transient failures; 1 MiB control body; 64 MiB result cap, configurable after testing long recordings. Stream large results to a bounded temporary file instead of unbounded stdout/HTTP buffers. Preserve the current 21,600-second execution limit; distinguish queue delay, HTTP timeout and inference timeout. The worker owns a durable cumulative execution budget so restarts cannot reset a job's time limit indefinitely.

## 6 Persistence and recovery rules

The application business queue remains authoritative. The worker's small accepted-job queue is an execution journal, not a second independent retry scheduler. Default worker execution concurrency is one and waiting capacity four; the app also starts with one worker. Capacity is configurable and tested, not inferred from GPU count.

Persist app submission identity before making the first HTTP request. Use a versioned envelope in `PodcastItem.TranscriptCheckpointJSON` containing backend, generation UUID, job UUID, attempt, deployment and worker instance IDs, input hash, options snapshot/configuration hash, engine profile, last revision, and any legacy checkpoint import descriptor. Persist a stable random deployment ID under `/config` and document copying it with the database/state during future relocation. Never derive identity from hostname or a transient container ID.

Store the original accepted options in the envelope so a restart with edited environment settings cannot silently change an in-flight request. A deliberate regeneration uses a new generation. A retry of a confirmed failed attempt uses a new job UUID and incremented attempt, with a validated reference to its prior checkpoint. Lost HTTP responses, polling errors, application restarts and unknown completion outcomes reuse the same job UUID. Do not use the existing retry counter alone as an idempotency key.

For each new generation, resolve the engine profile from authenticated capabilities and persist it before PUT. During a worker upgrade, an app may have persisted a request that the draining worker never accepted. Keep accepted jobs on their recorded profile. Only after the old worker is quiescent, the same persistent worker instance explicitly reports that UUID unknown, and no previous execution can remain active may the app replace that unaccepted submission with a new UUID/profile; record the supersession. Do not mutate/replay a possibly accepted UUID with different request content. Missing or restored journals require the explicit reconciliation procedure, not this normal upgrade shortcut.

Worker states: `queued -> running -> succeeded | failed | cancelled`; interrupted `running` attempts recover as `queued` after validating durable checkpoints. Cancellation may pass through `cancelling`. A completed result and cancelled state race must have one durable winner; never report cancellation while its child can still publish success.

Required failure behavior:

| Failure boundary | Required recovery |
| --- | --- |
| App fails before submitting | Persisted envelope is replayed on next eligible run |
| Worker accepts PUT but response is lost | Same UUID and digest reconnect to the existing attempt |
| App restarts while worker runs | Poll the existing attempt; do not spawn a duplicate |
| Worker process/container restarts | Reconcile stored jobs and resume last complete chunk, with bounded crash recovery; after three consecutive crash recoveries without progress, fail with a structured error |
| Worker finishes while app is unavailable | Keep result and checkpoints until the app retrieves and commits the result |
| App DB write fails | Do not acknowledge, clear checkpoint, export, or summarize a transcript that was not committed; retrieve the same result later |
| App commits then crashes before ACK | Reconcile completed rows with outstanding worker receipts and repeat ACK; never rerun inference |
| Worker state volume is missing/replaced | Changed instance ID or absent journal triggers a visible recovery condition. Do not silently assume previous jobs never existed |
| Disk full or checkpoint persistence failure | Reject intake or fail recoverably with last safe checkpoint retained; never claim durable acceptance/success |
| Media changed during processing | Reject stale result; preserve media; create a new generation only after revalidation |

Use worker-local SQLite transactions for submission uniqueness, state transitions, result pointers and ACKs. Use separate per-job directories beneath `/state/jobs`. Write files to temporary siblings, flush and atomically rename, then commit the corresponding state reference. Validate fingerprints and references during recovery. A succeeded state must always refer to a readable verified result. Orphan files after a crash are reconciled safely.

Only one worker process may own a state directory. Enforce ownership with a process-level lock and test a second start against the same directory. Do not host the worker's SQLite journal on an unvalidated network filesystem or share it across replicas.

Keep unacknowledged successful results until ACK; default no time-based deletion. Apply disk/capacity backpressure and expose counts instead of silently discarding them. Delete acknowledged large artifacts after a default seven-day retention; retain compact submission/result tombstones for idempotency. Never delete queued/running state or checkpoints referenced by a later attempt. Document backup and garbage-collection behavior and test references before cleanup.

The app must update only transcript-owned columns with a conditional check of the current generation/checkpoint, not save a stale full episode object after a long request. Preserve play status, bookmarks and unrelated concurrent edits. On success, commit transcript, canonical text and lineage before ACK and downstream work. Retain receipt information in the checkpoint until ACK succeeds, then clear it; terminal-row reconciliation must be bounded and must not change public transcript status values. Export/summary failures follow the existing repair paths and do not rerun transcription. Exactly-once external LLM billing is outside this change; do not claim a distributed transaction with the LLM service.

## 7 Checkpoint compatibility

Preserve the existing 120-second chunk behavior and transcript timing offsets. Separate transcription resume from final alignment/diarization: final stages may repeat after a restart if their intermediate state was not persisted. Do not claim all work resumes at substage precision.

Legacy checkpoints contain completed seconds and sometimes an absolute `segments_file` under `/assets`, with accumulated segments stored in an audio-adjacent sidecar. Support reading both legacy and new envelopes. At first HTTP submission, validate the old path is under the configured asset root, map it to the read-only worker media root, copy and validate its data into the worker's state, and leave the old copy intact through the rollback window. Never accept arbitrary absolute checkpoint paths from API requests.

Legacy checkpoints do not carry all the new fingerprints. Import them only when the baseline source/options/provenance can be established from the rollout inventory and data. When that evidence is insufficient, preserve the old files and explicitly restart from zero. Draining local jobs before initial cutover is the preferred way to minimize ambiguous imports.

If a checkpoint is corrupt, missing its segment data, inconsistent with the source, or incompatible with the pipeline, do not skip the recorded seconds. Restart that attempt from the beginning with an explicit reason. Do not silently omit the beginning of a transcript.

Change the worker execution path so the Python script cannot delete the last checkpoint before the supervisor durably records the result. The current script and Go runner both have completion cleanup; move responsibility to the supervisor for HTTP jobs, with characterization tests preserving local mode during transition.

Include input hash, option hash, pipeline version and checkpoint format in new checkpoint metadata. Resume across engine upgrades is disallowed unless compatibility is declared and tested. Normal upgrade procedure drains jobs first. An emergency upgrade with incompatible checkpoints preserves old state and starts explicitly from zero or finishes jobs on the old image; it must not mix partially transcribed versions invisibly.

## 8 Errors and cancellation

Define structured codes and typed Go errors, including `WORKER_UNAVAILABLE`, `BUSY`, `MODEL_UNAVAILABLE`, `MODEL_ACCESS_DENIED`, `INVALID_CONFIG`, `INPUT_MISSING`, `INPUT_CHANGED`, `INPUT_UNREADABLE`, `STATE_UNAVAILABLE`, `CHECKPOINT_INCOMPATIBLE`, `TIMEOUT`, `CANCELLED`, and `INTERNAL`. Include a sanitized message and `retryable` flag. Do not send raw Python stderr or tokens through public responses.

Network, protocol, worker-path, missing-model, OOM and permission errors must never invoke `resetItemForRedownload` or delete audio. A worker reporting `INPUT_MISSING` can indicate a bad mount while the app still sees a valid file. Only independently confirmed app-local media failure may retain the existing redownload behavior. Replace the current remote-path use of string matching for `No such file or directory` and `Failed to load audio` with typed classification.

Transport reconnects do not increment transcription retry count. Confirmed retryable terminal failures retain existing retry delay/backoff policy. Nonretryable failures remain visible and require a configuration correction or the existing explicit repair action. The scheduler must skip them without starving later eligible rows when max-items limits apply; add paginated eligibility tests rather than relying on a nil retry timestamp. Manual repair must deliberately create/rearm an attempt and preserve compatible checkpoints.

Use context cancellation throughout the adapter and supervisor. App shutdown stops new dispatch and releases local waiting resources but does not cancel already accepted remote work. Worker SIGTERM stops acceptance, checkpoints/quiesces children and reaps their process groups. A default 180-second worker stop grace accommodates a chunk boundary; enforce the deadline and test forced termination with durable previous chunks. `drain` is the separate operation that waits for whole jobs before planned upgrades.

When an application job-lock heartbeat fails or ownership is lost, stop new submissions and prevent stale persistence. Worker submission idempotency and app generation checks provide additional protection. Keep one Briefcast app instance per database for this release; do not use the existing blanket `UnlockAllJobs` startup behavior as a multi-instance coordination mechanism.

## 9 Filesystem and network boundaries

| Mount | App access | Worker access | Persistence |
| --- | --- | --- | --- |
| Existing assets root | `/assets`, read/write | `/media`, read-only | Required |
| Existing config | `/config`, read/write | None | Required |
| Worker execution state | None | `/state`, read/write | Required; include in future migration |
| Model caches | No new requirement | `/cache`, read/write | Persistent and reusable |
| Service token file | Read-only | Read-only | Secret; copied securely |

Map app paths to slash-separated paths relative to its configured asset root. Reject traversal, absolute/drive/UNC paths, unsafe symlink escapes, NULs, special files and attempts to read outside the root. Validate existing content using a streaming SHA-256 and size; worker verifies the source before execution and before accepting completion. Do not load an entire podcast into Go memory for hashing. Restrict mount writers to trusted app/host processes, and test symlink escape defenses. Any stronger adversarial shared-filesystem threat needs a separate file-descriptor/snapshot design.

The worker only processes mounted files in v1; it does not fetch arbitrary URLs. The client connects only to the operator-configured endpoint, rejects redirects/embedded credentials, and does not accept a backend URL from an episode or user request. Permit the configured private Compose endpoint through a purpose-specific transport. Do not enable unrestricted private outbound requests for feed fetching to make this work.

Publish no worker host port by default. Use Compose DNS `http://whisperx:8090`, with token authentication. Permit necessary model-download egress; marking the only network `internal: true` would prevent cold-start downloads unless a separate egress path or prewarmed offline mode is supplied. Bind the token through a file, not image build args. External-host TLS and uploads are deferred.

Run the worker as a configured non-root UID/GID, with a read-only image filesystem and writable `/state`, `/cache`, and bounded temporary storage. Test actual ownership on Windows Docker Desktop and Linux. Existing PUID/PGID values are not proof a process runs as that UID. Keep application privilege changes outside this split unless necessary for shared mounts. No Docker socket, privileged container, whole-config mount, or production DB secret is allowed in the worker.

## 10 Packaging and upgrade policy

Maintain independent app and worker image versions. Example worker tag structure: `ghcr.io/ctaylor1/briefcast-whisperx:worker-1.0.0-wx3.8.6-cu128` and a separate CPU variant. These are proposed tags, not existing images. Deployment pins an immutable digest with the human-readable tag recorded in its manifest.

First reproduce the installed dependency combination. Capture all installed packages and build-source details privately, then create reviewed CPU and CUDA runtime lockfiles including transitive versions, package indexes and hashes where tooling supports them. Validate wheel availability; do not blindly equate `pip freeze` with a portable reproducible lock. Pin base-image digests, record FFmpeg/system runtime versions, and emit an SBOM and image source/revision labels. Changes to these new runtime lockfiles are in scope; unrelated frontend and root tooling locks are not.

The worker's SQLite dependency may require CGO. Build its Go binary against a runtime-compatible libc and verify it inside both final images, including SQLite reads/writes and Python child execution. Do not assume that a successful cross-build or a copied binary proves runtime compatibility.

The worker lock is independent of root Python 3.14 tooling. Preserve the exact baseline model identifier/revision where available; image upgrades do not automatically change model weights. The compatibility manifest records API major, worker/pipeline version, WhisperX/Torch/CTranslate2/Python versions, model revisions, CPU/GPU variant, tested hardware, checkpoint format, source SHA and image digest.

Support updates by changing the worker dependency lock/profile and building a new worker image. No runtime `pip install -U`, unpinned Git main installation, or automatic production pull of `latest`. A separate candidate CI job may check new stable upstream releases; it must not deploy them. New stable versions can require coordinated Torch/CUDA/CTranslate2 changes, and an adapter change may be necessary if upstream Python APIs change. Promise a stable app contract, not compatibility with every future upstream release without testing.

On each upgrade: build candidate; run contract/recovery/real-audio comparisons; drain old worker; snapshot worker state; replace only the worker with a pinned candidate digest; verify capabilities; resume; retain the old digest and snapshot. Worker local-state migrations must be versioned and checked for downgrade compatibility. An incompatible state migration requires a verified pre-upgrade snapshot and explicit rollback path.

Persist Hugging Face, Torch/VAD, NLTK and other required caches at documented paths. The present NLTK cache lives in the container layer and must be accounted for. Never bake Hugging Face tokens into layers. Cache volumes are not shared writable between simultaneous candidate/production engines during testing; clone or isolate them. Cold and warm starts are both tested. Readiness does not repeatedly load a GPU model, and liveness never waits for downloads.

## 11 Compose and configuration

Use explicit files rather than an automatically loaded GPU override:

- `docker-compose.yml`: lean app and existing optional database service.
- `docker-compose.whisperx.yml`: worker, mounts, internal endpoint/token wiring, CPU defaults.
- `docker-compose.whisperx.gpu.yml`: CUDA worker image/runtime override and GPU reservation only for the worker.
- A clearly named legacy deployment file or preserved baseline package for transition/rollback.

Retire the tracked implicit GPU override from normal deployment and update every caller: `justfile`, `RELEASE.ps1`, `release.config.json`, README and CI. Keep the legacy file in the recovery package. Release helpers must carry the same ordered list of files and use argument arrays, not unsafe command-string concatenation. Keep image references separate from optional development build overrides so an ordinary update does not unexpectedly build source.

Proposed configuration:

| Variable | Purpose |
| --- | --- |
| `WHISPERX_ENABLED` | Existing enable flag; unchanged meaning |
| `WHISPERX_BACKEND` | `local` during transition, `http` in lean deployment; no silent fallback |
| `WHISPERX_SERVICE_URL` | Worker endpoint, normally `http://whisperx:8090` |
| `WHISPERX_SERVICE_TOKEN_FILE` | Mounted shared secret; fail closed when required and missing |
| `WHISPERX_IMAGE` | Immutable worker image reference |
| `WHISPERX_RUNTIME` | Operator selection `disabled`, `cpu` or `cuda` used by command wrappers |
| `HOST_WHISPERX_STATE_DIR` | Persistent worker journal and checkpoints |
| `HOST_WHISPERX_CACHE_DIR` | Persistent runtime/model caches |
| `HOST_WHISPERX_TOKEN_FILE` | Host location of the token file; outside tracked source |
| Existing model/ASR/VAD settings | Preserved and explicitly mapped to the request schema |
| Existing lock/retry/check-frequency settings | App-owned orchestration |
| Worker device/compute/batch/concurrency limits | Worker-owned deployment configuration |

Document every new variable, defaults, validation, secret status and owning service. Remove hardcoded diarization/concurrency overrides; explicitly retain current production diarization false and concurrency one at cutover. Do not unintentionally activate `.env.example` diarization true. Make the advanced env file honestly optional through supported tooling or require creating an empty file and document it; select and test a minimum Compose version rather than claiming all older NAS parsers work.

The app must start and serve the existing library even if the worker is unavailable, slow to warm, or disabled. Report transcription unavailability separately. Do not make app startup depend on worker readiness. Once the worker recovers, queued work resumes without restarting the app.

## 12 Observability and compatibility evidence

Use existing structured logging conventions with episode/job/generation/attempt IDs, status transitions, queue wait, execution duration, resume seconds, engine profile, retry reason and worker version. Redact bearer tokens, credentials, transcript text and sensitive filenames. Store bounded stderr separately in private diagnostic state with retention; public errors remain sanitized.

Expose queue/running/unacknowledged-result counts, storage capacity failures, restart recoveries, timeout/cancel counts and dependency/runtime information through authenticated status/capabilities or structured logs. A new Prometheus/Grafana stack is unnecessary. Missing worker, repeated OOM, missing mount, blocked model access, incompatible contract and exhausted disk must have distinguishable diagnostics.

The release evidence must prove:

1. App image no longer includes WhisperX/Torch/CUDA, but feedparser/mutagen functions still work.
2. Known real audio produces equivalent normalized output under the baseline engine and options, with timing/content tolerances stated before evaluation.
3. App and worker can restart separately during transcription without silent loss, duplicate active inference, or lost durable success.
4. A second compatible worker image can replace the first while the app image ID remains unchanged, followed by a successful rollback.
5. All data needed for future relocation has a documented mount, ownership expectation and backup procedure.

Detailed implementation stages, tests, rollout and recovery commands are in [the execution plan](EXECUTION_PLAN.md).
