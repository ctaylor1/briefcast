# briefcast - cross-platform command runner
#
# Install just:
#   winget install Casey.Just
#   brew install just
#   cargo install just
#
# Optional overrides:
#   ENV_FILE=.env just up
#   COMPOSE_FILE=docker-compose.yml just ps
#   BUILD_DIR=builds just release-build

set shell := ["bash", "-c"]
set windows-shell := ["powershell.exe", "-NoLogo", "-ExecutionPolicy", "Bypass", "-Command"]

env_file := env_var_or_default("ENV_FILE", ".env")
compose_file := env_var_or_default("COMPOSE_FILE", "docker-compose.yml")
build_dir := env_var_or_default("BUILD_DIR", "builds")
compose := "docker compose --env-file " + env_file + " -f " + compose_file

default:
    @just --list

# ---------------------------------------------------------------------------
# Bootstrap
# ---------------------------------------------------------------------------

# Install repository dependencies (Go, frontend, Python tooling).
bootstrap:
    go mod download
    npm --prefix frontend install
    uv sync --group dev

# Install Python tooling only.
bootstrap-python:
    uv sync --group dev

# Install Python tooling with lockfile enforcement (CI parity).
bootstrap-python-locked:
    uv sync --locked --group dev

# ---------------------------------------------------------------------------
# Version control
# ---------------------------------------------------------------------------

git-status:
    git status --short --branch

git-diff:
    git diff --stat

git-log count="20":
    git log --oneline -n {{count}}

git-fetch remote="origin":
    git fetch {{remote}} --tags

git-pull remote="origin" branch="main":
    git pull {{remote}} {{branch}}

git-push remote="origin" ref="HEAD":
    git push {{remote}} {{ref}}

git-tag version:
    git tag -a v{{version}} -m "Briefcast v{{version}}"

# ---------------------------------------------------------------------------
# Docker lifecycle
# ---------------------------------------------------------------------------

up:
    {{compose}} up -d

down:
    {{compose}} down

restart service="":
    {{compose}} restart {{service}}

rebuild service="":
    {{compose}} up -d --build {{service}}

ps:
    {{compose}} ps

logs service="" lines="200":
    {{compose}} logs -f --tail {{lines}} {{service}}

pull:
    {{compose}} pull

docker-build image="briefcast:local":
    docker build -t {{image}} .

docker-build-whisperx image="briefcast:with-whisperx":
    docker build --build-arg INSTALL_WHISPERX=true -t {{image}} .

shell service="briefcast":
    {{compose}} exec {{service}} sh

# DANGEROUS: stop stack and remove volumes.
clean:
    {{compose}} down -v --remove-orphans

# ---------------------------------------------------------------------------
# Testing and quality
# ---------------------------------------------------------------------------

lint:
    uv run ruff check .
    uv run ruff format --check .

format:
    uv run ruff format .
    uv run ruff check --fix .

typecheck:
    uv run mypy src
    npm --prefix frontend run build

test-go:
    go test ./...

test-python:
    uv run pytest

test-frontend:
    npm --prefix frontend run test

test-integration:
    go test ./service -run TestIntegrationFeedDownloadWhisperX

[unix]
test-whisperx-real:
    BRIEFCAST_WHISPERX_REAL=1 go test ./service -run TestWhisperXRealTranscription

[windows]
test-whisperx-real:
    $env:BRIEFCAST_WHISPERX_REAL='1'; go test ./service -run TestWhisperXRealTranscription

# Full local quality suite (lint + type checks + tests).
test-full:
    just lint
    just typecheck
    just test-go
    just test-integration
    just test-python
    just test-frontend

# CI quality gate for Python-only workflow.
ci-python:
    uv sync --locked --group dev
    uv run ruff check .
    uv run ruff format --check .
    uv run mypy src
    uv run pytest
    uv run pip-audit

# CI quality gate for full repository checks.
ci-full:
    uv sync --locked --group dev
    npm --prefix frontend ci
    just test-full
    uv run pip-audit

# ---------------------------------------------------------------------------
# Release pipeline (wrapper around RELEASE.ps1)
# ---------------------------------------------------------------------------

[windows]
release-help:
    powershell.exe -NoLogo -ExecutionPolicy Bypass -File .\RELEASE.ps1

[unix]
release-help:
    pwsh -NoLogo -File ./RELEASE.ps1

[windows]
release stage *args="":
    powershell.exe -NoLogo -ExecutionPolicy Bypass -File .\RELEASE.ps1 {{stage}} {{args}}

[unix]
release stage *args="":
    pwsh -NoLogo -File ./RELEASE.ps1 {{stage}} {{args}}

release-test *args="":
    just release test {{args}}

release-build *args="":
    just release build {{args}}

release-publish *args="":
    just release publish {{args}}

release-deploy *args="":
    just release deploy -EnvFile "{{env_file}}" -ComposeFile "{{compose_file}}" -BuildDir "{{build_dir}}" {{args}}

release-verify *args="":
    just release verify -EnvFile "{{env_file}}" -ComposeFile "{{compose_file}}" {{args}}

release-rollback *args="":
    just release rollback -EnvFile "{{env_file}}" -ComposeFile "{{compose_file}}" -BuildDir "{{build_dir}}" {{args}}

release-reset *args="":
    just release reset -EnvFile "{{env_file}}" -ComposeFile "{{compose_file}}" {{args}}

ship *args="":
    just release ship -EnvFile "{{env_file}}" -ComposeFile "{{compose_file}}" -BuildDir "{{build_dir}}" {{args}}
