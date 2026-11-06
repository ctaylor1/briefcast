# Briefcast — Repository Refactoring Assessment

- **Date:** 2026-11-05
- **Branch / commit:** `feat/show-note-link-tracking` @ `69252adde6d6a741fb70583ff9ed218d026434df`
- **Working tree:** dirty — `db/migrations.go` and `main.go` modified (+9 lines), `controllers/links.go`, `controllers/links_test.go`, `db/link_stats.go`, `db/link_stats_test.go` untracked. Line numbers below refer to the working tree.
- **Concurrent edits seen while writing:** at 21:49–21:50 local time, four frontend files changed on disk (`frontend/src/style.css`, `views/SummariesView.vue`, `components/dashboard/PodcastCard.vue`, `components/episodes/EpisodesFilters.vue`). They are CSS-only changes made outside this assessment. No line cited below is affected (the `SummariesView.vue` hunks start at line 1029, inside its style block).
- **Mode:** read-only. No source file was changed by this assessment. These deliverables are new, uncommitted files.
- **Location note:** `docs/refactor-assessment/` already holds a tracked assessment dated 2026-07-05 (`ASSESSMENT.md`, `BACKLOG.md`, `PROMPTS.md`, `backlog.yaml`). Existing files may not be modified, so this assessment lives in `docs/refactor-assessment/2026-11-05/`. Section 1.6 records what happened to the July items.

Companion files: [BACKLOG.md](BACKLOG.md), [PROMPTS.md](PROMPTS.md), [backlog.yaml](backlog.yaml).

---

## 1. Scope and confidence

### 1.1 What this repository is

A self-hosted podcast manager with three deployable units:

| Unit | Code | Chosen depth |
| --- | --- | --- |
| **Briefcast app** — Go 1.26 HTTP API, cron jobs and Vue 3 SPA, SQLite by default (Postgres optional) | `main.go`, `controllers/`, `service/`, `db/`, `model/`, `internal/{feedmeta,id3meta,logging,podcastmarkdown,sanitize}`, `frontend/` | **Deep dive** (highest risk: owns all user data, all inbound HTTP, all outbound fetches) |
| **WhisperX worker** — Go supervisor plus Python transcription child, private HTTP API | `cmd/whisperx-worker`, `internal/whisperxworker`, `internal/transcription`, `scripts/whisperx_transcribe.py`, `docker/whisperx/` | Sampled sweep, key findings re-verified |
| **Operator tools** | `cmd/migrate_to_pg`, `cmd/backfill_canonical_transcripts`, `cmd/transcription-rollback`, `scripts/*.py`, `scripts/*.ps1`, `RELEASE*.ps1` | Architecture level |

Languages and frameworks: Go (Gin, GORM, zap, robfig/cron, gorilla/websocket), TypeScript and Vue 3 (Vite 7, Tailwind 4, axios, marked, DOMPurify), Python (feedparser, mutagen, WhisperX), PowerShell release tooling, Docker Compose. 461 tracked files; about 23,100 lines of non-test Go, 17,600 lines of Go tests, 14,100 lines of frontend source, 5,700 lines of Python.

### 1.2 Branch divergence (read this before planning work)

Three lines have diverged, and this assessment covers only the checked-out one.

| Ref | Commit | Relation to `HEAD` |
| --- | --- | --- |
| `origin/main` | `2335855` | 1 commit ahead of the merge base: "Fix WhisperX script syntax for deployed Python 3.12". **Not in `HEAD`.** |
| `feat/whisperx-service` | `68d1fb7` | 15 commits not in `HEAD` (telemetry, download recovery, user-agent handling, search enrichment, ignore-pattern fix, tracked app lock file, a feed-parsing panic fix). It lacks this branch's link-tracking commits. |
| `feat/show-note-link-tracking` (`HEAD`) | `69252ad` | Assessed here. |

I checked whether `feat/whisperx-service` already fixes the top findings. It fixes only two things: the nil-`channel` panic site in [F8] (`ff2a576`) and the clean-checkout build break in [F20] (`a68271e`, `6d21495`). Everything else in section 5 is present on both branches.

### 1.3 Baseline — commands run

All commands ran offline (`GOPROXY=off`, `GOFLAGS=-mod=readonly`) with the local Go 1.27.3 toolchain (`go.mod` says 1.26; images build with 1.26.0).

| Command | Result | Time |
| --- | --- | --- |
| `go build ./...` | Pass | 2 s (warm cache) |
| `go vet ./...` | Pass, no findings | <1 s (cached) |
| `go test <every package except the root package> -count=1` | **Pass: 13 packages ok, 2 with no test files, 0 failures** | 112 s (`service` 110 s, `controllers` 33 s, `db` 27 s) |
| Same with `-cover` | Pass. Statement coverage: `service` 77.4%, `db` 79.5%, `controllers` 68.4%, `internal/whisperxworker` 60.6%, `internal/transcription` 81.6%, `internal/podcastmarkdown` 71.2%, `internal/feedmeta` 90.3%, `cmd/migrate_to_pg` 33.7%, `cmd/backfill_canonical_transcripts` 15.4%, `cmd/whisperx-worker` 0%, `cmd/transcription-rollback` 0% | 113 s |
| `gofmt -l .` | Lists most Go files. Not a real signal here: the Windows checkout has CRLF working-tree line endings (`git ls-files --eol` shows `i/lf w/crlf`). | — |
| `git status --short` after the runs | Unchanged | — |

**The suite is green, with two caveats.**

- **Different SQLite driver locally.** `CGO_ENABLED=0` on this machine, so `db.Init()` fails and the test helpers fall back to the pure-Go `glebarez/sqlite` driver (`db/dbfunctions_test.go:25-26`, `controllers/controllers_test.go:33-34`). CI and production use `mattn/go-sqlite3` through `gorm.io/driver/sqlite` (`db/db.go:9`). A local green run therefore does not exercise the production driver.
- **Linux-only worker tests did not run.** The supervisor and guardian tests are build-tagged for Linux, so the worker's 60.6% is a Windows number.

### 1.4 Commands not run

