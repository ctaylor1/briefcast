# Briefcast — Prioritized Refactor Backlog

- **Date:** 2026-11-05
- **Branch / commit:** `feat/show-note-link-tracking` @ `69252adde6d6a741fb70583ff9ed218d026434df` (dirty tree; see [ASSESSMENT.md](ASSESSMENT.md) §1)
- **Scoring:** `Total = 2×Risk + Blast radius + Frequency + Confidence + Payoff − Effort`. Effort: S = 1, M = 3, L = 5.
- Finding IDs `[F#]` refer to ASSESSMENT.md §5. Line numbers refer to the working tree at the commit above; when implementing, locate code by symbol.

## Verification commands (from the Phase 0 baseline)

| ID | Command | Run during this assessment? |
| --- | --- | --- |
| V1 | `go build ./...` | Yes — pass |
| V2 | `go vet ./...` | Yes — pass |
| V3 | `go test ./<package>/ -run '<pattern>' -count=1` | Yes — pattern form used on `db` |
| V4 | `go test ./... -count=1` (CI: `go test ./...`) | Yes for every package except the root package (112 s, all pass); the root package autoloads `.env` and was skipped |
| V5 | `go test -race ./internal/transcription/... ./internal/whisperxworker/... ./service/...` | No — needs cgo; CI runs it |
| V6 | `npm --prefix frontend run test:unit` | No — Node not installed |
| V7 | `npm --prefix frontend run build` | No — Node not installed |
| V8 | `just ci-python` (ruff, mypy, pytest, pip-audit) | No — `uv` and `just` not installed |
| V9 | `uv run pytest tests/test_packaging.py` | No — needs `uv`; one test also needs Docker and skips without it |

Local notes: Go lives at `C:\Program Files\Go\bin` and is not on the Git Bash `PATH`. With `CGO_ENABLED=0` the Go tests fall back to the `glebarez` SQLite driver; CI uses `mattn`.

## Summary

| # | Title | Total | Effort | Type | Findings |
| --- | --- | --- | --- | --- | --- |
| P1 | Secure compose defaults: loopback bind, auth warning, sane log output | 26 | S | behavior-changing (deployment) | F5, F22 |
| P2 | Explicit, driver-independent SQLite concurrency settings | 24 | S | behavior-changing (config) | F7 |
| P3 | Column-scoped episode updates instead of whole-row `Save` | 24 | M | behavior-preserving | F6 |
| P4 | Panic containment for background goroutines | 24 | S | behavior-preserving (failure path changes) | F8 |
| P5 | Podcast delete: path guards, then atomic ordering | 23 | S | behavior-changing (bug fix) | F1, F13 |
| P6 | HTTP server lifecycle: timeouts, shutdown, body limits | 23 | S | behavior-changing (limits) | F16 |
| P7 | `CheckMissingFiles`: only "not exist" means missing | 22 | S | behavior-changing (error path) | F2 |
| P8 | Remove state-changing GET aliases; encode IDs in the SPA | 22 | S | API-changing | F4 |
| P9 | Local transcription: never delete audio on a string match | 22 | S | behavior-changing (bug fix) | F9 |
| P10 | Delivery baseline: converge branches, add CI gates | 21 | M | mechanical / test-only | F20, F26 |
| P11 | Backup and migration safety | 20 | M | behavior-changing | F15, F25 |
| P12 | Download integrity: collisions, `If-Range`, in-flight guard | 20 | M | behavior-changing | F3 |
| P13 | Worker reliability: resume invariant, fatal fault, lifecycle | 20 | M | behavior-changing | F10, F11, F27 |
| P14 | Lightweight queries on hot paths | 20 | M | behavior-preserving | F18 |
| P15 | Outbound fetch and helper-process hygiene | 19 | S | behavior-changing (error path) | F14, F23 |
| P16 | Handler error responses and PATCH zero values | 19 | S | behavior-changing (API status codes) | F17 |
| P17 | Take LLM calls and remote fetches out of feed refresh | 19 | M | behavior-changing | F19 |
| P18 | `AddPodcast` error branch and uniqueness constraints | 18 | M | data-changing | F12 |
| P19 | Frontend save guards, latest-wins fetches, test harness | 17 | M | behavior-changing (UI) | F24 |
| P20 | Remove dead legacy UI and destructive legacy scripts | 16 | S | mechanical | F21, F28 |

Ties are ordered by the tie-breakers: security, correctness and data integrity first; then reliability; then test enablement; then architecture.

---

## P1 — Secure compose defaults: loopback bind, auth warning, sane log output

- **Category:** Security and privacy; Observability
- **Score:** 26 = 2×4 + 5 + 5 + 5 + 4 − 1
  - Risk 4: the full write API is open to anyone who can reach the host; an insecure default, not a code flaw.
  - Blast radius 5: the single auth boundary in front of every route.
  - Frequency 5: applies to every request on every default deployment.
  - Confidence 5: Confirmed with snippets [F5] [F22].
  - Payoff 4: removes the drive-by exposure class for default installs.
  - Effort 1: a few lines of compose, one log line, one test assertion, README text.
