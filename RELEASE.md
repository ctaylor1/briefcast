# Release Pipeline

A single PowerShell script (`RELEASE.ps1`) driven by a JSON config file (`release.config.json`).
Replaces the previous `RELEASE_BUILD.ps1`, `RELEASE_RUN.ps1`, `RELEASE_RESET.ps1`, and `build_tar.ps1` scripts.

## Quick Start

```powershell
# Show all stages, options, and examples
.\RELEASE.ps1

# Run all quality checks
.\RELEASE.ps1 test

# Full release pipeline (patch bump)
.\RELEASE.ps1 ship -Bump patch

# Build images only (no commit/deploy)
.\RELEASE.ps1 build

# Deploy the latest build
.\RELEASE.ps1 deploy

# Roll back to previous deployment
.\RELEASE.ps1 rollback
```

## Stages

| Stage       | Description |
|-------------|-------------|
| `test`      | Runs all quality checks defined in `release.config.json` (Go tests, frontend build, Python lint/typecheck/tests). |
| `build`     | Builds Docker images, runs smoke tests, exports `.tar` artifacts to `builds/`, writes `checksums.sha256`. |
| `publish`   | Bumps version in source files, commits all changes, creates git tag (`vX.Y.Z`), pushes to remote. |
| `deploy`    | Loads `.tar` artifacts into Docker, saves rollback state, runs `docker compose down/up`, then verifies health. |
| `verify`    | Checks all compose services are running via `docker compose ps`, polls HTTP health endpoints with a configurable timeout. |
| `rollback`  | Reads `builds/rollback-state.json`, re-tags previous images, restarts the compose stack, verifies. |
| `reset`     | **Destructive** — tears down stack, removes compose volumes, clears data directories, rebuilds from scratch. Prompts for confirmation unless `-Force`. |
| `ship`      | Full pipeline in safe order: test → bump version → build → commit+tag+push → deploy → verify. Tests pass before anything is committed; images build before git push; deploy only runs if `.env` is available. |

## Options

| Option            | Description |
|-------------------|-------------|
| `-Bump <type>`    | Version bump: `major`, `minor`, or `patch`. Required for `publish` and `ship`. |
| `-Version <X.Y.Z>`| Explicit version. Alternative to `-Bump`. |
| `-EnvFile <path>` | Path to `.env` file. Default: from config (`deploy.env_file`). |
| `-ComposeFile <path>` | Path to compose file. Default: from config (`deploy.compose_file`). |
| `-BuildDir <path>`| Artifact output directory. Default: `builds/`. |
| `-Remote <name>`  | Git remote name. Default: `origin`. |
| `-SkipTests`      | Skip test stage in `ship` pipeline. |
| `-SkipSmoke`      | Skip Docker smoke tests during `build`. |
| `-SkipVerify`     | Skip verify stage after `deploy`. |
| `-NoCache`        | Build Docker images with `--no-cache`. |
| `-NoPush`         | Skip git push in `publish`/`ship`. |
| `-Force`          | Skip confirmation prompts (e.g. `reset`). |

## Config File Schema (`release.config.json`)

```jsonc
{
  // Compose project name (used as -p flag and default container prefix)
  "project": "briefcast",

  // Version management
  "version": {
    "file": "pyproject.toml",     // Primary version file (auto-detected format)
    "sync": [                     // Additional files to keep in sync
      "frontend/package.json"
    ]
  },

  // Quality checks run during 'test' stage
  "test": [
    {
      "name": "Go tests",        // Display name
      "run": "go test ./...",     // Shell command
      "dir": "."                  // Working directory (relative to project root)
    }
  ],

  // Docker images to build
  "images": [
    {
      "name": "briefcast",                    // Image name
      "dockerfile": "Dockerfile",             // Dockerfile path
      "context": ".",                          // Build context
      "build_args": {                          // --build-arg flags
        "APP_VERSION": "{version}"             // {version} placeholder replaced at build time
      },
      "smoke_test": "docker run --rm {image} ./app --version",  // {image} placeholder
      "services": ["briefcast"]               // Compose services using this image
    }
  ],

  // Deployment configuration
  "deploy": {
    "compose_file": "docker-compose.yml",
    "env_file": ".env",
    "infra_services": ["postgres"],           // Services that are infrastructure (not app)
    "health_checks": [
      {
        "name": "Briefcast HTTP",
        "url": "http://localhost:{port}/",    // {port} resolved from env
        "port_env": "HOST_PORT",              // Env var to read port from
        "port_default": 8080,
        "expect_status": 200
      }
    ],
    "startup_timeout_seconds": 60,
    "data_dirs": [                            // Directories cleared during reset
      { "env": "HOST_CONFIG_DIR", "default": "./config" },
      { "env": "HOST_ASSETS_DIR", "default": "./assets" },
      { "env": "HOST_LOGS_DIR",   "default": "./logs" }
    ],
    "env_overrides": {                        // Env vars set during compose operations
      "BRIEFCAST_IMAGE": "{image}",
      "BRIEFCAST_CONTAINER_NAME": "{container_name}"
    },
    "container_name_env": "BRIEFCAST_CONTAINER_NAME",
    "container_name_default": "briefcast",
    "whisperx_env_file_key": "WHISPERX_ENV_FILE",
    "whisperx_env_file_default": ".env.whisperx"
  },

  // Git configuration
  "git": {
    "exclude_paths": [                        // Paths excluded from git add
      ".claude/worktrees",
      ".claude/worktrees/**"
    ]
  }
}
```

## Version File Support

The script auto-detects version format by filename:

