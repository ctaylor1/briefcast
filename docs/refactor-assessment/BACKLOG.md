# Briefcast — Prioritized Refactor Backlog

- **Date:** 2026-07-05 · **Commit:** `main` @ `6533936` (65339368a354821b563517b46d07485da6756d6b)
- Scoring: `Total = (2 × Risk) + Blast + Frequency + Confidence + Payoff − Effort`. Every dimension justified inline.
- **Baseline caveat:** no build/test commands could be run during this assessment (toolchain unavailable in sandbox; installs forbidden). All verification commands below come from `justfile`/CI and must be run by the implementer: `go test ./...`, `go test ./service -run <Name>`, `npm --prefix frontend run test:unit`, `npm --prefix frontend run build`, `just ci-python`. Establish a green baseline with `just test-full` before PR 1.
- Default refactor type is **behavior-preserving**; anything else is labeled.

## Implementation status — 2026-07-10 / v1.9.4

- **Completed:** P2 / PR1 (atomic background-job locking).
- **Completed:** P12 / PR2 (repository hygiene and stale credential-comment removal).
- **Completed:** P6 / PR3 (download cleanup and resumable partial retention).
- **Completed:** P1a / PR4 (state-transition characterization tests).
- **Next:** P1b / PR5 (column-scoped state-transition persistence). P1 remains open until this implementation slice lands.

---

## P1 — Replace whole-row `Save` with column-scoped state-transition updates (F1) — **Score 24** — P1a complete, P1b pending

- **Category:** Correctness / Data integrity
- **Score:** Risk 4 (silent user-visible state loss, can resurrect deleted episodes — not quite "outage" so not 5) ×2 = 8 · Blast 5 (`PodcastItem` is the core entity; ~15 setters + 3 workers) · Freq 4 (every download/transcription/user action; collisions guaranteed on multi-hour transcriptions) · Conf 5 (Confirmed, snippets) · Payoff 5 (removes an entire class of lost-update bugs) · Effort −3 (M: 2–3 PRs incl. tests)
- **Effort:** M · **Confidence:** Confirmed
- **Locations:** `db/dbfunctions.go:551-554` (`UpdatePodcastItem` → `DB.Omit("Podcast").Save`); callers doing load-modify-save: `service/podcastService.go:572-616` (`SetPodcastItemAsQueuedForDownload`, `...PreserveProgress`, `...AsDownloading`, `...AsPaused`), `:671-730` (`SetPodcastItemAsDownloaded`), `:733-746` (`SetPodcastItemAsNotDownloaded`), `:749-757` (`SetPodcastItemPlayedStatus`), `:655-668` (`SetPodcastItemBookmarkStatus`); long-holding workers: `service/whisperx.go:382-395`, `:451-453`, `:458-475`; `service/summarize.go:284-319` (`SummarizeEpisode`); `service/whisperx.go:986-1001` (`resetItemForRedownload`).
- **Current problem:** every state change loads the full row, mutates fields in memory, and saves all columns back. Workers hold rows for minutes-to-hours (WhisperX), then overwrite everything — including fields changed meanwhile by the user (played, bookmark) or other jobs (download status, file size). `UpdatePodcastItemDownloadProgress` (`db/dbfunctions.go:349-357`) already shows the correct column-scoped pattern.
- **Why it matters:** correctness of the library state machine is the product; lost updates are silent and unreproducible for users.
- **Evidence:** `tx := DB.Omit("Podcast").Save(&podcastItem)` (`db/dbfunctions.go:552`); `if err := db.UpdatePodcastItem(&item); err != nil {` after transcript completion (`service/whisperx.go:472`) — the `item` there was loaded before a transcription that can run for hours (timeout default 21,600 s, `whisperx.go:81`).
- **Refactor type:** behavior-preserving (intent-preserving; concurrency behavior improves)
- **Target state:** a small set of intent-named, column-scoped update functions in `db/` (e.g. `MarkDownloaded(id, path, size, ...)`, `MarkTranscriptComplete(id, transcriptJSON, canonical..., model)`, `SetPlayed(id, bool)`, `SetBookmark(id, ...)`), each using `Model(&PodcastItem{}).Where("id = ?", id).Updates(map[...])`; `UpdatePodcastItem` (whole-row) reserved for creation-adjacent flows and eventually deleted.
- **Step plan:**
  1. *Test-only PR:* characterization tests for each transition — given a stored item, call setter, assert exactly the intended columns changed (compare full row before/after) for: queued, downloading, paused, downloaded (incl. ID3 branch), not-downloaded, played, bookmark, transcript complete, transcript failed/retry, redownload reset, summary success/failure.
  2. Add column-scoped functions in `db/` mirroring each transition; port `service/podcastService.go` setters one-by-one; keep signatures identical.
  3. Port the workers (`whisperx.go`, `summarize.go`): completion writes only transcript/summary columns; progress callback already column-scoped for bytes — extend the same style to transcript progress fields.
  4. Delete or quarantine `UpdatePodcastItem` full save (grep for remaining callers; `alternate_feeds.go:81` and `AddPodcastItems` are near-insert-time and low risk — port last).
