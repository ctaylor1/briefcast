# Stage G acceptance follow-up — app crash boundaries

Prepared 2026-10-03. The execution plan defines stages A–H, with no Stage I. This follow-up implements two remaining app-crash checks from Stage G and fixes the recovery defect they exposed. It does not perform production cutover or clear the failed strict CUDA quality comparison.

## Implementation

- `whisperx_acceptance.py --app-crash-boundaries` tests app SIGKILL during inference, after worker success before result delivery, and after the database commit before ACK forwarding. It implies `--app-restarts`, requires a warm cache, validates the exact episode's export contents, retains source hashes/results and proves app image/container identity survives recovery.
- A Python standard-library proxy runs only in the isolated Compose project. It forwards to the fixed private worker endpoint, has no published port or database/token/config mount, and never logs authorization headers or bodies. Its only mount is the new fixture gate directory. Result gates hold a successful response; ACK gates stop before forwarding. The harness kills the app and stops the blocked proxy before reopening its gate, preventing an orphan request from acknowledging the killed app's result. Unreleased gates fail closed.
- The fixture's transcription schedule is one minute. Manual repair starts unfinished transcript work; scheduled transcription also reconciles receipts attached to already available transcripts. Production frequency is unchanged.
- Linux cache-copy tooling copies bytes and rebases internal links into an empty new cache. It rejects external/dangling links and special files before copying, propagates unreadable-directory failures and avoids changing bind-mount root attributes. Cache copying and Linux ownership provisioning run as root only against the new fixture destination; the cache source and helper script are read-only.
- Worker-runtime Python tests cover the proxy's real HTTP ordering and cache links/bounds. The bounded CPU release workflow now requests all three app crash checks. No workflow was dispatched and no image was published.

## Behavior change and regression evidence

`TestRemoteRestartAtResultAndReceiptBoundariesFinishesExports` failed before the production-code fix: restart at the ACK boundary cleared the receipt without producing the transcript export. Cancellation at the HTTP boundary models a process interruption; the durable worker result and app columns survive the next invocation.

Receipt reconciliation now validates the saved digest, transcript, identity, generation and lease before downstream work. It exports the committed transcript and attempts a missing summary before ACK/receipt clearing. Existing summaries are retained. Tests check exports, a stub summary provider, absence of repeat inference/submission, completed-summary preservation and invalid-receipt rejection. There is no database migration, public/private API version change or dependency change.

Export/summary failures retain existing best-effort behavior and structured logs. This change is not a general durable downstream outbox or a guarantee of exactly-once provider calls. A crash after normal receipt clearing remains outside these two added boundaries; the separate summary/backfill/export repair paths still matter.

## Validation

Focused Go regression tests passed after first demonstrating the export failure. The following commands were run:

```powershell
$env:CGO_ENABLED = '0'
go test ./internal/transcription/... ./internal/whisperxworker/... ./service/... -count=1
go test -json ./service/... -count=1 > "$FollowupRoot/go-host-service.jsonl"
$env:UV_NO_SYNC = '1'
uv run pytest
pwsh -NoProfile -File scripts/TestWorkerPython.ps1 -Image briefcast-whisperx-cpu:stage-f-verified
```

Integration database and real-engine Go opt-ins were unset. The first broad host run passed the two internal packages but failed the service package; its verbose output was truncated and the failing test was not retained. The captured service rerun passed **245 tests**, with no failed events. This unexplained initial failure is recorded, not presented as a clean first run. Host pytest passed **74 tests**, with platform skips for Linux flock, host symlink creation and FIFO support. The worker's shipped Python 3.12/Linux run passed **56 tests**, including all cache-link/FIFO tests. Ruff lint/format and CLI help passed.

A later host suite run under concurrent container rehearsal load failed the unchanged `test_multichunk_failure_resumes_completed_chunks_and_offsets_words`: its logged progress-file write failed and the final progress stayed `transcribing`. The focused host rerun passed, and the Linux/worker suite passed. Windows filesystem contention is a hypothesis; this follow-up does not change the frozen transcription pipeline to mask that failure.

Focused Linux/CGO race validation passed:

```powershell
docker run --rm --network none --mount "type=bind,source=$Repo,target=/repo,readonly" --mount "type=bind,source=$GoModuleCache,target=/modules,readonly" --mount "type=bind,source=$StageGRoot/go-cache,target=/go-cache" --env GOMODCACHE=/modules --env GOCACHE=/go-cache --env CGO_ENABLED=1 --workdir /repo golang:1.26 go test -race ./service -run 'TestRemote(RestartAtResultAndReceiptBoundariesFinishesExports|ReceiptReconcilesAfterLostAckResponse|CorruptReceiptIsRetainedAndDeferredWithoutAck|SummaryWritesPreserveConcurrentEdits)' -count=1
```

