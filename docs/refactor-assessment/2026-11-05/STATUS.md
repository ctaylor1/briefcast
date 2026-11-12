# Refactor backlog status

- **Checked:** 2026-11-09, on the release that implements the rest of the backlog. The base is `main` @ `8939fec`.
- **Scope:** progress on the items in [BACKLOG.md](BACKLOG.md). The assessment, backlog and prompts in this folder are a dated record and stay unedited; update this file as items change.
- Commit references are the titles in the release's commit series, one concern per commit.

## Summary

| Status | Items |
| --- | --- |
| Done | P1, P2 (including the WAL opt-in), P3, P4, P5, P6, P7, P8, P9, P10, P11, P12, P13, P14, P15, P16, P17, P19, P20 |
| Partly done | P18: episodes are deduplicated and unique; podcast URLs are not (see below) |

## Items

| ID | Title | Status | What changed |
| --- | --- | --- | --- |
| P1 | Secure compose defaults | Done (`425e610`) | Loopback by default, a startup warning without `PASSWORD`, `LOG_OUTPUT=stdout`. |
| P2 | Explicit SQLite concurrency settings | Done (`a383f57`, plus "opt-in SQLite WAL mode") | Busy timeout and pool size are explicit. `SQLITE_JOURNAL_MODE=wal` is opt-in, with documented disk and backup caveats. |
| P3 | Column-scoped episode updates | Done (`af4e875`, plus "write transcript and summary state by column") | Every episode write names its columns. `db.UpdatePodcastItem` is removed; tests use `savePodcastItemFixture`. A transcription or summary no longer undoes a change made while it ran, and an episode deleted mid-job stays deleted. |
| P4 | Panic containment | Done (`d89e565`, `c404231`) | The refresh lock heartbeat now also runs through `SafeGo`. |
| P5 | Podcast delete safety | Done (`477e895`, `0f438cb`, `8939fec`, plus "delete a podcast's rows in one transaction") | All rows go in one transaction before any file; file removal is best effort and logged. |
| P6 | HTTP server lifecycle | Done ("add server timeouts, graceful shutdown and a request body limit", "cap WebSocket message size") | Header and idle timeouts, graceful shutdown on SIGTERM, Recovery outside every middleware but the logger, a 1 MiB body cap (OPML keeps its own), a 15 s Podcast Index timeout and a 64 KiB WebSocket message cap. `TRUSTED_PROXIES` was not added; nothing needs it yet. |
| P7 | Missing-file check | Done | The pass skips when the data directory is unavailable. Only "does not exist" marks a file missing. A breaker stops a pass that would reset more than half of a library of 20 or more (`MISSING_FILES_MAX_PERCENT`). |
| P8 | State-changing GET aliases | Done (Go: "remove the state-changing GET aliases"; frontend: UI patch 05b) | The nine aliases are gone. The SPA encodes every ID in a path and the player accepts only episode IDs. |
| P9 | Local transcription keeps audio | Done | Error text never deletes audio; only a stat that proves the file is gone rearms downloading. |
| P10 | Delivery baseline | Done ("add vet, format, race, govulncheck and lock audits; pin actions by SHA") | CI runs `go vet`, a `gofmt` check, the race detector on every package, `govulncheck` and `pip-audit` on the three shipped lock files: blocking for the app image, report only for the two worker images (see "Worker image advisories"). Actions are pinned by SHA and `*.go` is LF. |
| P11 | Backup and migration safety | Done | `VACUUM INTO` snapshots of the configured SQLite path, with checked closes and no partial archives. Postgres is skipped with one log line. A failed migration stops startup. `RELEASE.ps1 deploy -Target app` needs a restore point or `-NoRestorePoint`. |
| P12 | Download integrity | Done | Colliding titles get an ID suffix. A resume continues only this episode's partial file (sidecar) and sends `If-Range`. One download per episode at a time. |
| P13 | Worker reliability | Done (five worker commits) | Resumed jobs seed totals. A storage fault exits non-zero. A manual retry resets a changed envelope. Abandoned jobs are cancelled. Finished prunes are skipped. See "Open follow-ups" for the part of F27 not covered. |
| P14 | Lightweight queries | Done (`428c15a`, plus "read only the columns hot paths use") | Media and image routes, search, retention and file sizes read only their columns. Settings are read on demand, not per request. File sizes are capped at 200 a run. |
| P15 | Outbound and helper hygiene | Done | Helpers get an allow-listed environment. The feed is passed to feedparser as content. Images need a 2xx response, are capped at 20 MB and are written atomically. Redirects stop at 10. Transports are shared per purpose. |
| P16 | Handler error responses | Done | Every branch answers once: 404 for missing records, 500 for other failures. `PATCH /podcastitems/:id` can clear values. |
| P17 | LLM out of feed refresh | Done | Refresh renews its lease, and a late renewal cannot revive a released lock. Feed-transcript summaries run in the summary backfill after the refresh. "Download all" answers 202 and queues in the background. |
| P18 | `AddPodcast` errors and uniqueness | Partly | Lookup failures are 500s, not "already exists". Adds of one URL are serialized in-process. Episodes are deduplicated (keeping the most complete row), unique on `(podcast_id, guid)`, and inserted under a per-podcast lock. **Not done:** a unique index on `podcasts.url`. If duplicates exist, the migration fails and, with P11, startup stops; removing duplicate podcasts would delete their episodes. That needs a maintainer decision. |
| P19 | Frontend save guards, latest-wins, harness | Done (UI patches 01 and 05c) | `@vue/test-utils` and jsdom are added. Forms render only after a successful load. Fetches are latest-wins. The sanitizer test uses the real function. |
| P20 | Legacy UI and scripts | Done ("remove the unrouted legacy UI", "remove the destructive legacy release scripts") | `client/`, the legacy page handlers and the `webassets/` libraries are gone; `blank.png` stays. `RELEASE_BUILD/RUN/RESET.ps1` are deleted. `/ws` and the hub stay (separate decision). |

