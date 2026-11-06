# Briefcast — Execution Prompts

- **Date:** 2026-11-05
- **Branch / commit assessed:** `feat/show-note-link-tracking` @ `69252adde6d6a741fb70583ff9ed218d026434df`
- Each prompt is self-contained. Paste one prompt into a fresh coding-agent session at the repository root.
- Line numbers are from the assessed commit. Find code by symbol name; the lines may have moved.
- Background and evidence: [ASSESSMENT.md](ASSESSMENT.md) and [BACKLOG.md](BACKLOG.md).

Contents:

1. Prompt 1 — P5 slice 1: podcast delete path guards (**recommended first PR**)
2. Prompt 2 — P1: secure compose defaults
3. Prompt 3 — P2: explicit SQLite concurrency settings
4. Prompt 4 — P3 slice A: column-scoped download-state updates
5. Prompt 5 — P4: panic containment for background goroutines
6. Reusable blank template
7. Proposed additions to `AGENTS.md` / `CLAUDE.md`

---

## Prompt 1 — P5 slice 1: podcast delete path guards (recommended first PR)

**Why this goes first.** It closes a confirmed path to irreversible data loss with the smallest possible change: one function and its caller, covered by temp-directory tests, with no schema, API or deployment change and no maintainer decision needed to start. P1 scores higher but changes deployment defaults and needs approval; P2 needs validation against a production-sized database; P3 is a multi-PR refactor. This PR also adds the first tests around asset-folder deletion, which later path work (P12) will rely on.