- **Effort:** S · **Confidence:** Confirmed
- **Locations:** `docker-compose.yml:14,20,56`; `docker-compose.legacy.yml:13,54`; `main.go:74-81` (`authRouterGroup`), `main.go:324-332` (`assetEnv`); `README.md:134,162,182`; `tests/test_packaging.py:32` (`test_compose_matrix`).
- **Current problem:** `- ${HOST_PORT:-8080}:8080` publishes on all interfaces and `PASSWORD=${PASSWORD:-}` disables authentication. `LOG_OUTPUT=${LOG_OUTPUT:-both}` is parsed as a file path named `both`.
- **Why it matters:** a default install exposes delete, download, settings and log endpoints to the local network, with a root container behind them. Default logs may never reach `docker logs`.
- **Evidence:** [F5], [F22].
- **Refactor type:** behavior-changing, deployment-config-changing. **Requires explicit maintainer approval** (AGENTS.md).
- **Target state:** default host bind is loopback, widened by one explicit variable; the app logs a warning at startup when `PASSWORD` is empty; `LOG_OUTPUT` defaults to `stdout`.
- **Step plan:**
  1. Test first: extend `test_compose_matrix` to assert the rendered port has host IP `127.0.0.1` and that `LOG_OUTPUT` renders as `stdout` when unset.
  2. Change the port line to `- ${HOST_BIND:-127.0.0.1}:${HOST_PORT:-8080}:8080` in `docker-compose.yml` and `docker-compose.legacy.yml`.
  3. Change the log default to `${LOG_OUTPUT:-stdout}`.
  4. In `assetEnv`, add one `Warnw` when `PASSWORD` is empty. Do not change `authRouterGroup`.
  5. Document `HOST_BIND` in `README.md` and add an upgrade note. Stop.
  6. Separate later PR, only if the maintainer agrees: run the app container as a non-root user.
- **Tests to add:** compose rendering assertions (pytest); a Go test that the startup warning is emitted when `PASSWORD` is empty.
- **Verification:** V9, V1, V2, `go test . -run 'TestAssetEnv|TestAuthRouterGroup' -count=1` (V3 form; the root package must be run by the maintainer or CI).
- **Rollback note:** operators who relied on LAN access must set `HOST_BIND=0.0.0.0`. Reverting the PR restores the old default; no data is involved.
- **Risks and edge cases:** existing users lose LAN access on upgrade until they set the variable — this is the intended, announced change. Reverse-proxy setups on the same host keep working. Compose versions below 2.24 are already unsupported.
- **Maintainer questions:** Is LAN exposure without a password an intended default for the NAS use case? If so, the alternative is to keep the bind and require either `PASSWORD` or an explicit `BRIEFCAST_ALLOW_NO_AUTH=true`. Does `.env.example` already set `LOG_OUTPUT`?

---

## P2 — Explicit, driver-independent SQLite concurrency settings

- **Category:** Correctness and reliability
- **Score:** 24 = 2×4 + 5 + 5 + 4 + 3 − 1
  - Risk 4: blocked or failed writes leave episodes in stale states; the maintainer has observed blocking in production.
  - Blast radius 5: every database call goes through this one connection setup.
  - Frequency 5: per-query.
  - Confidence 4: code Confirmed; production effect reasoned from driver defaults, not measured.
  - Payoff 3: removes a driver-dependent behaviour; the larger win (WAL) is gated behind P11.
  - Effort 1: one function plus tests.
- **Effort:** S · **Confidence:** Confirmed (code) / Likely (impact)
- **Locations:** `db/db.go:18-54` (`Init`); `db/config.go:134-165` (`normalizeSQLiteDSN`), `:183-197` (`applyConnectionPool`); `db/config_test.go`; `cmd/migrate_to_pg/main.go:41`, `cmd/backfill_canonical_transcripts/main.go:27`, `cmd/transcription-rollback/main.go` (open the same file with a different driver).
- **Current problem:** no journal mode, busy timeout or SQLite pool size is set. Production gets `mattn/go-sqlite3`'s implicit 5-second timeout with a 25-connection pool; the CLI tools and local tests use `glebarez/sqlite` with different defaults.
- **Why it matters:** in rollback-journal mode long reads block writers; a writer that waits past the timeout fails, and several failure-path writes discard their error.
- **Evidence:** [F7].
- **Refactor type:** behavior-changing (configuration). No schema or file-format change in the first slice.
- **Target state:** for SQLite, every pooled connection has an explicit busy timeout (default 5000 ms, overridable by `SQLITE_BUSY_TIMEOUT_MS`), and the pool defaults to a small size unless `DB_MAX_OPEN_CONNS` is set. Postgres behaviour is unchanged. WAL is available only as an explicit opt-in and only after P11.
- **Step plan:**
  1. Test first: a `db` test that opens the database through `Init`, holds N connections at once, and asserts `PRAGMA busy_timeout` reports the configured value on each. It must pass under both drivers (cgo in CI, fallback locally).
  2. Apply the timeout per connection in a way that works for the compiled-in driver (DSN parameter or connection hook). Do not rely on a single `Exec` — it reaches one pooled connection only.
  3. Make `applyConnectionPool` driver-aware: when the driver is SQLite and `DB_MAX_OPEN_CONNS` is unset, use a small default (suggest 4).
  4. Log the effective timeout, pool size and journal mode once at startup. Stop.
  5. Follow-up PR after P11: `SQLITE_JOURNAL_MODE=wal` opt-in, documented with its backup and filesystem caveats.
- **Tests to add:** the per-connection pragma test; table tests for the DSN helper; a test that Postgres configuration is untouched.
- **Verification:** `go test ./db/ -count=1` (V3), V4, V1, V2.
- **Rollback note:** revert the PR. If the WAL follow-up has been enabled on a database, switching back needs `PRAGMA journal_mode=DELETE` run once with the app stopped — document this in that PR.
- **Risks and edge cases:** a smaller pool increases queueing under heavy concurrent reads; the setting is overridable. DSNs that already carry parameters must be preserved. The `flexibleTime` scanner in `db/link_stats.go` exists because the two drivers return times differently — do not remove it.
- **Maintainer questions:** Is `/config` on local disk or a network share (WAL needs shared memory and does not work over NFS or SMB)? What busy-timeout and pool values match the production host?