## Findings after the assessment

| Finding | Status |
| --- | --- |
| The startup stats warmup held SQLite past the busy timeout, so clearing stale job locks failed. | Fixed: the warmup starts after the unlock. |
| The link backfill's back-to-back writes starved the per-minute transcription job of the lock. | Fixed: the backfill pauses 250 ms every 20 episodes. Not yet measured on a production-sized copy. |
| A summary of an episode deleted after it was loaded wrote an orphaned Markdown revision (found by the Postgres run). | Fixed. |
| "Download all" re-read the feed inside the request and could hang. | Fixed (see P17). |
| `pip-audit` failed the Python CI job on `main`: ten dev-tool packages in `uv.lock` had known advisories. | Fixed: all ten updated; the dev audit is clean ("update Python dependencies with known advisories"). |
| The worker images' `urllib3` 2.7.0 had three advisories. | Fixed: 2.8.0. |
| The worker locks have 119 more advisories in 12 baseline packages. | **Open** (see below). CI reports them without failing. |
| `pip-audit` cannot audit the worker's `torch`, `torchaudio`, `torchvision` and `torchcodec`: their `+cpu` and `+cu128` builds are not on PyPI. | **Open** (see below). |
| Production health check, 2026-11-12: the work-queue snapshot (`GET /settings/repair-work`) took 51.5 s and failed a stats write with `database is locked`. | Fixed: 0.12 s on a production copy ("stop the work-queue snapshot reading every transcript"). |
| `govulncheck` failed CI on `main`: 9 advisories in 5 modules, and about 33 in Go 1.26.0, which the images built with. | Fixed: modules updated and Go 1.26.11 ("patch Go vulnerabilities"). |
| Scheduled backups ran after 48 h of uptime, so restarts postponed them (none from 11-09 to 11-12). | Fixed: hourly check against the newest archive's time ("back up whenever the newest archive is 48 hours old"). |
| Every `transcription_completed` log said `resumed_from_checkpoint: true`. | Fixed: it reports the worker's record. |
| Scheduled refresh, retention and transcription runs were logged twice, under two job IDs. | Fixed: `service.RunJob` logs each run once. |
| A Windows Update restart left production down until the next sign-in (Docker Desktop starts at sign-in). | Operator setting: Windows auto sign-in (production `prod\README.txt`). Not a code change. |

## Open follow-ups

- **`podcasts.url` uniqueness** (P18): decide how to handle existing duplicates before adding the index.
- **F27, second part:** the app checks its execution timeout only after the worker accepts a job, and the two sides read different settings.
- **Export file names** include an episode number (rank by publish date) that changes when older or undated episodes are added. The transcript and summary exports of one episode can therefore get different names.
- **Worker image advisories:** on 2026-11-10, `pip-audit` reported 119 advisories in 12 packages of both worker locks: `aiohttp`, `certifi`, `filelock`, `fsspec`, `lightning`, `multidict`, `nltk`, `opentelemetry-api`, `pillow`, `pytorch-lightning`, `setuptools` and `transformers`. Three `transformers` advisories have no fixed release. These are the preserved WhisperX baseline; several fixes are major upgrades (`pillow` 13, `transformers` 5) that change the transcription engine and need a GPU acceptance run. Until then, CI audits the worker locks report only (maintainer decision, 2026-11-10). The step "Audit the worker image lock files (report only)" turns advisories into a warning and a job-summary table, and fails only when the audit cannot run. The app lock and the dev environment audits still block. Make the worker step blocking again once the upgrade lands. The job summary also lists the packages `pip-audit` skips (the PyTorch packages); check those against the PyTorch security advisories by hand.
- **Worker `VERSION`:** this release changes the worker (P13) and its `urllib3` pin, but `docker/whisperx/VERSION` is still `1.0.0`. Bump it before the next `worker-release.yml` publish so the published tag is not reused for different contents.
- **Decisions left open in the backlog:** whether `/ws` and the hub are removed; whether a tripped missing-file breaker should show in the UI.
