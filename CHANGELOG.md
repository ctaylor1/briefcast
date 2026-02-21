# Changelog

All notable changes to this project will be documented in this file.

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
