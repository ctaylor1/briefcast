# Stage C: transcription backend extraction

Prepared 2026-10-02 on `feat/whisperx-service`, starting from `c886feffafe2a6cc603ea390f13bea3551fa55ca`. The implementation is a local execution refactor. The application still selects the local backend; backend configuration and the HTTP adapter belong to Stage E.

## Boundaries

| File | Responsibility |
| --- | --- |
| `service/transcription_backend.go` | One internal interface for backend preflight and execution, with caller context and serialized progress callbacks. No global backend registration or mutable test hook. |
| `service/transcription_local.go` | Existing Python/script resolution, syntax preflight, subprocess settings, timeouts, progress polling, legacy resume handling and sidecar cleanup. Existing exported runner functions remain compatibility wrappers. |
| `internal/transcription/pipeline.go` | Existing script configuration, progress and legacy checkpoint JSON shapes, extracted without field/tag changes. Imports only `encoding/json`; no application service, database or ORM dependency. |
| `service/whisperx.go` | Configuration loading, application locking, queue selection, audio checks, progress persistence, retry/error bookkeeping, canonical transcript persistence, exports and summaries. |

The scheduler delegates preflight to its backend. A backend supplied to the internal orchestration function can execute with nonexistent local Python/script paths. The application checks its own downloaded audio before dispatch as before. The Python script remains the sole inference pipeline and is unchanged.

These shared types describe the existing Python process protocol. `ScriptConfig` includes deployment-owned fields and is not the HTTP request options schema. `LegacyCheckpoint.SegmentsFile` is a local path and must not become a worker API media reference. The OpenAPI v1 job/status/result/error implementation and validation remain Stage D work; this extraction does not change the frozen HTTP contract.

## Behavior and risk

Local remains the default and only configured implementation in this phase. Public application APIs, configuration keys/defaults, dependencies, schema, deployment files and checkpoint serialization are unchanged. Exported local runners still start with a background context and retain their existing timeout behavior. Internal adapter methods additionally accept caller cancellation. Preflight failures now use the common log event `transcription backend preflight failed`, preserving the underlying error; log consumers matching the former resolution/preflight event text need to account for this name.

The data integrity risks listed in [Stage B](STAGE_B.md) are preserved deliberately: full-record writes, downstream work after failed transcript persistence, early successful sidecar cleanup, string-based media error classification and syntactic-only result acceptance. Stages D/E must fix these at their planned boundaries. Historical line references in that report refer to the pre-extraction source. Existing characterization tests are unchanged and protect those baseline behaviors during this refactor.

Rollback is a revert of the Stage C extraction commit. There is no database/configuration migration or deployed worker state to reverse.

## Verification

Tests use temporary application state and synthetic audio, Python child stubs and local HTTP stubs. `BRIEFCAST_INTEGRATION_DB_URL` and `BRIEFCAST_WHISPERX_REAL` were removed from Go test process environments. No production database, production media, model downloads or deployment were used.

New tests cover execution and progress persistence without a local runtime, retained retry/checkpoint state after backend failure, preflight failure leaving episode columns untouched and releasing the scheduler lock, disabled transcription skipping preflight, and cancelled local preflight/execution retaining a sidecar. The pre-implementation run of these tests failed as expected because the shared package and seam did not exist yet.

| Command | Result |
| --- | --- |
| `go test ./service -run 'Test(LoadWhisperX\|ResolveWhisperX\|RunWhisperX\|WhisperX\|ApplyWhisperX\|ReadWhisperX\|ScheduleTranscript\|StartJobLock\|CollectWhisperX\|Characterize_WhisperX\|Characterize_Transcript\|IntegrationFeedDownloadWhisperX)' -count=1` | Passed before production edits (20.617 seconds). |
| `go test ./service ./internal/transcription -run 'Test(TranscriptionBackend\|LocalTranscriptionBackend\|LoadWhisperX\|ResolveWhisperX\|RunWhisperX\|WhisperX\|ApplyWhisperX\|ReadWhisperX\|ScheduleTranscript\|StartJobLock\|CollectWhisperX\|Characterize_(Transcript\|WhisperX)\|IntegrationFeedDownloadWhisperX\|BuildCanonicalTranscript\|Export)' -count=1` | Passed (service: 24.762 seconds). |
| `go vet ./service ./internal/transcription` | Passed. |
| `go test ./service -run '^Test(TranscriptionBackend\|LocalTranscriptionBackend)' -count=1` | Passed after final test refinements (3.576 seconds). |
| `go test ./...` | Passed (service: 97.153 seconds; other packages passed or had no tests). |
| `uv run --no-sync pytest` | Passed: 15 tests, including the actual Python script control flow tests. |
| `uv run --no-sync ruff check .` | Passed. |
| `uv run --no-sync ruff format --check .` | Passed. |
| `uv run --no-sync mypy src` | Passed. |
| `go list -f '{{.ImportPath}}: {{join .Imports ", "}}' ./internal/transcription` | Confirmed only `encoding/json` is imported. |
| `git diff --cached --check` | Passed. |

A mechanical comparison against the starting source confirmed the local runner body differs only in context plumbing and the moved progress/resume/path helpers are unchanged. Go formatting used `gofmt -w service/whisperx.go service/transcription_backend.go service/transcription_local.go service/transcription_backend_test.go internal/transcription/pipeline.go`. Existing characterization tests, Python script and dependency manifests have no diff. The commit hooks run the repository's Python formatting, lint, type and test gates; `UV_NO_SYNC=1` prevents dependency installation during them. The branch remains local and has not been deployed.

The host has `CGO_ENABLED=0` and uses the existing pure Go SQLite test fallback. The opt-in real WhisperX test is skipped. On Linux with CGO, run `go test -race ./service -run 'Test(TranscriptionBackend|LocalTranscriptionBackend|Characterize_(Transcript|WhisperX))' -count=1`. Real CPU/CUDA, worker restart/recovery, container replacement and rollback acceptance remain later gates. Frontend files are untouched and their tests were not repeated.
