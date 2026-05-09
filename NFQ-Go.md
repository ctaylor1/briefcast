# Non-Functional Scaffolding Instructions: Go Version

This document is an agent-ready specification for building the scaffolding of a podcast-management style application with the same non-functional qualities as this repository. It intentionally excludes podcast-management domain functionality. Build the shell, infrastructure, quality gates, observability, persistence plumbing, and frontend foundation only.

## Goal

Create a self-hosted full-stack app scaffold with:

- A Go HTTP API.
- A modern Vue/Vite/TypeScript single-page app served by the backend.
- SQLite by default with optional PostgreSQL.
- Structured logging with request IDs.
- `just` command automation.
- Docker and Docker Compose support.
- Unit tests, integration-test hooks, linting, formatting, type checks, and pre-commit hooks.
- Background job scaffolding and job-lock primitives.
- Clear configuration via environment variables and `.env`.

Do not implement podcast CRUD, downloads, audio playback, transcription, search, summaries, or settings logic beyond placeholder endpoints and placeholder screens.

## Repository Layout

Create this structure:

```text
.
├── cmd/
├── controllers/
├── db/
├── frontend/
│   ├── src/
│   │   ├── components/
│   │   ├── composables/
│   │   ├── lib/
│   │   ├── router/
│   │   ├── types/
│   │   └── views/
│   ├── package.json
│   ├── tsconfig*.json
│   └── vite.config.ts
├── internal/
│   └── logging/
├── service/
├── tests/
├── webassets/
├── .env.example
├── .pre-commit-config.yaml
├── docker-compose.yml
├── Dockerfile
├── go.mod
├── justfile
├── main.go
└── README.md
```

Use package boundaries as follows:

- `controllers`: HTTP handlers only, thin request/response layer.
- `service`: business orchestration, background jobs, filesystem helpers.
- `db`: database models, config, migrations, query helpers.
- `internal/logging`: app-wide logging and request middleware.
- `frontend/src/lib/api`: typed API clients.
- `frontend/src/components/ui`: reusable UI primitives.

## Backend Stack

Use Go with these core dependencies:

- `github.com/gin-gonic/gin` for HTTP routing.
- `github.com/gin-contrib/location` for URL/location middleware if URL generation is needed.
- `gorm.io/gorm` for ORM.
- `gorm.io/driver/sqlite` for local default persistence.
- `gorm.io/driver/postgres` for optional PostgreSQL.
- `github.com/google/uuid` for primary IDs and request/job IDs.
- `go.uber.org/zap` for structured logging.
- `gopkg.in/natefinch/lumberjack.v2` for rotating file logs.
- `github.com/joho/godotenv` for local `.env` autoloading.
- `github.com/robfig/cron/v3` for in-process recurring jobs.
- `github.com/gorilla/websocket` only if the scaffold includes websocket plumbing.

Initialize `go.mod` with the module name selected by the project owner. Pin dependencies through `go.mod` and `go.sum`.

## Backend App Skeleton

`main.go` must:

- Support `--version`.
- Load `.env` automatically for local development.
- Initialize logging before other services.
- Initialize DB from environment.
- Run migrations on startup.
- Build a `gin.New()` router with custom middleware, recovery, and optional basic auth.
- Serve the frontend from `frontend/dist` at `/app`.
- Redirect `/` to `/app`.
- Expose `/version`, `/healthz`, and placeholder API routes.
- Start background scheduler in a goroutine.
- Log fatal startup errors and sync logs on shutdown.

Use this route shape:

```text
GET  /healthz
GET  /version
GET  /app
GET  /app/
GET  /app/assets/*
GET  /api/status
GET  /api/settings
PATCH /api/settings
```

Use placeholder handlers that return JSON and compile. Do not add domain behavior.

## Configuration Requirements

Use environment variables only. Provide `.env.example` with:

```text
CONFIG=./config
DATA=./assets
DATABASE_URL=sqlite:///config/app.db
DB_DRIVER=
DATABASE_DRIVER=
PASSWORD=
CHECK_FREQUENCY=15
LOG_LEVEL=info
LOG_FORMAT=json
LOG_OUTPUT=stdout
LOG_FILE_MAX_SIZE_MB=50
LOG_FILE_MAX_BACKUPS=7
LOG_FILE_MAX_AGE_DAYS=14
LOG_FILE_COMPRESS=true
LOG_RUN_TIMESTAMP=
DB_MAX_IDLE_CONNS=10
DB_MAX_OPEN_CONNS=25
DB_CONN_MAX_LIFETIME_MINUTES=0
```

