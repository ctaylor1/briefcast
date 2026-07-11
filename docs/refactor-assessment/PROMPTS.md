# Briefcast — Copy/Paste Execution Prompts

- **Date:** 2026-07-05 · **Commit basis:** `main` @ `6533936` (v1.9.3)
- Each prompt is self-contained: the implementing agent needs no other context. Line numbers reference commit `6533936`; re-locate by symbol if drifted.
- All prompts assume: repo root is the working directory; `go test ./...` is green before starting (run it and stop if not — report instead of proceeding).

**Implementation status (2026-07-10 / v1.9.4):** Prompts 1–4 are complete. Prompt 5 is the next implementation slice. The completed prompts remain as historical work orders and verification references.

---

## Prompt 1 — PR 1: Unify background-job locking on atomic `TryLock` (P2) — completed

**Why first:** smallest safe change with the largest immediate risk cut. Three background jobs use a check-then-act lock idiom that lets cron and API-triggered runs execute concurrently, double-fetching feeds and interleaving writes. A correct, tested, atomic lock (`db.TryLock`) already exists and is used by the transcription job — this PR copies a proven in-repo pattern, requires no schema or API change, and unblocks later refactors by making job execution single-flight.

```text
GOAL
In the Go repo "briefcast", migrate three background jobs from the racy
GetLock/IsLocked/Lock idiom to the existing atomic db.TryLock, so each job is
single-flight even when cron and API-triggered runs collide.

CONTEXT
- Go 1.26, Gin + GORM app. Background jobs are scheduled in main.go and also
  triggered from HTTP handlers via `go service.RefreshEpisodes()`.
- The racy idiom (check-then-act, both racers proceed):
    service/podcastService.go:1047-1053  (RefreshEpisodes)
    service/podcastService.go:807-813    (DownloadMissingEpisodes)
    service/retention.go:31-37           (ApplyRetentionPolicies)
  Each does:
    lock := db.GetLock(jobName)
    if lock.IsLocked() { ...skip... }
    jobLock := db.Lock(jobName, 120)   // unconditional upsert — both racers "win"
    defer db.UnlockByID(jobLock.ID)
- The correct pattern to copy is in service/whisperx.go:217-241
  (TranscribePendingEpisodes): db.TryLock(name, durationMins) returns
  (lock, acquired bool, err); on !acquired it logs "job_skipped_lock_exists"
  and returns nil. It also starts a lease heartbeat via
  startJobLockHeartbeat(lockID, durationMins, refreshSecs, onErr) —
  see service/whisperx.go:504-530.
- TryLock implementation: db/dbfunctions.go:665-727. job_locks.name has a
  unique index (db/migrations.go:97-99). Locks are cleared at startup by
  service.UnlockAllJobs (main.go calls it via intiCron).

FILES LIKELY TOUCHED
- service/podcastService.go (RefreshEpisodes, DownloadMissingEpisodes)
- service/retention.go (ApplyRetentionPolicies)
- service/*_test.go for the three jobs (new tests)
- Possibly db/dbfunctions.go ONLY if you deprecate Lock/GetLock usage comments — do
  not delete Lock/GetLock in this PR (tests elsewhere may use them).

NON-GOALS
- No changes to TryLock itself, lock schema, cron wiring, or any other job.
- No behavior change beyond eliminating duplicate concurrent runs.
- Do not refactor unrelated code you pass by.

CONSTRAINTS
- Behavior-preserving: when the lock is not acquired, each job must log and
  return nil (not an error), matching the whisperx exemplar and current skip behavior.
- Keep lease duration 120 minutes for all three (current value).
- Add the heartbeat to DownloadMissingEpisodes (large libraries can exceed the
  lease); use the whisperx defaults pattern (refresh ~60s). RefreshEpisodes and
  ApplyRetentionPolicies may omit the heartbeat, but say so in the PR description.
- Follow existing logging style: jobLogger.Infow("job_skipped_lock_exists", ...).

STEP PLAN
1. Run `go test ./...`; record baseline. Stop and report if red.
2. In RefreshEpisodes: replace GetLock/IsLocked/Lock with
   jobLock, acquired, lockErr := db.TryLock(jobName, 120); handle lockErr
   (Errorw + return err), !acquired (Infow + return nil), then
   defer db.UnlockByID(jobLock.ID). Mirror whisperx.go:217-241 exactly,
   including the nil/empty-ID guard.
3. Same for DownloadMissingEpisodes + startJobLockHeartbeat/defer stop.
4. Same for ApplyRetentionPolicies (no heartbeat needed; 24h cadence, 120m lease).
5. Grep `db.GetLock(`/`db.Lock(` under service/ — only test helpers should remain.
6. Add tests (see below). Run focused tests, then the full suite.

REQUIRED TESTS
- For each job: with a held lock (create via db.TryLock in the test), the job
  returns nil without doing work (stub/observe via existing test seams: e.g.
  RefreshEpisodes with a DB containing one podcast pointing at an httptest feed
  server that fails the test if hit — see service/integration_test.go for the
  testFeedServer pattern).
- Race test: two goroutines invoke RefreshEpisodes concurrently against a feed
  server that counts requests; assert the feed is fetched at most once per episode
  set (i.e., only one run executed).

VERIFICATION COMMANDS
- go test ./service -run "Refresh|Download|Retention" 
- go test ./db -run Lock
- go test ./...

ACCEPTANCE CRITERIA
- No call sites of the GetLock→IsLocked→Lock idiom remain in service/.
- All three jobs use TryLock with UnlockByID on the acquired lock ID.
- New contention tests pass; full suite green.
- PR description states: behavior-preserving; duplicate-run race removed;
  heartbeat decision for each job.

ROLLBACK
- Pure code change; revert the PR. No schema/data impact.

COMPLETION REPORT (required)
- Files changed; tests added (names); commands run with pass/fail counts;
- explicit confirmation of acceptance criteria; any deviations and why.
  (You may choose a better local structure — e.g. a shared acquireJobLock helper —
  if behavior and tests are preserved; document the choice.)
```

---

## Prompt 2 — PR 2: Repository hygiene — remove committed agent-bypass config, tracked build artifact, and stale API-key comment (P12 + F8) — completed

```text
GOAL
Remove three hygiene/security liabilities from the "briefcast" repo:
(a) the committed Claude Code local-settings file that grants bypassPermissions,
(b) the stray tracked build artifact `coverage` at repo root,
(c) two commented-out lines containing a legacy third-party API key.

CONTEXT
- .claude/settings.local.json is tracked in git and contains
  "defaultMode": "bypassPermissions" plus blanket Bash/Write/WebFetch allows.
  It silently overrides the deliberately restrictive tracked .claude/settings.json
  for every contributor using Claude Code. Local settings files are meant to be
  personal and untracked.
- A 159 KB Go coverage output file named `coverage` (no extension) is tracked at
  the repo root (git ls-files shows it). .gitignore already ignores *.out but not
  this bare filename.
- service/podcastService.go lines 1216-1217 (function makeQuery) contain two
  commented-out Goodreads URLs embedding an API key. The key must not appear in
  the PR description or commit message — refer to it only by location.

FILES LIKELY TOUCHED
- .claude/settings.local.json (git rm)
- .gitignore (add `.claude/settings.local.json` and `/coverage`)
- coverage (git rm)
- service/podcastService.go (delete the two comment lines; nothing else)

NON-GOALS
- No changes to .claude/settings.json (the restrictive one stays).
- No git-history rewrite. Note in the PR that the key remains in history and
  should be treated as compromised/rotated by the owner if the account exists.
- No other cleanup in podcastService.go.

CONSTRAINTS
- Mechanical PR: no behavioral code changes. makeQuery must be byte-identical
  except for the two removed comment lines.
- Never print the key value anywhere (diff will contain the deletion — that is
  unavoidable and acceptable; do not quote it elsewhere).

STEP PLAN
1. git rm --cached .claude/settings.local.json && add to .gitignore.
2. git rm --cached coverage && add `/coverage` to .gitignore.
3. Delete service/podcastService.go:1216-1217 (the two `//link :=` comment lines).
4. go build ./... (or go vet ./...) to confirm compilation.
5. Run tests.

REQUIRED TESTS
- None new (mechanical). Full suite must stay green.

VERIFICATION COMMANDS
- git ls-files | grep -E '^coverage$|settings\.local' -> no output
- grep -rn "goodreads" service/ -> no output
- go test ./...

ACCEPTANCE CRITERIA
- Both files untracked and ignored; comment lines gone; suite green;
  PR description includes a rotation recommendation without the key value.

ROLLBACK
- Revert the PR (files restore from git).

COMPLETION REPORT (required)
- Files changed; verification command outputs; acceptance-criteria checklist;
  confirmation the key value appears nowhere in PR text.
```

---

## Prompt 3 — PR 3: Download loop — close files on all error paths; keep resumable partials (P6) — completed

```text
GOAL
Fix resource handling in the episode download loop of the "briefcast" Go repo:
(1) the output file must be closed on every exit path, and
(2) incomplete downloads must keep their partial file so the existing
    range-resume logic can continue them, instead of deleting them.

CONTEXT
- File: service/fileService.go, function Download (lines 62-196 at commit 6533936).
- The file handle is opened at :112-126 (os.OpenFile append for 206 resume, or
  os.Create). The only `defer file.Close()` is at :182, AFTER the read loop, so
  the mid-loop error returns leak the handle:
    :159-161  write error   -> return "", writeErr        (no close)
    :171-177  read error    -> return "", readErr         (no close)
  (The pause/cancel paths :140-155 close explicitly and are fine.)
- :184-191 deletes the file when downloadedBytes == 0 (keep this) and ALSO when
  downloadedBytes < totalBytes (change this): resume support at :81-89 sets a
  Range header from the existing partial file's size, so deleting partials
  defeats it. Callers treat errors by requeueing the item as NotDownloaded
  (service/podcastService.go:874-882), so a kept partial is retried naturally.
- ErrDownloadCancelled/ErrDownloadPaused are defined at :26-29.

FILES LIKELY TOUCHED
- service/fileService.go (Download)
- service/file_helpers_test.go or a new service/download_test.go for tests
  (httptest-based; see existing patterns in service tests using httptest.NewServer)

NON-GOALS
- No changes to resume/416 retry logic (:98-107), progress reporting, pause/cancel
  semantics, or DownloadWithFallback.
- No retry-count/backoff policy for partials in this PR (note as follow-up).

CONSTRAINTS
- Behavior-preserving EXCEPT the labeled change: partial files are now retained
  on incomplete downloads. Say this explicitly in the PR description.
- Empty (0-byte) results are still deleted (unchanged).
- Error values/messages returned to callers must keep their current types so
  IsHTTPClientError and callers' equality checks (== ErrDownloadPaused, etc.)
  still work.

STEP PLAN
1. Restructure: immediately after the file is successfully opened (both branches),
   `defer file.Close()` once; remove the explicit closes in pause/cancel paths and
   the late defer at :182. Ensure the final success path still calls
   changeOwnership after all writes complete. Note: on pause/cancel paths, the
   response body close remains explicit or via the existing defer at :127 —
   check double-close of resp.Body (defer at :127 plus explicit closes at
   :143/:151) and normalize to the single defer.
2. In the incomplete branch (:188-191): stop removing the file; return a wrapped
   error like fmt.Errorf("download incomplete: got %d of %d bytes for %s ...")
   (same message is fine) — just without the os.Remove.
3. Add tests.

REQUIRED TESTS (httptest servers)
- Server closes connection after sending half of a Content-Length body:
  Download returns an error, the partial file EXISTS with the partial bytes,
  and a subsequent Download call sends a Range header (assert via request
  inspection) — the resume path completes the file.
- Zero-byte body: file does not exist after the call (unchanged behavior).
- Write-error path is hard to force portably — cover the read-error path above
  and rely on the shared defer for close correctness; assert no panic/double-close
  by running the pause path test (existing pattern) too.

VERIFICATION COMMANDS
- go test ./service -run "Download"
- go test ./...

ACCEPTANCE CRITERIA
- Single close path for the output file (one defer); pause/cancel/error/success
  all close exactly once; resp.Body closed exactly once.
- Incomplete downloads keep partials; new resume test passes; suite green.
- PR labels the partial-retention change as behavior-changing with rationale.

ROLLBACK
- Revert the PR. Retained partials from the interim are harmless (resume or
  overwrite handles them).

COMPLETION REPORT (required)
- Files changed; tests added; commands + results; acceptance checklist;
  note any deviation (e.g., if you also fixed the 416-retry double-close, say so).
```

---

## Prompt 4 — PR 4: Characterization tests for PodcastItem state transitions (P1a, test-only) — completed

```text
GOAL
Add a characterization (golden-master) test suite for every PodcastItem state
transition in the "briefcast" Go repo, pinning EXACTLY which columns each
transition writes, as a safety net before the persistence refactor that will
replace whole-row Save with column-scoped updates.

CONTEXT
- PodcastItem model: db/podcast.go:49-123 (~40 persisted columns including large
  text blobs and transcript/summary state machines).
- All transitions currently go through db.UpdatePodcastItem
  (db/dbfunctions.go:551-554), which is DB.Omit("Podcast").Save(&item) — a
  whole-row write. The upcoming refactor will change each transition to a scoped
  Updates() call; these tests define "the intended columns" per transition.
- Transitions to characterize (all in service/podcastService.go unless noted):
    SetPodcastItemAsQueuedForDownload      :572-583
    SetPodcastItemAsQueuedPreserveProgress :586-594
    SetPodcastItemAsDownloading            :597-605
    SetPodcastItemAsPaused                 :608-616
    SetPodcastItemAsDownloaded             :671-730 (file-size + ID3 branches)
    SetPodcastItemAsNotDownloaded          :733-746
    SetPodcastItemPlayedStatus             :749-757
    SetPodcastItemBookmarkStatus           :655-668 (NOTE: also writes
       IsSummaryFavorited — pin this coupling as-is; do not "fix" it here)
    resetItemForRedownload (service/whisperx.go:986-1001)
    scheduleTranscriptRetry (service/whisperx.go:1003-1016) + the failure-path
       field writes in TranscribePendingEpisodes :438-453 (test via the helper)
    transcript-success field set: whisperx.go:458-471 — factor NOTHING out;
       characterize by calling the smallest callable unit available; if none,
       document the column list in the test file as the spec for the refactor PR.
    SummarizeEpisode success/failure column sets (service/summarize.go:284-319)
       using a stub HTTP server for the LLM endpoint (see summarize_test.go for
       existing patterns).
- Test DB setup patterns: service tests use temp/in-memory SQLite via glebarez —
  see main_test.go:44-48 and service/test_helpers_test.go for env/reset helpers.

FILES LIKELY TOUCHED
- NEW: service/state_transitions_characterization_test.go (or split by area)
- possibly small additions to service/test_helpers_test.go (row-snapshot helper)

NON-GOALS
- Zero production-code changes. This PR is test-only. If a transition is
  untestable without refactoring, document it in a code comment in the test file
  instead of changing production code.

CONSTRAINTS
- Tests must pass against CURRENT behavior (characterization, not aspiration).
- Snapshot technique: read the full row before and after via a helper that
  reflects over gorm-visible columns (or SELECT * into map[string]any using
  DB.Raw), then assert (a) intended columns changed to expected values and
  (b) ALL OTHER columns are byte-identical. UpdatedAt may change — exclude it
  explicitly and note why.
- Keep each test independent (fresh item per test) and hermetic (no network;
  stub LLM via httptest).

STEP PLAN
1. Build the row-snapshot diff helper (map-based; stable column ordering).
2. One test per transition listed above; name tests
   TestCharacterize_<TransitionName>.
3. For SetPodcastItemAsDownloaded, cover: file exists (size set), file missing
   (size branch skipped), ID3 extraction skipped when chapters already present
   (id3meta.ShouldExtract false path) — python-dependent paths may be skipped
   with requireWorkingPython (see service/test_helpers_test.go:30-44).
4. Run the suite; ensure green WITHOUT touching production code.

VERIFICATION COMMANDS
- go test ./service -run Characterize -v
- go test ./...

ACCEPTANCE CRITERIA
- Every listed transition has a characterization test asserting both changed and
  unchanged columns; suite green; no production diffs in the PR.

ROLLBACK
- n/a (test-only).

COMPLETION REPORT (required)
- Test names added; per-transition column lists discovered (paste the table —
  it becomes the spec for the follow-up refactor PR); commands + results;
  any transitions that could not be isolated and why.
```

---

## Prompt 5 — PR 5: Column-scoped state-transition updates (P1b) — next

```text
GOAL
Replace whole-row Save persistence for PodcastItem state transitions in the
"briefcast" Go repo with column-scoped updates, eliminating lost updates from
concurrent workers, WITHOUT changing any transition's intended effect.

PREREQUISITE
The characterization suite from the previous PR
(service/state_transitions_characterization_test.go) exists and is green.
Its per-transition column tables are the authoritative spec. Do not start
without it.

CONTEXT
- Current sink: db.UpdatePodcastItem = DB.Omit("Podcast").Save(&item)
  (db/dbfunctions.go:551-554). Load-modify-save callers listed in the
  characterization tests. Long-running holders that must stop full-row saving:
  transcript completion (service/whisperx.go:458-475), transcript failure path
  (:438-453), progress callback (:397-419 -> applyWhisperXProgressUpdate),
  SummarizeEpisode (service/summarize.go:284-319).
- Existing column-scoped exemplar: UpdatePodcastItemDownloadProgress
  (db/dbfunctions.go:349-357) using Model+Where+Updates(map).
- GORM notes: Updates(map) skips hooks (only hook is UUID BeforeCreate — fine);
  zero values in maps ARE written (unlike struct Updates).

FILES LIKELY TOUCHED
- db/dbfunctions.go (new intent-named update functions)
- service/podcastService.go (setters call new functions)
- service/whisperx.go, service/summarize.go (workers write scoped columns)
- characterization tests: should need NO assertion changes (that is the point);
  only plumbing changes if helpers referenced internals.

NON-GOALS
- Do not change transition semantics (including the bookmark/IsSummaryFavorited
  coupling — pinned by tests; a separate product decision).
- Do not remove db.UpdatePodcastItem yet if creation-adjacent callers remain
  (AddPodcastItems post-insert tweaks, alternate_feeds.go:81); list survivors in
  the PR description as follow-ups.
- No schema changes.

CONSTRAINTS
- Behavior-preserving per the characterization suite: every test must pass
  unmodified. If a test must change, STOP — that is a semantics change; report it.
- One transition per commit where practical; workers (whisperx/summarize) in
  their own commits.
- New db functions take explicit params (id + values), never a *PodcastItem
  to re-save.

STEP PLAN
1. Add db-layer functions per the characterization column tables, e.g.:
   MarkItemQueued(id, resetBytes bool), MarkItemDownloading(id),
   MarkItemPaused(id), MarkItemDownloaded(id, path string, size int64,
   downloadDate time.Time, transcriptInit ...), MarkItemNotDownloaded(id, status),
   SetItemPlayed(id, bool), SetItemBookmark(id, date, favorited),
   MarkTranscriptProcessing/Progress/Complete/Failed(...), MarkSummary
   Processing/Available/Failed(...), ResetItemForRedownload(id, reason).
   (Names are yours; keep them intent-revealing and consistent.)
2. Port service setters one by one, running the characterization suite each time.
3. Port whisperx completion/failure/progress and summarize success/failure.
   The in-memory `item` in those workers becomes read-mostly; fields still used
   for export (ExportTranscript/ExportSummary read item fields) must be set on
   the local struct AND persisted via the scoped call — keep them in sync.
4. Grep remaining db.UpdatePodcastItem callers; classify keep vs follow-up.
5. Full suite + integration test.

REQUIRED TESTS
- Characterization suite green, unmodified.
- NEW concurrency regression: insert item; goroutine A runs a stubbed slow
  "transcription complete" write (add a test seam or simulate by calling the
  new MarkTranscriptComplete after a delay); mid-delay, goroutine B calls
  SetItemPlayed(id, true); after A completes, assert is_played is STILL true
  and transcript fields are set. This test must FAIL against the old code path
  (verify once by temporarily pointing it at the old setter — mention in PR).

VERIFICATION COMMANDS
- go test ./service -run "Characterize|Concurrent" -v
- go test ./service -run TestIntegrationFeedDownloadWhisperX
- go test ./...

ACCEPTANCE CRITERIA
- No state transition uses whole-row Save; characterization tests unchanged and
  green; concurrency regression test passes; survivors of UpdatePodcastItem
  enumerated in PR description with justification.

ROLLBACK
- Revert the PR (no schema change). Data written in the interim is
  shape-identical.

COMPLETION REPORT (required)
- Files changed; new db functions; per-transition mapping old->new; test results
  incl. the once-run failure demonstration; remaining UpdatePodcastItem callers;
  risks/follow-ups.
```

---

## Blank reusable template

```text
GOAL
<one sentence: repo, outcome, user-visible effect if any>

CONTEXT
- <repo/stack facts the agent cannot infer>
- <exact files:lines and current behavior, with the key snippet described>
- <in-repo exemplar pattern to copy, if any>

FILES LIKELY TOUCHED
- <paths>

NON-GOALS
- <explicitly out of scope>

CONSTRAINTS
- <behavior-preserving? label any behavior/API/schema change>
- <project conventions to follow; dependency policy: no new deps without approval>

STEP PLAN
1. Run `go test ./...`; stop and report if red.
2. <smallest safe change first>
3. <next steps, one reviewable stop point each>

REQUIRED TESTS
- <characterization first if behavior unclear; regression for the bug; edge cases>

VERIFICATION COMMANDS
- <only commands from the repo baseline: go test ./..., go test ./<pkg> -run X,
  npm --prefix frontend run test:unit, npm --prefix frontend run build, just ci-python>

ACCEPTANCE CRITERIA
- <observable, checkable statements>

ROLLBACK
- <"revert the PR" or the specific flag/migration/data steps if not sufficient>

COMPLETION REPORT (required)
- Files changed; tests added/updated with results; commands run + outcomes;
  acceptance-criteria checklist; deviations and rationale; remaining risks.
(You may choose a better local solution if behavior and tests are preserved —
document the choice.)
```

---

## Proposed AGENTS.md / CLAUDE.md additions

Short enough to adopt; append to `AGENTS.md` (canonical) — `CLAUDE.md` already inherits it.

```markdown
## Canonical commands (run from repo root)
- Full gate: `just test-full` (lint + typecheck + Go/Python/frontend tests)
- Go: `go test ./...` · focused: `go test ./service -run <Name>`
- Frontend: `npm --prefix frontend run test:unit` · build: `npm --prefix frontend run build`
- Python tooling: `just ci-python`
- Never `npm install`/`uv sync` variants other than the justfile ones; never run RELEASE*.ps1 or `just release*`/`just clean` without explicit approval.

## Persistence rules (Go)
- Do NOT persist PodcastItem/Podcast state via whole-row `Save`. Use column-scoped
  `Updates` functions in `db/` (see UpdatePodcastItemDownloadProgress as the pattern).
- Background jobs must acquire `db.TryLock` (never GetLock+Lock); long jobs add
  `startJobLockHeartbeat`.
- New columns: add via AutoMigrate model tag AND a named entry in db/migrations.go
  (idempotent, `add column if not exists` form). Never edit or reorder existing entries.

## Security rules
- No state-changing GET routes. Mutations are POST/PATCH/DELETE only.
- All outbound HTTP goes through getRequest/doRequestWithHostLimit and the
  outbound URL policy (SSRF guard) — never raw http.Get.
- Never log or echo Setting.BriefpointAPIKey, LLM_API_KEY, WHISPERX_HF_TOKEN.
  GET /settings must expose only *Configured booleans for secrets.
- Do not commit .env*, .claude/settings.local.json, coverage/test binaries.

## Testing expectations
- Bug fix ⇒ regression test in the same PR.
- Refactor of unclear behavior ⇒ characterization tests FIRST
  (see docs/refactor-assessment/PROMPTS.md, Prompt 4 pattern).
- Handler changes pin HTTP status codes; job changes include lock-contention tests.
- Tests must stay hermetic: httptest servers, temp SQLite, env-gated real-WhisperX.

## Do-not-touch without maintainer approval
- service/whisperx.go checkpoint/progress protocol; internal/sanitize (vendored);
  db/migrations.go history; RELEASE*.ps1; auth model in main.go.

## Local conventions to preserve
- zap structured logging with job/request IDs (`logging.NewJobSugar`).
- Worker fan-out via runWorkerPool + boundedWorkerCount.
- Error taxonomy: typed errors (ErrDownloadPaused/Cancelled, model.*ExistsError)
  checked by identity — keep types stable.
```
