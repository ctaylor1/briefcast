# Stage H — operations and migration inventory

Prepared 2026-10-03. This phase documents the implemented split; it does not deploy it, clear Stage G acceptance failures, move the database/host, redesign scheduled backups or change runtime behavior.

## Source and changes

Branch `feat/whisperx-service`; starting HEAD `bfea2562d95462b2441da8729f3e8e95e00a211c`, clean. The Stage H documentation commit follows Stage G commits `1970205`, `491ddbe` and `bfea256`. The final SHA is available in `git log` and the private Stage H source-bundle manifest. The branch remains local/unpushed; no PR or registry publication was performed.

| File | Change |
| --- | --- |
| [OPERATIONS.md](OPERATIONS.md) | Implemented API/version/profile matrix, fresh isolated CPU/CUDA setup, explicit Compose/env/project selection, configuration ownership, health/model limits, drain/resume, replacement, both rollback paths, cutover/acceptance commands |
| [migration-inventory.md](migration-inventory.md) | Redacted DB/config/media/state/cache/HF/Torch/NLTK/token/identity inventory, permissions, immutable local image IDs, network assumptions, backup identifiers and unresolved destination decisions |
| [Backup and restore](../backup-and-restore.md) | Actual scheduled fixed-filename SQLite archive limits, PostgreSQL dump/role requirements, consistent SQLite snapshot guidance, implemented isolated restore and full worker-state recovery |
| [README](../../README.md) | Removed destructive migration cleanup/broad staging/push recipe; linked safe backup/operations procedures; clarified split publication/gates, HTTP configuration, shared media mounts and required worker recovery directory |
| [CONTRACT.md](CONTRACT.md) | Replaced stale “implementation does not exist” status, named shipped profiles/stable identity, clarified model readiness and internal envelope/example limits; no wire/schema change |
| [Worker packaging](../../docker/whisperx/README.md), [SPEC.md](SPEC.md) | Linked current runbooks/evidence and updated recovery/status guidance while preserving design/history and engine locks |

No production/API/schema/auth/configuration changes, dependencies, locks, build artifacts or tests were added. README examples now match the implemented required replacement arguments. Existing source tests were used for verification; no documentation-mirroring test suite was introduced.

## Verification performed

```powershell
$env:UV_NO_SYNC='1'
uv run pytest tests/test_packaging.py
just --list
pwsh -NoProfile -File RELEASE.ps1
uv run python scripts/briefcast_recovery.py --help
uv run python scripts/whisperx_acceptance.py --help
$env:CGO_ENABLED='0'
go run ./cmd/transcription-rollback --help
go run ./cmd/whisperx-worker serve --help
git diff --check
```

Packaging: **18 passed**, covering mode rendering, env precedence, targeted drain/snapshot/deploy/rollback and cutover gates with synthetic isolated/mocked configuration. The command list, release/recovery/acceptance/rollback help and documented flags were verified without starting services or accessing a database. Worker `serve --help` displays the correct defaults/flags but exits 1 because its existing `flag.ContinueOnError` path reports `flag: help requested`; this was observed, not reported as a passing exit status or changed in this documentation phase.

An additional `uv run python -` read-only probe invoked `scripts/Compose.ps1 -EnvFile <temporary-synthetic-env> -Runtime disabled|cpu|cuda -ProjectName briefcast-docs-check config --quiet`: all three rendered successfully, without printing effective configuration or starting containers. Standard-library checks verified **70 local links/anchors across eight documents and parsed 11 PowerShell blocks** without executing their commands. The link probe explicitly used UTF-8 after its initial Windows default-encoding error. README no longer contains `git clean -fdX`. `git diff --check` passed. Repository commit hooks provide the existing Ruff format/check, mypy and pytest gate.

Full Go/race/frontend/real CPU/CUDA/DB restore suites were not repeated for documentation-only changes. Their exact source/runtime, commands and results remain in [Stage F](STAGE_F.md) and [Stage G](STAGE_G.md); they are not relabeled as Stage H test runs. No dependency installation, source image rebuild, GitHub workflow or destructive recovery operation was performed.

## Deployment and recovery status

The observed production image remained `sha256:a7542b94500a5ac7e00271451ef2a51db233bd5e2bec92c615b240302b06d052`, started at `2026-10-02T20:40:04.862330437Z`; running-container inventory contained only `briefcast`. Runtime app/CPU/CUDA images are unchanged Stage F artifacts sourced from `7d8545f6c0a63ecbb182f318c5deb0b4b63b02bb`, with IDs/checksums in the inventory and Stage F report. A separately verified Stage H source bundle is a documentation/source checkpoint and does not replace the Stage G private cutover record or final database/media backup.

**Production acceptance remains incomplete:** strict GPU word-score comparison failed; final coherent production database/config/media protection and representative media restore, remaining matrix cases, canary/summary-provider validation and 24-hour observation remain outstanding. Optional gated diarization is unverified and the known baseline CUDA TorchCodec decoder limitation is preserved. Single-app/single-worker operation and same-host read-only worker media remain the supported scope; host migration and general scheduled-backup redesign are deferred.

Next operational work is resolving those documented gates and reviewing the private concrete cutover package. Stage H documentation completion is not authorization or evidence for production rollout.
