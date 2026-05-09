# Non-Functional Scaffolding Instructions: Python Version

This document converts the Go scaffolding brief into a Python-first implementation plan. It targets the same kind of self-hosted podcast-management application shell, but it intentionally excludes podcast-management domain functionality. Build only the infrastructure, app shell, tests, frontend, operational tooling, observability, persistence plumbing, and placeholder routes.

Current-date basis: May 1, 2026. Python recommendations below are based on current official documentation and should still be pinned through the generated lockfile at implementation time.

## Goal

Create a self-hosted full-stack app scaffold with:

- A FastAPI HTTP API.
- A modern Vue/Vite/TypeScript single-page app served by the backend.
- SQLite by default with optional PostgreSQL.
- SQLAlchemy 2.0 ORM and Alembic migrations.
- Pydantic Settings for environment configuration.
- Structured JSON logging with request IDs.
- `uv` for Python project management and lockfiles.
- `just` command automation.
- Docker and Docker Compose support.
- Unit tests, integration-test hooks, linting, formatting, static typing, dependency audit, and pre-commit hooks.
- Background job scaffolding and job-lock primitives.

Do not implement podcast CRUD, downloads, audio playback, transcription, search, summaries, or settings logic beyond placeholder endpoints and placeholder screens.

## Repository Layout

Create this structure:

```text
.
├── app/
│   ├── api/
│   │   ├── routes/
│   │   └── deps.py
│   ├── core/
│   │   ├── config.py
│   │   ├── logging.py
│   │   └── request_id.py
│   ├── db/
│   │   ├── base.py
│   │   ├── models.py
│   │   ├── session.py
│   │   └── migrations.py
│   ├── jobs/
│   │   ├── locks.py
│   │   └── scheduler.py
│   ├── services/
│   ├── static.py
│   └── main.py
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
├── migrations/
│   ├── env.py
│   ├── script.py.mako
│   └── versions/
├── tests/
│   ├── api/
│   ├── db/
│   └── jobs/
├── .env.example
├── .pre-commit-config.yaml
├── alembic.ini
├── docker-compose.yml
├── Dockerfile
├── justfile
├── pyproject.toml
├── README.md
└── uv.lock
```

Use package boundaries as follows:

- `app/api`: FastAPI routers, request/response schemas, dependency wiring.
- `app/core`: settings, logging, request IDs, application metadata.
- `app/db`: SQLAlchemy engine/session/models and migration helpers.
- `app/jobs`: scheduler and job locking.
- `app/services`: orchestration placeholders.
- `frontend/src/lib/api`: typed API clients.
- `frontend/src/components/ui`: reusable UI primitives.

## Python Backend Stack

Use Python 3.14 as the default target. Python 3.14.3 was the current maintenance release verified from the Python core team release blog in February 2026, and Python 3.13 remains a conservative fallback for host platforms that lag package support.

Use these dependencies:

- `fastapi[standard-no-fastapi-cloud-cli]` for the API framework and standard production/dev extras without cloud CLI coupling.
- `uvicorn[standard]` only if not already supplied by the FastAPI extra.
- `pydantic` and `pydantic-settings` for validation and typed environment configuration.
- `sqlalchemy[asyncio]` for SQLAlchemy 2.0 async ORM.
- `alembic` for versioned schema migrations.
- `aiosqlite` for SQLite async access.
- `psycopg[binary,pool]` for PostgreSQL.
- `structlog` for structured application logs.
- `orjson` for fast JSON responses if desired.
- `apscheduler` for in-process recurring jobs in a single-instance app. Use stable 3.x unless APScheduler 4.x is no longer pre-release when implementation begins.
- `python-dotenv` only if local `.env` loading is not handled by Pydantic Settings.

Development dependencies:

- `pytest`
- `pytest-asyncio`
- `httpx`
- `ruff`
- `mypy`
- `pip-audit`
- `pre-commit`
- `types-*` packages only when mypy requires them

Project management:

- Use `uv init --package` or manually create `pyproject.toml`.
- Add runtime dependencies with `uv add`.
- Add dev dependencies with `uv add --dev`.
- Commit `uv.lock`.
- Use `uv sync --locked` in CI.

## FastAPI App Skeleton

`app/main.py` must:

- Create an app through an application factory: `create_app(settings: Settings | None = None) -> FastAPI`.
- Configure structured logging before startup work.
- Register middleware for request IDs and HTTP access logs.
- Register exception handlers for unhandled errors and validation errors.
- Register routers under `/api`.
- Serve the Vue SPA from `frontend/dist` at `/app`.
- Redirect `/` to `/app`.
- Expose `/healthz`, `/version`, and placeholder API routes.
- Run DB initialization and scheduler startup through FastAPI lifespan.
- Shut down DB engines and scheduler cleanly on lifespan exit.

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

Use placeholder handlers that return typed Pydantic response models and compile. Do not add domain behavior.

## Configuration Requirements

