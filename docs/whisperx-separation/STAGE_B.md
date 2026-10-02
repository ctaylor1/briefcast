# Stage B: local transcription characterization

Prepared 2026-10-02 on `feat/whisperx-service`, starting from `ba76258`. This phase adds tests and this report. It does not change production execution, dependencies, application APIs, database schema or deployment configuration.

## Coverage

| Boundary | Added evidence |
| --- | --- |
| Go runner configuration | Every script inference option is captured from the child environment, including false booleans and nondefault values. Valid legacy resume JSON is forwarded; malformed JSON is omitted. Logging is forced to stderr and cache/telemetry/token wiring is checked with synthetic values. |
| Local subprocess outcomes | Exit failure, malformed output, timeout and launch failure retain checkpoint data. Successful silence output removes the sidecar. A final progress callback is delivered even for a short-lived failed child. |
| Actual Python pipeline control flow | A synthetic 300-second job fails during the second chunk, retains the first 120 seconds, then resumes only the remaining chunks. Segment and word offsets remain correct, earlier segments are not duplicated, and missing word timings remain missing. |
| Optional pipeline stages | Pyannote VAD failure retries with Silero. Alignment failure, missing diarization token and diarization exception preserve base segments and explicit fallback metadata. Successful alignment/diarization preserve pre-alignment text and add only supplied speaker/timing data. |
| Legacy checkpoint readers | Inline and sidecar segment data, inline precedence, filtering nonsegment values, malformed JSON and nonobject payloads are characterized. Missing sidecar behavior is recorded as a defect below. |
| Application orchestration | Failure persists progress/checkpoint and delayed retry while retaining valid audio. A due retry resumes the checkpoint, clears retry state on success, preserves raw metadata, commits canonical pre-alignment text, writes text/Markdown exports, then calls the LLM stub. Completed transcripts are not inferred again. |
| Downstream failure | Export directory failure and LLM failure leave a committed transcript available. A subsequent transcription scheduler run does not repeat inference or summarization. A controlled DB write failure demonstrates the unsafe legacy continuation described below. |

The new files are `service/whisperx_characterization_test.go` and `tests/test_whisperx_pipeline_characterization.py`. Existing tests continue to cover queue eligibility/order, canonical speaker merging, retry backoff, locks, repairs, export layout and basic state transitions. The new tests use temporary SQLite state, synthetic files and a local `httptest` server. Python tests import the real script and replace model/audio dependencies; they require no NumPy, Torch or WhisperX installation. They test pipeline control flow, not model quality or decoder correctness.

## Known defects, not requirements for the HTTP backend

Tests explicitly labeled as characterization of unsafe behavior are evidence of the local baseline. When a later authorized stage changes that behavior, replace those expectations with prevention assertions. Do not reproduce these defects in the new worker.