```text
You are working in the Briefcast repository: a self-hosted podcast manager written in Go
(Gin, GORM, SQLite by default) with a Vue frontend. Make one small, reviewable change.

GOAL
Stop podcast deletion from removing directories that belong to other podcasts.

CONTEXT
- Downloaded assets live under the directory named by the DATA environment variable, in
  category folders: audio, images, transcripts, summaries, markdown/transcripts,
  markdown/summaries. The names are constants in service/asset_paths.go
  (assetAudioDir and friends).
- Each podcast's files live in DATA/<category>/<sanitized podcast title>. The folder name
  comes from sanitizeAssetName(title) in service/asset_paths.go. Nothing else identifies
  the podcast in the path.
- service/fileService.go has deletePodcastFolder(folder string). It builds seven paths:
  dataPodcastFolderPath(folder), which is the legacy layout DATA/<sanitized title>, and
  dataCategoryPodcastFolderPath(<category>, folder) for the six categories. It calls
  os.RemoveAll on each one that passes IsPathWithinAssetsDir.
- service/podcastService.go has DeletePodcast(id string, deleteFiles bool). It deletes
  each episode's files (only when deleteFiles is true) and rows, then calls
  deletePodcastFolder(podcast.Title), then deletes the podcast row.

THE DEFECTS
1. A podcast whose sanitized title equals a top-level category directory name (audio,
   images, transcripts, summaries, markdown) makes the legacy path resolve to that
   category directory, so os.RemoveAll deletes every podcast's files in that category.
2. Two podcasts with the same sanitized title share all six category folders. Deleting
   one removes the other's files.
3. DeletePodcast calls deletePodcastFolder even when deleteFiles is false. The route
   DELETE /podcasts/:id/podcast (controllers.DeleteOnlyPodcastByID) passes false, so
   "delete only the podcast" still removes its folders. Whether that route should keep
   files is a maintainer question. Do NOT change this in this PR unless the maintainer
   has said so; report what you found instead.

FILES LIKELY TOUCHED
- service/fileService.go (deletePodcastFolder)
- service/podcastService.go (DeletePodcast)
- a new or existing test file under service/ (for example service/delete_podcast_test.go)
Read first: service/asset_paths.go, service/asset_paths_test.go,
service/coverage_extra_test.go (near the existing deletePodcastFolder and DeletePodcast
tests), service/retention_test.go for the standard test database setup.

NON-GOALS
- Do not change the folder layout or key folders by podcast ID.
- Do not reorder the database deletes or make DeletePodcast transactional (separate PR).
- Do not change sanitizeAssetName, IsPathWithinAssetsDir, or any download code.
- Do not touch controllers, routes, the frontend, or internal/sanitize.

CONSTRAINTS
- Smallest safe change. No new dependencies. No schema or API change.
- The only intended behaviour change: directories that are shared with another podcast,
  or that are category directories, are no longer removed.
- Do not read or print .env, or anything under secrets/, config/, assets/, logs/.
- Follow existing test patterns: t.TempDir(), t.Setenv("DATA", ...), temp SQLite.
- You may choose a better local design than the steps below if every test listed under
  REQUIRED TESTS passes and the non-goals hold.

PLAN
1. Write the tests first and confirm tests (a) and (b) fail on the current code.
2. In deletePodcastFolder, skip the legacy path DATA/<sanitized title> when the sanitized
   title equals a reserved top-level name. Derive the reserved names from the existing
   category constants (the first path segment of each) rather than hard-coding a second
   list. Compare case-insensitively, because some filesystems are case-insensitive.
3. In DeletePodcast, before calling deletePodcastFolder, check whether any OTHER podcast
   has the same sanitized title (db.FindAllPodcastsPlain returns all podcasts; the list
   is small). If one does, skip folder removal and log a warning with both podcast IDs.
   Episode files have already been removed one by one at that point.
4. Run the focused tests, then the package, then stop.

REQUIRED TESTS
(a) For each reserved name (audio, images, transcripts, summaries, markdown): create a
    file belonging to a different podcast under that category, call deletePodcastFolder
    with the reserved name as the title, and assert the other podcast's file still exists.
(b) Two podcasts with the same title, each with a file on disk in the shared folder:
    DeletePodcast on one must leave the other's file in place and must still remove the
    deleted podcast's rows.
(c) Regression: a podcast with a unique, ordinary title still has all of its category
    folders removed by DeletePodcast(id, true).
(d) Existing tests in service/ keep passing without weakening their assertions. If an
    existing test asserts the old behaviour for case 3 above, leave it and report it.

VERIFICATION COMMANDS
Go may not be on PATH in Git Bash on the maintainer's Windows machine; it is installed at
"C:\Program Files\Go\bin".
- go build ./...
- go vet ./...
- go test ./service/ -run 'TestDeletePodcast' -count=1
- go test ./service/ ./db/ ./controllers/ -count=1     (about two minutes)
Do not run the root package tests (go test .) locally: package main autoloads .env.

ACCEPTANCE CRITERIA
- Tests (a), (b), (c) exist and pass; (a) and (b) failed before the change.
- No directory outside DATA/<category>/<title> and DATA/<title> is ever removed, and
  DATA/<category> itself is never removed.
- go build, go vet and the three packages above pass.
- The diff touches only the files listed above.

ROLLBACK
Revert the commit. There is no data migration and no configuration change.

COMPLETION REPORT (required)
- Files changed, with a one-line reason each.
- Behaviour preserved and behaviour changed.
- Tests added, and the exact commands run with their results.
- Commands not run and why.
- What you found about DeletePodcast(id, false) and the tests that cover it.
- A check of each acceptance criterion: met or not met.
- Remaining risks and follow-ups.
```

---

## Prompt 2 — P1: secure compose defaults

**Needs maintainer approval before it is run.** It changes deployment defaults.