---

## P3 — Column-scoped episode updates instead of whole-row `Save`

- **Category:** Correctness and reliability (data integrity)
- **Score:** 24 = 2×4 + 5 + 4 + 5 + 5 − 3
  - Risk 4: silently reverts user actions and other jobs' state.
  - Blast radius 5: `UpdatePodcastItem` is the shared write path for the core table.
  - Frequency 4: every download, summary, transcription and user toggle.
  - Confidence 5: Confirmed [F6].
  - Payoff 5: removes the lost-update class and unblocks P12, P14, P17.
  - Effort 3: three reviewable PRs.
- **Effort:** M · **Confidence:** Confirmed
- **Locations:** `db/dbfunctions.go:563-568` (`UpdatePodcastItem`); 21 call sites — `service/podcastService.go` (10), `service/whisperx.go` (5), `service/summarize.go` (4), `service/alternate_feeds.go` (1), `service/chapters.go` (1); tests in `service/state_transitions_characterization_test.go`.
- **Current problem:** each setter loads the full row, changes a field or two, and saves every column. Long-running callers save snapshots that are minutes or hours old.
- **Why it matters:** [F6].
- **Evidence:** `db/dbfunctions.go:566`; `service/summarize.go:389,422`; `service/podcastService.go:994`.
- **Refactor type:** behavior-preserving for the fields each setter owns; lost updates disappear.
- **Target state:** every state transition writes only the columns it owns through one helper (`db.UpdatePodcastItemColumns(id, map)` with the same silent-logger session and a not-found error when no row matches). `UpdatePodcastItem` is deleted or unexported.
- **Step plan:**
  1. Confirm the 15 characterization tests pass unchanged (`go test ./service/ -run TestCharacterize -count=1`).
  2. Add the helper in `db` with tests, and one regression test that reproduces a lost update (stale snapshot saved after a concurrent `is_played` change).
  3. PR A: convert the download-state setters in `podcastService.go` (`SetPodcastItemAsQueuedForDownload`, `…PreserveProgress`, `…AsDownloading`, `…AsPaused`, `…AsDownloaded`, `…AsNotDownloaded`, `…PlayedStatus`, `…BookmarkStatus`, `downloadImageLocally`, the alternate-URL save in `DownloadWithFallback`). Stop.
  4. PR B: convert `SummarizeEpisode` and the summary paths. Stop.
  5. PR C: convert `whisperx.go`, `alternate_feeds.go`, `chapters.go`; remove `UpdatePodcastItem`.
- **Tests to add:** helper unit tests; the lost-update regression; keep characterization tests green without edits to their expectations.
- **Verification:** V3 on `./service/` and `./db/`, V4, V5 in CI, V1, V2.
- **Risks and edge cases:** GORM map updates write zero values and nil (needed for `bookmark_date` and `transcript_next_attempt`); `updated_at` must still advance; the remote-transcription commit path already does conditional writes and must not be changed here.
- **Maintainer questions:** Should bookmark and summary-favorite stay coupled (`SetPodcastItemBookmarkStatus` sets both)? Preserve as is unless told otherwise.

---

## P4 — Panic containment for background goroutines

- **Category:** Correctness and reliability
- **Score:** 24 = 2×4 + 5 + 3 + 5 + 4 − 1
  - Risk 4: one malformed input can stop the whole server.
  - Blast radius 5: the process.
  - Frequency 3: background work runs constantly; panics need unusual input.
  - Confidence 5: zero `recover()` calls in non-test Go; one concrete nil dereference [F8].
  - Payoff 4: removes a crash class across 26 launch sites.
  - Effort 1: one helper, mechanical call-site changes, two tests.
- **Effort:** S · **Confidence:** Confirmed (missing boundary) / Likely (the specific crash, not executed)
- **Locations:** `service/workerpool.go:23-47`; `service/podcastService.go:247-269` (`getItunesImageURL`); launch sites in `controllers/podcast.go:260,283,345,462,713,730,748`, `controllers/settings.go:369,388`, `controllers/downloads.go:129`, `service/podcastService.go:29,956,1120`, `service/summary_backfill.go:95,112,150`, `service/repair_work.go:165`, `service/briefpoint.go:147`, `main.go:50,53`.
- **Current problem:** a panic in any background goroutine terminates the process. `getItunesImageURL` dereferences a nil node for feeds without `<channel>`.
- **Evidence:** [F8].
- **Refactor type:** behavior-preserving on the success path; on panic the process now logs and continues.
- **Target state:** a single helper runs a function with `defer recover()`, logs the panic value and stack through zap with a name, and returns. `runWorkerPool` recovers per job so the pool drains and `wg.Done` runs. The nil is guarded.
- **Step plan:**
  1. Tests first: a `runWorkerPool` test where one job panics and the others complete; a `getItunesImageURL` test with an Atom document expecting `""`.
  2. Add the helper next to `runWorkerPool` and use it inside the pool.
  3. Guard `channel == nil`. If `feat/whisperx-service` has been merged (P10), take its fix (`ff2a576`) instead of writing a second one.
  4. Wrap the listed launch sites. Job flags that are reset by `defer` must still reset when the wrapped function panics.
  5. Stop. Do not add retries or change what the goroutines do.