| Command (source) | Why not |
| --- | --- |
| `go test .` (root package; part of CI `go test ./...`) | `main.go:18` autoloads `.env` from the repo root; that file holds real credentials and was off-limits. |
| `go test -race ./internal/transcription/... ./internal/whisperxworker/... ./service/...` (`ci.yml:29`) | Needs cgo; `CGO_ENABLED=0` locally. |
| `npm --prefix frontend run test:unit`, `npm --prefix frontend run build` (`ci.yml:51,54`) | Node and npm are not installed and `frontend/node_modules` is absent; installs are forbidden. |
| `just ci-python` = `uv sync --locked`, `ruff check`, `ruff format --check`, `mypy src`, `pytest`, `pip-audit` (`ci.yml:71`) | `uv` and `just` are not installed, there is no `.venv`, system Python is 3.12 (tooling requires 3.14); installs and network are forbidden. |
| `uv run pytest tests/test_packaging.py …`, image builds, acceptance scripts (`packaging.yml`, `worker-release.yml`) | Docker and network are forbidden. |

Maintainer action: run `go test . -count=1`, the race command, `npm --prefix frontend ci && npm --prefix frontend run test:unit && npm --prefix frontend run build`, and `just ci-python` once and record the result. Every verification command prescribed in `BACKLOG.md` and `PROMPTS.md` comes from the two tables above.

### 1.5 Review coverage