```text
You are working in the Briefcast repository: a self-hosted podcast manager (Go API and
Vue frontend) deployed with Docker Compose.

GOAL
Make a default deployment safe: publish the app on the loopback interface unless the
operator opts in to wider exposure, warn at startup when no password is set, and fix a
log-output default that is not a valid value.

CONTEXT
- docker-compose.yml, service "briefcast":
    "- PASSWORD=${PASSWORD:-}"            an empty PASSWORD disables authentication
    "- ${HOST_PORT:-8080}:8080"           no host address, so Docker binds all interfaces
    "- LOG_OUTPUT=${LOG_OUTPUT:-both}"    "both" is not a keyword
- docker-compose.legacy.yml has the same PASSWORD and port lines.
- main.go: authRouterGroup adds gin.BasicAuth (user "briefcast") only when PASSWORD is
  non-empty. assetEnv logs runtime configuration at startup.
- internal/logging/logging.go, resolveLogOutputs: only "stdout" and "stderr" are
  keywords; any other token is treated as a file path. With the current default, logs go
  to a file named "both" in the working directory and nothing reaches "docker logs".
- tests/test_packaging.py has test_compose_matrix, which renders compose configuration
  through scripts/Compose.ps1 and asserts on it (it needs pwsh and Docker, and skips
  without them).

FILES LIKELY TOUCHED
- docker-compose.yml, docker-compose.legacy.yml
- main.go (one warning log line in assetEnv), main_test.go
- tests/test_packaging.py
- README.md (the configuration section that documents PASSWORD and ports), CHANGELOG.md

NON-GOALS
- Do not change how authentication works, add users, or add rate limiting.
- Do not make the container run as a non-root user (separate decision).
- Do not change the worker compose files.
- Do not read .env or .env.example; they are off-limits to agents in this repository.

CONSTRAINTS
- This is a behaviour-changing, deployment-changing PR. Say so in the PR description and
  include an upgrade note.
- No new dependencies. Smallest diff that meets the acceptance criteria.
- You may choose a different variable name or mechanism if the acceptance criteria and
  the documented opt-in are preserved.

PLAN
1. Tests first: extend test_compose_matrix to assert that, with no overrides, the
   briefcast service's published port has host IP 127.0.0.1 and LOG_OUTPUT renders as
   "stdout". Add a Go test that assetEnv emits a warning when PASSWORD is empty and none
   when it is set (use the zap observer pattern if the logging package already supports
   it; otherwise extract the decision into a small pure function and test that).
2. Change the port mapping to "- ${HOST_BIND:-127.0.0.1}:${HOST_PORT:-8080}:8080" in both
   compose files.
3. Change the log default to "${LOG_OUTPUT:-stdout}" in docker-compose.yml.
4. Add the startup warning in assetEnv. Do not log the password or its length.
5. Document HOST_BIND in README.md next to PASSWORD, and add a CHANGELOG entry that
   tells existing operators to set HOST_BIND=0.0.0.0 to keep LAN access.

REQUIRED TESTS
- The compose rendering assertions in step 1.
- The startup-warning test in step 1.

VERIFICATION COMMANDS
- go build ./...
- go vet ./...
- go test . -run 'TestAssetEnv|TestAuthRouterGroup' -count=1
  (root package; it autoloads .env, so run it in CI or ask the maintainer to run it)
- uv run pytest tests/test_packaging.py
- just ci-python

ACCEPTANCE CRITERIA
- Rendered default compose configuration binds 127.0.0.1 and sets LOG_OUTPUT=stdout.
- Setting HOST_BIND=0.0.0.0 restores the previous exposure.
- A warning is logged at startup exactly when PASSWORD is empty.
- README and CHANGELOG describe the change and the opt-in.

ROLLBACK
Operators can restore the old behaviour without a code change by setting
HOST_BIND=0.0.0.0. Reverting the PR restores the old defaults. No data is involved.

COMPLETION REPORT (required)
Files changed; behaviour changed; tests added; exact commands run and results; commands
not run and why; acceptance-criteria check; remaining risks.
```

---

## Prompt 3 — P2: explicit SQLite concurrency settings

