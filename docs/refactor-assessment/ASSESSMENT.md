# Briefcast — Repository Refactoring Assessment

- **Date:** 2026-07-05
- **Branch / commit:** `main` @ `65339368a354821b563517b46d07485da6756d6b` (v1.9.3)
- **Working tree:** clean except untracked `docker-compose.override.yml`
- **Assessment mode:** read-only; deliverables written as new files under `docs/refactor-assessment/` only

### Implementation update — 2026-07-10 / v1.9.4

- **Completed:** P2/F2 atomic job locking, including a lease heartbeat for missing-download jobs.
- **Completed:** P12/F5/F8 repository hygiene: local agent settings and coverage output are ignored/untracked, and stale credential-bearing comments are removed.
- **Completed:** P6/F6 download cleanup and resumable partial retention.
- **Completed:** P1a characterization coverage for podcast-item state transitions; P1b column-scoped persistence remains the next foundational implementation PR.
- The original assessment evidence and line references below remain anchored to `6533936`; use symbols rather than historical line numbers when implementing remaining work.

---

## 1. Scope & confidence

**Repo:** single deployable unit — a Go 1.26 HTTP API + cron worker binary (Gin, GORM, SQLite default / optional Postgres), serving a Vue 3 + Vite + TS frontend from `frontend/dist`, shelling out to Python helpers (`feedparser`, `mutagen`, WhisperX) for feed parsing, ID3 extraction, and transcription. Python `src/briefcast_tools` is repo tooling only. 307 tracked files, 107 commits (2026-02-12 → 2026-07-03).

**Commands run (all read-only):** `git rev-parse`, `git status --porcelain`, `git log`, `git ls-files`, churn analysis (`git log --since='12 months ago' --format='' --name-only | sort | uniq -c | sort -rn`), `wc -l`, file reads.

**Baseline build/test/lint commands: SKIPPED — none were run.** The assessment sandbox has no Go toolchain, Node 22 (repo requires 24), Python 3.10 (repo requires 3.14), and installing dependencies is forbidden for this pass. The canonical commands, taken from `justfile` and `.github/workflows/ci.yml`, are:

| Command | Source | Status here |
|---|---|---|
| `go test ./...` | CI `go-test` job, `just test-go` | **Not run** — no Go toolchain in sandbox |
| `go test ./service -run TestIntegrationFeedDownloadWhisperX` | `just test-integration` | **Not run** |
| `npm --prefix frontend run test:unit` && `npm --prefix frontend run build` | CI `frontend-quality` | **Not run** — requires `npm ci` (install forbidden), Node 24 |
| `just ci-python` (`uv sync --locked`, `ruff check .`, `ruff format --check .`, `mypy src`, `pytest`, `pip-audit`) | CI `python-quality` | **Not run** — requires `uv sync` (install forbidden) |

**Maintainer action:** run `just test-full` on a dev machine before starting any backlog item and record the pass/fail baseline. Every verification command prescribed in `BACKLOG.md` is one of the commands above. Test code reviewed (httptest feed servers, in-memory/temp SQLite, env-gated real-WhisperX test) looks hermetic, so `go test ./...` should be network-free.

**Reviewed fully:** `main.go`; all of `controllers/` (non-test); `service/`: podcastService, whisperx, fileService, downloadsService, download_manager, workerpool, hostLimiter, summarize, briefpoint, outbound_url_policy, retention, asset_paths, feedparser, export, alternate_feeds, local_search; all of `db/` (non-test); `cmd/migrate_to_pg`; `scripts/feedparser_parse.py`; CI workflows; Dockerfile; docker-compose; justfile; AGENTS.md/CLAUDE.md.

**Sampled:** `service/app_logs.go`, `service/repair_work.go`, `service/canonical_transcript.go`, gpodder/itunes services, `internal/logging`, frontend `lib/api/*`, `package.json`, test-helper files.

**Skipped:** `scripts/whisperx_transcribe.py` (874 lines) internals, `scripts/mutagen_id3_extract.py`, `RELEASE*.ps1` (~80 KB PowerShell), frontend views/components (only sizes noted), `internal/sanitize` / `feedmeta` / `id3meta` internals, test file contents in depth.

