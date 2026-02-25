# Changelog

All notable changes to this project will be documented in this file.

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
