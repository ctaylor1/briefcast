# Changelog

All notable changes to this project will be documented in this file.

## [1.9.1] - 2026-06-03

### Added
- Added a unified favorites flow for episodes and summaries, including favorites-only filtering and favorite state in detail responses.
- Added Obsidian folder settings and summary export links for the summaries reader and episode drawer.

### Changed
- Kept summarization prompt fields populated from the effective configured prompts in settings responses and UI forms.
- Prioritized in-progress WhisperX jobs before pending jobs when selecting transcription work.

### Fixed
- Kept bookmark and summary favorite state synchronized when toggling either favorite path.
- Sanitized Obsidian folder paths before storing settings.

## [1.9.0] - 2026-05-27

### Added
- Added Go and frontend CI gates alongside the existing Python quality workflow.
- Added regression coverage for OPML upload limits, outbound URL policy enforcement, media redirect safety, static route exposure, scoped asset deletion, and write-only Briefpoint API key handling.
- Added durable agent instructions in `AGENTS.md` and `CLAUDE.md`.

### Changed
- Hardened outbound feed/media/search/Briefpoint/LLM HTTP calls with URL validation, redirect validation, transport-level private-address checks, request timeouts, host limiting, and bounded in-memory response reads.
- Made OPML import refresh behavior testable while preserving the asynchronous production refresh path.
- Updated Briefpoint settings so API keys are write-only in API responses and frontend state.
- Scoped podcast and episode media serving/deletion to the configured assets directory.
- Updated CI documentation and release/version references for `1.9.0`.

### Fixed
- Fixed pause/unpause behavior so missing podcast IDs return an error while existing podcasts remain idempotent for repeated pause or unpause calls.
- Fixed OPML upload handling to reject oversized file parts and oversized multipart request bodies.
- Fixed media fallback handling to reject unsafe non-HTTP(S) redirect targets.

### Removed
- Removed public static serving for backups and the broad `/assets` storage directory.

### Security
- Prevented stored Briefpoint API keys from being returned to clients.
- Reduced SSRF risk for outbound HTTP integrations and redirect chains.
- Prevented arbitrary local path serving and destructive deletion outside the configured assets directory.

## [1.2.2] - 2026-02-28

### Added
- Added a microphone-only browser tab favicon (`frontend/public/favicon.svg`) and wired it in `frontend/index.html`.

### Changed
- Updated frontend dev dependency `@types/node` to `24.11.0` (non-breaking patch update).
- Updated README release/version references and examples to `1.2.2`.

### Fixed
- Replaced the default Vite tab icon with project-branded favicon metadata.

### Deprecated
- None.

### Removed
- None.

### Security
- Re-ran dependency and static security audits (`npm audit`, `pip-audit`, `govulncheck`, `gosec`) with no HIGH/CRITICAL findings.

## [1.2.1] - 2026-02-28

### Added
- Added an About screen in the frontend with repo link and running-version display.
- Added `/version` API response wiring to expose runtime version and repository URL.
- Added version-resolution tests for runtime/env fallback behavior in `main_test.go`.

### Changed
- Updated Docker and `build_tar.ps1` flows to pass `APP_VERSION` into build/runtime metadata.
- Updated frontend dependency `axios` to `1.13.6` (patch, non-breaking).
- Updated README release references and examples to `1.2.1`.

### Fixed
- Fixed release artifact metadata so runtime version can be surfaced in the UI.

### Deprecated
- None.

### Removed
- None.

### Security
- Re-ran dependency and static security audits (`npm audit`, `pip-audit`, `govulncheck`, `gosec`) with no HIGH/CRITICAL findings.

## [1.2.0] - 2026-02-27

### Added
- Added an APG-compliant command palette dialog/combobox with focus trapping, keyboard navigation, and focus-return behavior.
- Added stronger form semantics in shared `UiInput`/`UiSelect` controls (`label` wiring, `aria-invalid`, and deterministic `aria-describedby` IDs).
- Added ARIA progress semantics for download progress indicators and modal drawer overlay/outside-click behavior on mobile.
- Added destructive-action protections for download cancellation (bulk confirm and per-item undo affordance).

### Changed
- Standardized player launch behavior to a predictable in-app flow with explicit pop-out fallback handling.
- Updated success messaging behavior to auto-dismiss stale success states while keeping errors persistent.
- Increased checkbox/toggle hit areas for touch ergonomics and updated text/metadata contrast tokens to meet AA targets.
- Honored `prefers-reduced-motion` by disabling shimmer/transition animations when reduced motion is enabled.