Implement `app/core/config.py` with `pydantic_settings.BaseSettings`.

Required settings:

```text
APP_NAME=podcast-scaffold
APP_VERSION=dev
APP_REPO_URL=
APP_ENV=local
CONFIG=./config
DATA=./assets
DATABASE_URL=sqlite+aiosqlite:///./config/app.db
DB_DRIVER=
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
DB_POOL_SIZE=5
DB_MAX_OVERFLOW=10
DB_POOL_RECYCLE_SECONDS=0
```

Rules:

- Default to SQLite under `CONFIG` if `DATABASE_URL` is empty.
- Accept `sqlite+aiosqlite:///`, local SQLite file paths, `postgresql+psycopg://`, `postgres://`, and `postgresql://`.
- Normalize PostgreSQL URLs to SQLAlchemy async driver URLs.
- Sanitize credentials before logging DSNs.
- Use a cached `get_settings()` function, but allow tests to pass explicit settings into `create_app`.
- Keep environment parsing in one place; do not read `os.environ` throughout the codebase.

## Database Scaffolding

Use SQLAlchemy 2.0 typed declarative models.

Create base model behavior:

- UUID string primary keys.
- `created_at`.
- `updated_at`.
- nullable `deleted_at`.
- UTC timestamps.

Scaffold models only:

- `Setting`
- `JobLock`
- `AuditEvent`

Alembic requirements:

- Initialize Alembic in `migrations/`.
- Configure `migrations/env.py` to import `app.db.base.Base.metadata`.
- Read DB URL from `Settings`, not hardcoded `alembic.ini`.
- Support async SQLAlchemy engines.
- Add an initial migration for scaffold tables and job-lock unique index.
- Add `just migration message` and `just migrate` recipes.

Session requirements:

- Create `async_sessionmaker[AsyncSession]`.
- Provide `get_session()` FastAPI dependency.
- Use transaction-per-request only where needed; handlers should explicitly commit through service functions.
- Tests should use a temporary SQLite database and run migrations before API tests.

Job-lock requirements:

- `acquire_lock(name, duration_minutes)`.
- `try_lock(name, duration_minutes)`.
- `unlock_by_id(id)`.
- `unlock(name)`.
- `unlock_missed_jobs()`.
- Enforce one current row per job name with a unique index.
- Use DB time or UTC application time consistently.

## Logging Requirements

Use `structlog` integrated with the standard `logging` module.

Implement:

- `configure_logging(settings: Settings)`.
- `get_logger(name: str | None = None)`.
- `bind_request_id(request_id: str)`.
- `new_job_logger(job_name: str) -> tuple[BoundLogger, str]`.

Supported env:

- `LOG_LEVEL`: `debug`, `info`, `warning`, `error`.
- `LOG_FORMAT`: `json` or `text`.
- `LOG_OUTPUT`: comma-separated `stdout`, `stderr`, or file paths. Accept `file:/path/to/app-{startup_ts}.log`.
- Rotation envs listed in `.env.example`.

Request middleware must:

- Read `X-Request-ID`.
- Generate a UUID if absent.
- Store the value in a `contextvars.ContextVar`.
- Add `X-Request-ID` to every response.
- Log method, matched path, status, latency, client IP, user agent, and errors.
- Clear contextvars after each request.

Job logging must include:

- `job_name`
- `job_id`
- `duration_ms`
- `job_started`, `job_completed`, and `job_failed` events.

Add optional OpenTelemetry extension points, but do not require telemetry exporters in the base scaffold.

## Background Jobs

Use in-process scheduling for the scaffold. For a single-container self-hosted app, `apscheduler` is sufficient. If the product will run multiple API replicas, move jobs to a separate worker process with a queue such as Celery/RQ/Arq before adding domain logic.

Create placeholder jobs:

- `refresh_external_data`.
- `cleanup_missing_files`.
- `apply_retention_policies`.
- `unlock_missed_jobs`.
- `create_backup`.

Every job must:

- Be injectable for tests through a job registry object.
- Log start/completion/failure.
- Be scheduled from `CHECK_FREQUENCY`.
- Avoid overlapping runs through DB job locks.
- Return without real side effects.

FastAPI lifespan must start the scheduler only when `settings.scheduler_enabled` is true. Tests should disable the scheduler by default.

## Frontend Stack

Use the same frontend architecture as the Go version:

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
- Vite dev server proxies `/api`, `/healthz`, `/version`, `/assets`, and `/ws` to `http://localhost:8080`.

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

## Python Project Configuration

Create `pyproject.toml` with:

```toml
[project]
name = "podcast-scaffold"
version = "0.1.0"
requires-python = ">=3.14"
dependencies = []

[dependency-groups]
dev = []

[tool.ruff]
target-version = "py314"
line-length = 100
src = ["app", "tests"]

[tool.ruff.lint]
select = ["E", "F", "I", "UP", "B", "ASYNC", "SIM"]

[tool.mypy]
python_version = "3.14"
files = ["app", "tests"]
strict = true
warn_unused_configs = true
plugins = []

[tool.pytest.ini_options]
testpaths = ["tests"]
addopts = "-q"
asyncio_mode = "auto"
```

