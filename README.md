# Briefcast

Briefcast is a self-hosted podcast downloader and library manager with a Go API, scheduled workers, and a Vue web UI served from `/app`.

- Backend: Go, Gin, Gorm, SQLite by default, optional Postgres
- Frontend: Vue 3, Vite, TypeScript, Tailwind
- Automation: feed refresh, downloads, artwork sync, file checks, backups, retention cleanup, transcription
- Media layout: one mounted `/assets` tree for audio, images, transcripts, and summaries
- Transcription: independent CPU/CUDA WhisperX worker, enabled by configuration
- Summaries: OpenAI-compatible LLM summarization from episode transcripts

## Table Of Contents

- [Features](#features)
- [Repository Layout](#repository-layout)
- [Task Runner](#task-runner)
- [Quick Start](#quick-start)
- [Docker](#docker)
- [Storage Layout](#storage-layout)
- [Configuration](#configuration)
- [Back Catalog Downloads](#back-catalog-downloads)
- [Transcripts And Summaries](#transcripts-and-summaries)
- [Scheduled Jobs](#scheduled-jobs)
- [Frontend Development](#frontend-development)
- [Testing](#testing)
- [Release Basics](#release-basics)
- [Reset Runtime Data](#reset-runtime-data)
- [Migration Notes](#migration-notes)

## Features

- Subscribe to podcast RSS feeds and keep episodes current.
- Import and export OPML.
- Search external providers and local library records.
- Download, pause, resume, and cancel episode media through the queue.
- Store audio under `/assets/audio/<podcast>/`.
- Store artwork under `/assets/images/<podcast>/`.
- Generate and export transcripts under `/assets/transcripts/<podcast>/`.
- Generate and export summaries under `/assets/summaries/<podcast>/`.
- Backfill legacy transcript and summary exports through the `export-all` API.
- Configure initial back-catalog downloads by count, age in months, or all feed entries.
- Read embedded ID3 chapters and feed chapters.
- Tag podcasts and expose RSS feeds for all podcasts, individual podcasts, and tags.
- Mark episodes played/unplayed, bookmark episodes, and favorite summaries.
- Apply retention policies per podcast.
- Use WhisperX for local transcription with progress checkpoints and retry behavior.
- Use an OpenAI-compatible API for LLM summaries.
- Run scheduled backups and maintenance jobs.

## Repository Layout

- `main.go`: application entrypoint, route registration, and scheduler wiring
- `controllers/`: HTTP handlers and API response shaping
- `service/`: podcast, download, export, transcription, summary, file, and retention logic
- `db/`: database models, migrations, connection setup, and helpers
- `internal/`: shared internals such as logging and version helpers
- `frontend/`: Vue application; production build is served at `/app`
- `scripts/`: Python helper scripts for feed parsing, ID3 extraction, and WhisperX
- `.github/`: CI and release workflows
- `docker-compose.yml`: runtime stack for Docker Compose
- `justfile`: common local, Docker, test, and release commands

## Task Runner

Use `just` as the primary command surface.

Install `just`:

```bash
# Windows
winget install Casey.Just

# macOS
brew install just

# Any platform with Rust
cargo install just
```

Common commands:

```bash
just --list
just bootstrap
just up
just ps
just logs
just down
just test-full
just release-help
```

You can override the compose and environment files per command:

```bash
ENV_FILE=.env COMPOSE_FILE=docker-compose.yml just up
```

## Quick Start

### Prerequisites

- Go 1.26+
- Node 24+
- Python 3.14+ for repository tooling
- `uv` for Python tooling: https://docs.astral.sh/uv/
- Docker, if running the containerized app

The app image includes the server and feedparser/mutagen helpers. Optional transcription runs in a separately versioned worker image. PowerShell 7 and Docker Compose 2.24 or newer are required for the command wrappers.

### Install Dependencies

```bash
just bootstrap
```

For local source runs outside Docker, install the helper packages into the Python interpreter used by `FEEDPARSER_PYTHON` and `MUTAGEN_PYTHON`:

```bash
python -m pip install feedparser mutagen
```

### Configure Local Source Runs

For `go run`, create a `.env` file or export the same values in your shell:

```bash
CONFIG=./config
DATA=./assets
DATABASE_URL=sqlite:///config/briefcast.db
CHECK_FREQUENCY=15
LOG_OUTPUT=stdout,file:./logs/briefcast-{startup_ts}.log
PASSWORD=
```

Create the local folders if needed:

```bash
mkdir -p config assets logs
```

### Build The Frontend

The backend serves `frontend/dist` at `/app`.

```bash
npm --prefix frontend run build
```

### Run The Backend

```bash
go run ./main.go
```

Open:

- App UI: `http://localhost:8080/app`
- Root path `/` redirects to `/app`

If `PASSWORD` is set, basic auth is enabled with username `briefcast`.

## Docker

### Docker Compose

Start from `.env.example`:

```bash
cp .env.example .env
```

Edit `.env`, especially these values:

```bash
BRIEFCAST_IMAGE=ghcr.io/ctaylor1/briefcast:latest
HOST_CONFIG_DIR=./config
HOST_ASSETS_DIR=./assets
HOST_LOGS_DIR=./logs
HOST_PORT=8080
PASSWORD=
DATABASE_URL=sqlite:///config/briefcast.db
```

Start the app:

```bash
just up
```

Useful lifecycle commands:

```bash
just logs
just ps
just restart
just down
```

### Build Local Code Into Docker

Ordinary `just up` uses image references. `just rebuild` explicitly adds the source-build layers. Select the same runtime for lifecycle and release commands using `WHISPERX_RUNTIME=disabled|cpu|cuda` in the primary env file.

```bash
just docker-build image=briefcast:local
# Set BRIEFCAST_IMAGE=briefcast:local in the development env file.
just up
# Explicitly build selected services from source:
just rebuild service=briefcast
```

Use isolated data and container names for development. Source builds never deploy automatically.

### Pull Published Images

```bash
docker pull ghcr.io/ctaylor1/briefcast:1.9.4
docker pull ghcr.io/ctaylor1/briefcast:latest
```

Example `docker run` with SQLite:

```bash
docker run -d \
  --name briefcast \
  --restart unless-stopped \
  -p 8080:8080 \
  -v briefcast_config:/config \
  -v briefcast_assets:/assets \
  -v briefcast_logs:/logs \
  -e CONFIG=/config \
  -e DATA=/assets \
  -e DATABASE_URL=sqlite:///config/briefcast.db \
  ghcr.io/ctaylor1/briefcast:1.9.4
```

### External Postgres

```bash
docker run -d \
  --name briefcast \
  --restart unless-stopped \
  -p 8080:8080 \
  -v /srv/briefcast/config:/config \
  -v /srv/briefcast/assets:/assets \
  -v /srv/briefcast/logs:/logs \
  -e CONFIG=/config \
  -e DATA=/assets \
  -e DB_DRIVER=postgres \
  -e DATABASE_URL=postgres://operator:${BRIEFCAST_DB_PASSWORD}@192.168.1.2:5432/briefcast?sslmode=disable \
  ghcr.io/ctaylor1/briefcast:1.9.4
```

### Independent WhisperX Images

The default stack disables transcription. To use a worker, set `WHISPERX_RUNTIME=cpu` or `cuda`, `WHISPERX_ENABLED=true`, and an immutable `WHISPERX_IMAGE` reference. Keep `WHISPERX_DIARIZATION=false` and `WHISPERX_MAX_CONCURRENCY=1` to preserve the production baseline at cutover. Create the state/cache directories with ownership matching `WHISPERX_UID/GID`. Provide a private service token file (at least 32 characters) and an HF token file (may be empty). Copy the preserved model caches before using the default offline mode. See [packaging details](docker/whisperx/README.md).

```bash
# CPU and CUDA archives have an independent worker version.
just release-build -Target worker -Runtime cpu -Version 1.0.0
just release-build -Target worker -Runtime cuda -Version 1.0.0
# Deploy uses the checksum-verified archive manifest and replaces only whisperx.
just release-deploy -Target worker -Runtime cpu -Version 1.0.0
```

The new Compose files do not deploy the split to production automatically. Complete the recovery and acceptance gates in [the execution plan](docs/whisperx-separation/EXECUTION_PLAN.md) before cutover. The former monolith definitions are preserved as `docker-compose.legacy.yml` and `docker-compose.legacy-gpu.yml`; the implicit GPU override has been retired.

## Storage Layout

Briefcast now uses `/assets` for all generated library output. There is no separate `/exports` mount or export directory.

| Container Path | Required | Purpose |
| --- | --- | --- |
| `/config` | yes | SQLite database, app config, backups, caches |
| `/assets` | yes | audio, images, transcript exports, summary exports |
| `/logs` | yes | application logs |

Asset subfolders:

```text
/assets/
  audio/<podcast>/
  images/<podcast>/
  transcripts/<podcast>/
  summaries/<podcast>/
  markdown/transcripts/<podcast>/
  markdown/summaries/<podcast>/
```

Audio files should be under `audio/<podcast>/`. Podcast and episode artwork should be under `images/<podcast>/`. Transcript and summary text exports are written to their respective folders, with markdown copies under `markdown/`.

The `/assets` directory is storage, not a public static route. Use the application media endpoints for audio and artwork access; transcript and summary exports are served through their API views.

To move assets to another drive or host path:

1. Stop the container.
2. Move the existing asset folder contents to the new host path, preserving the subfolder layout.
3. Update `HOST_ASSETS_DIR` in `.env`.
4. Start the container again.

No database update is normally required when only the host mount path changes and the files keep the same relative layout inside `/assets`.

## Configuration

For Docker Compose end users, most deployments only need `.env` values for:

- `BRIEFCAST_IMAGE`
- `HOST_CONFIG_DIR`
- `HOST_ASSETS_DIR`
- `HOST_LOGS_DIR`
- `HOST_PORT`
- `PASSWORD`
- `CHECK_FREQUENCY`
- `DATABASE_URL`
- `LOG_OUTPUT`
- WhisperX and LLM settings, if those features are enabled

### Core Runtime

- `CONFIG`: config directory inside the container or local runtime
- `DATA`: assets directory inside the container or local runtime
- `CHECK_FREQUENCY`: base scheduler interval in minutes; invalid values fall back to `30`
- `PASSWORD`: optional basic auth password; username is always `briefcast`
- `GIN_MODE`: set to `release` for production
- `PUID`, `PGID`: optional ownership mapping for created files on NAS/Linux hosts

### Database

- `DATABASE_URL`: recommended database connection string
- `DB_DRIVER`: optional `sqlite` or `postgres`
- `DATABASE_DRIVER`: alias for `DB_DRIVER`
- `DB_MAX_IDLE_CONNS`: default `10`
- `DB_MAX_OPEN_CONNS`: default `25`
- `DB_CONN_MAX_LIFETIME_MINUTES`: default `0`

SQLite:

```bash
DATABASE_URL=sqlite:///config/briefcast.db
```

Postgres:

```bash
DATABASE_URL=postgres://briefcast:briefcast@postgres:5432/briefcast?sslmode=disable
DB_DRIVER=postgres
```

### Networking And Concurrency

- `PER_HOST_MAX_CONCURRENCY`: per-host outbound request cap; default `2`
- `PER_HOST_RATE_LIMIT_RPS`: per-host rate limit; default `2.0`; `0` disables pacing
- `HTTP_TIMEOUT_SECONDS`: outbound HTTP timeout; default `900`; `0` disables
- `BRIEFCAST_ALLOW_PRIVATE_OUTBOUND_URLS`: allow feed/media/image requests to private, loopback, link-local, or hostnames resolving to private addresses; default `false`
- `BRIEFCAST_ALLOW_PRIVATE_BRIEFPOINT_URLS`: allow Briefpoint requests to private or loopback hosts; default `true`
- `BRIEFCAST_ALLOW_PRIVATE_LLM_URLS`: allow LLM summary requests to private or loopback hosts for local OpenAI-compatible providers; default `true`
- `BRIEFCAST_MAX_OUTBOUND_RESPONSE_BYTES`: maximum in-memory response size for feed/search/transcript metadata and LLM responses; default `20971520`

### Logging

- `LOG_LEVEL`: `debug`, `info`, `warn`, or `error`; default `info`
- `LOG_FORMAT`: `json` or `text`; default `json` for Go services
- `LOG_OUTPUT`: comma-separated outputs such as `stdout,file:/logs/briefcast-{startup_ts}.log`
- `LOG_FILE_MAX_SIZE_MB`: default `50`
- `LOG_FILE_MAX_BACKUPS`: default `7`
- `LOG_FILE_MAX_AGE_DAYS`: default `14`
- `LOG_FILE_COMPRESS`: default `true`
- `LOG_RUN_TIMESTAMP`: optional shared timestamp token

Log path tokens include `{startup_ts}`, `{timestamp}`, and `{run_ts}`.

### Search Providers

- `PODCASTINDEX_KEY`: optional PodcastIndex API key
- `PODCASTINDEX_SECRET`: optional PodcastIndex API secret

### Python Helpers

Feed parsing:

- `FEEDPARSER_PYTHON`: interpreter path
- `FEEDPARSER_SCRIPT`: default `scripts/feedparser_parse.py`
- `FEEDPARSER_TIMEOUT_SECONDS`: default `30`; `0` disables

ID3 extraction:

- `MUTAGEN_PYTHON`: interpreter path; falls back to `FEEDPARSER_PYTHON`
- `MUTAGEN_SCRIPT`: default `scripts/mutagen_id3_extract.py`
- `MUTAGEN_TIMEOUT_SECONDS`: default `20`; `0` disables

### WhisperX

The lean app uses `WHISPERX_BACKEND=http`. CPU/CUDA worker images support Linux amd64 and retain baseline WhisperX 3.8.6. Local interpreter/script settings below apply only to transitional monolith/source runs; device, compute, batch size and execution capacity in HTTP mode belong to the worker.

- `WHISPERX_ENABLED`: default `false`
- `WHISPERX_PYTHON`: interpreter path
- `WHISPERX_SCRIPT`: default `scripts/whisperx_transcribe.py`
- `WHISPERX_TIMEOUT_SECONDS`: default `21600`
- `WHISPERX_MODEL`: default `medium.en`
- `WHISPERX_LANGUAGE`: default `en`
- `WHISPERX_DEVICE`: `auto`, `cuda`, or `cpu`; default `auto`
- `WHISPERX_COMPUTE_TYPE`: `auto`, `float16`, `int8`, or `float32`; default `auto`
- `WHISPERX_BATCH_SIZE`: auto-sized when unset
- `WHISPERX_BEAM_SIZE`: default `5`
- `WHISPERX_PATIENCE`: default `1`
- `WHISPERX_CONDITION_ON_PREVIOUS_TEXT`: default `true`
- `WHISPERX_INITIAL_PROMPT`: optional prompt
- `WHISPERX_VAD_METHOD`: default `pyannote`; falls back to `silero` if pyannote VAD fails
- `WHISPERX_ALIGN`: default `true`
- `WHISPERX_DIARIZATION`: Compose default `false` (source/local default `true`)
- `WHISPERX_DIARIZATION_MODEL`: default `pyannote/speaker-diarization-3.1`
- `WHISPERX_MIN_SPEAKERS`: default `2`
- `WHISPERX_MAX_SPEAKERS`: default `2`
- `HOST_WHISPERX_HF_TOKEN_FILE`: private worker token file for gated Hugging Face models
- `WHISPERX_MAX_CONCURRENCY`: default `1`
- `WHISPERX_MAX_ITEMS`: default `0`, no limit
- `WHISPERX_RETRY_FAILED`: default `true`
- `WHISPERX_RETRY_DELAY_SECONDS`: default `300`
- `WHISPERX_RETRY_MAX_DELAY_SECONDS`: default `21600`
- `WHISPERX_CHUNK_SECONDS`: default `120`
- `WHISPERX_PROGRESS_POLL_MILLIS`: default `2000`
- `WHISPERX_CHECK_FREQUENCY`: defaults to `CHECK_FREQUENCY`
- `WHISPERX_HF_HOME`: default `/config/.cache/huggingface` when `/config` exists
- `WHISPERX_DISABLE_TELEMETRY`: default `true`
- `WHISPERX_THIRD_PARTY_LOG_LEVEL`: default `warning`

When diarization is enabled, Briefcast forces WhisperX worker count to `1` for host stability. Progress checkpoints are persisted so interrupted transcriptions can resume from the last completed chunk.

Advanced app overrides can live in optional `.env.whisperx`. The shared Compose selector loads it after the primary env file, so overrides participate in interpolation. Worker containers receive only explicitly mapped runtime settings and secret-file mounts.

### LLM Summaries

Summarization uses an OpenAI-compatible chat completion API and requires transcripts.

- `LLM_ENABLED`: default `false`
- `LLM_PROVIDER`: default `openai`
- `LLM_API_KEY`: API key; required when summaries are enabled
- `LLM_BASE_URL`: default `https://api.openai.com/v1`
- `LLM_MODEL`: default `gpt-4o-mini`
- `LLM_MAX_TOKENS`: default `1024`
- `LLM_TEMPERATURE`: default `0.3`
- `LLM_TIMEOUT_SECONDS`: default `120`
- `LLM_SUMMARIZATION_PROMPT`: optional system prompt override
- `LLM_SUMMARIZATION_USER_PROMPT`: optional user prompt template override

For OpenRouter, set `LLM_PROVIDER=openrouter`,
`LLM_BASE_URL=https://openrouter.ai/api/v1`, and `LLM_API_KEY` to your
OpenRouter key. The Settings page shows the configured provider and offers a
refreshable list of OpenRouter models suited to long transcripts. The server
fetches the list from OpenRouter's public catalog without sending your API key;
it shows each model's context limit and current token prices. Selecting a model
changes future summaries after you save the summarization settings. The default
option continues to use `LLM_MODEL` from the environment. Existing summaries
retain the actual model reported by the completion response, including when
`LLM_MODEL=openrouter/auto` routes dynamically.

## Back Catalog Downloads

The app does not intentionally truncate RSS feed entries during parsing. It stores the entries returned by the podcast RSS feed.

The setting that controls what gets queued for download when a podcast is first added is `InitialDownloadMode`:

- `count`: queue the first `InitialDownloadCount` feed entries.
- `months`: queue entries with a publish date in the last `InitialDownloadMonths` months.
- `all`: queue every entry exposed by the feed.

Values are managed in the Settings UI and through the settings API. A count or month value of `0` means no app-side limit for that mode.

If a provider's RSS feed only exposes the newest 10 to 20 episodes, Briefcast can only ingest those entries from that feed. To recover older episodes, use a full-history/archive feed, a provider-specific API, another catalog source such as PodcastIndex, or a custom import/backfill that has access to older episode metadata and media URLs.

The Episodes page defaults to paginated views, so check pagination and row count before assuming episodes are missing.

## Transcripts And Summaries

Newly generated transcripts and summaries are exported automatically:

- transcripts: `/assets/transcripts/<podcast>/<episode>.txt`
- summaries: `/assets/summaries/<podcast>/<episode>.txt`
- transcript markdown: `/assets/markdown/transcripts/<podcast>/<episode>.md`
- summary markdown: `/assets/markdown/summaries/<podcast>/<episode>.md`

Feed-provided transcripts, WhisperX transcripts, and LLM summaries all write text and markdown files into `/assets` when they are saved.

The `export-all` API writes existing database transcripts and summaries back into the assets tree:

```bash
curl -X POST http://localhost:8080/settings/export-all
curl http://localhost:8080/settings/export-all
```

`export-all` also handles legacy transcript rows that only have raw `transcript_json`: it builds the canonical transcript text, persists the canonical fields, and writes the transcript file.

If basic auth is enabled:

```bash
curl -u briefcast:<password> -X POST http://localhost:8080/settings/export-all
```

## Scheduled Jobs

Based on `CHECK_FREQUENCY=N` minutes:

- `RefreshEpisodes`: every `N`
- `CheckMissingFiles`: every `N`
- `DownloadMissingImages`: every `N`
- `UnlockMissedJobs`: every `2N`
- `UpdateAllFileSizes`: every `3N`
- `TranscribePendingEpisodes`: every `WHISPERX_CHECK_FREQUENCY`, or every `N` when unset
- `RetentionCleanup`: every `24h`
- `CreateBackup`: every `48h`

## Frontend Development

Run the Vite dev server:

```bash
npm --prefix frontend run dev
```

Build for backend serving:

```bash
npm --prefix frontend run build
```

## Testing

Full suite:

```bash
just test-full
```

Targeted suites:

```bash
just test-go
just test-python
just test-frontend
just test-integration
just test-whisperx-real
```

Formatting and linting:

```bash
just lint
just format
just typecheck
```

Python tooling:

```bash
uv sync --locked --group dev
uv run ruff check .
uv run ruff format --check .
uv run mypy src
uv run pytest
uv run pip-audit
```

## Release Basics

The app version is defined in `pyproject.toml`; the worker version is independent in `docker/whisperx/VERSION`. `-Target worker -Runtime cpu|cuda` selects a worker build/release. Artifact manifests record immutable image IDs, source revision, archive checksum and variant. Deploy/verify/rollback all use the same explicit Compose selection and target only the chosen service. Worker replacement drains acceptance and requires an empty queue; retain a quiescent state backup and verify compatibility before resuming.

Release helpers:

```bash
just release-help
just release-test
just release-build
just release-publish -Bump patch
just release-deploy
just release-verify
just release-rollback
just ship -Bump patch
```

One-command GitHub Actions release:

```bash
gh workflow run release.yml --ref <default-branch> -f bump=patch
gh workflow run release.yml --ref <default-branch> -f bump=minor
gh workflow run release.yml --ref <default-branch> -f bump=major
gh workflow run release.yml --ref <default-branch> -f version=1.9.4
```

Dry run:

```bash
gh workflow run release.yml --ref <default-branch> -f bump=patch -f dry_run=true
```

Manual image publish:

```bash
docker buildx build --platform linux/amd64 --build-arg APP_VERSION=1.9.4 \
  -t ghcr.io/ctaylor1/briefcast:1.9.4 \
  -t ghcr.io/ctaylor1/briefcast:latest \
  --push .
```

## Reset Runtime Data

Destructive reset (`RELEASE.ps1 reset`, `just clean`, and the former shell reset helper) is disabled for the split stack. The old commands can erase media referenced by durable worker jobs. Use the reviewed recovery/restore procedures in [the execution plan](docs/whisperx-separation/EXECUTION_PLAN.md); Stage G supplies and rehearses those tools.

## Migration Notes

Default SQLite filename is `briefcast.db`. If an older deployment used another SQLite filename, keep using the existing database by setting `DATABASE_URL` explicitly.

Older deployments may have audio, artwork, transcripts, or summaries directly under `/assets/<podcast>` or under a separate export directory. The current layout is:

```text
/assets/audio/<podcast>/
/assets/images/<podcast>/
/assets/transcripts/<podcast>/
/assets/summaries/<podcast>/
/assets/markdown/transcripts/<podcast>/
/assets/markdown/summaries/<podcast>/
```

After moving files into the current layout, run `export-all` once to regenerate transcript and summary text and markdown files from the database.

For a clean publish state:

```bash
git status --short
git clean -fdX
uv sync --locked --group dev
just test-full
git add .
git commit -m "release: ship-ready"
git tag -a v1.9.4 -m "Briefcast v1.9.4"
git push origin HEAD
git push origin v1.9.4
```