### Fixed
- Corrected `UiAlert` live-region roles by tone and marked announcements as atomic for better screen reader output.
- Removed duplicate native `<audio controls>` from the player to avoid redundant tab stops and duplicate SR controls.
- Fixed remaining command-palette focus restore edge cases after close transitions.

### Deprecated
- None.

### Removed
- None.

### Security
- Re-ran dependency and static security audits (`npm audit`, `pip-audit`, `govulncheck`, `gosec`) with no HIGH/CRITICAL findings.

## [1.1.0] - 2026-02-27

### Added
- Added `canonical_transcript`, `canonical_transcript_version`, and `canonical_updated_at` database columns for `podcast_items`.
- Added canonical transcript formatting/backfill services and a CLI backfill command at `cmd/backfill_canonical_transcripts`.
- Added unit/integration coverage for canonical transcript formatting, migration presence, and batch backfill behavior.

### Changed
- Updated WhisperX script output to include `segments_pre_align` and preserve a word-array-free segment snapshot for canonical text generation.
- Updated ingestion/transcription flows to persist canonical transcript text and version metadata alongside transcript JSON.
- Updated README release/version references for `v1.1.0`.

### Fixed
- Fixed UI copy to read `Briefcast transcription in progress.`.

### Deprecated
- None.

### Removed
- None.

### Security
- Re-ran dependency and static security audits (`npm audit`, `pip-audit`, `govulncheck`, `gosec`) with no HIGH/CRITICAL findings.

## [1.0.11] - 2026-02-27

### Added
- None.

### Changed
- Updated frontend dev dependency `@types/node` to `24.10.15` (non-breaking patch update).
- Documented that legacy `golint` exported-comment/naming findings remain deferred due to broad API-churn risk.

### Fixed
- Upgraded `golang.org/x/net` to `v0.51.0` to remediate `GO-2026-4559`.

### Deprecated
- None.

### Removed
- None.

### Security
- Re-ran dependency and static security audits (`npm audit`, `pip-audit`, `govulncheck`, `gosec`) with no HIGH/CRITICAL findings.

## [1.0.10] - 2026-02-26

### Added
- Added targeted coverage for `ParseEntryDate` and WhisperX queue snapshot aggregation paths.

### Changed
- Updated frontend lockfile patches to `autoprefixer@10.4.27` and `@types/node@24.10.14`.

### Fixed
- Fixed WhisperX temp progress-file lifecycle to avoid eager deletion and satisfy high-severity path-traversal scanning.
- Fixed WhisperX queue snapshot aggregation to avoid GORM filter leakage between queries.
- Fixed WhisperX queue `MIN(transcript_next_attempt)` scanning on SQLite by parsing normalized timestamp text.

### Deprecated
- None.

### Removed
- None.

### Security
- Re-ran dependency and static security audits (`npm audit`, `pip-audit`, `govulncheck`, `gosec`) with no HIGH/CRITICAL findings.

## [1.0.9] - 2026-02-25

### Added
- Added resumable WhisperX progress/checkpoint persistence and expanded lock/transcription regression coverage.

### Changed
- Changed WhisperX chunk transcription to stream audio chunks via `ffprobe`/`ffmpeg` to reduce peak memory on long episodes.
- Changed lock acquisition/refresh flow to use atomic lock attempts and active lock heartbeat updates.

### Fixed
- Fixed stale lock handling to evaluate lock expiration at read time, reducing false `job_skipped_lock_exists` skips.
- Fixed transcription checkpoint write amplification by persisting segment payloads to a sidecar file instead of repeating full payloads in progress checkpoints.
- Fixed remaining production `DB.Debug()` query logging and hardened ordered ID query construction in `GetAllPodcastItemsByIds`.

### Deprecated
- None.

### Removed
- None.

### Security
- Re-ran dependency/security audits (`npm audit`, `pip-audit`, `govulncheck`) with no vulnerabilities found.

## [1.0.8] - 2026-02-24

### Added
- Added targeted unit tests for logging and database wrapper paths to raise release coverage and lock in behavior.

### Changed
- Updated Go module metadata and lockfile state (`go mod tidy`) to keep builds reproducible with current transitive dependencies.

### Fixed
- Fixed release build failures caused by stale module graph resolution for `google.golang.org/protobuf/proto`.