**Blind spots:** no runtime/profiling data; no DB with production-scale rows; SQLite lock behavior under the glebarez driver not empirically verified; WhisperX/py subprocess behavior not executed; release pipeline scripts unreviewed.

**Overall confidence: Medium-High.** Code-level findings are Confirmed with snippets; scale/runtime claims are labeled Likely or Hypothesis.

---

## 2. Executive summary

- **Top production risk 1 — lost updates on `podcast_items`:** every state change loads the whole row and `Save()`s the whole row back (`db/dbfunctions.go:552`). Long-running workers (multi-hour WhisperX transcriptions, downloads) silently revert concurrent user actions (mark played, bookmark, delete). [F1] → P1.
- **Top production risk 2 — racy job locks:** `RefreshEpisodes`, `DownloadMissingEpisodes`, and `RetentionCleanup` use a check-then-act `GetLock`/`Lock` idiom while a correct atomic `TryLock` already exists and is used by transcription. Concurrent triggers (cron tick + API-initiated refresh) can double-run jobs. [F2] → P2.
- **Top security risk — state-changing GET routes:** deprecated-but-live `GET /podcastitems/:id/delete`, `/download`, `/markPlayed`, etc. make destructive actions CSRF-able; combined with browser-cached Basic Auth (and `PASSWORD=` empty by default) a hostile page can delete a library. [F4] → P3.
- **Quick win 1:** unify job locking on `db.TryLock` — small, mechanical, kills a real race (P2, recommended first PR).
- **Quick win 2:** close leaked file handles on download error paths and stop deleting resumable partial files (`service/fileService.go:159-191`) (P6, completed in v1.9.4).
- **Quick win 3:** add `http.Server` timeouts and stop hitting the DB once per request (+ once per outbound request) for the settings row (P4).
- **Foundational refactor 1:** column-scoped state-transition updates for `PodcastItem` behind characterization tests (P1).
- **Foundational refactor 2:** decouple LLM summarization and transcript/chapter fetching from the feed-refresh loop; refresh should never block on a 120 s LLM call (P8).
- **Foundational refactor 3:** lighten hot queries — cron jobs and local search currently drag full transcript/summary text through memory (P11).
- **Biggest testing gap:** no characterization tests for `PodcastItem` state transitions (download/transcript/summary status machine) — the exact area the riskiest refactor touches; controller tests exist but don't pin error-path status codes.
- **Biggest architectural risk:** shared global `db.DB` + whole-struct saves + fire-and-forget goroutines (~20 `go func` sites) with no cancellation/ownership model — concurrency correctness rests on convention.
- **Recommended next PR:** P1b (column-scoped state-transition persistence), now protected by the completed P1a characterization suite.

---

## 3. Category scorecard

| Category | Rating | Rationale |
|---|---|---|
| Correctness & reliability | 🔴 Red | Lost-update saves [F1], racy locks [F2], silent no-response handlers [F16], fd leak + partial-file deletion in download loop [F6], error conflated to 409 [F3] |
| Security & privacy | 🟡 Yellow | Strong SSRF/outbound policy and log redaction; but CSRF-able GET mutations [F4], auth-off default, stale API key in comment [F8], `bypassPermissions` committed [F5] |
| Architecture | 🟡 Yellow | Clean controller/service/db layering and good seams (cronJobSet, SearchService); undermined by global DB, whole-row saves, dead legacy page layer [F15] |
| Complexity | 🟡 Yellow | `AddPodcastItems` (~200 lines, network+DB+LLM in one loop), 14-positional-arg `UpdateSettings` [F19], 1,900-line Vue views |
| Performance | 🟡 Yellow | Full-text columns preloaded in cron loops [F10], `lower(transcript_json) LIKE` scans, per-request settings query [F9], HEAD-per-episode size job |
| Testing | 🟡 Yellow | Real suite (≈9.9 k Go test lines), hermetic patterns; but coverage-chasing test files, no state-transition characterization, error-path codes unpinned |
| Observability | 🟢 Green | Structured zap logs, request IDs, per-job loggers with duration, secret redaction in log viewer, queue snapshots |
| Delivery & DX | 🟡 Yellow | Good justfile + CI skeleton; no Go vet/lint gate, bleeding-edge toolchain pins (Go 1.26/Node 24/Py 3.14), tracked stray `coverage` artifact, compose/env drift |

