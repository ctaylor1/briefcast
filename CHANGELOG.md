# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

- Added locked `uv` CI checks for lint, format-check, type-check, tests, and dependency audit.
- Added automated secret scanning workflow.
- Added release/readiness documentation updates and safe `.env.example`.

## [1.0.1] - 2026-02-19

- Fixed Docker runtime packaging to include `src/briefcast_tools`, resolving Python helper import errors in container deployments.
- Updated Docker/compose configuration guidance for external Postgres connections.
- Updated release instructions for publishing `ghcr.io/ctaylor1/briefcast:1.0.1` and moving `latest` to the same build.

## [1.0.0] - 2026-02-17

- Initial public release baseline for Briefcast.
