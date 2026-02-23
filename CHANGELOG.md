# Changelog

All notable changes to this project will be documented in this file.

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