- **Tests to add:** the two above, plus one asserting a job-running flag (for example the summary job) is cleared after a panic.
- **Verification:** `go test ./service/ -run 'TestRunWorkerPool|TestGetItunesImageURL' -count=1` (V3), V4, V5 in CI, V1, V2.
- **Risks and edge cases:** recovering hides bugs unless the log line is loud — log at error level with the stack. Do not recover inside `main` itself.
- **Maintainer questions:** none.

---

## P5 — Podcast delete: path guards, then atomic ordering

- **Category:** Correctness and reliability (data integrity)
- **Score:** 23 = 2×5 + 3 + 2 + 5 + 4 − 1
  - Risk 5: irreversible deletion of other podcasts' media.
  - Blast radius 3: the asset storage subsystem.
  - Frequency 2: podcast deletion is occasional.
  - Confidence 5: Confirmed [F1] [F13].
  - Payoff 4: closes the path-collision class for deletes.
  - Effort 1: one function and its caller, for the first slice.
- **Effort:** S (slice 1), S (slice 2) · **Confidence:** Confirmed (not reproduced)
- **Locations:** `service/fileService.go:604-628` (`deletePodcastFolder`); `service/asset_paths.go:11-18,63-80`; `service/podcastService.go:1167-1210` (`DeletePodcast`); `db/dbfunctions.go:182-190` (`DeletePodcastByID`).
- **Current problem:** folders are keyed by sanitized title only. Shared titles and category-named titles delete other podcasts' files. Deletion is multi-step with early returns that leave a podcast with no episodes.
- **Evidence:** [F1], [F13].
- **Refactor type:** behavior-changing (bug fix): some directories that are removed today will be kept.
- **Target state:** slice 1 — folder removal never touches a category directory and never removes a folder another podcast shares. Slice 2 — all rows are removed in one transaction first; file removal is best-effort and logged.
- **Step plan:**
  1. Slice 1, tests first, with a temp `DATA` directory: (a) for each of `audio`, `images`, `transcripts`, `summaries`, `markdown` as a podcast title, another podcast's files under that category survive; (b) two podcasts with one sanitized title — deleting one keeps the other's files; (c) the ordinary case still removes all seven directories.
  2. In `deletePodcastFolder`, skip the legacy `DATA/<title>` path when the sanitized title equals a reserved category name (compare case-insensitively).
  3. In `DeletePodcast`, skip `deletePodcastFolder` when any other podcast has the same sanitized title; log why. Per-item files are already removed individually.
  3a. `DeletePodcast` calls `deletePodcastFolder` even when `deleteFiles` is false (`service/podcastService.go:1199` is outside the `if deleteFiles` block). First read what `service/coverage_extra_test.go:549` and `service/coverage_more_test.go:274` assert. If the maintainer confirms the route should keep files, move the call inside the block in this PR; otherwise leave it and record the answer.
  4. Stop. Open slice 2 as its own PR.
  5. Slice 2, tests first: a failing folder removal no longer leaves the podcast row behind. Then delete items, links, Markdown revisions, `podcast_tags` rows and the podcast in one transaction before touching the filesystem.
- **Tests to add:** the cases in steps 1 and 5.
- **Verification:** `go test ./service/ -run 'TestDeletePodcast' -count=1` (V3), `go test ./service/ ./db/ ./controllers/ -count=1`, V1, V2.
- **Rollback note:** revert the PR; no data migration.
- **Risks and edge cases:** case-insensitive filesystems; titles that sanitize to `_`; orphan directories left behind when a shared title is detected (acceptable and logged). Slice 2 changes the order of side effects — file errors no longer abort the DB delete.
- **Maintainer questions:** Should `DELETE /podcasts/:id/podcast` keep the podcast's files on disk? Today it removes them. Should per-podcast folders eventually be keyed by podcast ID? That is a data migration and is on the do-not-do list for now.

---

## P6 — HTTP server lifecycle: timeouts, shutdown, body limits

- **Category:** Correctness and reliability
- **Score:** 23 = 2×3 + 5 + 5 + 5 + 3 − 1
  - Risk 3: hung connections and unbounded bodies on an unauthenticated-by-default server.
  - Blast radius 5: every inbound request.
  - Frequency 5: per-request.
  - Confidence 5: Confirmed [F16].
  - Payoff 3: standard hardening; enables clean restarts.
  - Effort 1: one file plus tests; best split in two small PRs.
- **Effort:** S · **Confidence:** Confirmed
- **Locations:** `main.go:33-59` (`main`), `:61-72` (`buildRouter`), `:311-322` (`intiCron`); `service/itunesService.go:61-82`; `controllers/pages.go:375`.
- **Current problem:** `r.Run()` with a default `http.Server`; no signal handling; `gin.Recovery` registered after other middleware; no body limit except OPML; the PodcastIndex client has no timeout.
- **Refactor type:** behavior-changing in limits only: oversized JSON bodies get 413; the process exits cleanly on SIGTERM.
- **Target state:** an explicit `http.Server` with `ReadHeaderTimeout` and `IdleTimeout` (no `WriteTimeout`, because media is streamed); graceful shutdown with a deadline; `gin.Recovery` first; a default body cap on non-multipart routes; PodcastIndex search uses a client with a timeout.
- **Step plan:**
  1. Tests first: a builder function returns a server with the expected timeouts; a 2 MiB JSON body to `PATCH /settings` returns 413; `Recovery` wraps a panicking middleware.
  2. PR A: server construction, same address resolution as `gin.Run` (`PORT` or `:8080`), signal handling, shutdown. Stop.
  3. PR B: middleware order, body cap (suggest 1 MiB, OPML keeps its own), `podcastindex.NewClientWithConfig` with a bounded client.