Use `hatchling` or `uv` defaults for packaging. Do not use `requirements.txt` as the primary source of truth.

## Command Automation

Create a `justfile` with:

```just
default:
    @just --list

bootstrap:
    uv sync --group dev
    npm --prefix frontend install

bootstrap-locked:
    uv sync --locked --group dev
    npm --prefix frontend ci

dev-api:
    uv run fastapi dev app/main.py --host 0.0.0.0 --port 8080

dev-frontend:
    npm --prefix frontend run dev

up:
    docker compose --env-file .env -f docker-compose.yml up -d

down:
    docker compose --env-file .env -f docker-compose.yml down

logs service="" lines="200":
    docker compose --env-file .env -f docker-compose.yml logs -f --tail {{lines}} {{service}}

lint:
    uv run ruff check .
    uv run ruff format --check .

format:
    uv run ruff format .
    uv run ruff check --fix .

typecheck:
    uv run mypy app tests
    npm --prefix frontend run build

test-python:
    uv run pytest

test-frontend:
    npm --prefix frontend run test

audit:
    uv run pip-audit

migration message:
    uv run alembic revision --autogenerate -m "{{message}}"

migrate:
    uv run alembic upgrade head

test-full:
    just lint
    just typecheck
    just test-python
    just test-frontend
    just audit
```

## Testing Requirements

Backend tests:

- Use `pytest`, `pytest-asyncio`, and `httpx.AsyncClient` with ASGI transport.
- Use temporary directories for `CONFIG` and `DATA`.
- Use temporary SQLite DBs.
- Run Alembic migrations before DB/API tests.
- Test `/healthz`, `/version`, and placeholder API routes.
- Test request ID propagation.
- Test settings parsing and DB URL normalization.
- Test migration idempotency.
- Test job lock acquire/release/stale unlock.
- Test scheduler helper functions without sleeping.
- Keep tests deterministic; no real network calls.

Frontend tests:

- At scaffold stage, `npm run test` may be `npm run build`.
- The build must run `vue-tsc -b && vite build`.

Quality gates:

- `uv run pytest` must pass.
- `uv run ruff check .` must pass.
- `uv run ruff format --check .` must pass.
- `uv run mypy app tests` must pass.
- `npm --prefix frontend run build` must pass.
- `just test-full` must pass.

## Pre-Commit

Create `.pre-commit-config.yaml` with local hooks:

- `ruff format`.
- `ruff check`.
- `mypy app tests`.
- `pytest`.
- `npm --prefix frontend run build`.

Use `uv run` for Python hooks so the lockfile environment is used.

## Docker

Use a multi-stage Dockerfile:

1. `node:<current-lts>-alpine` builds frontend.
2. `python:3.14-slim` runtime installs Python dependencies with `uv sync --locked --no-dev`.
3. Runtime copies `app`, `migrations`, `alembic.ini`, web assets, and `frontend/dist`.

Runtime requirements:

- Expose `8080`.
- Set `CONFIG=/config`.
- Set `DATA=/assets`.
- Create `/config` and `/assets`.
- Serve `frontend/dist` from FastAPI.
- Start with `uvicorn app.main:create_app --factory --host 0.0.0.0 --port 8080`.

Compose requirements:

- Main service maps `${HOST_PORT:-8080}:8080`.
- Mount `${HOST_CONFIG_DIR:-./config}:/config`.
- Mount `${HOST_ASSETS_DIR:-./assets}:/assets`.
- Mount `${HOST_LOGS_DIR:-./logs}:/logs`.
- Set logging, DB, and scheduler env.
- Include optional `postgres` profile using a current supported PostgreSQL Alpine image.
- Use `restart: unless-stopped`.

## CI

Create GitHub Actions workflows:

- `ci.yml`: checkout, install uv, set up Python 3.14, set up Node, run `uv sync --locked --group dev`, run `npm ci`, run `just test-full`.
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
- Alembic migrations run cleanly.
- Background jobs start only when enabled and log placeholder activity.
- No podcast-specific functionality exists beyond generic placeholder screens.

## Source Notes

- Python 3.14.3 release status was checked against the Python core team release blog.
- FastAPI’s current standard installation, dependency model, automatic docs, and Uvicorn relationship were checked against official FastAPI docs.
- `uv` project and lockfile workflow was checked against official Astral uv docs.
- Pydantic Settings usage was checked against official Pydantic docs.
- SQLAlchemy 2.0 async ORM guidance was checked against official SQLAlchemy docs.
- Alembic migration setup was checked against official Alembic docs.
- Ruff formatter/linter positioning was checked against official Ruff docs.
- FastAPI test-client behavior was checked against official FastAPI docs.