---

## 4. Architecture map

### Modules

| Module | Responsibility | Key dependencies | Risk | Notes |
|---|---|---|---|---|
| `main.go` | Entry, routes, auth, cron wiring | controllers, service, db, gin, cron | High | Per-request settings middleware; optional Basic Auth; deprecated GET aliases |
| `controllers/` | HTTP handlers, response shaping | service, db (direct!), gin | High | Mixes service calls with direct `db.DB` writes; several silent error paths |
| `service/` | Feeds, downloads, transcription, summaries, retention, export, Briefpoint | db, internal/*, python subprocesses | High | Core business logic; job locks; worker pools; outbound HTTP policy |
| `db/` | GORM models, queries, migrations, job locks | gorm, sqlite/postgres drivers | High | Whole-row `Save`; heavy `Preload(clause.Associations)` helpers; hand-rolled migration table |
| `internal/feedmeta`, `id3meta` | Feed/ID3 metadata extraction | — | Med | Pure-ish, tested |
| `internal/logging` | zap setup, request/job loggers | zap, lumberjack | Low | Solid |
| `internal/sanitize` | Vendored filename sanitizer (own LICENSE) | — | Low | Vendored fork; freeze |
| `scripts/*.py` | feedparser/mutagen/WhisperX helpers (subprocess) | Python 3, feedparser, mutagen, whisperx | Med | JSON-over-stdio contract with Go |
| `frontend/` | Vue 3 SPA served at `/app` | axios, DOMPurify, marked | Med | DOMPurify used for HTML; two views >1,600 lines |
| `cmd/migrate_to_pg` | SQLite→Postgres one-shot migration | db, gorm | Med | Correctly uses `SkipHooks` to preserve UUIDs |
| `client/`, most of `controllers/pages.go` | **Dead** legacy HTML UI | — | Low | No `LoadHTMLGlob`, no routes registered [F15] |

### Core workflows

**Request flow:** gin → `RequestLoggerMiddleware` → `setupSettings` (DB read per request, `main.go:194`) → optional `gin.BasicAuth` (single user `briefcast`, only when `PASSWORD` set) → handler. Handlers frequently spawn `go func()` background work and return 200/202 immediately.

**Feed refresh (cron `@every CHECK_FREQUENCY m` + API-triggered):** `RefreshEpisodes` → job lock (racy idiom) → worker pool over all podcasts → `AddPodcastItems` per podcast: Python feedparser subprocess → per-new-episode synchronous fetches of chapters/transcripts (`makeQuery`) → row insert → optional synchronous LLM summarization (`SummarizeEpisode`, up to `LLM_TIMEOUT_SECONDS=120` each) → then fire-and-forget `DownloadMissingEpisodes`.

**Downloads:** `DownloadMissingEpisodes` → job lock (racy idiom) → worker pool (`MaxDownloadConcurrency`, default 5) → `DownloadWithFallback` → `Download` (range-resume, pause/cancel flags in process-local maps, progress rows) → `SetPodcastItemAsDownloaded` (whole-row save + ID3 extraction subprocess).

**Transcription:** cron `TranscribePendingEpisodes` → **atomic** `db.TryLock` + heartbeat → preflight `py_compile` → worker pool → `RunWhisperXWithProgress` (exec python, progress file polling, checkpoint resume) → whole-row save → export files → optional synchronous summarization.

**Cross-cutting:** config via env + singleton `settings` row (mixed ownership); errors logged via zap with request/job IDs; outbound HTTP centrally validated for SSRF (scheme/userinfo/private-IP at dial time, redirect re-validation, response size caps, per-host rate limiting) — this part is genuinely well built; auth is a single optional Basic Auth boundary; no metrics/traces (logs only); RSS/OPML endpoints expose library (behind same optional auth).

### Hotspots (rank = churn × size × criticality; churn is computed, risk is judgment)

| # | Location | Churn (12 mo) | Lines | Why it matters | Risk |
|---|---|---|---|---|---|
| 1 | `service/podcastService.go` | 18 | 1,340 | Feed refresh, add/delete, state transitions, locks | High |
| 2 | `db/dbfunctions.go` | 28 | 892 | All persistence; `Save` semantics; locks | High |
| 3 | `controllers/podcast.go` | 16 | 1,019 | Biggest API surface; silent error paths | High |
| 4 | `service/whisperx.go` | 15 | 1,116 | Long-running subprocess orchestration | Med-High |
| 5 | `service/fileService.go` | 16 | 653 | Download loop, backups, folder ops | High |
| 6 | `main.go` | 26 | 337 | Routes, auth, middleware, cron | High |
| 7 | `db/migrations.go` | 23 | 322 | Hand-rolled migrations | Med |
| 8 | `controllers/settings.go` | 19 | 478 | Settings patch surface | Med |
| 9 | `db/podcast.go` (models) | 25 | 275 | Schema source of truth | Med |
| 10 | `service/summarize.go` | 8 | 322 | LLM client; called from refresh & transcription | Med |
| 11 | `frontend/src/views/SettingsView.vue` | 17 | 1,893 | God-view | Med (UI) |
| 12 | `controllers/websockets.go` | — | 184 | Hub blocks on slow clients | Med |

---

## 5. Critical findings

Severity: Critical / High / Medium. Confidence: Confirmed (snippet re-verified 2026-07-05 against `6533936`) / Likely / Hypothesis.

---

**[F1] Whole-row `Save` causes lost updates on `podcast_items` — High, Confirmed, Correctness/Data integrity**
`db/dbfunctions.go:551-553`:
```go
func UpdatePodcastItem(podcastItem *PodcastItem) error {
	tx := DB.Omit("Podcast").Save(&podcastItem)
```
Used by ~15 state setters (`SetPodcastItemAsDownloaded`, `SetPodcastItemPlayedStatus`, …) and by workers that hold a row in memory for hours: `service/whisperx.go:472` saves the full item after transcription completes (`if err := db.UpdatePodcastItem(&item); err != nil {`). Any field changed by the user or another job between load and save (played, bookmark, deletion, download status) is silently overwritten with stale values.
**Why it matters:** user-visible state loss; can resurrect deleted episodes; worst on multi-hour transcriptions.
**Action:** replace state transitions with column-scoped `Updates` (pattern already exists: `UpdatePodcastItemDownloadProgress`, `dbfunctions.go:349-357`). Characterization tests first.
**Tests needed:** state-transition characterization (see P1). **Blocks refactoring:** yes — do before other service refactors.

---

**[F2] Check-then-act job locks allow concurrent duplicate jobs — High, Confirmed, Correctness/Concurrency**
`service/podcastService.go:1047-1052` (`RefreshEpisodes`):
```go
lock := db.GetLock(jobName)
if lock.IsLocked() { ... return nil }
jobLock := db.Lock(jobName, 120)
```
Same idiom in `DownloadMissingEpisodes` (`podcastService.go:807-812`) and `ApplyRetentionPolicies` (`service/retention.go:31-36`). `db.Lock` is an unconditional upsert — two racers both pass `IsLocked()`, both "acquire". Concurrent triggers exist: cron plus `go RefreshEpisodes()` from `AddPodcast`/`DownloadAllEpisodesByPodcastID`/`UploadOpml` handlers. An atomic `db.TryLock` with lease + compare-and-swap already exists (`db/dbfunctions.go:665-727`) and is used by transcription (`service/whisperx.go:217`).
**Why it matters:** duplicate feed fetches/downloads, doubled writes amplifying F1, wasted bandwidth on every collision.
**Action:** migrate the three jobs to `TryLock` (+ heartbeat where runs can exceed the lease). **Tests:** lock-contention unit tests (exist for TryLock; extend to these jobs). **Blocks refactoring:** no, but do it first — it's cheap.

---

**[F3] `AddPodcast` misreports DB errors as "already exists"; no unique index on URL — High, Confirmed, Correctness**
`service/podcastService.go:273-318`:
```go
err := db.GetPodcastByURL(url, &podcast)
...
if errors.Is(err, gorm.ErrRecordNotFound) { ... create ... }
return podcast, &model.PodcastAlreadyExistsError{URL: url}
```
Any non-NotFound error (DB locked, I/O) falls through to the 409 "already exists" branch with a zero-value podcast. `Podcast.URL` has no unique constraint (`db/podcast.go:19`, no index migration), so two concurrent adds of the same URL insert duplicates.
**Action:** handle `err != nil && !NotFound` explicitly; dedupe then add unique index (data-changing migration).

---

**[F4] Deprecated GET routes perform destructive/state-changing actions (CSRF) — High, Confirmed, Security**
`main.go:143`:
```go
router.GET("/podcastitems/:id/delete", controllers.DeletePodcastItem) // deprecated
```
Also GET aliases for download (`main.go:106`, `:135`), pause/unpause (`:110`, `:112`), markPlayed/Unplayed (`:126`, `:128`), bookmark/unbookmark (`:130`, `:132`). GET + cookie-less Basic Auth is exactly the combination browsers auto-replay: any page an operator visits can fire `<img src="http://nas:8080/podcastitems/<id>/delete">`. With `PASSWORD` unset (default, `.env.example` ships `PASSWORD=`) the API is open to anyone with network reach — the README frames this as a LAN app, but compose maps the port broadly (`0.0.0.0` default).
**Why it matters:** drive-by data loss on the exact deployment profile this app targets (NAS on a home LAN).
**Action:** remove the GET aliases (API-changing; frontend already uses the verb routes — `frontend/src/lib/api/*` use POST/PATCH/DELETE). Escalate: **if any instance is internet-exposed without a password, treat as stop-the-line.**

---

**[F5] Committed `.claude/settings.local.json` grants `bypassPermissions` to any agent in this repo — Medium, Confirmed, Security/Dev-tooling**
`.claude/settings.local.json:2-3`:
```json
"permissions": {
    "defaultMode": "bypassPermissions",
```
This file is tracked, so every contributor's coding agent starts in bypass mode with blanket `Bash`/`Write`/`WebFetch` allows — silently overriding the carefully restricted tracked `settings.json` next to it. Per assessment rules this is also flagged as repository content that relaxes agent guardrails (it looks like local convenience that got committed, not an attack).
**Action:** delete from git, add `.claude/settings.local.json` to `.gitignore` (upstream default behavior), keep the restrictive `settings.json`.

---

**[F6] Download loop: fd leak on error paths; incomplete downloads delete resumable partials — Medium, Confirmed, Reliability**
`service/fileService.go:159-161` (and read-error path `:171-177`) return without closing `file`; the `defer file.Close()` is only registered after the loop (`:182`):
```go
if _, writeErr := file.Write(buffer[:n]); writeErr != nil {
	logError("error saving file", writeErr, "path", finalPath, "url", link)
	return "", writeErr
```
And `:188-190` deletes the partial file on short reads, defeating the resume support built at `:81-89`:
```go
if totalBytes > 0 && downloadedBytes < totalBytes {
	_ = os.Remove(finalPath)
	return "", fmt.Errorf("download incomplete: ...")
```
**Why it matters:** flaky feeds are the norm; leaked descriptors accumulate in a long-lived process; deleting partials turns transient network blips into full re-downloads.
**Action:** close on all paths (single `defer` after open); decide partial-retention policy with maintainer.

---

**[F7] Backup job: nil-deref risk and non-transactional SQLite copy; no Postgres story — Medium, Confirmed (code) / Likely (corruption), Reliability/Data**
`service/fileService.go:411-413` — `filepath.Walk` callback ignores its `err` and dereferences possibly-nil `info`:
```go
err := filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
	if !info.IsDir() {
```
`service/fileService.go:487` backs up only the raw DB file while the app is writing to it, and misses `-wal`/`-shm` sidecars:
```go
dbPath := path.Join(configPath, "briefcast.db")
```
On Postgres deployments the 48 h `CreateBackup` cron simply errors ("could not find db file") forever.
**Action:** use SQLite's online backup (`VACUUM INTO` is available via a plain `Exec`), handle Walk errors, and either implement or explicitly log-and-disable backups under Postgres.

---

**[F8] Stale third-party API key in committed comment — Medium, Confirmed, Security/Secrets**
`service/podcastService.go:1216-1217` contains two commented-out Goodreads URLs embedding what appears to be a real (legacy) Goodreads API key. Value intentionally not reproduced here.
**Action:** delete both comment lines; treat the key as compromised and rotate/revoke if the account still exists. Gitleaks runs in CI (`.github/workflows/secret-scan.yml`) but evidently doesn't match this pattern — consider a custom rule. History rewrite is not warranted for a dead service, but note the key remains in git history.

---

**[F9] Settings row fetched from DB on every request and every outbound HTTP request — Medium, Confirmed, Performance**
`main.go:194` (middleware, runs for every route including static and media streaming):
```go
setting := db.GetOrCreateSetting()
c.Set("setting", setting)
```
plus `service/fileService.go:569-573` inside `getRequestWithMethod` — one more settings query per outbound fetch. `GetOrCreateSetting` (`db/dbfunctions.go:592-625`) also performs up to four self-healing writes.
**Action:** cache with short TTL or invalidate-on-write; only attach to routes that need it.

---

**[F10] Cron/search queries drag full transcript+summary text through memory — Medium, Confirmed, Performance**
`db/dbfunctions.go:298-302` — `CheckMissingFiles` loads every downloaded item with `Preload(clause.Associations)` and all text blobs every 15 minutes:
```go
func GetAllPodcastItemsAlreadyDownloaded() (*[]PodcastItem, error) {
	var podcastItems []PodcastItem
	result := podcastItemsWithAssociations(DB).Where("download_status=?", Downloaded).Find(&podcastItems)
```
Same shape for `GetAllPodcastItemsToBeDownloaded` (`:291-295`), RSS (`GetAllPodcastItemsByPodcastIds`, `:226-230`), `ExportAll` (`service/export.go:162-165`, unbounded), and local search matching on `lower(transcript_json) like ?` (`db/search_helpers.go:29`). A lightweight column list already exists (`podcastItemListColumns`, `dbfunctions.go:200-214`) but is used by only one query.
**Why it matters:** with hundreds of transcribed episodes (multi-MB text each) these become the dominant memory/CPU cost on NAS hardware.
**Action:** column-scope the cron queries; move transcript search to dedicated snippet queries (service layer already post-processes matches).

---

**[F11] WebSocket hub can be wedged by one slow client — Medium, Confirmed (pattern), Reliability**
`controllers/websockets.go:92-97` — `WriteJSON` with no write deadline, executed inside the single `Run` goroutine that owns all state; `getItemsToPlay` DB queries also run in this loop (`:140`). A stalled TCP peer blocks the hub; all `Handler` goroutines then block sending to the unbuffered `h.broadcast` channel.
**Action:** set write deadlines + ping/pong, drop slow clients, move DB work off the hub goroutine.

---

**[F12] No HTTP server timeouts — Medium, Confirmed, Reliability**
`main.go:55`: `if err := r.Run(); err != nil {` — default `http.Server` (no `ReadHeaderTimeout`, `IdleTimeout`). Slow-loris and dead connections accumulate; media streaming means `WriteTimeout` must stay 0/large, but header/idle timeouts are free.
**Action:** explicit `http.Server` with `ReadHeaderTimeout`/`IdleTimeout`.

---

**[F13] Feed refresh does synchronous LLM calls and transcript/chapter fetches per new episode — Medium, Confirmed, Performance/Reliability**
`service/podcastService.go:498-506` (inside `AddPodcastItems` insert loop):
```go
if summarizationEnabled && transcriptStatus == "available" && ... {
	if sumErr := SummarizeEpisode(&podcastItem, llmCfg, ...); sumErr != nil {
```
plus network fetches at `:407` (chapters) and `:427` (transcripts). A single podcast with N new transcript-bearing episodes serializes N LLM calls (default timeout 120 s each) inside the refresh worker, starving the pool and holding the job lock.
**Action:** persist rows with `llm_summary_status='pending'` and let the existing backfill/repair machinery summarize asynchronously.

---

**[F14] SQLite opened without busy-timeout/WAL configuration — Medium, Likely, Reliability**
`db/db.go:40` opens with default `gorm.Config{}`; `normalizeSQLiteDSN` (`db/config.go:134-165`) passes DSNs through without pragmas, and `applyConnectionPool` allows up to 25 connections. Under concurrent writers (downloads + transcription + API) the glebarez driver will surface `database is locked` / `SQLITE_BUSY` errors unless pragmas are set. No `_pragma=busy_timeout` or journal-mode setting exists anywhere in the repo (grep: no matches).
**Action:** set `busy_timeout` and `journal_mode=WAL` via DSN params or post-open `Exec`, and cap open conns for SQLite (1 writer). Needs maintainer confirmation of observed symptoms (Hypothesis on frequency, Likely on mechanism).

---

**[F15] Dead legacy UI layer still compiled and shipped — Low, Confirmed, DX/Complexity**
`main.go:68` says "Legacy HTML templates removed; modern Vue app is the only UI", yet `controllers/pages.go` retains 9 page handlers (`AddPage`, `HomePage`, `PodcastPage`, `PlayerPage`, `SettingsPage`, `BackupsPage`, `AllEpisodesPage`, `AllTagsPage`, `AddNewPodcast`) that render templates never loaded (no `LoadHTMLGlob` anywhere — would 500/panic if ever wired), `client/*.html` (13 files) is orphaned, `controllers/podcast.go:411` defines misnamed unrouted `DeletePodcasDeleteOnlyPodcasttEpisodesByID`, and `db/dbfunctions.go:23` `GetPodcastsByURLList` is unused and buggy (`First` into a slice). A stray build artifact `coverage` (159 KB) is git-tracked at repo root. `Base.DeletedAt *time.Time` (`db/base.go:15`) is not `gorm.DeletedAt`, so soft delete never happens — the column and index are dead weight and the name is misleading.
**Action:** mechanical deletion PR + decide soft-delete intent.

---

**[F16] Handlers that return nothing (or wrong status) on error — Medium, Confirmed, Correctness/API**
`controllers/podcast.go:101-117` (`GetAllPodcasts` — bind failure ⇒ empty 200), `:783-786` (`GetTagByID` — DB error ⇒ empty 200), `:935-937` (`DeleteTagByID`), `:969-973`/`:983-987` (tag add/remove), `controllers/pages.go:336-355` (`Search`), and `GetRssForPodcastByID` writes 400 then keeps working (`:861-863` missing `return`).
**Action:** one sweep adding explicit error responses; pin with handler tests (behavior-changing: silent 200 → 4xx/5xx).

---

**[F17] `PatchPodcastItemByID` can't set false/empty values and ignores errors — Medium, Confirmed, Correctness**
`controllers/podcast.go:696`:
```go
db.DB.Model(&podcast).Updates(input)
```
Struct-based `Updates` skips zero values: `{"isPlayed": false}` or clearing a title is silently ignored; the `*gorm.DB` error is unchecked. Frontend uses dedicated markPlayed/markUnplayed routes, so blast radius is small today.
**Action:** bind to pointer struct, build a column map, check the error.

---

**[F18] Bookmark and summary-favorite are conflated — Low, Confirmed (code) / Hypothesis (intent)**
`service/podcastService.go:666` (`SetPodcastItemBookmarkStatus` sets `IsSummaryFavorited = bookmark`) and `db/dbfunctions.go:420-429` (`SetSummaryFavorited` writes `bookmark_date`). Unbookmarking an episode silently unfavorites its summary and vice versa. Maintainer question: is this deliberate "one heart" semantics?

---

**[F19] `UpdateSettings` 14-positional-parameter API + duplicate settings write paths — Low, Confirmed, Complexity/API**
`service/podcastService.go:1280-1282` (9 bools in a row) fed by legacy `POST /settings` (`main.go:185`), while `PATCH /settings` (`controllers/settings.go:143`) is the modern pointer-based path covering different fields. Two write paths, one full-row `Save` each — concurrent PATCHes last-write-win.

---

**No exploitable-right-now RCE/injection was found.** SQL is parameterized throughout (raw queries use placeholders/named args); `exec.Command` uses fixed argv (no shell); path deletion is fenced by `IsPathWithinAssetsDir` with symlink resolution (`service/asset_paths.go:33-61`); outbound SSRF policy pins IPs at dial time (`service/outbound_url_policy.go:98-148`); XML parsing uses `encoding/xml` (entity expansion not supported by design) with a 5 MB OPML cap. The security posture concerns are the auth model and CSRF-able GETs above, plus repository hygiene items [F5][F8].