- **Read fully (by me):** `main.go`; `controllers/podcast.go`, `settings.go`, `links.go`, `websockets.go`, `episode_media.go`, `downloads.go`, `summaries.go`, `local_search.go`, `favorites.go`; `db/dbfunctions.go`, `db.go`, `config.go`, `podcast.go`, `migrations.go`, `query_helpers.go`, `base.go`, `search_helpers.go`, `show_note_links.go`; `service/podcastService.go` (lines 73–141 and 160–1376), `fileService.go`, `outbound_url_policy.go`, `download_manager.go`, `downloadsService.go`, `asset_paths.go`, `retention.go`, `summarize.go`, `briefpoint.go`, `workerpool.go`, `feedparser.go`, `itunesService.go`, `gpodderService.go`; `internal/logging/gin.go`; `docker-compose.yml`; `Dockerfile`; `ci.yml`, `packaging.yml`, `secret-scan.yml`; `justfile`; `pyproject.toml`; `go.mod`; the July assessment.
- **Sampled (by me):** `controllers/pages.go`, `db/link_stats.go`, `service/export.go`, `app_logs.go`, `summary_backfill.go`, `chapters.go`, `hostLimiter.go`, `internal/logging/logging.go`, `internal/sanitize/sanitize.go`, `main_test.go`, test helpers.
- **Covered by three delegated read-only sweeps** (worker unit and its client; frontend; delivery, CI and scripts). Each sweep read its files fully. I re-read every line cited from them in section 5 before writing it down; items I did not re-read are marked *Likely* or attributed.
- **Not read:** `service/markdown.go`, `markdown_provenance.go`, `repair_work.go`, `local_search.go`, `canonical_transcript*.go`, `openrouter_models.go` beyond greps; most test bodies; `frontend/src/style.css`; lock files (metadata only); `webassets/` (vendored); `docs/whisperx-separation/STAGE_*.md`.
- **Deliberately not opened:** `.env`, `secrets/`, `config/`, `assets/`, `logs/`, `worker-state/`, `worker-cache/`. `.env.example` could not be read (blocked by the repo's own agent permission rules), so claims about defaults come from compose files and the README.

### 1.6 Status of the July 2026 backlog

| July item | Status at `69252ad` | Now tracked as |
| --- | --- | --- |
| P1 column-scoped state updates | Characterization tests landed (`service/state_transitions_characterization_test.go`, 15 tests). Implementation not done: 21 `db.UpdatePodcastItem` call sites remain. | P3 |
| P2 atomic job locks | Done (`RefreshEpisodes`, `DownloadMissingEpisodes`, `ApplyRetentionPolicies` use `db.TryLock`). `RefreshEpisodes` has no lease heartbeat. | Heartbeat in P17 |
| P3 state-changing GET aliases | Open (`main.go:106,110,112,126,128,130,132,135,143`) | P8 |
| P4 server timeouts, settings caching | Open (`main.go:55`, `main.go:198`) | P6, P14 |
| P5 SQLite busy timeout and WAL | Open, and restated: see [F7] | P2 |
| P6 download fd hygiene | Done (`service/fileService.go:128-133`) | — |
| P7 backup integrity | Open | P11 |
| P8 decouple LLM calls from refresh | Open | P17 |
| P9 silent handler error paths | Open | P16 |
| P10 `AddPodcast` error branch, unique URL | Open | P18 |
| P11 column-scoped heavy queries | Open | P14 |
| P12 repo hygiene | Done | — |
| P13 dead legacy UI | Open | P20 |
| P14 WebSocket hub hardening | Open; the SPA has no WebSocket code at all | P20 (delete, pending a maintainer answer) |
| P15 consolidate settings writes | Open; deferred, see the do-not-do list | — |
| P16 `go vet` in CI | Open | P10 |
| P17 bound `UpdateAllFileSizes` | Open | P14 |
| P18 bookmark and favorite coupling | Open; still a maintainer question | Open question |

### 1.7 Blind spots and confidence

No runtime, profiling, or production-scale data. No behaviour was executed beyond the test suite. Docker, Postgres and network paths were not exercised. Release scripts were read, not run.

**Overall confidence: Medium-High for the app, Medium for the worker, frontend and delivery.** Code-level findings carry snippets re-verified on 2026-11-05. Runtime consequences that were reasoned rather than reproduced say so.

---

## 2. Executive summary

- **Risk 1 — data loss on podcast delete.** Asset folders are keyed only by the sanitized podcast title. Deleting a podcast whose title matches another podcast's, or matches a category directory name such as `audio`, removes other podcasts' files. [F1] → P5.
- **Risk 2 — a storage hiccup resets the library.** The 15-minute `CheckMissingFiles` job treats any `stat` error as "file deleted", clears the stored path, and re-queues or marks deleted every downloaded episode. [F2] → P7.
- **Risk 3 — open by default.** Compose publishes port 8080 on all interfaces with an empty `PASSWORD`, the app container runs as root, and nine `GET` routes still delete, download or change state. A crafted player link is enough to delete an episode. [F4] [F5] → P1, P8.
- **Quick win 1:** guard podcast folder deletion (P5) — one function, small tests, no approval needed.
- **Quick win 2:** contain panics in background goroutines (P4). There is no `recover()` anywhere in non-test code, so one malformed feed during an OPML import can stop the server. [F8]
- **Quick win 3:** stop the local transcription backend deleting audio when child output merely contains "No such file or directory" (P9). [F9]
- **Foundational 1:** replace whole-row `Save` with column-scoped updates (P3). The characterization tests the July assessment asked for now exist. [F6]
- **Foundational 2:** make SQLite concurrency explicit and driver-independent, then offer WAL once backups are snapshot-based (P2, P11). [F7] [F15]
- **Foundational 3:** converge the three branches and add the missing CI gates (P10). At `HEAD` a clean checkout cannot build the app image. [F20] [F26]
- **Biggest testing gap:** the frontend has no component, composable or end-to-end tests, and its sanitizer test never imports the production sanitizer. On the Go side, local runs silently use a different SQLite driver than CI and production. [F24] [F7]
- **Biggest architectural risk:** process-wide mutable state (`db.DB`, pause and cancel maps, job flags) plus 26 fire-and-forget goroutines with no owner, no cancellation and no panic boundary. Correctness depends on convention.
- **Recommended first PR:** P5, podcast delete path guards. It is the smallest change that closes a confirmed data-loss path. P1 scores higher but changes deployment defaults and needs a maintainer decision first.

---

## 3. Category scorecard

| Category | Rating | Rationale |
| --- | --- | --- |
| Correctness and reliability | 🔴 Red | Three data-integrity paths in delete, missing-file and download code [F1–F3]; lost updates from whole-row saves [F6]; no panic containment [F8] |
| Security and privacy | 🟡 Yellow | Insecure deployment defaults and state-changing GETs [F4] [F5]. Offsetting strengths: dial-time SSRF policy, parameterized SQL, constant-time worker auth, server-side log redaction, no secrets in tracked files |
| Architecture | 🟡 Yellow | Clear controller, service and db layering and a well-isolated worker; undermined by global state, unowned goroutines, and a dead legacy UI layer still compiled in |
| Complexity | 🟡 Yellow | `transcribePendingEpisodes` is 292 lines, `AddPodcastItems` 196, `SettingsView.vue` 2,074; transcript parsing duplicated in two frontend files |
| Performance | 🟡 Yellow | Thumbnail and media handlers load whole rows including transcripts [F18]; cron jobs load every downloaded row with text blobs; `LIKE` scans over transcript JSON |
| Testing | 🟡 Yellow | Go suite green with 68–80% statement coverage in core packages and real characterization tests; but driver mismatch locally, race detector on three trees only, zero frontend component tests |
| Observability | 🟡 Yellow | Structured logs, request IDs, per-job loggers, redaction. No metrics on this branch, no app healthcheck, worker fault causes not logged, and a compose default that sends logs to a file named `both` [F22] |
| Delivery and developer experience | 🔴 Red | Diverged branches; clean checkout cannot build the app image [F20]; destructive legacy release scripts still runnable [F21]; no `go vet`, format or vulnerability gates [F26]. Strengths: digest-pinned bases, hashed locks, SBOM and provenance, hardened worker container |

---

## 4. Architecture map

### 4.1 Modules

| Module | Responsibility | Key dependencies | Risk | Notes |
| --- | --- | --- | --- | --- |
| `main.go` | Entry point, routes, optional Basic Auth, cron wiring | controllers, service, db, gin, cron | High | `r.Run()` with no server config; settings row read on every request; nine deprecated GET aliases |
| `controllers/` | HTTP handlers | service, db (6 direct `db.DB` uses), gin | High | 9 fire-and-forget goroutines; several handlers return nothing on error |
| `service/` (88 files) | Feeds, downloads, transcription client, summaries, retention, export, Briefpoint, link extraction | db, internal/*, Python helpers, outbound HTTP | High | Core logic and all filesystem writes; process-local pause, cancel and job flags |
| `db/` | GORM models, queries, hand-rolled migrations, job locks | gorm, sqlite and postgres drivers | High | Whole-row `Save`; `Preload(clause.Associations)` helpers; migrations log and continue on failure |
| `internal/transcription` | Worker protocol types and canonical transcript | — | Med | Shared contract between app and worker |
| `internal/whisperxworker`, `cmd/whisperx-worker` | Job journal, supervisor, Python runner, private HTTP API | modernc sqlite, `os.Root`, Python | Med | Careful durability and auth; liveness gaps [F10] [F11] |
| `internal/feedmeta`, `id3meta`, `podcastmarkdown` | Feed, ID3 and Markdown export logic | — | Low–Med | Mostly pure, well tested |
| `internal/logging` | zap setup, request and job loggers | zap, lumberjack | Low | Solid |
| `internal/sanitize` | Vendored filename sanitizer | — | Low | Leave alone |
| `scripts/*.py` | Feed parsing, ID3 extraction, transcription, recovery, acceptance | feedparser, mutagen, whisperx | Med | JSON over stdio with Go |
| `frontend/` | Vue 3 SPA served at `/app` | axios, marked, DOMPurify | Med | No state library; polling only; no tests beyond three `lib` files |
| `client/`, most of `controllers/pages.go`, `controllers/websockets.go`, most of `webassets/` | Legacy UI | — | Low | Dead: no template loader, routes unregistered, SPA never opens `/ws` [F28] |
| `RELEASE*.ps1`, `scripts/*.ps1` | Release, deploy, recovery | docker, pwsh | Med | New pipeline is careful; three legacy scripts are destructive [F21] |

### 4.2 Core workflows

**Request flow.** gin → request logger (assigns a request ID) → `setupSettings` (one settings query per request, `main.go:198`) → `gin.Recovery` → optional Basic Auth for the fixed user `briefcast`, only when `PASSWORD` is set (`main.go:75`) → handler. Handlers often start a goroutine and return 200 or 202 immediately. There is no CSRF defence, no request body limit except on OPML upload, and no server timeouts.

**Feed refresh** (cron every `CHECK_FREQUENCY` minutes, plus API triggers). `RefreshEpisodes` takes a 120-minute lease with no heartbeat, then runs a worker pool over all podcasts. `AddPodcastItems` fetches the feed, parses it in a Python subprocess, and for each new episode synchronously fetches chapters and transcripts, inserts the row, extracts show-note links, exports files, and may call the LLM. It then starts `DownloadMissingEpisodes` in a goroutine.

**Downloads.** `DownloadMissingEpisodes` (lease with heartbeat) → worker pool → `DownloadWithFallback` → `Download` (Range resume, pause and cancel flags in process memory, progress updates) → `SetPodcastItemAsDownloaded` (whole-row save plus an ID3 subprocess).

**Transcription.** Cron `TranscribePendingEpisodes` → DB lease with heartbeat → backend chosen by `WHISPERX_BACKEND`. Compose pins `http`: the app hashes the audio, persists an envelope, `PUT`s the job to the worker, polls every two seconds, validates and commits the result with a receipt, then acknowledges. The code default is `local`, which runs the Python script in-process.

**Deletion and retention.** `DeletePodcast` deletes item files, deletes item rows one transaction at a time, removes title-keyed folders, then deletes the podcast row. `CheckMissingFiles` reconciles the DB against disk every cycle. `ApplyRetentionPolicies` runs daily.

### 4.3 Cross-cutting patterns

- **Config:** environment variables plus a singleton `settings` row; `GetOrCreateSetting` is called from 22 places and performs self-healing writes.
- **Errors:** logged through zap with context; 24 explicitly discarded errors in `service`, `controllers` and `db`, mostly state-setter calls on failure paths.
- **Logging:** structured, request and job IDs, server-side redaction for the log viewer. No metrics or traces on this branch.
- **Authentication:** one optional shared password over plain HTTP; the worker uses a bearer token file with constant-time comparison.
- **Data access:** package-level `db.DB`; GORM with parameterized queries; one string-built `ORDER BY CASE` that escapes quotes by hand (`db/dbfunctions.go:251-263`).
- **External calls:** one outbound policy (scheme, userinfo, private-IP check at dial time, redirect re-validation, 20 MB body cap, per-host limiter). Exceptions: media and image downloads have no size cap, redirects have no hop limit (`service/fileService.go:542-545`), and each call builds a new `http.Transport`.
- **Subprocesses:** fixed argv, no shell, timeouts. The app passes its whole environment to helpers; the worker uses an allow-list.
- **Testing:** table tests, `httptest`, temp-dir SQLite, env-gated real-model tests; good characterization suites for state transitions and WhisperX.

### 4.4 Hotspots

Churn is computed from `git log --since='12 months ago'`. Lines and longest-function figures are computed. Risk is judgment.

| # | Location | Churn | Lines | Package coverage | Why it matters | Risk |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | `service/podcastService.go` | 21 | 1,376 | 77.4% | Refresh, add, delete, state setters, locks; `AddPodcastItems` is 196 lines | High |
| 2 | `service/fileService.go` | 17 | 653 | 77.4% | Download loop (134 lines), folder deletion, backup | High |
| 3 | `db/dbfunctions.go` | 31 | 897 | 79.5% | All persistence, whole-row save, locks | High |
| 4 | `main.go` | 27 | 341 | not measured | Routes, auth boundary, server lifecycle | High |
| 5 | `controllers/podcast.go` | 16 | 1,019 | 68.4% | Largest API surface; silent error paths | High |
| 6 | `service/whisperx.go` | 19 | 772 | 77.4% | `transcribePendingEpisodes` is 292 lines; local backend deletes audio | High |
| 7 | `db/migrations.go` | 25 | 348 | 79.5% | Hand-rolled migrations that continue after failure | Med |
| 8 | `service/transcription_remote*.go`, `transcription_http.go` | new | 1,260 | 77.4% | Envelope and receipt state machine | Med |
| 9 | `internal/whisperxworker/worker.go` | new | 615 | 60.6% (Windows) | Job journal and fault handling | Med |
| 10 | `controllers/settings.go` | 20 | 519 | 68.4% | Settings patch surface | Med |
| 11 | `service/summarize.go` | 10 | 505 | 77.4% | LLM client; four whole-row saves around a 120 s call | Med |
| 12 | `frontend/src/views/SettingsView.vue` | 18 | 2,074 | none | Saves defaults after a failed load | Med |
| 13 | `frontend/src/views/SummariesView.vue` | 10 | 1,632 | none | Only `v-html` sink; stale-response races | Med |
| 14 | `docker-compose.yml` | 16 | 71 | — | Exposure and logging defaults | High |
| 15 | `scripts/whisperx_transcribe.py` | 8 | 1,074 | Python tests not run | Child contract with the worker | Med |
| 16 | `db/podcast.go` | 28 | 322 | 79.5% | Schema source of truth; no uniqueness on URL or GUID | Med |

---

## 5. Critical findings

Severity: High or Medium (no finding is rated Critical). Confidence: **Confirmed** = the cited lines were re-read on 2026-11-05 and the snippet is verbatim; **Likely** = strong structural inference; **Hypothesis** = needs maintainer validation. "Not reproduced" means the consequence was derived by reading, not by running.

### Stop-the-line notes

- **[F1]** Until P5 lands, do not delete a podcast that shares its title with another podcast, or whose title is `audio`, `images`, `transcripts`, `summaries` or `markdown`.
- **[F2]** Until P7 lands, treat any outage of the `/assets` mount as an incident: stop the app before the next `CheckMissingFiles` cycle if the volume is unavailable.
- **[F4] [F5]** If any instance is reachable from outside a trusted LAN without `PASSWORD`, fix that today; it does not need a code change.
- **[F9]** If any deployment runs `WHISPERX_BACKEND=local` (the code default; the legacy compose file does not set it), a missing `ffmpeg` or model path deletes queued audio.

---

**[F1] Podcast deletion removes folders shared with other podcasts — High, Confirmed (not reproduced), Data integrity** → P5

`service/fileService.go:607` and `:623`:

```go
        dataPodcastFolderPath(folder),
...
        if err := os.RemoveAll(folderPath); err != nil && firstErr == nil {
```

`service/asset_paths.go:75`: `return path.Join(resolveAssetsDir(), sanitizeAssetName(podcastName))`

- **Observed.** `deletePodcastFolder` is called with `podcast.Title` (`service/podcastService.go:1199`) and removes seven directories derived only from the sanitized title: `DATA/<title>` and `DATA/<category>/<title>` for six categories. The sanitizer keeps case and does not lower-case (`internal/sanitize/sanitize.go:362-380`).
- **Three ways it goes wrong.** (a) Two podcasts with the same sanitized title share every category folder, so deleting one removes the other's audio, images, transcripts, summaries and Markdown. This happens when a show moves feeds and the user adds the new URL before deleting the old one. (b) The legacy path `DATA/<title>` collides with the category directories themselves (`service/asset_paths.go:12-17`): a podcast titled exactly `audio` resolves to `DATA/audio`, which passes `IsPathWithinAssetsDir` and is removed for every podcast. (c) The call at `:1199` sits outside the `if deleteFiles` block that starts at `:1182`, so `DELETE /podcasts/:id/podcast` (`DeleteOnlyPodcastByID`, which passes `false`) removes the folders too. Whether "keep the files" was the intent of that route is a maintainer question; the parameter name says it was.
- **Why it matters.** Unrecoverable loss of other podcasts' media; the DB still says "downloaded", so [F2] then re-queues everything.
- **Action.** Skip the legacy path when the sanitized title is a reserved category name; skip folder removal when another podcast shares the sanitized title (per-item files are already deleted individually).
- **Tests.** Temp-dir tests for both cases. **Blocks refactoring:** yes for anything touching asset paths.

---

**[F2] `CheckMissingFiles` treats any stat error as a deleted file — High, Confirmed mechanism / Likely impact, Data integrity** → P7

`service/fileService.go:402-403`:

```go
    _, err := os.Stat(filePath)
    return err == nil
```

`service/podcastService.go:924-925` and `:931`:

```go
        fileExists := FileExists(item.DownloadPath)
        if !fileExists {
...
                if setErr := SetPodcastItemAsNotDownloaded(item.ID, db.NotDownloaded); setErr != nil {
```

- **Observed.** The job runs every `CHECK_FREQUENCY` minutes over every downloaded episode. Permission errors, I/O errors and an unmounted volume all read as "missing". `SetPodcastItemAsNotDownloaded` then clears `DownloadPath` (`service/podcastService.go:735`) and sets the status to `NotDownloaded`, or to `Deleted` when "don't re-download deleted files" is on.
- **Why it matters.** One transient mount failure re-queues the whole library for download, or marks it all deleted. The stored paths are gone either way.
- **Action.** Treat only `fs.ErrNotExist` as missing; abort the pass when the data root is unavailable; add a circuit breaker for implausibly large "missing" counts.
- **Tests.** Missing root, mixed present and absent files, injected stat error. **Blocks refactoring:** no.

---

**[F3] Episode files are named by title only, and resume appends without validation — High, Confirmed (not reproduced), Data integrity** → P12

`service/fileService.go:74`, `:82-83`, `:88`, `:116`:

```go
    fileName := getFileName(link, episodeTitle, ".mp3")
...
    if info, statErr := os.Stat(finalPath); statErr == nil {
        resumeOffset = info.Size()
...
        req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeOffset))
...
        file, err = os.OpenFile(finalPath, os.O_WRONLY|os.O_APPEND, 0o644)
```

- **Observed.** The path is `audio/<podcast>/<kebab-title><ext>`, plus an optional date or number prefix that is off by default (`db/podcast.go:210-211`). Any file already at that path is treated as a partial download of *this* URL.
- **Consequences.** Two episodes of one podcast with the same title ("Trailer", "Bonus") collide: the second either appends its tail to the first file (on `206`) or deletes and replaces it (on `416`), and both rows then point at one file. There is no `If-Range` or ETag check, so resuming against hosts that insert ads dynamically stitches two different encodes. Nothing prevents two concurrent downloads of one item (`DownloadSingleEpisode` takes no lock; `service/download_manager.go` holds only pause and cancel flags).
- **Action.** Disambiguate on collision with a short episode-ID suffix, send `If-Range` when resuming, and add an in-flight guard per item.
- **Tests.** `httptest` servers for both collision outcomes and for a changed entity. **Blocks refactoring:** no.

---

**[F4] State-changing GET routes, reachable through a crafted player link — High, Confirmed, Security** → P8

`main.go:143`:

```go
    router.GET("/podcastitems/:id/delete", controllers.DeletePodcastItem) // deprecated
```

`frontend/src/lib/api/episodes.ts:40`:

```ts
    return httpClient.get<PodcastItem>(`/podcastitems/${id}`);
```

- **Observed.** Nine deprecated GET aliases remain: download-all, pause, unpause, mark played or unplayed, bookmark, unbookmark, download, delete. The SPA never calls them on purpose, but `PlayerView.vue` takes `itemIds` from the URL (`frontend/src/views/PlayerView.vue:71`) and passes each one unencoded to `getById` (`:158`). Go redirects `/player?…` into the SPA (`main.go:90-96`).
- **Why it matters.** `/app/#/player?itemIds=<uuid>/delete` makes the user's own browser issue an authenticated same-origin `GET /podcastitems/<uuid>/delete`. Any page can also fire the aliases cross-site with an `<img>` tag; cached Basic Auth credentials are sent automatically.
- **Action.** Remove the nine aliases (API-changing) and `encodeURIComponent` every ID in `frontend/src/lib/api/*`.
- **Tests.** Route test asserting GET on those paths is not routed; a unit test for path building. **Blocks refactoring:** no.

---

**[F5] Unauthenticated by default, published on all interfaces, running as root — High, Confirmed, Security** → P1

`docker-compose.yml:14` and `:56`:

```yaml
      - PASSWORD=${PASSWORD:-}
...
      - ${HOST_PORT:-8080}:8080
```

`Dockerfile:38-39` (and no `USER` before `ENTRYPOINT` at `:59`):

```dockerfile
RUN chmod 777 /config; \
    chmod 777 /assets
```

- **Observed.** With `PASSWORD` empty the router has no auth middleware at all (`main.go:75`). The port mapping has no host address, so Docker binds every interface. The same defaults are in `docker-compose.legacy.yml:13,54`.
- **Why it matters.** Anyone who can reach the host can delete the library, queue unlimited downloads, read logs, and change the Briefpoint server URL so the next sync sends the API key and transcripts elsewhere (`service/briefpoint.go:225`, `controllers/settings.go:241-243`). The process doing this runs as root over host bind mounts.
- **Action.** Bind `127.0.0.1` by default with an explicit variable to widen it, and warn at startup when `PASSWORD` is empty. Non-root is a separate, riskier change.
- **Tests.** Rendered-compose assertion in `tests/test_packaging.py::test_compose_matrix`. **Blocks refactoring:** no. Deployment-changing: needs maintainer approval.

---

**[F6] Whole-row `Save` loses concurrent updates — High, Confirmed, Data integrity** → P3

`db/dbfunctions.go:566`:

```go
    tx := DB.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).Omit("Podcast").Save(&podcastItem)
```

- **Observed.** 21 call sites (10 in `service/podcastService.go`, 5 in `whisperx.go`, 4 in `summarize.go`, 1 each in `alternate_feeds.go` and `chapters.go`). Many save a snapshot loaded long before: `SummarizeEpisode` saves at `service/summarize.go:389` and again at `:422` either side of an LLM call that can take 120 seconds; `DownloadWithFallback` saves the job-start snapshot at `service/podcastService.go:994`.
- **Why it matters.** Anything the user or another job changed in between — played, bookmark, download status — is silently overwritten.
- **Action.** Column-scoped updates, one setter family per PR, protected by the existing characterization suite.
- **Blocks refactoring:** yes — do this before restructuring the refresh, download or summary flows.

---

**[F7] SQLite concurrency settings are implicit and differ by driver — High, Confirmed code / Likely impact, Reliability** → P2

`db/db.go:40`: `db, err := gorm.Open(dialector, &gorm.Config{})`
`db/config.go:185`: `maxOpen := getEnvInt("DB_MAX_OPEN_CONNS", 25)`

- **Observed.** The app sets no journal mode, busy timeout or SQLite-specific pool size. A grep for `PRAGMA`, `busy_timeout` and `journal_mode` matches only the worker's own store (`internal/whisperxworker/store.go:66`). Production inherits `mattn/go-sqlite3`'s built-in 5-second busy timeout and rollback-journal mode. The CLI tools and local tests use `glebarez/sqlite`, whose defaults differ.
- **Why it matters.** In rollback-journal mode a long read blocks every writer. The code has such reads: `lower(transcript_json) like ?` (`db/search_helpers.go:29`) and full-row loads of every downloaded episode each cycle (`db/dbfunctions.go:312`). A writer blocked for more than five seconds fails, and failure-path state writes are often discarded (`_ = SetPodcastItemAsNotDownloaded(...)`, `service/podcastService.go:867,880,895`), leaving items stuck. The maintainer's notes already record that production SQLite blocks on long reads.
- **Action.** Set the busy timeout explicitly on every pooled connection for whichever driver is compiled in, cap the SQLite pool, and make WAL an opt-in that waits for P11.
- **Tests.** A test that reads `PRAGMA busy_timeout` on several concurrently held connections. **Blocks refactoring:** no.

---

**[F8] No panic boundary around background work — High, Confirmed, Reliability** → P4

`service/workerpool.go:33-37`:

```go
        go func() {
            defer wg.Done()
            for job := range jobChannel {
                fn(job)
```

`service/podcastService.go:252` and `:257`:

```go
    channel, err := xmlquery.Query(doc, "//channel")
...
    iimage := channel.SelectElement("itunes:image")
```

- **Observed.** `recover()` appears nowhere in non-test Go. There are 26 goroutine launch sites in `main.go`, `controllers/` and `service/`. `gin.Recovery` covers only request goroutines and cron has its own wrapper; everything else is unprotected.
- **A concrete trigger.** `xmlquery.Query` returns `nil, nil` when nothing matches, so a feed with no `<channel>` (Atom) and no image dereferences nil at line 257. During an OPML import that runs inside `runWorkerPool` (`service/podcastService.go:184-185`), which ends the process. `feat/whisperx-service` fixes this one site (`ff2a576`, "Avoid panic when a feed has no RSS channel element"); `HEAD` does not have it. *The crash itself is Likely — not executed here.*
- **Related.** `main.go:55` and `:321` have no signal handling, so there is no graceful shutdown either (see [F16]).
- **Action.** A small `safeGo` helper and a per-job recover in the pool; guard the nil.
- **Tests.** A pool test with a panicking job; a feed fixture without `<channel>`. **Blocks refactoring:** no.

---

**[F9] Local transcription backend deletes audio on a substring match — High, Confirmed, Data integrity** → P9

`service/whisperx.go:400` and `:408`:

```go
            if strings.Contains(errMsg, "Failed to load audio") || strings.Contains(errMsg, "No such file or directory") {
...
                if removeErr := os.Remove(item.DownloadPath); removeErr != nil && !os.IsNotExist(removeErr) {
```

- **Observed.** The error text includes the child's output. A missing `ffmpeg` binary or model cache path produces "No such file or directory", so valid audio is deleted and the item reset for download with no backoff. The repo already pins this as a known defect (`service/whisperx_characterization_test.go:291-293`). The code default backend is `local` (`service/whisperx.go:101`); `docker-compose.yml:29` pins `http`, the legacy compose file does not.
- **Action.** Delete or reset only when a local `os.Stat` proves the file is absent; otherwise schedule a retry.
- **Tests.** Flip the `missing-model-path` characterization case. **Blocks refactoring:** no.

---

**[F10] Resumed worker jobs can be blocked permanently by a status mismatch — High, Confirmed on both sides / Likely at runtime, Contract** → P13

`internal/whisperxworker/worker.go:468-469`:

```go
    if r.Checkpoint != nil {
        r.Status.CompletedSeconds = max(r.Status.CompletedSeconds, r.Checkpoint.CompletedSeconds)
```

- **Observed.** A job submitted with `resume_from_job_id` inherits the checkpoint (`worker.go:265-268`) but starts with `TotalSeconds` 0. Until the script reports the real total, status reads completed = N, total = 0. The client rejects any status where `s.TotalSeconds < s.CompletedSeconds` (`service/transcription_http.go:408`) as non-retryable and marks the episode blocked. *Found by the worker sweep; I re-read the three cited sites.*
- **Why it matters.** Checkpoint resume exists for long CPU jobs; those are the ones that get stranded and need manual repair, while the worker keeps running the job.
- **Action.** Seed `TotalSeconds` from the previous attempt in the resume branch. **Tests.** A resumed run whose first progress tick has total 0.

---

**[F11] A worker storage fault is sticky, silent and never fatal — Medium, Confirmed, Reliability** → P13

`internal/whisperxworker/worker.go:120` and `:124`:

```go
    w.fault = err
...
    slog.Error("worker_state_unavailable")
```

- **Observed.** `fault` is never cleared and the serve loop has no exit on it (`cmd/whisperx-worker/main.go:125-137`). The error value is not logged, here or at `main.go:134` (`slog.Error("worker_gc_failed")`). Jobs keep reporting `running`; the client polls with no overall deadline and holds the app's transcription lease.
- **Action.** Exit non-zero on fault so the restart policy runs recovery; log the cause.

---

**[F12] `AddPodcast` reports DB errors as "already exists"; nothing enforces uniqueness — Medium, Confirmed, Correctness** → P18

`service/podcastService.go:276` and `:319`:

```go
    if errors.Is(err, gorm.ErrRecordNotFound) {
...
    return podcast, &model.PodcastAlreadyExistsError{URL: url}
```

- **Observed.** Any error other than not-found falls through to the 409 branch. `Podcast.URL` (`db/podcast.go:19`) and `PodcastItem.GUID` (`db/podcast.go:65`) have no unique index. `SetAllEpisodesToDownload` calls `AddPodcastItems` from a request handler without the refresh lock (`service/podcastService.go:761`), so it can race the cron refresh and insert duplicate episodes (*Likely*).
- **Action.** Handle the error branch; dedupe and add unique indexes behind a backup (data-changing).

---

**[F13] `DeletePodcast` is not atomic — Medium, Confirmed, Data integrity** → P5 (second slice)

`service/podcastService.go:1193`, `:1199`, `:1204`:

```go
            if deleteErr := db.DeletePodcastItemByID(item.ID); deleteErr != nil {
...
    err = deletePodcastFolder(podcast.Title)
...
    err = db.DeletePodcastByID(id)
```

- **Observed.** Items are deleted one transaction each; then folders; then the podcast row. If folder removal fails, the function returns with every item gone and the podcast still present. The next refresh sees every GUID as new and, with auto-download on, re-downloads the back catalogue. `podcast_tags` rows are never removed.
- **Action.** One transaction for all rows first, then best-effort file removal.

---

**[F14] Image downloads leak response bodies and accept any response — Medium, Confirmed, Reliability** → P15

`service/fileService.go:291-293` (the same shape at `:335-338`):

```go
    if _, err := os.Stat(finalPath); !os.IsNotExist(err) {
        changeOwnership(finalPath)
        return finalPath, nil
```

- **Observed.** The HTTP request is made first; `defer resp.Body.Close()` is registered only at `:302`, after this early return and after the `os.Create` error return. There is no status check and no size cap, so an error page is saved as `.jpg` and a large response fills the disk. Each call also builds a fresh `http.Transport` (`service/outbound_url_policy.go:87`).

---

**[F15] Scheduled backup is not a consistent snapshot — Medium, Confirmed code / Likely corruption, Data** → P11

`service/fileService.go:411-412`:

```go
    err := filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
        if !info.IsDir() {
```

- **Observed.** The walk callback ignores `err` and dereferences `info`. `CreateBackup` creates the tarball (`:481`) before checking the DB file exists (`:487-491`), so Postgres deployments accumulate empty archives every 48 hours. It copies the live database file (`:499`) with no snapshot, always under the fixed name `briefcast.db` even when `DATABASE_URL` points elsewhere, and close errors on the tar and gzip writers are dropped by `defer`. `docs/backup-and-restore.md:7-13` already documents that this backup is not consistent.

---

**[F16] No server timeouts, body limits or graceful shutdown — Medium, Confirmed, Reliability** → P6

`main.go:55`: `if err := r.Run(); err != nil {`

- **Observed.** Default `http.Server`; no `ReadHeaderTimeout` or `IdleTimeout`; no signal handling. The only request size limit is on OPML upload (`controllers/pages.go:375`). `gin.Recovery` is registered after the settings middleware (`main.go:64-65`). `podcastindex.NewClient` (`service/itunesService.go:70`) uses `http.DefaultClient`, which has no timeout, so `GET /search` can hang a request indefinitely. The WebSocket upgrader sets no read limit or deadlines (`controllers/websockets.go:93`).

---

**[F17] Handlers that answer nothing, or the wrong thing, on error — Medium, Confirmed, Correctness** → P16

`controllers/podcast.go:861-863`:

```go
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
        }
```

- **Observed.** No `return` after the 400, so the handler continues. `GetAllPodcasts` returns an empty 200 when binding fails (`:101`); `GetTagByID` when the query fails (`:783-786`); `DeleteTagByID`, `AddTagToPodcast`, `RemoveTagFromPodcast` and the three image and file handlers likewise. `PatchPodcastItemByID` ignores the update error and cannot set false or empty values because it passes a struct to `Updates` (`:696`). `DeletePodcastItem` returns 200 before the deletion runs.

---

**[F18] Hot handlers and cron jobs load whole rows including transcripts — Medium, Confirmed, Performance** → P14

`controllers/podcast.go:526` (image handler; `:569` is the media handler): `err := db.GetPodcastItemByID(searchByIDQuery.ID, &podcast)`
`db/dbfunctions.go:163`: `result := podcastItemsWithAssociations(DB).First(&podcastItem, "id=?", id)`

- **Observed.** Every thumbnail request and every Range request for audio loads the full episode row — transcript JSON, canonical transcript, summary, prompts — plus the podcast (`db/query_helpers.go:19`). `CheckMissingFiles` loads every downloaded row the same way each cycle (`db/dbfunctions.go:312`). The settings row is read on every request (`main.go:198`). A lightweight column list exists (`db/dbfunctions.go:212-226`) and is used by one query.

---

**[F19] Feed refresh blocks on LLM calls and remote fetches, under a lease with no heartbeat — Medium, Confirmed, Reliability** → P17

`service/podcastService.go:494`:

```go
                if sumErr := SummarizeEpisode(&podcastItem, llmCfg, summarizationPrompt, summarizationUserPrompt); sumErr != nil {
```

- **Observed.** Per new episode, inside the refresh worker: a chapters fetch (`:409`), transcript fetches (`:429`) and an LLM call. `RefreshEpisodes` takes a 120-minute lease (`:1061`) and releases it with a plain `defer` (`:1075`); unlike the download job it never renews it.

---

**[F20] A clean checkout of `HEAD` cannot build the app image — Medium, Confirmed, Delivery** → P10

`.gitignore:19`: `app` and `Dockerfile:27`: `COPY docker/app/requirements.lock /opt/briefcast/requirements.lock`

- **Observed.** The unanchored ignore pattern matches `docker/app/`. `git check-ignore -v docker/app/requirements.lock` reports that rule and `git ls-files docker/app` is empty; a local copy on disk hides the problem. Fixed on `feat/whisperx-service` by `a68271e` and `6d21495`. `HEAD` also lacks `origin/main`'s Python 3.12 syntax fix (`2335855`).

---

**[F21] Destructive legacy release scripts are still tracked and runnable — Medium, Confirmed, Operations** → P20

`RELEASE_RESET.ps1:296`:

```powershell
    $resetSql = "TRUNCATE TABLE IF EXISTS podcast_tags,podcast_items,podcasts,tags,settings,migrations,job_locks RESTART IDENTITY CASCADE;"
```

- **Observed.** After one typed `yes`, the script deletes the SQLite files, the backups directory and everything under the assets directory (`:301-322`). It contains no `$LASTEXITCODE` check (count 0, also 0 in `RELEASE_RUN.ps1`), so a failed step does not stop the deletions. The table list omits four current tables (`show_note_links`, `link_targets`, `prompt_versions`, `markdown_revisions`). The supported pipeline refuses `reset` (`scripts/ReleasePackaging.ps1:31`), but the old file runs directly. *Found by the delivery sweep; cited lines re-read.*

---

**[F22] Compose default `LOG_OUTPUT=both` is not a recognised value — Medium, Confirmed, Observability** → P1 (second slice)

`docker-compose.yml:20`: `- LOG_OUTPUT=${LOG_OUTPUT:-both}` and `internal/logging/logging.go:138-139`:

```go
        default:
            path := strings.TrimPrefix(token, "file:")
```

- **Observed.** Only `stdout` and `stderr` are keywords; anything else is a file path. Unless the env file sets `LOG_OUTPUT`, logs go to a file named `both` in `/api` and nothing reaches `docker logs`. Whether `.env.example` overrides it was not checked (file unreadable here).

---

**[F23] Helper subprocesses inherit every secret; the feed helper may open the payload as a path — Medium, Confirmed (environment) / Likely (path), Security** → P15

`service/feedparser.go:117`: `base := os.Environ()` and `service/transcription_local.go:143-144`:

```go
    cmd.Env = append(
        os.Environ(),
```

- **Observed.** The Python processes that parse untrusted feeds and audio receive the database URL, LLM key and password. The worker already uses an allow-list (`internal/whisperxworker/python.go:53-59`).
- **Likely.** `scripts/feedparser_parse.py:54` calls `feedparser.parse(raw)` with bytes. feedparser 6 tries its argument as a filename before treating it as content, so a feed body that is exactly a path such as `/dev/zero` would be opened. Not verified against the installed library (not available locally). Passing `io.BytesIO(raw)` removes the question.

---

**[F24] Frontend: saves over real data after a failed load; no component tests — Medium, Confirmed (static), Data integrity and Testing** → P19

`frontend/src/views/SettingsView.vue:256-257`:

```ts
  } catch (error) {
    setError(getErrorMessage(error, "Failed to load settings."));
```

- **Observed.** After a failed `GET /settings`, every form still renders (`v-if="!isLoading"`, seven cards) with empty defaults, and Save sends them: summarization off, prompts and model empty, Briefpoint URL empty. The alternate-feeds dialog does the same (`frontend/src/views/DashboardView.vue:160-161`).
- **Also confirmed.** The player never applies `start` or autoplays on first load: `playAt` runs while `<audio>` is unmounted (`PlayerView.vue:178-179`, `:192-193`, template `:374` and `:458`). List and reader fetches have no latest-wins guard. `frontend/src/lib/sanitize.test.ts:9-10` defines its own `sanitizeHtml` and never imports the production one. *Found by the frontend sweep; cited lines re-read.*

---

**[F25] Migrations continue after failure, and app deploys take no restore point — Medium, Confirmed, Data** → P11

`db/migrations.go:304-306`:

```go
        if err := ExecuteAndSaveMigration(mig.Name, mig.Query); err != nil {
            logging.Sugar().Errorw("database migration failed", "name", mig.Name, "error", err)
        }
```

- **Observed.** A failed `AutoMigrate` logs and returns (`db/db.go:58-60`) and `main.go:48` starts serving anyway. A failed dedupe migration is followed by the unique-index migration that depended on it. On deploy, only the worker path snapshots state (`scripts/ReleasePackaging.ps1:201`); the app path recreates the container and migrates forward, and rollback only reselects the old image.

---

**[F26] CI gates are thin — Medium, Confirmed, Delivery** → P10

`.github/workflows/ci.yml:29`:

```yaml
        run: go test -race ./internal/transcription/... ./internal/whisperxworker/... ./service/...
```

- **Observed.** No `go vet`, format check, static analysis or `govulncheck`. The race detector skips `controllers`, `db`, `cmd` and the root package. The frontend has no lint or audit step. `pip-audit` sees only the dev tooling environment, not the three shipped lock files. No action is pinned by commit SHA. Shipped Python runs on 3.12 while lint and type checks target 3.14 (`pyproject.toml:28`).

---

**[F27] Worker client lifecycle gaps — Medium, Confirmed in code (not re-verified line by line except where cited), Reliability** → P13

- An envelope pins the audio fingerprint and engine profile forever; a manual retry reuses it and blocks again.
- The app's expected execution timeout (`service/transcription_remote_state.go:204`) is compared only after the worker has accepted the job, and the two sides read different variables.
- The client never calls `/cancel`, so abandoned jobs occupy the single slot (`service/transcription_remote.go:235`).
- Worker job directories and journal rows are never deleted, and hourly pruning re-touches every already-pruned job under the global mutex (`internal/whisperxworker/artifacts.go:321`).

---

**[F28] Dead legacy UI layer — Low, Confirmed, Complexity** → P20

`main.go:68` says the legacy templates were removed, yet `controllers/pages.go` keeps eight handlers that call `c.HTML` with no template loader anywhere, `client/` holds 13 orphaned templates, `controllers/podcast.go:411` defines an unrouted misnamed handler, and `webassets/` ships Vue 2, axios 0.21 and other libraries of which the SPA uses only `blank.png`. The SPA contains no WebSocket code, so `/ws` and the hub have no in-repo caller.

---

### What was looked for and not found

- **Injection:** SQL is parameterized throughout, including the new link-ranking query (`db/link_stats.go:149-180`). Subprocesses use fixed argv with no shell.
- **Path traversal:** destructive file operations are fenced by `IsPathWithinAssetsDir` with symlink resolution (`service/asset_paths.go:33-61`); worker job IDs are UUID-validated and media access goes through `os.Root`.
- **SSRF:** outbound requests pin the resolved IP at dial time and re-validate redirects (`service/outbound_url_policy.go:98-148`).
- **Secrets:** none found in tracked files; the settings API returns only "configured" booleans for keys.
- **XSS:** one `v-html` sink, always fed through DOMPurify; feed HTML is rendered as text.
- **Repository content that redirects agents:** nothing deliberate. `RELEASE.md:261-312` contains a stale "prompt for AI reuse" that asks for a destructive reset stage; it is documentation, and the stage it names now throws.