- **Tests to add:** as in step 1; a search test with a stalled upstream.
- **Verification:** `go test . -count=1` (maintainer or CI), `go test ./service/ ./controllers/ -count=1` (V3), V1, V2.
- **Risks and edge cases:** long uploads or slow clients behind a reverse proxy; the WebSocket route needs no body cap; shutdown must not wait indefinitely for streaming responses.
- **Maintainer questions:** Is the app ever run behind a proxy that needs `X-Forwarded-For` trusted? If so, add a `TRUSTED_PROXIES` setting in PR B rather than hard-coding.

---

## P7 — `CheckMissingFiles`: only "not exist" means missing

- **Category:** Correctness and reliability (data integrity)
- **Score:** 22 = 2×4 + 3 + 4 + 4 + 4 − 1
  - Risk 4: library-wide state reset and a re-download storm; files themselves survive.
  - Blast radius 3: downloads and library state.
  - Frequency 4: the job runs every check cycle.
  - Confidence 4: mechanism Confirmed; the trigger (storage fault) is environmental.
  - Payoff 4: makes a routine storage blip harmless.
  - Effort 1: one function.
- **Effort:** S · **Confidence:** Confirmed mechanism / Likely impact
- **Locations:** `service/podcastService.go:915-938` (`CheckMissingFiles`); `service/fileService.go:400-405` (`FileExists`); `db/dbfunctions.go:309-314`.
- **Current problem and evidence:** [F2].
- **Refactor type:** behavior-changing on error paths only.
- **Target state:** the pass aborts with an error when the data root is unavailable; only `fs.ErrNotExist` marks a file missing; other stat errors are logged and skipped; a pass that would flip an implausible share of the library aborts without writing.
- **Step plan:**
  1. Tests first, temp `DATA`: present and absent files; `DATA` pointing at a missing directory (no row changes); an injected stat error (no row change); the circuit breaker.
  2. Add a small stat seam so errors can be injected in tests.
  3. Implement root check, error classification and the breaker (suggest: abort when more than half of at least 20 downloaded items look missing).
  4. Leave `FileExists` itself alone — other callers depend on it. Stop.
- **Tests to add:** the four cases above.
- **Verification:** `go test ./service/ -run 'TestCheckMissingFiles' -count=1` (V3), V4, V1, V2.
- **Risks and edge cases:** a user who really deletes most of the library by hand will hit the breaker; log clearly how to proceed. Windows and Linux report different errors for unmounted paths — use the seam, not real mounts.
- **Maintainer questions:** What threshold suits your library? Should a tripped breaker surface in the UI?

---

## P8 — Remove state-changing GET aliases; encode IDs in the SPA

- **Category:** Security and privacy
- **Score:** 22 = 2×4 + 3 + 3 + 5 + 4 − 1
  - Risk 4: one click or one embedded image deletes or queues content.
  - Blast radius 3: the episode and podcast mutation routes.
  - Frequency 3: common user paths.
  - Confidence 5: Confirmed on both sides [F4].
  - Payoff 4: removes GET-based CSRF and path smuggling from the SPA.
  - Effort 1: nine route lines, a helper, tests.
- **Effort:** S · **Confidence:** Confirmed
- **Locations:** `main.go:106,110,112,126,128,130,132,135,143`; `main_test.go:77` (`TestMutatingRoutesUseCorrectHTTPMethods`); `frontend/src/lib/api/episodes.ts`, `podcasts.ts`, `downloads.ts`, `summaries.ts`, `settings.ts`; `frontend/src/views/PlayerView.vue:69-98`.
- **Refactor type:** API-changing (nine GET routes removed). Keep the Go and frontend changes in separate PRs.
- **Target state:** mutations exist only under POST, PATCH or DELETE; every ID interpolated into a path is encoded; the player ignores IDs that are not UUIDs.
- **Step plan:**
  1. Go PR, test first: extend the route test to assert each alias is no longer registered; then delete the nine lines. Stop.
  2. Frontend PR, test first: a `src/lib` unit test that `getById("a/delete")` requests `/podcastitems/a%2Fdelete`; then add one path helper and use it in the five API modules; validate IDs in `parseItemIds`.
- **Tests to add:** as above.
- **Verification:** `go test . -run 'TestMutatingRoutes|TestBuildRouter' -count=1` (maintainer or CI), V6, V7, V1, V2.
- **Risks and edge cases:** external scripts or bookmarks that use the GET forms will break — call this out in the changelog. The only in-repo callers are the dead templates under `client/`.
- **Maintainer questions:** Does any external automation call the GET aliases?

---

## P9 — Local transcription: never delete audio on a string match

- **Category:** Correctness and reliability (data integrity)
- **Score:** 22 = 2×5 + 3 + 2 + 5 + 3 − 1
  - Risk 5: valid media is deleted; older episodes may no longer be in the feed.
  - Blast radius 3: the local transcription path.
  - Frequency 2: only deployments with `WHISPERX_BACKEND=local`.
  - Confidence 5: Confirmed and already pinned as a known defect [F9].
  - Payoff 3: one branch.
  - Effort 1.
- **Effort:** S · **Confidence:** Confirmed
- **Locations:** `service/whisperx.go:398-414`; `service/whisperx_characterization_test.go:286-296`; `service/transcription_remote.go:272-288` (the HTTP path already does this correctly).
- **Refactor type:** behavior-changing (bug fix).
- **Target state:** the item is reset for download only when `os.Stat` proves the audio is absent; every other failure schedules a transcript retry with backoff. Nothing in this path calls `os.Remove`.
- **Step plan:**
  1. Change the `missing-model-path` characterization case to expect the file retained and a retry scheduled; add a case for a missing `ffmpeg` message.
  2. Replace the substring branch with a stat check, mirroring the HTTP path.
  3. Stop.
