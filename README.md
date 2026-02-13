# Briefcast
Podcast downloader and manager with a classic server-rendered UI and a modern Vue 3 app.

## What is in this repo

- Go backend (`gin` + `gorm`) with REST endpoints and HTML pages
- Legacy UI served from `client/*` at `/`
- Modern Vue 3 + Vite + TypeScript + Tailwind app served at `/app`
- SQLite/Postgres database support via `DATABASE_URL`
- Background jobs for feed refresh, downloads, image sync, backups, and maintenance

## Quick start (local)

Prerequisites:

- Go `1.26+`
- Node `24+` (for modern frontend build/dev)

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

Available UIs:

- Legacy UI: `http://localhost:8080/`
- Modern UI: `http://localhost:8080/app` (requires `frontend/dist` build output)

If `PASSWORD` is set, HTTP basic auth is enabled with username `briefcast`.

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

Logging:

- `LOG_LEVEL`: `debug|info|warn|error` (default `info`)
- Incoming `X-Request-ID` is propagated; otherwise generated
- Background jobs include `job_name` and `job_id` in logs

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

## Notes after renaming to Briefcast

Default SQLite filename is now `briefcast.db`.  
If you are migrating from an older deployment that used a different SQLite filename, set `DATABASE_URL` explicitly to that existing file path.