Rules:

- Default to SQLite under `CONFIG` if `DATABASE_URL` is empty.
- Accept `sqlite://`, `sqlite3://`, local file paths, `postgres://`, and `postgresql://`.
- Allow explicit driver via `DB_DRIVER` or `DATABASE_DRIVER`.
- Sanitize credentials before logging DSNs.
- Configure SQL connection pool from env.

## Database Scaffolding

Create a reusable base model:

```go
type Base struct {
    ID        string `sql:"type:uuid;primary_key"`
    CreatedAt time.Time
    UpdatedAt time.Time
    DeletedAt *time.Time `gorm:"index"`
}
```

Add a `BeforeCreate` hook that assigns `uuid.NewString()`.

Create scaffold models only:

- `Setting`
- `Migration`
- `JobLock`
- `AuditEvent` or another generic placeholder model

Migration requirements:

- Run `AutoMigrate` for scaffold models.
- Maintain a `migrations` table with migration name and timestamp.
- Provide an idempotent local migration runner.
- Support SQLite differences when an SQL statement uses `ADD COLUMN IF NOT EXISTS`.
- Add tests for migration idempotency.

Job-lock requirements:

- `Lock(name, durationMinutes)`.
- `TryLock(name, durationMinutes)`.
- `UnlockByID(id)`.
- `Unlock(name)`.
- `UnlockMissedJobs()`.
- Enforce one current row per job name with a unique index.

## Logging Requirements

Use `zap` as the only logging API exposed to app code.

Implement:

- `logging.Base() *zap.Logger`.
- `logging.Sugar() *zap.SugaredLogger`.
- `logging.LoggerWithRequestID(requestID string)`.
- `logging.NewJobLogger(jobName string) (*zap.Logger, string)`.
- `logging.NewJobSugar(jobName string) (*zap.SugaredLogger, string)`.
- `logging.Sync()`.

Supported env:

- `LOG_LEVEL`: `debug`, `info`, `warn`, `error`.
- `LOG_FORMAT`: `json` or `text`.
- `LOG_OUTPUT`: comma-separated `stdout`, `stderr`, or file paths. Accept `file:/path/to/app-{startup_ts}.log`.
- Rotation envs listed in `.env.example`.

Request middleware must:

- Read `X-Request-ID`.
- Generate a UUID if absent.
- Store request ID in Gin context.
- Add `X-Request-ID` to every response.
- Log method, matched path, status, latency, client IP, user agent, and errors.

Job logging must include:

- `job_name`
- `job_id`
- `duration_ms`
- `job_started`, `job_completed`, and `job_failed` events.

## Background Jobs

Use `robfig/cron/v3`.

Create a scheduler with placeholder jobs:

- `RefreshExternalData`
- `CleanupMissingFiles`
- `ApplyRetentionPolicies`
- `UnlockMissedJobs`
- `CreateBackup`

Every job must:

- Be injectable for tests through a job function struct.
- Log start/completion/failure.
- Be scheduled from `CHECK_FREQUENCY`.
- Recover panics through cron middleware.

Do not implement real work. Return `nil` from placeholders.

## Frontend Stack

Use:

- Vue 3.
- TypeScript.
- Vite.
- Vue Router with hash history under `/app/`.
- Tailwind CSS via `@tailwindcss/vite`.
- Axios for HTTP.
- `@headlessui/vue` where accessible popovers/dialogs are needed.
- `clsx` and `tailwind-merge` for class merging.
- `marked` only if markdown rendering placeholders are needed.

Use this dependency pattern in `frontend/package.json`:

```json
{
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vue-tsc -b && vite build",
    "preview": "vite preview",
    "test": "npm run build"
  }
}
```

Create placeholder routes:

- `/`
- `/items`
- `/downloads`
- `/settings`
- `/about`

All screens should render real layout placeholders, not TODO text. Use reusable UI primitives:

- `UiButton`
- `UiCard`
- `UiAlert`
- `UiBadge`
- `UiInput`
- `UiSelect`
- `UiDialog`
- `UiDrawer`
- `UiDropdown`
- `UiTooltip`