- **Tests to add:** as in step 1.
- **Verification:** `go test ./service/ -run 'TestCharacterize|TestWhisperX' -count=1` (V3), V4, V1, V2.
- **Risks and edge cases:** truly corrupt audio is no longer re-downloaded automatically; it retries and then fails visibly.
- **Maintainer questions:** Should a confirmed "Failed to load audio" still trigger an automatic re-download? Is the local backend still supported in production?

---

## P10 — Delivery baseline: converge branches, add CI gates

- **Category:** Delivery and developer experience
- **Score:** 21 = 2×3 + 5 + 4 + 5 + 4 − 3
  - Risk 3: a clean checkout cannot build the app image; a known production syntax fix is missing here.
  - Blast radius 5: every later PR builds on this.
  - Frequency 4: every push.
  - Confidence 5: Confirmed [F20] [F26].
  - Payoff 4: restores a trustworthy baseline and catches classes of defects automatically.
  - Effort 3: one maintainer-led merge plus two or three small CI PRs.
- **Effort:** M · **Confidence:** Confirmed
- **Locations:** refs `origin/main` (`2335855`), `feat/whisperx-service` (`68d1fb7`); `.gitignore:19-20`; `Dockerfile:27`; `.github/workflows/ci.yml:26-29`; `.gitattributes`; `justfile:118-119`.
- **Refactor type:** mechanical (merge) and test-only (CI).
- **Target state:** one integration branch containing all three lines; CI runs `go vet`, the race detector on all packages, and a format check that works on Windows checkouts.
- **Step plan:**
  1. Maintainer: merge `origin/main` and reconcile with `feat/whisperx-service`. Expect conflicts in `service/podcastService.go`, `service/link_backfill.go`, `service/export.go`, `main.go` and the link-tracking files. Confirm `git ls-files docker/app` lists the lock file afterwards.
  2. CI PR: add `go vet ./...` and widen the race step to `./...`. Fix anything the wider race run reports in separate PRs.
  3. Formatting PR: add `*.go text eol=lf` to `.gitattributes`, renormalize, then add a `gofmt -l` check. Mechanical only.
  4. CI-only follow-ups: `govulncheck`, `pip-audit` against the three shipped lock files, pin actions by commit SHA.
- **Tests to add:** none; the gates are the tests.
- **Verification:** V1, V2, V4, V5, V8 in CI.
- **Risks and edge cases:** the wider race run may surface real races; do not suppress them. `govulncheck` needs network and belongs in CI only.
- **Maintainer questions:** Which branch is the integration target, and in what order should the two feature branches land?

---

## P11–P20 (condensed)

**P11 — Backup and migration safety** · 20 = 2×4 + 5 + 2 + 4 + 4 − 3 · M · behavior-changing
- **Where:** `service/fileService.go:407-419,474-504`; `db/db.go:56-63`; `db/migrations.go:301-326`; `main.go:48`; `scripts/ReleasePackaging.ps1:196-208`.
- **Problem:** the backup copies a live file under a fixed name, drops close errors, and leaves empty archives on Postgres; migrations log and continue; app deploys take no restore point [F15] [F25].
- **Target:** snapshot the configured SQLite database with `VACUUM INTO`, check every close, skip with one clear log line on Postgres; `Migrate` returns an error and `main` exits on it; app deploy requires recovery evidence or an explicit override.
- **Tests:** restore-and-open test of a produced archive; a failing migration stops startup; mocked deploy test in `tests/test_packaging.py`.
- **Verify:** V3 on `./service/` and `./db/`, V4, V9.

**P12 — Download integrity** · 20 = 2×4 + 3 + 4 + 4 + 4 − 3 · M · behavior-changing
- **Where:** `service/fileService.go:63-196` (`Download`), `:630-649` (`getFileName`); `service/podcastService.go:1008-1049`.
- **Problem:** title-only file names collide; resume appends with no validator; no in-flight guard [F3].
- **Target:** when the target path belongs to another episode, add a short ID suffix; send `If-Range` with the stored validator and restart on mismatch; one download per item at a time.
- **Tests:** `httptest` servers for same-title episodes, a changed entity, and two concurrent calls.
- **Verify:** V3 on `./service/`, V4. Depends on P3 for clean state writes.

**P13 — Worker reliability** · 20 = 2×4 + 3 + 3 + 5 + 4 − 3 · M · behavior-changing
- **Where:** `internal/whisperxworker/worker.go:119-125,262-269,464-470`; `cmd/whisperx-worker/main.go:123-137`; `service/transcription_remote.go:235,251-288`; `service/transcription_remote_state.go:165-204`; `internal/whisperxworker/artifacts.go:284-330`.
- **Problem:** resumed jobs trip the client's status check; a storage fault never exits and is not logged; envelopes pin stale inputs; abandoned jobs are never cancelled; state grows without bound [F10] [F11] [F27].
- **Target (one PR each, in this order):** seed totals on resume; exit non-zero on fault and log causes; reset the envelope on manual retry when the fingerprint or profile changed; best-effort cancel; mark pruned jobs so they are skipped.
- **Tests:** a resumed run whose first tick has total 0; fault injection asserting exit; retry after audio change.
- **Verify:** V3 on `./internal/whisperxworker/` and `./service/`, V5 in CI.