- **Tests to add:** transition characterization suite (`service/` or `db/` level, temp SQLite); a concurrency regression test: start a slow "transcription" (stubbed), flip `IsPlayed` mid-flight, assert played survives completion.
- **Verification:** `go test ./...` (must be green in baseline first); `go test ./service -run TestIntegrationFeedDownloadWhisperX`.
- **Risks / edge cases:** transcript completion touches many columns — enumerate them from `whisperx.go:458-471` exactly; `SetPodcastItemAsDownloaded` mixes file-size lookup and ID3 extraction — keep that logic in service, only persistence changes. GORM `Updates(map)` doesn't run hooks — fine (only hook is UUID-on-create).
- **Maintainer questions:** none blocking.

---

## P2 — Unify job locking on atomic `TryLock` (F2) — **Score 23** — completed in v1.9.4

- **Category:** Correctness / Concurrency
- **Score:** Risk 4 (duplicate concurrent jobs double-write state and hammer feeds; amplifies F1) ×2 = 8 · Blast 3 (jobs subsystem) · Freq 4 (every cron tick + every API-triggered refresh can collide) · Conf 5 (Confirmed) · Payoff 4 (one locking idiom, removes a race class) · Effort −1 (S)
- **Effort:** S · **Confidence:** Confirmed
- **Locations:** `service/podcastService.go:1047-1053` (`RefreshEpisodes`), `service/podcastService.go:807-813` (`DownloadMissingEpisodes`), `service/retention.go:31-37` (`ApplyRetentionPolicies`); reference implementation `db/dbfunctions.go:665-727` (`TryLock`) + heartbeat `service/whisperx.go:504-530`, exemplar usage `service/whisperx.go:217-241`.
- **Current problem:** `GetLock` → `IsLocked()` → `Lock()` is check-then-act; `Lock` (`db/dbfunctions.go:642-661`) unconditionally upserts, so two concurrent callers both proceed. API handlers spawn `go RefreshEpisodes()` (`controllers/podcast.go:462-466`, `:748-752`; `service/podcastService.go:27-29` OPML) concurrently with cron.
- **Why it matters:** double-runs duplicate feed fetches and downloads and interleave whole-row saves (F1), producing the hardest-to-diagnose state corruption in the app.
- **Evidence:** `lock := db.GetLock(jobName)` / `if lock.IsLocked() {` / `jobLock := db.Lock(jobName, 120)` (`podcastService.go:1047-1052`).
- **Refactor type:** behavior-preserving
- **Target state:** all four background jobs acquire via `db.TryLock(name, leaseMins)`; jobs that can exceed the lease get `startJobLockHeartbeat` (already generic); `db.GetLock`+`db.Lock` demoted to test helpers or deleted.
- **Step plan:** (1) swap idiom in the three jobs, mirroring `whisperx.go:217-241` including the `!acquired → log + return nil` path; (2) pick sane leases (RefreshEpisodes/Downloads currently 120 min — keep, add heartbeat to downloads which can legitimately run long); (3) grep for remaining `db.Lock(`/`db.GetLock(` callers and clean up.
- **Tests to add:** per-job "second acquisition is rejected" tests (pattern exists for TryLock in `db/dbfunctions_test.go` — extend); a two-goroutine race test on `RefreshEpisodes` with a stubbed feed asserting single execution.
- **Verification:** `go test ./...`; focused: `go test ./service -run Refresh`, `go test ./db -run Lock`.
- **Risks:** none notable — TryLock is already production-proven by the transcription job. Unlock path must use the ID returned by TryLock (it does in the exemplar).
- **Maintainer questions:** none.

---

## P3 — Remove state-changing GET route aliases (F4) — **Score 22**