Frontend API requirements:

- `frontend/src/lib/api/http.ts` exports one Axios client.
- API modules are small and typed.
- Types live in `frontend/src/types/api.ts`.
- Errors are normalized by `getErrorMessage`.
- Vite dev server proxies backend routes to `http://localhost:8080`.

## Styling and Accessibility

Use a quiet operational UI, suitable for repeated use:

- No landing page.
- No marketing hero.
- Responsive layout with navigation.
- Cards only for repeated items, modals, and framed tools.
- Avoid nested cards.
- Buttons have stable sizes and accessible labels.
- Forms use visible labels.
- Tables and lists have mobile alternatives where needed.
- Use CSS variables for theme tokens.

## Command Automation

Create a `justfile` with:

```just
default:
    @just --list

bootstrap:
    go mod download
    npm --prefix frontend install

git-status:
    git status --short --branch

up:
    docker compose --env-file .env -f docker-compose.yml up -d

down:
    docker compose --env-file .env -f docker-compose.yml down

logs service="" lines="200":
    docker compose --env-file .env -f docker-compose.yml logs -f --tail {{lines}} {{service}}

lint:
    gofmt -w .
    go vet ./...

test-go:
    go test ./...

test-frontend:
    npm --prefix frontend run test

typecheck:
    npm --prefix frontend run build

test-full:
    just lint
    just test-go
    just test-frontend
```

Add Docker and release recipes as needed, but keep the initial scaffold small.

## Testing Requirements

Backend tests:

- Use `testing`, `httptest`, and Gin test mode.
- Use temporary directories for `CONFIG` and `DATA`.
- Use SQLite test DBs.
- Test `/healthz`, `/version`, and placeholder API routes.
- Test request ID propagation.
- Test DB config parsing.
- Test migration idempotency.
- Test job lock acquire/release/stale unlock.
- Test scheduler helper functions without sleeping.

Frontend tests:

- At scaffold stage, `npm run test` may be `npm run build`.
- The build must run `vue-tsc -b && vite build`.

Quality gates:

- `go test ./...` must pass.
- `npm --prefix frontend run build` must pass.
- `just test-full` must pass.

## Pre-Commit

Create `.pre-commit-config.yaml` with local hooks:

- `gofmt`
- `go test ./...`
- `npm --prefix frontend run build`

If Python utility tooling is present, add Ruff, mypy, and pytest hooks as separate local hooks.

## Docker

Use a multi-stage Dockerfile:

1. `node:<version>-alpine` builds frontend.
2. `golang:<version>` builds Go binary with `-ldflags "-X main.appVersion=${APP_VERSION}"`.
3. Slim runtime image copies binary, web assets, and `frontend/dist`.

Runtime requirements:

- Expose `8080`.
- Set `CONFIG=/config`.
- Set `DATA=/assets`.
- Create `/config` and `/assets`.
- Serve `frontend/dist` from backend.

Compose requirements:

- Main service maps `${HOST_PORT:-8080}:8080`.
- Mount `${HOST_CONFIG_DIR:-./config}:/config`.
- Mount `${HOST_ASSETS_DIR:-./assets}:/assets`.
- Mount `${HOST_LOGS_DIR:-./logs}:/logs`.
- Set logging, DB, and scheduler env.
- Include optional `postgres` profile.
- Use `restart: unless-stopped`.

## CI

Create GitHub Actions workflows:

- `ci.yml`: checkout, set up Go, set up Node, install frontend deps, run `go test ./...`, run frontend build.
- `secret-scan.yml`: run a secret scanner or documented placeholder.
- `release.yml`: build Docker image and attach version metadata when tags are pushed.

## Acceptance Criteria

The scaffold is complete when:

- `just bootstrap` installs dependencies.
- `just test-full` passes.
- `docker compose up -d --build` starts the app.
- `GET /healthz` returns success.
- `GET /version` returns JSON with version and repo URL.
- `GET /app` serves the Vue SPA.
- Logs are structured JSON by default and include request IDs.
- SQLite works out of the box.
- PostgreSQL can be enabled by env.
- Background jobs start and log placeholder activity.
- No podcast-specific functionality exists beyond generic placeholder screens.
