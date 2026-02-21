# Briefcast

Podcast downloader and library manager with a modern web UI.

- **Backend:** Go (`gin` + `gorm`) REST API + background workers
- **Frontend:** Vue 3 + Vite + TypeScript + Tailwind (served at `/app`)
- **Database:** SQLite (default) or Postgres via `DATABASE_URL`
- **Automation:** scheduled refresh, downloads, image sync, backups, and maintenance
- **Optional:** WhisperX transcription pipeline (external deps)

---

## Table of contents

- [Briefcast](#briefcast)
  - [Table of contents](#table-of-contents)
  - [Features](#features)
  - [Repo layout](#repo-layout)
  - [Quick start (local)](#quick-start-local)
    - [Prerequisites](#prerequisites)
    - [1) Install dependencies](#1-install-dependencies)
    - [2) Configure environment](#2-configure-environment)
    - [3) Build the frontend (recommended)](#3-build-the-frontend-recommended)
    - [4) Run the backend](#4-run-the-backend)
    - [Open the UI](#open-the-ui)
  - [Docker](#docker)
    - [docker compose (recommended)](#docker-compose-recommended)
    - [Run the published image](#run-the-published-image)
    - [Use external Postgres](#use-external-postgres)
    - [Storage (containers)](#storage-containers)
  - [Configuration](#configuration)
    - [Core runtime](#core-runtime)
    - [Database](#database)
    - [Networking \& concurrency](#networking--concurrency)
    - [Logging](#logging)
    - [Search providers](#search-providers)
    - [Python helpers](#python-helpers)
    - [WhisperX transcription (optional)](#whisperx-transcription-optional)
  - [Database URL examples](#database-url-examples)
  - [Scheduled jobs](#scheduled-jobs)
  - [Frontend development](#frontend-development)
  - [Testing](#testing)
  - [Linting / formatting](#linting--formatting)
  - [Regression testing](#regression-testing)
  - [Python tooling](#python-tooling)
  - [Release basics](#release-basics)
  - [One-command release](#one-command-release)
  - [Reset and share on GitHub](#reset-and-share-on-github)
  - [Migration note (rename to Briefcast)](#migration-note-rename-to-briefcast)

---

## Features

- Subscribe to podcast feeds and keep episodes up-to-date
- Download episode media and manage a local library
- Sync episode/podcast artwork and track file sizes
- Built-in backups and periodic maintenance jobs
- Optional WhisperX transcription workflow

---

## Repo layout

- `main.go` / Go packages: backend API + scheduler/workers
- `frontend/`: Vue 3 app (build output served by backend at `/app`)
- `scripts/`: Python helper scripts used by the backend
- `.github/`: CI workflows (tests, linting, secret scan, etc.)

---

## Quick start (local)

### Prerequisites

- Go **1.26+**
- Node **24+**
- Python **3.10+** with:

  ```bash
  pip install feedparser mutagen
  ```

- `uv` (Python tooling): https://docs.astral.sh/uv/

> WhisperX transcription is optional and has additional dependencies (see below).

### 1) Install dependencies

```bash
go mod download
npm --prefix frontend install
```

If you plan to run Python checks locally:

```bash
uv sync --group dev
```

### 2) Configure environment

For local source runs (`go run`), create a `.env` (or export vars in your shell):

```bash
CONFIG=.
DATA=./assets
CHECK_FREQUENCY=10
PASSWORD=
```

Recommended (explicit DB path):

```bash
DATABASE_URL=sqlite:///config/briefcast.db
```

For Docker deployments, use `.env.example` as your starting point instead.

### 3) Build the frontend (recommended)

The modern UI is served from `frontend/dist`. Build it once:

```bash
npm --prefix frontend run build
```

### 4) Run the backend

```bash
go run ./main.go
```

### Open the UI

- Modern UI: `http://localhost:8080/app`
- `/` redirects to `/app`

**Basic auth:** if `PASSWORD` is set, HTTP basic auth is enabled with:

- username: `briefcast`
- password: value of `PASSWORD`

---

## Docker

### docker compose (recommended)

```bash
docker compose up -d
```

- Copy `.env.example` to `.env`, then edit only the values you care about (password, logs, check frequency, paths, image tag).
- Compose now explicitly loads `.env` into the container runtime environment.
- Default logs go to both container stdout and `/logs/briefcast-{startup_ts}.log`.
- Advanced WhisperX tuning can live in `.env.whisperx` (template: `whisperx.env.example`).

- Default service uses **SQLite**
- A **Postgres** service is available under the `postgres` profile

#### Synology Container Manager: verify `.env` and logs

1. Put `docker-compose.yml` and `.env` in the same shared folder on the NAS.
2. Create/update the Synology project from that folder, then redeploy.
3. From NAS SSH, verify values and mounts:

```bash
docker inspect briefcast --format '{{range .Config.Env}}{{println .}}{{end}}' | grep '^LOG_OUTPUT='
docker inspect briefcast --format '{{range .Config.Env}}{{println .}}{{end}}' | grep '^LOG_LEVEL='
docker inspect briefcast --format '{{range .Mounts}}{{println .Source "->" .Destination}}{{end}}' | grep ' -> /logs'
docker exec briefcast sh -lc 'ls -lah /logs'
```

If the `LOG_OUTPUT` value and `/logs` mount are correct, log files should appear under your host `HOST_LOGS_DIR`.

### Run the published image

```bash
docker pull ghcr.io/ctaylor1/briefcast:1.0.4
docker pull ghcr.io/ctaylor1/briefcast:latest

docker run -d \
  --name briefcast \
  --restart unless-stopped \
  -p 8080:8080 \
  -v briefcast_config:/config \
  -v briefcast_data:/assets \
  -e DATABASE_URL=sqlite:///config/briefcast.db \
  ghcr.io/ctaylor1/briefcast:1.0.4
```

`latest` should point to the current release tag (`1.0.4`).

### Use external Postgres

If you already run Postgres separately, point Briefcast at it:

```bash
docker run -d \
  --name briefcast \
  --restart unless-stopped \
  -p 8080:8080 \
  -v briefcast_config:/config \
  -v briefcast_data:/assets \
  -e DB_DRIVER=postgres \
  -e DATABASE_URL=postgres://operator:${BRIEFCAST_DB_PASSWORD}@192.168.1.2:5432/briefcast?sslmode=disable \
  ghcr.io/ctaylor1/briefcast:1.0.4
```

### Storage (containers)

| Path       | Required | Purpose                                  | Sizing guidance |
|-----------|----------|-------------------------------------------|-----------------|
| `/config` | ✅       | SQLite DB, app config, backups            | start 5–20 GB   |
| `/assets` | ✅       | downloaded media + episode/podcast images | often 100+ GB   |
| `/logs`   | ✅       | rotating application log files            | start 1–5 GB    |

Recommendations:

- Back up at least `/config`.
- Back up `/assets` if you cannot easily re-download the media.
- Prefer **bind mounts** in production if you want easy host access:

```bash
docker run -d \
  --name briefcast \
  --restart unless-stopped \
  -p 8080:8080 \
  -v /srv/briefcast/config:/config \
  -v /srv/briefcast/assets:/assets \
  -e DATABASE_URL=sqlite:///config/briefcast.db \
  ghcr.io/ctaylor1/briefcast:1.0.4
```

---

## Configuration

For Docker Compose end users, most setups only need:
`PASSWORD`, `CHECK_FREQUENCY`, `LOG_OUTPUT`, optional `DATABASE_URL`, plus host path/port values in `.env` (`HOST_CONFIG_DIR`, `HOST_ASSETS_DIR`, `HOST_LOGS_DIR`, `HOST_PORT`).

### Core runtime

- `CONFIG`: config directory
  - default behavior may fall back to `.`, but set explicitly in real deployments
- `DATA`: media/assets directory
- `CHECK_FREQUENCY`: base interval in minutes (defaults to `30` if invalid)
- `PASSWORD`: enables HTTP basic auth (username `briefcast`)
- `GIN_MODE`: set `release` for production
- `PUID`, `PGID`: optional ownership mapping for created files/folders

### Database

- `DATABASE_URL`: connection string/path (**recommended**)
- `DB_DRIVER`: optional override (`sqlite` or `postgres`)
- `DATABASE_DRIVER`: alias for `DB_DRIVER`
- `DB_MAX_IDLE_CONNS`: default `10`
- `DB_MAX_OPEN_CONNS`: default `25`
- `DB_CONN_MAX_LIFETIME_MINUTES`: default `0` (disabled)

### Networking & concurrency

- `PER_HOST_MAX_CONCURRENCY`: per-host in-flight outbound request cap (default `2`)
- `PER_HOST_RATE_LIMIT_RPS`: per-host pacing cap (default `2.0`; `0` disables pacing)
- `HTTP_TIMEOUT_SECONDS`: outbound HTTP timeout for feeds/downloads/images (default `900`; `0` disables)

### Logging

- `LOG_LEVEL`: `debug|info|warn|error` (default `info`)
- `LOG_FORMAT`: `json|text` (default `json` for Go services; Python helper defaults to `text`)
- `LOG_OUTPUT`: comma-separated outputs: `stdout`, `stderr`, or a file path  
  e.g. `LOG_OUTPUT=stdout,file:/logs/briefcast-{startup_ts}.log`
- `LOG_FILE_MAX_SIZE_MB`: default `50`
- `LOG_FILE_MAX_BACKUPS`: default `7`
- `LOG_FILE_MAX_AGE_DAYS`: default `14`
- `LOG_FILE_COMPRESS`: default `true`
- `LOG_RUN_TIMESTAMP`: optional shared run ID for timestamp token expansion (auto-generated if empty)

Notes:

- Incoming `X-Request-ID` is propagated; otherwise generated.
- Background jobs include `job_name` and `job_id` in logs.
- File paths can include `{startup_ts}`, `{timestamp}`, or `{run_ts}` tokens.
- Python helpers also honor `LOG_LEVEL` / `LOG_FORMAT` / `LOG_OUTPUT`, and redact common secret fields.

### Search providers

- `PODCASTINDEX_KEY`: PodcastIndex API key (optional)
- `PODCASTINDEX_SECRET`: PodcastIndex API secret (optional)

### Python helpers

Feed parsing:

- `FEEDPARSER_PYTHON`: interpreter path (default `python3`/`python`)
- `FEEDPARSER_SCRIPT`: default `scripts/feedparser_parse.py`
- `FEEDPARSER_TIMEOUT_SECONDS`: default `30` (`0` disables)

ID3 extraction:

- `MUTAGEN_PYTHON`: interpreter path (falls back to `FEEDPARSER_PYTHON`)
- `MUTAGEN_SCRIPT`: default `scripts/mutagen_id3_extract.py`
- `MUTAGEN_TIMEOUT_SECONDS`: default `20` (`0` disables)

### WhisperX transcription (optional)

**Not bundled** in the default Docker image. You must install WhisperX + dependencies yourself.

To build an image with WhisperX preinstalled (currently `linux/amd64` only):

```bash
docker buildx build --platform linux/amd64 --build-arg INSTALL_WHISPERX=true -t ghcr.io/ctaylor1/briefcast:with-whisperx --push .
```

- `WHISPERX_ENABLED`: `true|false` (default `false`)
- `WHISPERX_PYTHON`: interpreter path (falls back to `FEEDPARSER_PYTHON`)
- `WHISPERX_SCRIPT`: default `scripts/whisperx_transcribe.py`
- `WHISPERX_TIMEOUT_SECONDS`: default `7200` (`0` disables)
- `WHISPERX_MODEL`: default `medium.en`
- `WHISPERX_LANGUAGE`: default `en`
- `WHISPERX_DEVICE`: `auto|cuda|cpu` (default `auto`)
- `WHISPERX_COMPUTE_TYPE`: `auto|float16|int8|float32` (default `auto`)
- `WHISPERX_BATCH_SIZE`: auto (`16` CUDA, `4` CPU)
- `WHISPERX_BEAM_SIZE`: default `5`
- `WHISPERX_PATIENCE`: default `1`
- `WHISPERX_CONDITION_ON_PREVIOUS_TEXT`: default `true`
- `WHISPERX_INITIAL_PROMPT`: optional string
- `WHISPERX_VAD_CHUNK_SIZE`: default `45`
- `WHISPERX_VAD_ONSET`: default `0.50`
- `WHISPERX_VAD_OFFSET`: default `0.50`
- `WHISPERX_VAD_METHOD`: default `pyannote`
- `WHISPERX_ALIGN`: default `true`
- `WHISPERX_DIARIZATION`: default `true`
- `WHISPERX_DIARIZATION_MODEL`: default `pyannote/speaker-diarization-3.1`
- `WHISPERX_MIN_SPEAKERS`: default `2`
- `WHISPERX_MAX_SPEAKERS`: default `2`
- `WHISPERX_HF_TOKEN`: Hugging Face token (required for pyannote diarization)
- `WHISPERX_MAX_CONCURRENCY`: default `1`
- `WHISPERX_MAX_ITEMS`: default `0` (no limit)
- `WHISPERX_RETRY_FAILED`: default `false`
- `WHISPERX_CHECK_FREQUENCY`: defaults to `CHECK_FREQUENCY`

Recommended config split:

- Keep `WHISPERX_ENABLED`, `WHISPERX_DIARIZATION`, and `WHISPERX_CHECK_FREQUENCY` in `.env`
- Put advanced WhisperX overrides in `.env.whisperx` (start from `whisperx.env.example`)
- Compose loads that optional file via `WHISPERX_ENV_FILE` (default `.env.whisperx`)

---

## Database URL examples

SQLite:

```bash
DATABASE_URL=sqlite:///config/briefcast.db
```

Postgres:

```bash
DATABASE_URL=postgres://briefcast:briefcast@postgres:5432/briefcast?sslmode=disable
DB_DRIVER=postgres
```

External Postgres host:

```bash
DATABASE_URL=postgres://operator:${BRIEFCAST_DB_PASSWORD}@192.168.1.2:5432/briefcast?sslmode=disable
DB_DRIVER=postgres
```

---

## Scheduled jobs

Based on `CHECK_FREQUENCY` (N minutes):

- `RefreshEpisodes`: every `N`
- `CheckMissingFiles`: every `N`
- `DownloadMissingImages`: every `N`
- `UnlockMissedJobs`: every `2N`
- `UpdateAllFileSizes`: every `3N`
- `CreateBackup`: every `48h`

---

## Frontend development

Run Vite dev server:

```bash
npm --prefix frontend run dev
```

Build for backend serving:

```bash
npm --prefix frontend run build
```

---

## Testing

```bash
go test ./...
npm --prefix frontend run test
uv run pytest
```

---

## Linting / formatting

```bash
uv run ruff check .
uv run ruff format --check .
```

Fix formatting:

```bash
uv run ruff format .
```

Type check:

```bash
uv run mypy src
```

---

## Regression testing

Go tests:

```bash
go test ./...
```

Frontend regression (build + typecheck):

```bash
npm --prefix frontend run test
```

Integration flow (feed parse → download → transcript stub):

```bash
BRIEFCAST_INTEGRATION=1 go test ./service -run TestIntegrationFeedDownloadWhisperX
```

Real WhisperX regression:

```bash
BRIEFCAST_WHISPERX_REAL=1 WHISPERX_TEST_AUDIO=/path/to/audio.mp3 go test ./service -run TestWhisperXRealTranscription
```

Windows helper:

```powershell
./scripts/regression.ps1
```

---

## Python tooling

This repo uses `uv`, a project-local `.venv`, and a committed `uv.lock`.

Setup:

```bash
uv sync --group dev
```

CI-parity (enforce lockfile):

```bash
uv sync --locked --group dev
```

Quality checks:

```bash
uv run ruff check .
uv run ruff format --check .
uv run mypy src
uv run pytest
uv run pip-audit
```

Secret hygiene:

- Keep secrets in local `.env` or CI secret stores.
- CI secret scanning runs in `.github/workflows/secret-scan.yml`.

---

## Release basics

- Package version is defined in `pyproject.toml`.
- Keep release notes in `CHANGELOG.md` (update `Unreleased` before tagging).
- Recommended tag format: `vX.Y.Z`.
- For `v1.0.4`, publish container tags `ghcr.io/ctaylor1/briefcast:1.0.4` and `ghcr.io/ctaylor1/briefcast:latest` from the same image digest.

## One-command release

Use GitHub Actions workflow `.github/workflows/release.yml`.

What it does:

- Resolves the next version (`version` or `bump`)
- Updates `pyproject.toml` version when static
- Runs tests + package build + `twine check --strict`
- Builds/publishes container image to GHCR with tags `<version>` and `latest`
- Creates annotated git tag `vX.Y.Z` and GitHub Release with `dist/*` assets
- Optionally publishes to PyPI via Trusted Publishing

Required setup:

- Run the workflow from the default branch only.
- Ensure Actions permissions allow package write (workflow sets this with `packages: write`).
- If using PyPI publish, configure PyPI Trusted Publishing for this repository/workflow.

Run from CLI with `gh` (single command each):

Patch bump:

```bash
gh workflow run release.yml --ref <default-branch> -f bump=patch
```

Minor bump:

```bash
gh workflow run release.yml --ref <default-branch> -f bump=minor
```

Major bump:

```bash
gh workflow run release.yml --ref <default-branch> -f bump=major
```

Explicit version:

```bash
gh workflow run release.yml --ref <default-branch> -f version=1.2.3
```

Explicit version + publish to PyPI:

```bash
gh workflow run release.yml --ref <default-branch> -f version=1.2.3 -f publish_pypi=true
```

Dry run (validate everything without push/tag/release/publish):

```bash
gh workflow run release.yml --ref <default-branch> -f bump=patch -f dry_run=true
```

You can also run the same workflow from the GitHub Actions UI (`Actions` -> `release` -> `Run workflow`).

Legacy/manual image publish command:

```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  -t ghcr.io/ctaylor1/briefcast:1.0.4 \
  -t ghcr.io/ctaylor1/briefcast:latest \
  --push .
```

---

## Reset and share on GitHub

Use this to create a clean, reproducible state before publishing.

1) Verify branch and changes:

```bash
git branch --show-current
git status --short
```

2) Remove ignored generated files:

```bash
git clean -fdX
```

3) Recreate env + run checks:

```bash
uv sync --locked --group dev
uv run ruff check .
uv run ruff format --check .
uv run mypy src
uv run pytest
uv run pip-audit
go test ./...
npm --prefix frontend run test
```

4) Commit:

```bash
git add .
git commit -m "release: ship-ready"
```

5) Tag (recommended):

```bash
git tag -a v1.0.4 -m "Briefcast v1.0.4"
```

6) Push:

```bash
git remote add origin https://github.com/<your-org-or-user>/briefcast.git
git push -u origin <branch-name>
git push origin v1.0.4
```

PowerShell variant for step 2:

```powershell
git clean -fdX
```

**Destructive reset (discard all uncommitted edits):** only after backing up work:

```bash
git restore --staged .
git restore .
git clean -fd
```

---

## Migration note (rename to Briefcast)

Default SQLite filename is now `briefcast.db`.

If you are migrating from an older deployment that used a different SQLite filename, set `DATABASE_URL` explicitly to your existing DB file path.