```text
You are working in the Briefcast repository: a Go service using GORM. SQLite is the
default database; Postgres is optional.

GOAL
Make SQLite concurrency settings explicit and identical regardless of which SQLite
driver is compiled in: a busy timeout on every pooled connection and a SQLite-specific
pool size. Postgres behaviour must not change.

CONTEXT
- db/db.go, Init(): resolves configuration, opens with gorm.io/driver/sqlite (which uses
  mattn/go-sqlite3 and needs cgo) or the Postgres driver, then calls
  applyConnectionPool.
- db/config.go: normalizeSQLiteDSN builds the data source string; applyConnectionPool
  reads DB_MAX_IDLE_CONNS (default 10), DB_MAX_OPEN_CONNS (default 25) and
  DB_CONN_MAX_LIFETIME_MINUTES for every driver.
- Nothing in the app sets a journal mode, busy timeout or SQLite pool size. Production
  therefore depends on mattn's built-in 5 second busy timeout. The command-line tools
  under cmd/ and the test helpers open the same database with github.com/glebarez/sqlite
  (pure Go), whose defaults differ.
- When cgo is disabled, Init() fails and the test helpers fall back to glebarez (see
  setupDBForTest in db/dbfunctions_test.go). CI has cgo enabled. Your change must work
  and be tested under both.
- In rollback-journal mode a long read blocks writers; a writer that waits longer than
  the busy timeout fails. The maintainer has seen production block on long reads.

FILES LIKELY TOUCHED
- db/db.go, db/config.go
- db/config_test.go and a new test in db/
- README.md (document the new variables)

NON-GOALS
- Do NOT enable WAL or change journal_mode by default. A later PR may add an opt-in
  after the backup job is fixed, because the current backup copies the live file.
- Do not change queries, models, migrations or the cmd/ tools in this PR.
- Do not remove the flexibleTime type in db/link_stats.go; it exists because the two
  drivers return timestamps differently.

CONSTRAINTS
- A single "PRAGMA busy_timeout" Exec after Open is NOT sufficient: it applies to one
  pooled connection. Use a mechanism that reaches every connection for the driver in
  use (a data-source parameter or a connection hook).
- Preserve any parameters already present in a user-supplied DATABASE_URL, and do not
  override a timeout the operator set there.
- Configuration-only behaviour change. No new dependencies.
- You may choose a different mechanism than suggested if the tests below pass on both
  drivers.

PLAN
1. Tests first:
   - A test that opens the database through db.Init (using the existing fallback helper
     pattern), acquires several connections at the same time (for example with
     sql.DB.Conn held open concurrently), and asserts "PRAGMA busy_timeout" returns the
     configured value on each one.
   - Table tests for the data-source helper: plain path, sqlite:// URL, file: URL, URL
     with existing parameters, URL that already sets a timeout.
   - A test that a Postgres configuration is returned unchanged.
2. Implement the per-connection timeout. Default 5000 ms, overridable with
   SQLITE_BUSY_TIMEOUT_MS; ignore invalid or negative values and use the default.
3. Make applyConnectionPool driver-aware: for SQLite, when DB_MAX_OPEN_CONNS is unset,
   use a small default (suggest 4); an explicit value always wins.
4. Log the effective busy timeout, pool size and current journal mode once at startup at
   info level.
5. Document the variables. Stop.

REQUIRED TESTS
The three tests in step 1.

VERIFICATION COMMANDS
Go may not be on PATH in Git Bash on Windows; it is at "C:\Program Files\Go\bin".
- go build ./...
- go vet ./...
- go test ./db/ -count=1
- go test ./db/ ./service/ ./controllers/ -count=1
- CI must also pass; it runs with cgo and therefore the production driver:
  go test ./...
  go test -race ./internal/transcription/... ./internal/whisperxworker/... ./service/...

ACCEPTANCE CRITERIA
- Every pooled SQLite connection reports the configured busy timeout under both drivers.
- The SQLite pool default is small and overridable; the Postgres pool is unchanged.
- journal_mode is not modified by this PR.
- Startup logs show the effective settings. No secrets are logged (the display data
  source is already sanitized by sanitizeDataSource; keep using it).

ROLLBACK
Revert the PR. No file-format or schema change is made.

COMPLETION REPORT (required)
Files changed; behaviour changed; mechanism chosen for each driver and why; tests added;
exact commands run and results, stating whether cgo was enabled locally; commands not
run and why; acceptance-criteria check; remaining risks, including pool-size effects on
read-heavy pages.
```

---

## Prompt 4 — P3 slice A: column-scoped download-state updates

