# ADR: Briefcast-owned WhisperX worker

Date: 2026-10-02
Status: accepted for implementation; no deployment decision

## Context and decision

Briefcast currently launches `scripts/whisperx_transcribe.py` from the application process. The same app image carries Torch, CUDA and model dependencies. The script's 120-second chunks, sidecar checkpoints, alignment and diarization fallbacks, and result fields are part of the behavior to preserve. The application owns episode selection, retry policy, canonical text, exports and summary scheduling.

Build a separately versioned Briefcast worker with an authenticated asynchronous HTTP v1 contract. Its supervisor will use Go `net/http` and the repository's existing SQLite dependency, and will continue to execute the existing Python pipeline. The worker owns accepted attempts, a bounded execution journal, child processes, checkpoint and result storage, and cancellation. The app retains its business queue and database. Keep the existing local runner while introducing and characterizing the seam. The initial worker uses the running 3.8.6 engine profile; a dependency upgrade is a later change.

The reviewed wrappers are useful packaging or API references, but none establishes compatibility with Briefcast's durable chunk resume and result contract. The latest reported releases at review time were unchanged from the handoff. This is a decision about this application's requirements, not a general judgment of the wrappers.

| Candidate | Reviewed immutable source | Release checked 2026-10-02 | Decision evidence |
| --- | --- | --- | --- |
| ahmetoner/whisper-asr-webservice | [routes](https://github.com/ahmetoner/whisper-asr-webservice/blob/ec32cd50b508412e0b8ebe81c26cd7a7dc900b7a/app/webservice.py), [endpoints](https://github.com/ahmetoner/whisper-asr-webservice/blob/ec32cd50b508412e0b8ebe81c26cd7a7dc900b7a/docs/endpoints.md) | [v1.10.0](https://github.com/ahmetoner/whisper-asr-webservice/releases/tag/v1.10.0), published 2026-08-09 | `/asr` is a synchronous transcription request. An asynchronous durable Briefcast job and chunk checkpoint protocol is not documented. Its README reports WhisperX 3.4.5, below the installed 3.8.6. |
| jim60105/docker-whisperX | [Dockerfile](https://github.com/jim60105/docker-whisperX/blob/c56400f3c70a4bd3b1c3b0b0cadf403473bcb93f/Dockerfile), [README](https://github.com/jim60105/docker-whisperX/blob/c56400f3c70a4bd3b1c3b0b0cadf403473bcb93f/README.md) | GitHub latest-release API returned 404; repository pushed 2026-09-27 | CLI packaging and cache conventions, with no HTTP execution journal. A possible future base image, subject to compatibility checks. |
| murtaza-nasir/whisperx-asr-service | [README](https://github.com/murtaza-nasir/whisperx-asr-service/blob/4d6acc6f6f608adb3070008beb0926a9c92ece86/README.md), [LICENSE](https://github.com/murtaza-nasir/whisperx-asr-service/blob/4d6acc6f6f608adb3070008beb0926a9c92ece86/LICENSE) | [v0.4.2](https://github.com/murtaza-nasir/whisperx-asr-service/releases/tag/v0.4.2), published 2026-09-30 | Alpha API with CPU/CUDA variants. No verified durable Briefcast chunk-checkpoint contract; its default image uses a different Torch/CUDA profile. |
| Nyralei/whisperx-api-server | [status tracker](https://github.com/Nyralei/whisperx-api-server/blob/5203d417625b1f12824162bd4769b6c7eb268648/src/whisperx_api_server/request_status.py), [README](https://github.com/Nyralei/whisperx-api-server/blob/5203d417625b1f12824162bd4769b6c7eb268648/README.md) | [v1.4.0.1](https://github.com/Nyralei/whisperx-api-server/releases/tag/v1.4.0.1), published 2026-09-24 | More lifecycle features, but the inspected simple status path is process local; distributed durability adds infrastructure outside this scope. GPL-3.0 source cannot be copied into this MIT repository without a licensing decision. |

Release dates and SHAs above came from GitHub's repository, default-branch commit, and latest-release APIs on 2026-10-02. They pin the reviewed source, not necessarily the release tag's tree. The candidate evidence is a bounded review; no candidate image or upgrade was run.

## Consequences and compatibility

- The private API contract lives in `docs/whisperx-separation/openapi.yaml`; wire major 1 is independent of image and WhisperX versions. Stage A fixtures are illustrative contract cases, not observed transcription output.
- The worker adds a local SQLite journal and persistent `/state` and `/cache` mounts. It receives only relative media paths under a read-only `/media` mount, a secret-file bearer token and its own model credentials. It never receives the app database, LLM credentials or Docker socket.
- The app must persist request identity before PUT, commit transcript-owned fields conditionally before ACK, and reconnect to accepted jobs after failures. Local mode remains during transition. Public app APIs and the production app schema stay unchanged.
- A worker upgrade requires contract and checkpoint compatibility tests, drain, state snapshot, targeted replacement and rollback. An incompatible profile cannot silently resume old chunks.
- Stage A does not alter runtime behavior. Stage B must characterize current local output, progress, resume and downstream behavior before the seam is extracted.

## Alternatives revisited

A third-party synchronous endpoint plus an app queue cannot recover that engine's internal 120-second chunk checkpoints or guarantee the same alignment and speaker behavior. Splitting the audio upstream changes that pipeline. Kafka/S3 or a general worker framework adds operational requirements outside the same-host, one-worker target. These alternatives can be reconsidered only with the acceptance suite from `EXECUTION_PLAN.md` and a separate licensing and dependency review.
