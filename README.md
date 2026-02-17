# Briefcast
Podcast downloader and manager with a modern Vue 3 app.

## What is in this repo

- Go backend (`gin` + `gorm`) with REST endpoints
- Modern Vue 3 + Vite + TypeScript + Tailwind app served at `/app`
- SQLite/Postgres database support via `DATABASE_URL`
- Background jobs for feed refresh, downloads, image sync, backups, and maintenance

## Quick start (local)

Prerequisites:

- Go `1.26+`
- Node `24+` (for modern frontend build/dev)
- Python `3.10+` with `feedparser` and `mutagen` (`pip install feedparser mutagen`)
- `uv` (`https://docs.astral.sh/uv/`) for Python project tooling
- Optional: WhisperX transcription requires `whisperx`, `torch`, and `pyannote` (see WhisperX env vars below)

1. Install deps:

```bash
go mod download
npm --prefix frontend install
```

2. Set environment variables (or edit `.env`):

```bash
CONFIG=.
DATA=./assets
CHECK_FREQUENCY=10
PASSWORD=
```

3. (Optional) build modern frontend:

```bash
npm --prefix frontend run build
```

4. Run backend:

```bash
go run ./main.go
```

Available UI:

- Modern UI: `http://localhost:8080/app` (requires `frontend/dist` build output). The root path `/` redirects to `/app`.

If `PASSWORD` is set, HTTP basic auth is enabled with username `briefcast`.

## Install

```bash
go mod download
npm --prefix frontend install
uv sync --group dev
```

## Run

```bash
go run ./main.go
```

## Test

```bash
go test ./...
npm --prefix frontend run test
uv run pytest
```

## Lint / Format

```bash
uv run ruff check .
uv run ruff format --check .
```

Fix formatting locally:

```bash
uv run ruff format .
```

## Type Check

```bash
uv run mypy src
```

## Docker

Use `docker-compose.yml`:

```bash
docker compose up -d
```

The default service uses SQLite. A Postgres service is included under profile `postgres`.

## Environment variables

Core runtime:

- `CONFIG`: config directory (default used by app logic when not provided is `.`, but set this explicitly in real deployments)
- `DATA`: media/assets directory
- `CHECK_FREQUENCY`: cron base interval in minutes (defaults to `30` if invalid)
- `PASSWORD`: optional basic auth password (username is `briefcast`)
- `GIN_MODE`: Gin mode (commonly `release`)
- `PUID`, `PGID`: optional ownership mapping for downloaded files/folders

Database:

- `DATABASE_URL`: connection string/path (recommended)
- `DB_DRIVER`: optional override (`sqlite` or `postgres`)
- `DATABASE_DRIVER`: alias for `DB_DRIVER`
- `DB_MAX_IDLE_CONNS`: default `10`
- `DB_MAX_OPEN_CONNS`: default `25`
- `DB_CONN_MAX_LIFETIME_MINUTES`: default `0` (disabled)

Networking/concurrency:

- `PER_HOST_MAX_CONCURRENCY`: per-host in-flight outbound request cap, default `2`
- `PER_HOST_RATE_LIMIT_RPS`: per-host request pacing cap, default `2.0` (`0` disables pacing)
- `HTTP_TIMEOUT_SECONDS`: outbound HTTP request timeout cap used by download/feed/image requests (default `900`; set `0` to disable)

Logging:

- `LOG_LEVEL`: `debug|info|warn|error` (default `info`)
- `LOG_FORMAT`: `json|text` (default `json` for Go services, `text` default in Python helper unless overridden)
- `LOG_OUTPUT`: comma-separated outputs. Supports `stdout`, `stderr`, or a file path (for example `LOG_OUTPUT=stdout,/var/log/briefcast/app.log`)
- `LOG_FILE_MAX_SIZE_MB`: max size per log file (default `50`)
- `LOG_FILE_MAX_BACKUPS`: number of rotated files to keep (default `7`)
- `LOG_FILE_MAX_AGE_DAYS`: max age of rotated logs (default `14`)
- `LOG_FILE_COMPRESS`: `true|false` to gzip rotated logs (default `true`)
- Incoming `X-Request-ID` is propagated; otherwise generated
- Background jobs include `job_name` and `job_id` in logs
- Python helper scripts also honor `LOG_LEVEL`/`LOG_FORMAT`, emit logs to `stderr`, and redact common secret fields (tokens/passwords/api keys)

Search providers:

- `PODCASTINDEX_KEY`: API key for PodcastIndex search (optional)
- `PODCASTINDEX_SECRET`: API secret for PodcastIndex search (optional)

Feed parsing:

- `FEEDPARSER_PYTHON`: path to Python interpreter (defaults to `python3`/`python`)
- `FEEDPARSER_SCRIPT`: path to `feedparser_parse.py` (default `scripts/feedparser_parse.py`)
- `FEEDPARSER_TIMEOUT_SECONDS`: timeout for feedparser subprocess execution (default `30`; set `0` to disable)

ID3 extraction:

- `MUTAGEN_PYTHON`: path to Python interpreter (defaults to `MUTAGEN_PYTHON` > `FEEDPARSER_PYTHON` > `python3`/`python`)
- `MUTAGEN_SCRIPT`: path to `mutagen_id3_extract.py` (default `scripts/mutagen_id3_extract.py`)
- `MUTAGEN_TIMEOUT_SECONDS`: timeout for mutagen subprocess execution (default `20`; set `0` to disable)