```text
You are working in the Briefcast repository: a Go podcast manager using GORM.

GOAL
Stop lost updates on the podcast_items table. This PR converts the download-state
setters from "load the whole row, change a field, save every column" to updates that
write only the columns each setter owns.

CONTEXT
- db/dbfunctions.go, UpdatePodcastItem: saves the entire row with
  DB.Session(<silent logger>).Omit("Podcast").Save(&podcastItem). The silent logger is
  deliberate: SQL logs could otherwise contain whole transcripts.
- 21 call sites use it. Any field another request or job changed between the load and
  the save is overwritten with the stale value (for example a user marks an episode
  played while a download finishes).
- A correct pattern already exists: db.UpdatePodcastItemDownloadProgress uses
  DB.Model(&PodcastItem{}).Where("id=?", id).Updates(map[string]interface{}{...}).
- service/state_transitions_characterization_test.go pins the current field-level
  behaviour of every setter (15 tests named TestCharacterize_*). They are your safety
  net. Their expectations must not change.

SCOPE OF THIS PR (service/podcastService.go only)
SetPodcastItemAsQueuedForDownload, SetPodcastItemAsQueuedPreserveProgress,
SetPodcastItemAsDownloading, SetPodcastItemAsPaused, SetPodcastItemAsDownloaded,
SetPodcastItemAsNotDownloaded, SetPodcastItemPlayedStatus, SetPodcastItemBookmarkStatus,
downloadImageLocally, and the alternate-URL save inside DownloadWithFallback.

FILES LIKELY TOUCHED
- db/dbfunctions.go (new helper), db/dbfunctions_test.go
- service/podcastService.go
- a new regression test under service/

NON-GOALS
- Do not convert service/summarize.go, service/whisperx.go, service/alternate_feeds.go
  or service/chapters.go here; they are the next two PRs.
- Do not remove UpdatePodcastItem yet.
- Do not change which fields each setter writes, HTTP handlers, or the schema.
- Do not touch the remote-transcription commit path
  (service/transcription_remote_state.go); it already uses conditional writes.

CONSTRAINTS
- Behaviour-preserving for the columns each setter owns. The only observable difference
  is that unrelated columns are no longer overwritten.
- Keep the silent-logger session on the new helper.
- The helper must return an error when no row matches the ID (gorm.ErrRecordNotFound),
  because today's setters return an error for a missing episode.
- Use column-name maps so zero values (false, empty string, zero time) and NULL are
  written. Struct-based Updates skips zero values and would be a bug here.
- updated_at must still advance.
- No new dependencies. You may design the helper differently if the tests below pass.

PLAN
1. Run the characterization suite and confirm it is green before changing anything:
   go test ./service/ -run TestCharacterize -count=1
2. Add db.UpdatePodcastItemColumns(id string, columns map[string]interface{}) error with
   unit tests: writes zero values, writes NULL for a nil pointer column, returns
   not-found for an unknown ID, advances updated_at, leaves other columns untouched.
3. Add a regression test that fails today: load an episode, change is_played through
   SetPodcastItemPlayedStatus, then call a download-state setter that works from an
   older snapshot, and assert is_played kept its new value. For setters that take only
   an ID, simulate the interleaving by calling the two setters in sequence around a
   direct column update, and add one test for DownloadWithFallback's alternate-URL save,
   which does save a caller-held snapshot.
4. Convert the setters listed under SCOPE one at a time. For SetPodcastItemAsDownloaded,
   read what it needs (transcript status, chapters and ID3 fields) and write back only
   the columns it changes.
5. Re-run the characterization suite after each conversion. Stop when the scope is done.

REQUIRED TESTS
- Helper unit tests (step 2).
- The lost-update regression tests (step 3).
- All existing TestCharacterize_* tests pass with unchanged expectations.

VERIFICATION COMMANDS
Go may not be on PATH in Git Bash on Windows; it is at "C:\Program Files\Go\bin".
- go build ./...
- go vet ./...
- go test ./service/ -run TestCharacterize -count=1
- go test ./db/ ./service/ ./controllers/ -count=1     (about two minutes)
- In CI: go test ./... and
  go test -race ./internal/transcription/... ./internal/whisperxworker/... ./service/...

ACCEPTANCE CRITERIA
- None of the setters in scope calls db.UpdatePodcastItem.
- The regression tests fail before the change and pass after it.
- Characterization tests are unmodified and green.
- No change to HTTP responses or to the columns each setter is responsible for.

ROLLBACK
Revert the PR. No schema or data change.

COMPLETION REPORT (required)
Files changed; for each converted setter, the columns it now writes; tests added; exact
commands run and results; commands not run and why; acceptance-criteria check; any
setter whose behaviour you found ambiguous and how you resolved it.
```

---

## Prompt 5 — P4: panic containment for background goroutines