**P14 — Lightweight queries on hot paths** · 20 = 2×3 + 3 + 5 + 5 + 4 − 3 · M · behavior-preserving
- **Where:** `controllers/podcast.go:519-581`; `db/dbfunctions.go:161-165,291-314`; `db/search_helpers.go:25-36`; `main.go:195-204`; `service/podcastService.go:543-564`.
- **Problem:** thumbnail and media handlers, cron jobs and search load full rows with transcripts; settings are read per request [F18].
- **Target:** handlers and cron select only the columns they use; `setupSettings` attaches to the routes that need it; `UpdateAllFileSizes` is bounded per run.
- **Tests:** handler tests stay green; a query-shape test asserting heavy columns are absent.
- **Verify:** V3 on `./controllers/` and `./db/`, V4. Do the handler slice first; it is independent of P3.

**P15 — Outbound fetch and helper-process hygiene** · 19 = 2×3 + 3 + 4 + 4 + 3 − 1 · S · behavior-changing (error paths)
- **Where:** `service/fileService.go:269-356,538-552`; `service/feedparser.go:116-127`; `service/transcription_local.go:142-147`; `scripts/feedparser_parse.py:52-54`; `service/outbound_url_policy.go:82-106`.
- **Problem:** image downloads leak bodies and accept any status or size; redirects are unbounded; helpers inherit all secrets; the feed helper may treat the payload as a path [F14] [F23].
- **Target:** close on every path, require 2xx, cap size, cap redirects at 10, reuse one client per purpose; pass helpers an allow-listed environment; wrap the payload in `io.BytesIO`.
- **Tests:** `httptest` for 404 and oversized images; a sentinel environment variable invisible to a stub helper; a pytest feeding a path-shaped payload.
- **Verify:** V3 on `./service/`, V8.

**P16 — Handler error responses and PATCH zero values** · 19 = 2×3 + 3 + 3 + 5 + 3 − 1 · S · behavior-changing (status codes)
- **Where:** `controllers/podcast.go:98-118,519-581,676-702,780-790,856-877,931-991`; `controllers/pages.go:334-357`.
- **Problem:** empty 200 responses on failure, a missing `return`, an unchecked update that cannot clear values [F17].
- **Target:** every branch writes exactly one response; PATCH binds pointers and updates a column map.
- **Tests:** one handler test per fixed branch pinning the status code.
- **Verify:** V3 on `./controllers/`, V4.

**P17 — Take LLM calls and remote fetches out of feed refresh** · 19 = 2×3 + 3 + 4 + 5 + 4 − 3 · M · behavior-changing
- **Where:** `service/podcastService.go:324-519,1052-1133`; `service/summary_backfill.go`.
- **Problem:** refresh blocks on per-episode LLM and HTTP calls under a lease that is never renewed [F19].
- **Target:** first PR adds the lease heartbeat (reuse `startJobLockHeartbeat`); second marks new episodes pending and lets the existing backfill summarize them.
- **Tests:** refresh completes with a stalled LLM stub; lease is renewed during a long refresh.
- **Verify:** V3 on `./service/`, V4. Depends on P3.

**P18 — `AddPodcast` error branch and uniqueness** · 18 = 2×3 + 3 + 3 + 5 + 4 − 3 · M · data-changing
- **Where:** `service/podcastService.go:272-321,755-765`; `db/podcast.go:19,65`; `db/migrations.go`.
- **Problem:** DB errors reported as "already exists"; no unique index on feed URL or on podcast plus GUID; an unlocked `AddPodcastItems` call [F12].
- **Target:** explicit error branch (first PR, no data change); then dedupe migrations and unique indexes, following the pattern used for `show_note_links`.
- **Tests:** injected DB error returns 5xx; concurrent adds yield one row.
- **Verify:** V3 on `./service/` and `./db/`. Depends on P11 (backup before a dedupe migration).

**P19 — Frontend save guards, latest-wins fetches, test harness** · 17 = 2×3 + 3 + 3 + 5 + 3 − 3 · M · behavior-changing (UI)
- **Where:** `frontend/src/views/SettingsView.vue:256-262,476-497`; `DashboardView.vue:154-164`; `PlayerView.vue:137-196`; `SummariesView.vue:215-261`; `composables/useEpisodeDrawer.ts`; `lib/sanitize.ts`, `lib/sanitize.test.ts`.
- **Problem:** forms save defaults after a failed load; the player ignores `start`; stale responses overwrite newer state; the sanitizer test does not test production code [F24].
- **Target:** render forms only after a successful load; mount `<audio>` before seeking; a small latest-wins helper; the sanitizer test imports the real function.
- **Tests:** `src/lib` unit tests for the helper and sanitizer now; component tests once a harness is approved (new dev dependency).
- **Verify:** V6, V7.

**P20 — Remove dead legacy UI and destructive legacy scripts** · 16 = 2×3 + 2 + 1 + 5 + 3 − 1 · S · mechanical
- **Where:** `controllers/pages.go:54-331,443-467`; `controllers/podcast.go:410-425`; `client/`; `webassets/` except `blank.png`; `RELEASE_BUILD.ps1`, `RELEASE_RUN.ps1`, `RELEASE_RESET.ps1`; `RELEASE.md`.
- **Problem:** unreachable handlers and templates still ship; legacy scripts can wipe data with unchecked errors [F21] [F28].
- **Target:** delete them in two mechanical PRs (code, then scripts and docs); decide separately whether `/ws` and the hub go too.
- **Tests:** route test asserting `/webassets/blank.png` still serves; a pytest asserting the legacy scripts are gone.
- **Verify:** V1, V2, V4, V9. Depends on P8.

---

## PR slicing plan