WhisperX transcription (optional):

- Requires installing WhisperX + dependencies (not bundled in the default Docker image).
- `WHISPERX_ENABLED`: `true|false` to enable transcription (default `false`)
- `WHISPERX_PYTHON`: path to Python interpreter (defaults to `WHISPERX_PYTHON` > `FEEDPARSER_PYTHON` > `python3`/`python`)
- `WHISPERX_SCRIPT`: path to `whisperx_transcribe.py` (default `scripts/whisperx_transcribe.py`)
- `WHISPERX_TIMEOUT_SECONDS`: timeout for WhisperX subprocess execution (default `7200`; set `0` to disable)
- `WHISPERX_MODEL`: WhisperX model name (default `medium.en`)
- `WHISPERX_LANGUAGE`: language code (default `en`)
- `WHISPERX_DEVICE`: `auto|cuda|cpu` (default `auto`)
- `WHISPERX_COMPUTE_TYPE`: `auto|float16|int8|float32` (default `auto`)
- `WHISPERX_BATCH_SIZE`: override batch size (default auto: `16` on CUDA, `4` on CPU)
- `WHISPERX_BEAM_SIZE`: beam search size (default `5`)
- `WHISPERX_PATIENCE`: beam search patience (default `1`)
- `WHISPERX_CONDITION_ON_PREVIOUS_TEXT`: `true|false` (default `true`)
- `WHISPERX_INITIAL_PROMPT`: initial prompt string
- `WHISPERX_VAD_CHUNK_SIZE`: VAD chunk size in seconds (default `45`)
- `WHISPERX_VAD_ONSET`: VAD onset threshold (default `0.50`)
- `WHISPERX_VAD_OFFSET`: VAD offset threshold (default `0.50`)
- `WHISPERX_VAD_METHOD`: VAD method (default `pyannote`)
- `WHISPERX_ALIGN`: `true|false` word-level alignment (default `true`)
- `WHISPERX_DIARIZATION`: `true|false` speaker diarization (default `true`)
- `WHISPERX_DIARIZATION_MODEL`: diarization model (default `pyannote/speaker-diarization-3.1`)
- `WHISPERX_MIN_SPEAKERS`: minimum speakers (default `2`)
- `WHISPERX_MAX_SPEAKERS`: maximum speakers (default `2`)
- `WHISPERX_HF_TOKEN`: Hugging Face token for diarization (required for pyannote models)
- `WHISPERX_MAX_CONCURRENCY`: worker count (default `1`)
- `WHISPERX_MAX_ITEMS`: max items per run (`0` = no limit)
- `WHISPERX_RETRY_FAILED`: retry items marked `failed` (default `false`)
- `WHISPERX_CHECK_FREQUENCY`: minutes between transcription runs (default: `CHECK_FREQUENCY`)

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

## Scheduled jobs

Based on `CHECK_FREQUENCY`:

- `RefreshEpisodes`: every `N` minutes
- `CheckMissingFiles`: every `N` minutes
- `DownloadMissingImages`: every `N` minutes
- `UnlockMissedJobs`: every `2N` minutes
- `UpdateAllFileSizes`: every `3N` minutes
- `CreateBackup`: every `48h`

## Frontend development

Run Vite dev server:

```bash
npm --prefix frontend run dev
```

Build for backend serving:

```bash
npm --prefix frontend run build
```

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

On Windows, you can also run:

```powershell
./scripts/regression.ps1
```

## Python quality tooling

The repository includes a Python tooling spine using `uv`, project-local `.venv`, and a committed `uv.lock`.

Setup:

```bash
uv sync --group dev
```

CI parity setup (enforced lockfile):

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

- Do not commit real credentials; keep secrets in local `.env` or CI secret stores.
- Automated secret scanning runs in CI (`.github/workflows/secret-scan.yml`).

## Release basics

- Current package version is defined in `pyproject.toml`.
- Keep release notes in `CHANGELOG.md` (update `Unreleased` before tagging).
- Recommended tag format: `vX.Y.Z`.

## Reset And Share On GitHub

Use this process when you want a clean, reproducible repository state before publishing.

1. Verify current branch and changes:

```bash
git branch --show-current
git status --short
```

2. Remove local-only generated files (ignored files only):

```bash
git clean -fdX
```

3. Recreate dev environment from lockfile and rerun quality checks:

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

4. Stage and commit:

```bash
git add .
git commit -m "release: ship-ready"
```

5. Tag release (recommended):

```bash
git tag -a v0.1.0 -m "Briefcast v0.1.0"
```

6. Push to GitHub:

```bash
git remote add origin https://github.com/<your-org-or-user>/briefcast.git
git push -u origin <branch-name>
git push origin v0.1.0
```

PowerShell variant for step 2 (same behavior):

```powershell
git clean -fdX
```

If you intentionally want to discard all local uncommitted edits (destructive), run this only after backing up work:

```bash
git restore --staged .
git restore .
git clean -fd
```

## Notes after renaming to Briefcast

Default SQLite filename is now `briefcast.db`.  
If you are migrating from an older deployment that used a different SQLite filename, set `DATABASE_URL` explicitly to that existing file path.