- **Category:** Security (CSRF) / API
- **Score:** Risk 4 (drive-by destructive actions in the app's target deployment; not 5 because it requires a victim browser on the LAN or an exposed instance) ×2 = 8 · Blast 3 (API surface only) · Freq 3 (routes always live) · Conf 5 (Confirmed) · Payoff 4 (removes the CSRF class for mutations) · Effort −1 (S)
- **Effort:** S · **Confidence:** Confirmed
- **Locations:** `main.go:106, 110, 112, 126, 128, 130, 132, 135, 143` (all lines marked `// deprecated`).
- **Current problem:** `GET /podcastitems/:id/delete`, `/download`, `/markPlayed`, `/markUnplayed`, `/bookmark`, `/unbookmark`, `GET /podcasts/:id/download|pause|unpause` mutate state. GET requests are sent cross-origin by `<img>`/`<link>` tags with Basic Auth credentials attached automatically once cached.
- **Evidence:** `router.GET("/podcastitems/:id/delete", controllers.DeletePodcastItem) // deprecated` (`main.go:143`).
- **Refactor type:** **API-changing / behavior-changing** (removes deprecated endpoints).
- **Target state:** only verb-appropriate routes remain (they already exist and the Vue client uses them — `frontend/src/lib/api/episodes.ts`, `podcasts.ts` use POST/PATCH/DELETE).
- **Step plan:** (1) delete the nine GET registrations; (2) `grep -rn "GET.*deprecated" main.go` → zero; (3) grep frontend `dist`-independent sources and docs/README for the GET forms; update `docs/external_ingest.md` if referenced; (4) CHANGELOG entry: external scripts/bookmarklets must switch verbs.
- **Tests to add:** route-table test asserting these paths 404/405 for GET (extend `TestBuildRouterRegistersRoutes`, `main_test.go:43`); keep positive tests for verb routes.
- **Verification:** `go test ./...`; `npm --prefix frontend run build` (unchanged client, sanity).
- **Rollback:** revert PR (no data impact).
- **Risks / edge cases:** users with podcast apps hitting `GET /podcasts/:id/rss` are unaffected (read-only GET stays). Anyone scripting the deprecated GETs breaks — that's the point; release notes required.
- **Maintainer questions:** are there known external consumers (e.g., Home Assistant automations) of the GET aliases?

---

## P4 — Server hardening: HTTP timeouts + settings caching (F9, F12) — **Score 22**

- **Category:** Reliability / Performance
- **Score:** Risk 3 (resource exhaustion / connection pile-up on an always-on NAS service) ×2 = 6 · Blast 5 (every request; every outbound fetch) · Freq 5 (per-request hot path) · Conf 4 (Confirmed code; impact Likely) · Payoff 3 · Effort −1 (S)
- **Effort:** S · **Confidence:** Confirmed (code) / Likely (impact)
- **Locations:** `main.go:55` (`r.Run()`), `main.go:191-200` (`setupSettings` middleware), `service/fileService.go:568-574` (settings query per outbound request), `db/dbfunctions.go:592-625` (`GetOrCreateSetting` self-healing writes).
- **Current problem:** default `http.Server` (no header/idle timeouts); settings singleton re-read from DB on every inbound request and every outbound HTTP request.
- **Refactor type:** behavior-preserving (timeouts) + **behavior-changing (bounded)** for settings freshness (staleness window ≤ TTL).
- **Target state:** explicit `http.Server{ReadHeaderTimeout: 10s, IdleTimeout: 120s}` (WriteTimeout stays 0 — media streaming); a `service/settingscache` with ~5 s TTL + explicit invalidation from the two settings write paths; middleware attached only where `c.MustGet("setting")` is actually used (`getBaseURL` in RSS/OPML paths).
- **Step plan:** (1) introduce `http.Server` wrapper around gin engine; (2) add cached accessor + invalidation on `db.UpdateSettings`; (3) swap middleware + `getRequestWithMethod` to cached accessor; (4) delete per-request `X-Clacks-Overhead`? — no, keep (harmless, zero cost).
- **Tests to add:** cache invalidation unit test (write settings → next read reflects change); middleware behavior test for RSS `getBaseURL` fallback.
- **Verification:** `go test ./...`; manual: stream an episode >2 min to confirm no write-timeout regression (documented manual step).
- **Risks:** stale settings within TTL (pause-downloads uses process-local atomics, unaffected); pick conservative TTL.
- **Maintainer questions:** none.

---

## P5 — SQLite concurrency configuration: busy_timeout + WAL + writer cap (F14) — **Score 22**

- **Category:** Reliability
- **Score:** Risk 4 (intermittent `SQLITE_BUSY` failures across every subsystem under concurrent writes) ×2 = 8 · Blast 5 (all persistence) · Freq 4 (concurrent writers are the normal mode: downloads + transcription + API) · Conf 3 (Likely — mechanism certain, incidence unverified without runtime) · Payoff 3 · Effort −1 (S)
- **Effort:** S · **Confidence:** Likely
- **Locations:** `db/db.go:40` (`gorm.Open(dialector, &gorm.Config{})`), `db/config.go:134-165` (`normalizeSQLiteDSN` — passes DSN through, no pragmas), `db/config.go:183-197` (`applyConnectionPool` — up to 25 open conns for both drivers).
- **Current problem:** no `busy_timeout`, no explicit journal mode, and a 25-connection pool against SQLite. Also directly worsens F7 (WAL sidecars unaccounted in backups) once WAL is enabled — coordinate with P7.
- **Refactor type:** behavior-preserving (config)
- **Target state:** after `gorm.Open` for SQLite: `PRAGMA busy_timeout=10000; PRAGMA journal_mode=WAL;` (via `DB.Exec`), and SQLite-specific pool caps (`SetMaxOpenConns(1)` for writes is the blunt-but-correct option with glebarez; alternatively keep small pool + busy_timeout and measure). Postgres path untouched.
- **Step plan:** (1) add driver-conditional post-open configuration in `db.Init`; (2) make pool env defaults driver-aware; (3) document in README config section.
- **Tests to add:** unit test asserting pragmas applied for sqlite driver (query `PRAGMA busy_timeout`); existing suite exercises temp SQLite heavily — watch for behavior shifts.
- **Verification:** `go test ./...`.
- **Rollback note:** WAL mode persists in the DB file after rollback (harmless — SQLite reads it fine either way; delete `-wal`/`-shm` after clean shutdown if reverting deliberately).
- **Maintainer questions:** have `database is locked` errors been observed in logs? (Would raise Risk to Confirmed-5 and justify the single-writer cap immediately.)

---

## P6 — Download loop hygiene: close files on all paths; keep resumable partials (F6) — **Score 20** — completed in v1.9.4

- **Category:** Correctness / Reliability
- **Score:** Risk 3 (fd leak in long-lived process; lost resume progress) ×2 = 6 · Blast 3 (download subsystem) · Freq 4 (hot path, flaky feeds routine) · Conf 5 (Confirmed) · Payoff 3 · Effort −1 (S)
- **Effort:** S · **Confidence:** Confirmed
- **Locations:** `service/fileService.go:112-196` (`Download`): error returns at `:160-161` (write error) and `:175-176` (read error) leak `file`; `defer file.Close()` mis-placed at `:182`; partial-file deletion at `:184-191`.
- **Current problem:** see F6. Note duplicate `defer resp.Body.Close()` risk is absent (closes are explicit on early paths) but `file` is not closed on the two mid-loop error returns; incomplete downloads delete partials that the resume logic (`:81-89`) exists to exploit.
- **Refactor type:** behavior-preserving (fd fix) + **behavior-changing** (partial retention — flag it in PR)
- **Target state:** single `defer file.Close()` immediately after successful open; on `downloadedBytes < totalBytes`, keep the partial and return a typed retryable error (resume picks it up next run); empty-file (0 bytes) deletion stays.
- **Step plan:** (1) restructure open/defer; (2) introduce `ErrDownloadIncomplete`; caller `DownloadMissingEpisodes` already requeues on generic error (`podcastService.go:874-882`) — verify requeue keeps `NotDownloaded` (it does, `:881`); (3) tests.
- **Tests to add:** httptest server that drops connection mid-body → assert file closed (no descriptor growth via `t.TempDir` re-open), partial retained, item requeued; 416-retry path regression test (`:98-107`).
- **Verification:** `go test ./service -run Download`; `go test ./...`.
- **Risks:** servers that don't support ranges → next attempt gets 200, code already truncates via `os.Create` (`:117-122`). Disk usage from retained partials — bounded by retry cadence; mention in PR.
- **Maintainer questions:** intended policy for partials on *persistent* failure (delete after N attempts?).

---

## P7 — Backup integrity: online SQLite backup, WAL sidecars, Walk error handling, Postgres story (F7) — **Score 19**

- **Category:** Reliability / Data
- **Score:** Risk 4 (backups are the last line against data loss; current ones may be silently inconsistent) ×2 = 8 · Blast 3 · Freq 2 (48 h cron, restore is rare-but-critical) · Conf 4 (code Confirmed; corruption incidence Likely) · Payoff 4 (trustworthy restores) · Effort −2 (S/M)
- **Effort:** S/M · **Confidence:** Confirmed (code), Likely (impact)
- **Locations:** `service/fileService.go:475-504` (`CreateBackup` — raw copy of live `briefcast.db` only), `:408-419` (`GetAllBackupFiles` — Walk callback ignores `err`, nil-`info` deref), `:430-446` (`deleteOldBackup` keeps 5), `:519-524` (tar header uses absolute `filePath` as `Name`).
- **Refactor type:** behavior-preserving (better backups) — **data-adjacent, treat as risky**
- **Target state:** SQLite: `VACUUM INTO '<tmp>'` then tar the vacuumed copy (transactionally consistent, WAL-independent); Walk callback checks `err != nil || info == nil`; tar entry name relative (`briefcast.db`); Postgres: log a clear "backups not supported for postgres; use pg_dump" warning once instead of erroring every 48 h.
- **Step plan:** (1) fix Walk error handling (2 lines, ship with P13 if preferred); (2) switch to `VACUUM INTO` via `db.DB.Exec` + temp file; (3) tar relative name (restore-tooling note in CHANGELOG since existing tarballs used absolute paths); (4) Postgres guard.
- **Tests to add:** backup-then-open test: create DB with rows, run `CreateBackup` while a writer goroutine spins, untar, open copy, assert row count & `PRAGMA integrity_check` ok; retention test exists (`retention_test.go`) — extend for `deleteOldBackup` ordering.
- **Verification:** `go test ./service -run Backup`; `go test ./...`.
- **Rollback note:** revert is safe; old and new tarballs must both be restorable — document both layouts.
- **Maintainer questions:** is anyone restoring these tarballs today (layout compatibility)?

---

## P8 — Decouple summarization & transcript/chapter fetching from feed refresh (F13) — **Score 19**

- **Category:** Performance / Reliability
- **Score:** Risk 3 (refresh starvation, lock held for hours in worst case) ×2 = 6 · Blast 3 (refresh pipeline) · Freq 4 (every cron tick with new episodes) · Conf 5 (Confirmed) · Payoff 4 (predictable refresh; retries owned by one subsystem) · Effort −3 (M)
- **Effort:** M · **Confidence:** Confirmed
- **Locations:** `service/podcastService.go:404-445` (chapter/transcript fetches inside insert loop), `:495-507` (synchronous `ExportTranscript` + `SummarizeEpisode` per new episode); async machinery that already exists: `service/summary_backfill.go`, `service/repair_work.go`, transcription cron.
- **Current problem:** a new podcast with a transcript-rich back catalog serializes N LLM calls (120 s timeout each) inside a refresh worker while the `RefreshEpisodes` lock is held.
- **Refactor type:** **behavior-changing** (summaries become eventually-consistent; refresh returns sooner)
- **Target state:** `AddPodcastItems` persists rows with `llm_summary_status = "pending"` (new status) and transcript assets fetched with a small bounded budget (or also deferred); a summarization pass (reuse `BackfillSummaries`/repair-work loop, respecting `Setting.LLMConcurrency`) picks up pending rows on its own cadence.
- **Step plan:** (1) characterization test capturing current statuses after `AddPodcastItems` with feed-provided transcripts; (2) introduce `pending` summary status + repair-work pickup (extend `GetRepairWorkStatus` counts); (3) remove `SummarizeEpisode` call from the loop; (4) optional follow-up: defer transcript-asset fetches the same way.
- **Tests to add:** refresh-with-transcripts completes without LLM stub being called; backfill stub summarizes pending rows; status surfacing in `/settings/repair-work`.
- **Verification:** `go test ./service -run "Summar|Refresh|Repair"`; `go test ./...`.
- **Risks:** UI expectation that summary exists immediately after add — check `SummariesView` polling (it lists `available` only, safe); document latency change.
- **Maintainer questions:** acceptable summary latency target?

---

## P9 — Fix silent/incorrect handler error paths (F16, F17) — **Score 19**

- **Category:** Correctness / API
- **Score:** Risk 3 (empty 200s hide failures from UI and operators) ×2 = 6 · Blast 3 (a dozen endpoints) · Freq 3 · Conf 5 (Confirmed) · Payoff 3 · Effort −1 (S)
- **Effort:** S · **Confidence:** Confirmed
- **Locations:** `controllers/podcast.go:98-118` (`GetAllPodcasts` bind-fail ⇒ empty 200), `:780-790` (`GetTagByID`), `:930-941` (`DeleteTagByID`), `:965-991` (`AddTagToPodcast`/`RemoveTagFromPodcast`), `:856-877` (`GetRssForPodcastByID` missing `return` after 400 at `:862-863`), `controllers/pages.go:333-357` (`Search`), `controllers/podcast.go:675-702` (`PatchPodcastItemByID`: struct `Updates` skips zero values; unchecked error at `:696`).
- **Refactor type:** **behavior-changing** (silent 200 → explicit 4xx/5xx; PATCH accepts false/empty)
- **Target state:** every handler writes exactly one response; `PatchPodcastItem` binds `*bool`/`*string` and builds a column map, checking `.Error`.
- **Step plan:** mechanical sweep + handler tests per endpoint pinning status codes; do `PatchPodcastItemByID` as its own commit (semantic change).
- **Tests to add:** table-driven handler tests (pattern exists in `controllers/controllers_test.go`) for each fixed path; PATCH zero-value regression (`isPlayed:false` actually persists).
- **Verification:** `go test ./controllers`; `go test ./...`; `npm --prefix frontend run test:unit` (client error-message extraction reads `message`/`error` keys — shapes preserved).
- **Risks:** UI code paths that relied on empty-200; grep frontend for affected endpoints (tags UI, search).
- **Maintainer questions:** none.

---

## P10 — `AddPodcast` error handling + unique podcast URL (F3) — **Score 18**

- **Category:** Correctness / Data integrity
- **Score:** Risk 3 (wrong 409s mask DB failures; duplicate podcasts corrupt library) ×2 = 6 · Blast 3 · Freq 3 (add/OPML-import paths) · Conf 5 · Payoff 4 · Effort −3 (M: needs data-dedupe migration)
- **Effort:** M · **Confidence:** Confirmed
- **Locations:** `service/podcastService.go:271-320`; `db/podcast.go:19` (no unique tag); `db/migrations.go` (no URL index); OPML concurrent adds `service/podcastService.go:181-192`.
- **Refactor type:** part behavior-preserving (error branch), part **data/schema-changing** (unique index + dedupe migration)
- **Target state:** `GetPodcastByURL` error handled three-way (found / not-found / real error); unique index on `podcasts(url)` after a dedupe migration (keep oldest, re-point `podcast_items.podcast_id`, merge tags); `CreatePodcast` conflict returns `PodcastAlreadyExistsError`.
- **Step plan:** (1) fix error branch + test (S, ship first); (2) dedupe migration in `db/migrations.go` following existing `DedupeJobLocksByName` precedent (`migrations.go:80-95`) — but this one must re-parent children, so write it as Go migration code, not raw SQL, and dry-run against a copy; (3) add unique index migration; (4) OPML import races now resolve via constraint.
- **Tests to add:** unit: DB-error → error (not 409); migration test with fixture rows incl. duplicate URLs with items attached (pattern: `db/migrations_test.go`).
- **Verification:** `go test ./db -run Migrat`; `go test ./...`.
- **Rollback note:** index drop is trivial; **dedupe is destructive** — migration must be idempotent and log every merged ID; recommend automatic pre-migration backup (`CreateBackup()` call) before executing.
- **Maintainer questions:** merge policy for duplicate podcasts with diverging items?

---

## Condensed items (P11–P18)

**P11 — Lighten hot-path queries (F10) — Score 18** · Perf · M · Confirmed
`db/dbfunctions.go:291-302` (cron loads all columns + associations), `:226-230` (RSS), `service/export.go:162-165` (unbounded), `db/search_helpers.go:26-36` (`lower(transcript_json) LIKE`). Reuse `podcastItemListColumns` (`dbfunctions.go:200-214`) for cron/RSS; batch `ExportAll` (limit/offset loop); split local search into metadata query + per-item snippet queries. Behavior-preserving. Verify: `go test ./...` + `go test ./service -run "Search|Export"`. (Risk 2·2=4, Blast 3, Freq 5, Conf 5, Payoff 4, Effort −3.)

**P12 — Remove `.claude/settings.local.json` bypassPermissions from git (F5) — Score 17 — completed in v1.9.4** · Security/DX · S · Confirmed
Delete tracked file (`.claude/settings.local.json:2-3` `"defaultMode": "bypassPermissions"`), add to `.gitignore`. Also remove the stray tracked `coverage` artifact (git ls-files shows root `coverage`, 159 KB). Mechanical. Verify: `git ls-files | grep -E '^coverage$|settings.local'` → empty. (Risk 3·2=6, Blast 2, Freq 2, Conf 5, Payoff 3, Effort −1.)

**P13 — Delete dead legacy UI layer & unused/broken db helpers (F15) — Score 13** · DX/Complexity · S · Confirmed
`controllers/pages.go` page handlers (`:54`, `:60`, `:68`, `:148`, `:201`, `:214`, `:260`, `:286`, `:443`) — keep `Search`, `GetOPML`, `UploadOpml`, `SettingModel`, helpers; `client/*.html` (13 files); `controllers/podcast.go:410-425` (`DeletePodcasDeleteOnlyPodcasttEpisodesByID`); `db/dbfunctions.go:22-26` (`GetPodcastsByURLList` — also buggy `First` into slice), `:113-132` (`GetPaginatedPodcastItems`, only dead-page caller), `:525-530` (`GetPodcastByTitleAndAuthor` unused). Mechanical, test-guarded by compile + route test. Verify: `go test ./...`, `npm --prefix frontend run build`. (Risk 1·2=2, Blast 2, Freq 2, Conf 5, Payoff 3, Effort −1... net 13; bumped ordering for review-noise payoff with P12 pairing.)

**P14 — WebSocket hub hardening (F11) — Score 15** · Reliability · M · Confirmed(pattern)
`controllers/websockets.go:90-183`: add `SetWriteDeadline` before every write, ping/pong with read deadlines, per-connection outbound buffer (drop-on-full), move `getItemsToPlay` DB call out of hub goroutine (resolve before enqueueing to hub). Behavior-preserving. Tests: slow-reader test with `httptest` + `gorilla` dialer asserting hub stays responsive. Verify: `go test ./controllers -run Hub`. (Risk 3·2=6, Blast 2, Freq 3, Conf 4, Payoff 3, Effort −3.)

**P15 — Settings write-path consolidation (F19) — Score 15** · Complexity/API · M · Confirmed
Retire `POST /settings` (`main.go:185`) + `UpdateSetting` (`controllers/podcast.go:994-1019`) + 14-arg `service.UpdateSettings` (`service/podcastService.go:1280-1304`) in favor of `PATCH /settings` (extend `SettingsPatch` with the missing download/appearance fields it doesn't cover: `AppendDateToFileName`, `MaxDownloadConcurrency`, `UserAgent`, etc.). **API-changing** — confirm the Vue settings view's calls first (`frontend/src/lib/api/settings.ts`). Verify: `go test ./...`, `npm --prefix frontend run build` + `test:unit`. (Risk 2·2=4, Blast 3, Freq 3, Conf 5, Payoff 3, Effort −3.)

**P16 — CI gates: `go vet` + golangci-lint + frontend tests already present; wire go lint (—) — Score 15** · DX/Testing · S · Confirmed
`.github/workflows/ci.yml` runs `go test` only — no vet/lint/staticcheck for the Go majority of the codebase (Python has ruff+mypy; frontend has tsc via build). Add `go vet ./...` (zero-dependency) to CI and justfile (`just test-go` step or new `lint-go`); golangci-lint optional later (dependency policy: needs approval). Several findings here (unchecked errors, dead code) are vet/lint-detectable. Verify: CI run. (Risk 1·2=2, Blast 3, Freq 3, Conf 5, Payoff 3, Effort −1.)

**P17 — `UpdateAllFileSizes` bounded work (part of F10) — Score 14** · Perf · S · Confirmed
`service/podcastService.go:549-569`: sequential HEAD per sizeless episode every `CHECK_FREQUENCY*3` min, no cap. Add per-run limit (e.g. 200) + skip-after-N-failures column or in-memory backoff; already ordered by pub_date. Verify: `go test ./service -run FileSize`. (Risk 2·2=4, Blast 2, Freq 4, Conf 5, Payoff 2, Effort −1.)

**P18 — Bookmark vs summary-favorite semantics (F18) — Score 12** · Product/Correctness · S · Confirmed(code)/Hypothesis(intent)
`service/podcastService.go:655-668` & `db/dbfunctions.go:420-430` cross-write each other's fields. Blocked on maintainer decision; then either document "one heart" or split fields (small migration). (Risk 2·2=4, Blast 2, Freq 2, Conf 3, Payoff 2, Effort −1.)

**Also logged, unscored (hygiene):** remove stale Goodreads-key comment lines `service/podcastService.go:1216-1217` and rotate the key (F8 — fold into P12's hygiene PR); docker-compose hardcodes `WHISPERX_MAX_CONCURRENCY=4`, `WHISPERX_DIARIZATION=false`, `WHISPERX_MAX_ITEMS=25` while `.env.example` documents different defaults (`docker-compose.yml:28,30-31` vs `.env.example:42,44`) — make them `${VAR:-default}` passthroughs; `Dockerfile` `chmod 777` + `createFolder` `0o777` (`fileService.go:586`) — consider 0o775 with PUID/PGID; validate `ThemeMode`/hours in `PatchSettings` (`controllers/settings.go:225-236`).

---

## PR slicing plan

| PR | Backlog | Goal | Risk | Depends on | Validation | Rollback |
|---|---|---|---|---|---|---|
| 1 | P2 | TryLock everywhere; kill duplicate-job race | Low | baseline green | `go test ./...`, lock-race tests | revert |
| 2 | P12 (+F8 comment removal) | Repo hygiene: bypassPermissions file, tracked `coverage`, key comment | Low | — | `git ls-files` checks, `go test ./...` | revert |
| 3 | P6 | Download fd/partials fixes | Low-Med | 1 | download unit tests + `go test ./...` | revert |
| 4 | P1a (test-only) | Characterization tests for item state transitions | None | 1 | new tests green against current code | n/a |
| 5 | P1b | Column-scoped state transitions (service setters, then workers) | Med | 4 | characterization suite + integration test | revert (no schema change) |
| 6 | P3 | Remove GET mutation aliases | Med (**API-changing**) | — | route tests; release notes | revert |
| 7 | P9 | Handler error-path sweep + PATCH fix | Med (**behavior-changing** statuses) | — | controller tests | revert |
| 8 | P4 | Server timeouts + settings cache | Low-Med | — | `go test ./...` + manual long-stream check | revert |
| 9 | P5 | SQLite pragmas + pool caps | Med (**config/behavioral**) | — | `go test ./...`; monitor for BUSY errors | revert; WAL persists (harmless, documented) |
| 10 | P7 | Backup integrity (`VACUUM INTO`) | Med (**data-adjacent**) | 9 (WAL) | backup-under-write test | revert; both tar layouts restorable |
| 11 | P10 | AddPodcast errors, then URL dedupe **migration** + unique index | High (**data-changing**) | 4,5 | migration fixture tests; auto-backup before dedupe | index drop simple; dedupe irreversible → pre-backup mandatory |
| 12 | P8 | Async summarization decoupling | Med (**behavior-changing** latency) | 5 | refresh/backfill tests | revert; pending rows drain via backfill either way |
| 13 | P13 | Dead-code deletion (mechanical only) | Low | 6 (route removals settle surface) | compile + route tests + frontend build | revert |
| 14 | P11+P17 | Query lightening + bounded size job | Med | 5 | perf-sensitive tests; `go test ./...` | revert |

Mechanical changes (PR 2, 13) never share a PR with behavioral ones; API/schema changes (PR 6, 11) are isolated; PR 11 requires a pre-run backup and migration dry-run; no feature flags exist in this codebase — PR 12's status addition is forward-compatible (unknown status strings are treated as non-available everywhere checked).

---

## Open questions & assumptions

1. **Exposure model:** is any real deployment internet-reachable or password-less beyond a trusted LAN? (Raises F4/auth severity to stop-the-line.)
2. **SQLite symptoms:** are `database is locked`/`SQLITE_BUSY` errors present in `/settings/logs`? (Confirms P5 priority.)
3. **Partial-download policy** on persistent failure (P6) and **duplicate-podcast merge policy** (P10).
4. **Bookmark/favorite** semantics (P18) — intentional coupling?
5. **External consumers** of deprecated GET routes or backup tar layout?
6. **Restore drills:** has a backup tarball ever been restored successfully?
7. Assumption: single-instance deployment (locks are DB-backed but pause state is in-process — `service/download_manager.go:8-14`; multi-replica would break it). Confirm nobody runs replicas.
8. Assumption: `go test ./...` is green at `6533936` (CI badge suggests yes; not verifiable here).

## Do-not-do list

- **Do not swap GORM for sqlc/ent or otherwise rewrite the persistence layer.** The lost-update fix (P1) is achievable with scoped `Updates`; an ORM migration is a multi-week epic with no user-visible payoff now.
- **Do not introduce a message queue / background-job framework** for summarization (P8). The existing lock+cron+backfill machinery is adequate at this scale; a queue adds ops burden to a NAS app.
- **Do not add auth frameworks / multi-user support** while fixing F4. Removing GET mutations and documenting the password is the right-sized change; session/JWT work is speculative.
- **Do not split the Go module into packages-per-domain or microservices.** Layering is fine; the problems are idioms, not boundaries.
- **Do not refactor `service/whisperx.go` internals** (checkpoint/progress protocol) until P1's characterization tests exist — it's the best-engineered risky code in the repo; touching it without tests invites regressions.
- **Do not "fix" `internal/sanitize`** (vendored, has own LICENSE) beyond security patches — treat as frozen third-party code.
- **Do not chase the 1,900-line Vue views** until backend correctness work (P1–P10) lands; UI refactors are lower risk-payoff and the views work today.
- **Do not rewrite migrations to a framework** (goose/atlas): the hand-rolled table works, has precedent for data migrations, and a swap risks the existing migration history.