Test-enablement comes before risky refactors. Mechanical and behavioral changes are never in the same PR.

| PR | Items | Goal | Risk | Depends on | Validation | Rollback |
| --- | --- | --- | --- | --- | --- | --- |
| 0 | P10 (step 1) | Maintainer merges `origin/main` and `feat/whisperx-service`; clean checkout builds | Med (conflicts) | — | V1, V2, V4, V8, image build in CI | Abandon the merge commit |
| 1 | P5 (slice 1) | Delete path guards with tests | Low | — | V3 `./service/`, V4 | Revert |
| 2 | P10 (step 2) | CI: `go vet`, race on all packages | Low | PR 0 | CI green | Revert |
| 3 | P7 | Missing-file guard and circuit breaker | Low | — | V3 `./service/`, V4 | Revert |
| 4 | P4 | Panic containment | Low | PR 0 preferred | V3 `./service/`, V4, V5 | Revert |
| 5 | P8 (Go) | Remove nine GET aliases — **API change, release note** | Low | — | root-package tests in CI | Revert |
| 6 | P8 (frontend) | Encode IDs, validate player IDs | Low | — | V6, V7 | Revert |
| 7 | P9 | Local backend never deletes audio | Low | — | V3 `./service/` | Revert |
| 8 | P6 (A) | Server timeouts and graceful shutdown | Med | — | root-package tests in CI, manual restart check | Revert |
| 9 | P1 | Compose defaults — **deployment change, needs approval, release note** | Med | maintainer decision | V9 | Operators set `HOST_BIND=0.0.0.0`; or revert |
| 10 | P2 | Explicit SQLite settings — **monitor lock errors after rollout** | Med | — | V3 `./db/`, V4, CI on cgo | Revert; see P2 rollback note for WAL |
| 11 | P3 (A) | Column-scoped download-state setters | Med | PR 2 | V3 characterization, V4, V5 | Revert |
| 12 | P3 (B, C) | Summary and transcription writes; remove `UpdatePodcastItem` | Med | PR 11 | V3, V4, V5 | Revert |

After PR 12: P5 slice 2, P6 (B), P11, P13, P14, then the rest by score. P18's migration and P2's WAL opt-in both wait for P11.

---

## Open questions and assumptions

**Maintainer questions**

1. Is password-less LAN access an intended default? (P1)
2. Which branch is the integration target, and in what order do the feature branches merge? (P10)
3. Is `/config` on local disk or a network share? (P2, WAL)
4. Does any external tool call the deprecated GET routes or the legacy `POST /settings`? (P8, P20)
5. Is `WHISPERX_BACKEND=local` still supported in production? (P9, P13)
6. Should corrupt audio still be re-downloaded automatically? (P9)
7. Is the `/ws` remote-player hub still a feature? The SPA never opens it. (P20)
8. Are bookmark and summary-favorite meant to be one flag? (carried from July)
9. Are any deployments on Postgres? (P11, P18)
10. What threshold should stop a mass "missing files" pass? (P7)
11. Is running the app container as non-root acceptable, given `PUID`/`PGID` ownership handling? (P1 follow-up)
12. How long should failed worker job artifacts be kept? (P13)

**Assumptions**

- One app instance per database; `UnlockAllJobs` at startup relies on this.
- A single trusted user on a home or small-office network.
- Findings on `feat/whisperx-service` were checked only for the top items (ASSESSMENT.md §1.2).

**Missing context**

- Contents of `.env.example`; production database size and query timings; CI run history; real feed corpus for collision frequency.

**Areas that need a dedicated review**

- **Security:** the auth model (one shared password, Basic over plain HTTP, no rate limit); CSRF on POST routes (`ShouldBindJSON` does not check the content type, so a cross-site form with a text body can post JSON); DOMPurify configuration and the absence of a Content-Security-Policy; the Briefpoint server URL as a way to redirect the stored API key.
- **Domain:** episode identity when feeds reuse titles or GUIDs; retention policy semantics.
- **Operations:** restore drills for both databases.

---

## Do-not-do list

- **Do not enable WAL by default** before P11 lands and the filesystem question is answered.
- **Do not re-key asset folders by podcast ID yet.** It is the right end state but needs a data migration and a rollback story; P5 removes the data-loss path without it.
- **Do not make the container non-root in the same PR as the compose defaults.** File ownership on existing volumes needs its own plan.
- **Do not replace the global `db.DB` with injected repositories now.** No current defect requires it, and it would touch every file under active change.
- **Do not split `SettingsView.vue` or `SummariesView.vue`** until component tests exist.
- **Do not rewrite `AddPodcastItems` or `transcribePendingEpisodes` wholesale.** Change them in the slices described in P3 and P17, behind the characterization suites.
- **Do not remove the legacy `POST /settings`** until its eight fields (file-name options, download concurrency, user agent, base URL and others) are reachable through `PATCH /settings`.
- **Do not add unique indexes** before the dedupe migration and a verified backup (P18 after P11).
- **Do not restructure the worker's journal, checkpoint or guardian code.** It is deliberately fail-closed and well tested; P13 lists targeted fixes only.
- **Do not edit `internal/sanitize`.** It is a vendored fork with its own licence.
- **Do not run `RELEASE_RESET.ps1`, `RELEASE_RUN.ps1` or `RELEASE_BUILD.ps1`** for any reason; P20 deletes them.
- **Do not run `gofmt -w` across the repo from a Windows checkout** before `.gitattributes` pins Go files to LF (P10 step 3).
- **Do not replace the Python feed parser with a Go library** as part of this backlog; it is a behaviour change across every feed.