### Deprecated
- None.

### Removed
- None.

### Security
- Re-ran dependency/security audits (`npm audit`, `pip-audit`, `govulncheck`, `gosec`) with no HIGH/CRITICAL findings.

## [1.0.7] - 2026-02-23

### Added
- Added database migrations to deduplicate `job_locks` rows and enforce a unique index on `job_locks.name`.
- Added root-package tests for router wiring, cron scheduling helpers, and frontend app serving paths.

### Changed
- Increased default WhisperX process timeout to `21600` seconds to support long-form transcriptions.
- Refactored startup router/cron wiring into testable helpers while preserving runtime behavior.
- Updated frontend toolchain dependencies to latest non-breaking releases.

### Fixed
- Fixed lock contention by using atomic lock upsert, ID-safe unlocks, and stale-lock cleanup via row-id updates.
- Removed leftover debug/commented blocks from legacy HTML and Go service/controller files.

### Deprecated
- None.

### Removed
- None.

### Security
- Re-ran dependency audits across Go, npm, and Python tooling with no HIGH/CRITICAL vulnerabilities.

## [1.0.6] - 2026-02-23

### Added
- Added icon-based episode track controls (play, stop, played toggle, bookmark toggle, download states) with 44x44 hit targets, tooltips, ARIA labels, and keyboard-focus styling.
- Added cross-window player stop messaging so episode list stop controls can halt the active `/player` session.
- Added focused coverage expansion tests for controllers and service packages.

### Changed
- Changed played and bookmark toggles to optimistic UI updates with rollback on API failure.
- Changed transcript badges in episode lists/tables to clearly display queued, in-progress, ready, and failed states.
- Changed episode actions layout in table and list views to use a unified control component.

### Fixed
- Fixed feed refresh failures caused by helper stdout log noise corrupting feedparser JSON output handling.
- Fixed bookmark-state detection for zero-date values by centralizing bookmark-date parsing logic.
- Fixed WhisperX retry behavior to back off failed transcripts and requeue them for later processing without tight retry loops.

### Security
- Verified dependency audits for npm, Python, and Go toolchain with no HIGH/CRITICAL vulnerabilities.

## [1.0.5] - 2026-02-22

- Enforced WhisperX in all container builds by default (`INSTALL_WHISPERX=true`) and made non-WhisperX builds fail fast.
- Updated release automation to publish WhisperX-enabled images on `linux/amd64` and pass `INSTALL_WHISPERX=true` during image builds.
- Updated `build_tar.ps1` to always build WhisperX-enabled `linux/amd64` images before saving versioned tar artifacts.
- Added `scripts/reset_app_data.sh` to reset runtime/transactional data (DB records, assets, logs, backups) while preserving configuration files.
- Improved Synology deployment compatibility and docs: short `env_file` compose syntax, safer `LOG_OUTPUT` handling, and expanded NAS runbook guidance.
- Added targeted maintenance comments in download and transcription code paths to clarify lock behavior, transcript eligibility, and resume-download semantics.

## [1.0.4] - 2026-02-21

- Added deterministic download queue ordering in the modern UI so active downloads are shown first.
- Updated paused download presentation to hide progress bars and clearly mark paused rows.
- Updated Docker Compose to explicitly load `.env` at runtime and documented Synology verification steps for `LOG_OUTPUT` and `/logs` bind mounts.
- Improved `build_tar.ps1` packaging flow to build `briefcast:latest`, save versioned tar files under `builds/`, and optionally copy artifacts to a network path.

## [1.0.3] - 2026-02-21

- Fixed top-right global search behavior in the modern UI by wiring the command palette to local library search results (`/search/local`) in addition to route shortcuts.
- Added live search result states in the command palette (loading, errors, no-match) and selection routing into Episodes filters for podcast/episode/chapter/transcript matches.
- Updated Episodes view query handling so route query updates (`q`, `podcastIds`) immediately sync with filters and trigger fresh results.

## [1.0.1] - 2026-02-19

- Fixed Docker runtime packaging to include `src/briefcast_tools`, resolving Python helper import errors in container deployments.
- Updated Docker/compose configuration guidance for external Postgres connections.
- Updated release instructions for publishing `ghcr.io/ctaylor1/briefcast:1.0.1` and moving `latest` to the same build.

## [1.0.0] - 2026-02-17

- Initial public release baseline for Briefcast.