| File             | Pattern                           | Example                    |
|------------------|-----------------------------------|----------------------------|
| `pyproject.toml` | `version = "X.Y.Z"`              | `version = "1.3.3"`       |
| `package.json`   | `"version": "X.Y.Z"`             | `"version": "1.3.3"`      |
| `Cargo.toml`     | `version = "X.Y.Z"`              | `version = "0.1.0"`       |
| `VERSION`        | Plain `X.Y.Z` (entire content)   | `1.3.3`                   |

The `version.file` field specifies the primary source of truth. Additional files listed in `version.sync` are updated to match during publish/ship.

## Build Artifacts

All artifacts are written to `builds/` (configurable via `-BuildDir`):

```
builds/
├── briefcast_v1.3.3.tar      # Docker image export
├── checksums.sha256           # SHA-256 checksums for all .tar files
└── rollback-state.json        # Previous deployment state (auto-managed)
```

## Rollback State

`rollback-state.json` is saved automatically before each deploy:

```json
{
  "timestamp": "2026-03-07T12:00:00.0000000-05:00",
  "version": "1.3.3",
  "images": [
    {
      "name": "briefcast",
      "tag": "briefcast:latest",
      "id": "sha256:abc123...",
      "services": ["briefcast"]
    }
  ]
}
```

## Examples

```powershell
# Patch release (most common)
.\RELEASE.ps1 ship -Bump patch

# Minor release, skip tests (you already ran them)
.\RELEASE.ps1 ship -Bump minor -SkipTests

# Build only, don't commit or deploy
.\RELEASE.ps1 build -Version 2.0.0

# Publish version bump + tag without building
.\RELEASE.ps1 publish -Bump minor

# Publish but don't push (review first)
.\RELEASE.ps1 publish -Bump patch -NoPush

# Deploy with a custom env file
.\RELEASE.ps1 deploy -EnvFile ./production.env

# Force rebuild with no Docker cache
.\RELEASE.ps1 build -NoCache

# Check if deployment is healthy
.\RELEASE.ps1 verify

# Emergency rollback
.\RELEASE.ps1 rollback

# Full factory reset (prompts for confirmation)
.\RELEASE.ps1 reset

# Full factory reset (no prompt, for CI)
.\RELEASE.ps1 reset -Force
```

## Migration from Previous Scripts

| Old Script          | New Equivalent |
|---------------------|----------------|
| `RELEASE_BUILD.ps1 -Patch` | `.\RELEASE.ps1 ship -Bump patch` |
| `RELEASE_BUILD.ps1 -Minor` | `.\RELEASE.ps1 ship -Bump minor` |
| `RELEASE_BUILD.ps1 -Major -SkipTests` | `.\RELEASE.ps1 ship -Bump major -SkipTests` |
| `RELEASE_RUN.ps1 -Version 1.3.3` | `.\RELEASE.ps1 deploy -Version 1.3.3` |
| `RELEASE_RESET.ps1 -Version 1.3.3` | `.\RELEASE.ps1 reset` |
| `build_tar.ps1 -Version 1.3.3` | `.\RELEASE.ps1 build -Version 1.3.3` |

## Portability

Only `release.config.json` changes between projects. The `RELEASE.ps1` script is fully generic:

- Version file format is auto-detected
- All test commands, image definitions, and deploy config come from the JSON
- Docker Compose v1 and v2 are both supported
- Works on Windows, Linux (pwsh), and macOS (pwsh)

---

## Prompt for AI Reuse

<details>
<summary>Expand to see the prompt used to generate this pipeline</summary>

```
Create a release pipeline for this project using a single PowerShell script (RELEASE.ps1)
and a JSON config file (release.config.json), plus a RELEASE.md documenting it.

Requirements:
0. Review all existing release code and include its functionality in the new workflow.

1. RELEASE.ps1 is a single-file, project-agnostic release script with these stages:
   - test: runs all quality checks (lint, typecheck, unit tests) defined in release.config.json
   - build: builds Docker images, runs smoke tests, exports .tar artifacts to builds/ with
     version in the filename, writes a checksums file
   - publish: bumps version (supports -Bump major|minor|patch or -Version X.Y.Z), commits
     all changes, creates a git tag (vX.Y.Z), pushes to remote
   - deploy: loads .tar artifacts into Docker, tags images for compose services, saves rollback
     state, runs docker compose down/up, then verifies
   - verify: checks all compose services are running+healthy via docker compose ps, polls HTTP
     health endpoints with a timeout
   - rollback: reads rollback-state.json from builds/, re-tags previous images, restarts stack,
     verifies
   - reset: destructive — tears down stack, removes compose volumes, clears data directories
     listed in config, rebuilds from scratch (prompts for confirmation unless -Force)
   - ship: full pipeline — test, bump version in file, build, git commit+tag+push, deploy,
     verify. Safe order: tests pass before anything is committed; images build before git push;
     deploy only happens if env file is available

2. release.config.json contains ALL project-specific configuration:
   - project: compose project name
   - version.file: path to the file containing the version (auto-detects pyproject.toml,
     package.json, Cargo.toml, or plain VERSION files)
   - test[]: array of {name, run, dir} objects
   - images[]: array of {name, dockerfile, context, smoke_test, services}
   - deploy: {infra_services, health_checks, startup_timeout_seconds, data_dirs}

3. RELEASE.ps1 parameters: -Stage (positional), -Bump, -Version, -EnvFile, -ComposeFile,
   -BuildDir, -Remote, -SkipTests, -SkipSmoke, -SkipVerify, -NoCache, -NoPush, -Force.
   Running with no arguments shows help text.

4. Key design principles:
   - All helpers defined exactly once
   - Version read/write auto-detects format by filename
   - Deploy resolves compose project name from config
   - Rollback state is a simple JSON with compose tags and image IDs
   - The script is fully portable — only release.config.json changes between projects

5. RELEASE.md documents all stages, options, config schema, version file support, examples,
   and includes this prompt for reuse.
```

</details>