```text
You are working in the Briefcast repository: a Go HTTP service (Gin) with cron jobs and
many background goroutines.

GOAL
Make a panic in background work log an error and stop that unit of work instead of
terminating the whole process.

CONTEXT
- gin.Recovery protects request goroutines and the cron scheduler has its own recover
  wrapper (main.go, intiCron). Nothing else is protected: recover() does not appear in
  any non-test Go file.
- service/workerpool.go, runWorkerPool: starts workers that call fn(job) with no recover.
  It runs feed refresh, downloads and OPML import.
- Goroutines are started directly at these sites (find them by searching for "go func"
  and "go " at the start of a statement):
    controllers/podcast.go (7 sites), controllers/settings.go (2),
    controllers/downloads.go (1), service/podcastService.go (3),
    service/summary_backfill.go (3), service/repair_work.go (1),
    service/briefpoint.go (1), main.go (controllers.DefaultHub.Run and intiCron).
- One concrete crash: service/podcastService.go, getItunesImageURL. xmlquery.Query(doc,
  "//channel") returns (nil, nil) when a feed has no <channel> element (an Atom feed),
  and the next statement calls channel.SelectElement on that nil. AddPodcast calls this
  when the feed has no image. During an OPML import AddPodcast runs inside
  runWorkerPool, so the process exits.
- A sibling branch (feat/whisperx-service, commit ff2a576 "Avoid panic when a feed has
  no RSS channel element") already fixes that one site. If that commit is present in
  your checkout, keep it and do not write a second fix.

FILES LIKELY TOUCHED
- service/workerpool.go, service/workerpool_test.go
- service/podcastService.go and a test for getItunesImageURL
- the launch sites listed above
- internal/logging if you place the helper there

NON-GOALS
- Do not add retries, change job semantics, or restructure goroutine ownership.
- Do not add recover() to main() itself or swallow panics silently.
- Do not change the cron wrapper or gin middleware order (a separate PR covers that).

CONSTRAINTS
- Behaviour-preserving on the success path.
- On panic: log at error level with a name for the unit of work, the panic value and the
  stack trace, using the existing zap logger. Never log request bodies, tokens or
  environment values.
- In runWorkerPool, recover per job so the remaining jobs still run and wg.Wait returns.
- Flags that mark a job as running (for example the summary-job and export flags) must
  still be cleared when the wrapped function panics; check each site's defer order.
- No new dependencies. You may choose a different helper shape if the tests pass.

PLAN
1. Tests first:
   - runWorkerPool with several jobs where one panics: the others complete and the call
     returns.
   - getItunesImageURL with an Atom document that has no <channel>: returns "" and does
     not panic.
   - One job-flag test: after a panic inside a wrapped background job, the "running"
     flag reads false.
2. Add a small helper (for example safeGo(name string, fn func()) and a matching
   non-goroutine form for use inside the pool).
3. Use it inside runWorkerPool.
4. Guard the nil channel in getItunesImageURL (skip if already fixed).
5. Replace the bare goroutine launches at the listed sites with the helper. Keep each
   site's existing error logging.
6. Stop.

REQUIRED TESTS
The three tests in step 1.

VERIFICATION COMMANDS
Go may not be on PATH in Git Bash on Windows; it is at "C:\Program Files\Go\bin".
- go build ./...
- go vet ./...
- go test ./service/ -run 'TestRunWorkerPool|TestGetItunesImageURL' -count=1
- go test ./service/ ./controllers/ ./db/ -count=1     (about two minutes)
- In CI: go test ./... and
  go test -race ./internal/transcription/... ./internal/whisperxworker/... ./service/...
Do not run go test . locally: package main autoloads .env.

ACCEPTANCE CRITERIA
- A panic inside any converted background unit is logged with a stack and does not end
  the process.
- runWorkerPool completes when a job panics.
- No bare "go func" remains at the listed sites, or each remaining one is justified in
  the completion report.
- All listed test commands pass.

ROLLBACK
Revert the PR. No data or configuration change.

COMPLETION REPORT (required)
Files changed; every launch site converted (file and function); behaviour preserved and
changed; tests added; exact commands run and results; commands not run and why;
acceptance-criteria check; remaining risks.
```

---

## Reusable blank template

```text
You are working in the Briefcast repository: <one-sentence description of the part of
the system this touches>.

GOAL
<One or two sentences. What is true after this PR that is not true now?>

CONTEXT
- <File and symbol>: <what it does today, stated precisely>.
- <The defect or limitation, with the mechanism>.
- <Existing tests or patterns the implementer should reuse>.

FILES LIKELY TOUCHED
- <path> (<symbol>)
Read first: <paths>.

NON-GOALS
- <Adjacent work that must not be done in this PR>.

CONSTRAINTS
- Refactor type: <behavior-preserving | behavior-changing | API-changing |
  data-changing | test-only | mechanical>. <State the exact behaviour change, if any.>
- No new dependencies. Smallest safe change. No schema, API or deployment change unless
  stated above.
- Do not read or print .env, or anything under secrets/, config/, assets/, logs/.
- You may choose a better local design if the required tests pass and the non-goals hold.

PLAN
1. Tests first: <characterization or failing regression tests>.
2. <Smallest change>.
3. <Focused validation>.
4. Stop. <Name the reviewable stop point.>

REQUIRED TESTS
- <Test and what it proves>.

VERIFICATION COMMANDS
- go build ./...
- go vet ./...
- go test ./<package>/ -run '<pattern>' -count=1
- go test ./db/ ./service/ ./controllers/ -count=1
- <frontend: npm --prefix frontend run test:unit ; npm --prefix frontend run build>
- <python: just ci-python>

ACCEPTANCE CRITERIA
- <Observable, checkable statements>.

ROLLBACK
<"Revert the PR" or the exact extra steps for flags, migrations or data>.

COMPLETION REPORT (required)
Files changed; behaviour preserved and changed; tests added; exact commands run and
results; commands not run and why; acceptance-criteria check; remaining risks.
```