`$Repo`, `$GoModuleCache` and `$StageGRoot` were resolved existing paths. No dependencies were installed. Full frontend/build/race suites were not repeated for this scoped change.

## Container rehearsals and provenance

Private evidence is under the directory referenced by `%TEMP%/briefcast-acceptance-followup-directory.txt`. Configuration and raw results stay outside Git. Each final run uses a new project, SQLite/config/media/state/token/cache tree, copied self-synthesized `speech.flac`, disabled LLM and isolated worker images. No production audio or database is used.

```powershell
uv run python scripts/whisperx_acceptance.py --runtime cpu --app-image briefcast:acceptance-followup --worker-image briefcast-whisperx-cpu:stage-f-verified --replacement-image briefcast-whisperx-cpu:stage-g-replacement --cache $KnownGoodPrivateCache --audio tests/fixtures/whisperx/speech.flac --run-dir "$FollowupRoot/cpu-crash-verified" --timeout 600 --app-crash-boundaries
uv run python scripts/whisperx_acceptance.py --runtime cuda --app-image briefcast:acceptance-followup --worker-image briefcast-whisperx-cu128:stage-f-verified --replacement-image briefcast-whisperx-cu128:stage-g-replacement --cache $KnownGoodPrivateCache --audio tests/fixtures/whisperx/speech.flac --run-dir "$FollowupRoot/cuda-crash-verified" --timeout 600 --app-crash-boundaries
```

| Final run | Result |
| --- | --- |
| `cpu-crash-verified` | PASS: 14 checks; SIGKILL during inference, after success before result delivery, and after DB commit before ACK; same jobs, correct exports, bookmark/play preservation, durable results/receipts and unchanged app identity |
| `cuda-crash-verified` | PASS: the same 14 checks on the pinned CUDA worker |

Both evidence files have `complete: true` and record all three crash boundaries. They also include real inference, malformed-media preservation, authenticated API, drain/resume, full worker-state restore and compatible replacement/rollback. All fixture containers were stopped. These are recovery rehearsals without `--baseline-image`, not new representative corpus/quality attestations.

The local app candidate is `sha256:ae6c66e8f47eca6dcb80a0e0887e068a8df608345484125dde41bf38791c8042`. An attempted offline full Docker build failed because its Go module-download layer was absent. Instead, `golang:1.26` compiled the changed app offline with CGO against the existing module/cache mounts; a private Dockerfile copied that binary onto the unchanged Stage F app runtime and regenerated runtime metadata. Its revision explicitly identifies the `11705f5` acceptance working tree, rather than pretending this is a commit-pinned release build. Worker/replacement IDs are unchanged Stage F/G images; the runs record their resolved immutable IDs. A production release still requires fresh commit-pinned archives and a regenerated cutover package.

Earlier failed/incomplete rehearsal artifacts remain private: Windows traversal omitted model snapshot links; a first Linux copy rejected bind-root attribute changes; a Python 3.12 syntax check caught a formatter target mismatch; early crash runs were stopped by inspected project labels while waiting for the inherited 30-minute scheduler. Only owned fixture processes/containers were stopped, and no volumes or images were pruned. Final runs use the corrected scripts from startup.

Two subsequent starts hit Docker's exhausted default subnet pool. Three empty, label-verified networks from this task's stopped fixture projects were removed after confirming zero endpoints. No production network or data was changed. Failed/incomplete evidence is not marked complete or reused as a passed final run.

## Remaining gates and rollback

Stage G's strict CUDA confidence-score comparison remains failed with its original assertions and evidence. Coherent production database/config/media recovery, positive gated diarization, real-container OOM/disk exhaustion, an upstream/state-schema upgrade, production summary/playback canaries and the production observation window remain outstanding as recorded in [Stage G](STAGE_G.md). Completing these crash rehearsals does not make the cutover ready.

Production remains on the archived monolith. Revert this cohesive source change if needed and retain its private evidence; there is no schema/state migration to undo. Keep existing recovery bundles and archives. Do not reuse the prior cutover package as proof for this changed app source.

Final read-only production inspection retained container `12a316ad6ad8c1b146832bda4581e8a90030e6e0fc938346e2abc46bef33d7ea`, image `sha256:a7542b94500a5ac7e00271451ef2a51db233bd5e2bec92c615b240302b06d052` and start time `2026-10-02T20:40:04.862330437Z`. Only `briefcast` remained running. The follow-up commit and verified private source bundle identify source recovery, not a deployed split or accepted release.