| Priority / confidence | Location and observation | Why it matters; required action and verification |
| --- | --- | --- |
| Required / Confirmed by test | `scripts/whisperx_transcribe.py:350`, `load_resume_checkpoint`: a missing sidecar still returns a supplied nonzero completed-seconds value. `main` uses that offset at line 580. | Earlier audio can be skipped without corresponding segments. Stage D must verify segment data and fingerprints before resuming; corrupt/missing data must explicitly restart at zero. Add worker corruption/missing-artifact tests. |
| Required / Confirmed by fault injection | `service/whisperx.go:473`, `TranscribePendingEpisodes`: after a transcript DB save fails, execution still reaches export at line 477 and summarization at line 486. The test rejects all available-transcript writes and observes exports plus an LLM request while the database has no transcript. | Output can be published and LLM work billed before durable application success. Stage E must stop downstream work and retain the worker receipt after a failed commit. Inject DB failure and test retrieval/retry of the same result without new inference. |
| Required / Confirmed by test and source | `service/whisperx.go:750`, `RunWhisperXWithProgress`, and `scripts/whisperx_transcribe.py:787`, `main`: successful child completion removes the last sidecar before application persistence. | A later DB failure loses resumable local work. Stage D must make HTTP checkpoint cleanup the supervisor's responsibility after durable result storage; retain current local behavior during Stage C. Test crashes at result/cleanup boundaries. |
| Required / Confirmed by test | `service/whisperx.go:422`, `TranscribePendingEpisodes`: stderr containing `No such file or directory: model.cache` causes deletion of a valid 2048-byte audio fixture and resets the episode for download. | A model or worker filesystem problem can delete valid media. Stage E must use typed errors and independently verify app-local media failure before redownload. Test missing mount/model/network failures with unchanged audio hashes. |
| Required / Confirmed by static inspection | `service/whisperx.go:742`, `RunWhisperXWithProgress`: output acceptance checks only `json.Valid`, then removes the sidecar. | A syntactically valid error object is not a transcript. Both worker and HTTP client must validate provider/profile, segments, words, finite timing and bounds. Add malformed-result and JSON-error-object cases in the contract suite. |
| Required / Confirmed by static inspection | `scripts/whisperx_transcribe.py:222`, `persist_segments_checkpoint`: a write error is logged and swallowed; the pipeline can report later progress even when segment persistence failed. | A progress offset may exceed durable resume data. Stage D must fail safely with the last verified checkpoint on persistence failure. Inject disk-full/unwritable-state failures and check no false durable success. |
| Required / Likely; concurrency reproduction remains | `db/dbfunctions.go:552`, `UpdatePodcastItem`, saves the whole episode. Transcription uses an episode object loaded before a potentially long child run. | Concurrent bookmark/play edits can be overwritten. Stage E must conditionally update only transcript-owned columns after generation/input validation. Add concurrent edit and stale-generation tests there. |

## Verification and limits

The focused suite and added tests are run with `BRIEFCAST_INTEGRATION_DB_URL` and `BRIEFCAST_WHISPERX_REAL` removed from the test process environment. No live database or model workload is used. The host uses the existing pure Go SQLite test fallback because CGO is disabled; this does not verify the deployed SQLite/CGO binary.

| Command | Result |
| --- | --- |
| `go test ./service -run 'Test(LoadWhisperX\|ResolveWhisperX\|RunWhisperX\|WhisperX\|ApplyWhisperX\|ReadWhisperX\|ScheduleTranscript\|StartJobLock\|CollectWhisperX\|Characterize_Transcript\|IntegrationFeedDownloadWhisperX)' -count=1` | Existing focused baseline passed before edits. |
| `go test ./service -run 'Test(LoadWhisperX\|ResolveWhisperX\|RunWhisperX\|WhisperX\|ApplyWhisperX\|ReadWhisperX\|ScheduleTranscript\|StartJobLock\|CollectWhisperX\|Characterize_(Transcript\|WhisperX)\|IntegrationFeedDownloadWhisperX\|BuildCanonicalTranscript\|Export)' -count=1` | Passed with all added Go tests and relevant existing tests. |
| `go test ./...` | Passed; unchanged packages used cached results, service package ran successfully in 89.785 seconds. |
| `uv run --no-sync pytest` | Passed: 15 tests, including 11 new parameterized Python cases. |
| `uv run --no-sync ruff check .` | Passed. |
| `uv run --no-sync ruff format --check .` | Passed. |
| `uv run --no-sync mypy src` | Passed. |
| `git diff --cached --check` | Passed. |

The expanded focused Go suite initially caught a Windows CRLF comparison in the new invocation-marker assertion. The assertion now compares trimmed content; the complete focused suite was repeated and passed. No production code was changed to make tests pass. Go output was captured to temporary text logs and filtered for failures/package results; Python tooling used the existing environment with `--no-sync`, so no dependency installation or lock update was needed.

The opt-in real WhisperX test was deliberately skipped. Race tests were not run on this CGO-disabled Windows host; a supported Linux/CGO environment should run `go test -race ./service -run 'TestCharacterize_(Transcript|WhisperX)' -count=1`. Real CPU/CUDA speech, worker crash recovery, container replacement, backup restore and production acceptance remain later gates from the execution plan. Frontend code is untouched; its tests are not repeated for this phase.