---

## Proposed additions to `AGENTS.md` / `CLAUDE.md`

Short enough to adopt as one section. Nothing here contradicts the existing files.

```markdown
## Repository facts for agents

### Canonical commands

| Purpose | Command | Notes |
| --- | --- | --- |
| Build | `go build ./...` | |
| Static check | `go vet ./...` | |
| Go tests | `go test ./... -count=1` | About two minutes. The root package autoloads `.env`; run it in CI, or locally only with a scratch `.env`. |
| Focused Go test | `go test ./service/ -run 'TestName' -count=1` | |
| Race detector | `go test -race ./internal/transcription/... ./internal/whisperxworker/... ./service/...` | Needs cgo. |
| Frontend | `npm --prefix frontend run test:unit` then `npm --prefix frontend run build` | Node 24. |
| Python | `just ci-python` | Needs `uv`; tooling targets Python 3.14, shipped scripts run on 3.12. |
| Packaging | `uv run pytest tests/test_packaging.py` | Needs pwsh; one test needs Docker. |

- On Windows, Go is at `C:\Program Files\Go\bin` and may not be on the Git Bash `PATH`.
- With `CGO_ENABLED=0`, Go tests fall back to the pure-Go SQLite driver. CI and
  production use the cgo driver. Say which one you ran.
- `gofmt -l` is noisy on Windows checkouts (CRLF). Do not reformat files you did not edit.

### Data-safety rules

- Never call `os.RemoveAll` or `os.Remove` on a path built from a podcast or episode
  title without a test that proves no other podcast's files are affected.
- Treat only `fs.ErrNotExist` as "file is gone". Any other stat error means "unknown":
  do not change database state because of it.
- Write episode state with column-scoped updates. Do not add new callers of
  `db.UpdatePodcastItem`.
- Start background goroutines through the panic-safe helper once it exists; until then,
  add a `defer recover()` that logs with a stack.
- State-changing routes use POST, PATCH or DELETE. Do not add GET routes with side effects.
- Every ID placed in a frontend request path goes through `encodeURIComponent`.

### Change-control rules

- Compose defaults, `Dockerfile`, migrations, public routes and anything under `scripts/`
  that deploys or deletes are approval-required changes. Label them in the PR title.
- One concern per PR. Mechanical moves, behaviour changes and test-only changes are
  separate PRs.
- A migration that deletes or dedupes rows needs a verified backup step and a note on
  how to restore.

### Do not touch without an explicit request

- `internal/sanitize` (vendored fork).
- `internal/whisperxworker` journal, checkpoint and guardian code (fail-closed by design).
- `RELEASE_RESET.ps1`, `RELEASE_RUN.ps1`, `RELEASE_BUILD.ps1` (legacy and destructive;
  never run them).
- `.env`, `secrets/`, `config/`, `assets/`, `logs/`, `worker-state/`, `worker-cache/`.

### Local conventions to preserve

- Handlers bind into small request structs and answer with `gin.H{"error": ...}`.
- Logging is zap with key-value pairs and IDs (`podcast_id`, `podcast_item_id`); never
  log transcripts, prompts, tokens or database URLs.
- All outbound HTTP goes through `validateOutboundURL` and `outboundHTTPTransport`.
- Tests use `t.TempDir()`, `t.Setenv`, `httptest` servers and temp-file SQLite.
- Raw-SQL migrations go in `db/migrations.go` with a dated, unique name and must be
  safe to run on both SQLite and Postgres.
```
