# AGENTS.md

## Purpose

This file contains persistent engineering instructions for coding agents working in this repository.

Follow these instructions for code review, refactoring, implementation, testing, and PR preparation. These rules are intentionally conservative: optimize for correctness, safety, maintainability, and reviewability over speed or cleverness.

## Operating mode

- Start by understanding the task, repository context, and relevant files before editing.
- For codebase-wide review, architecture review, security review, or refactoring assessment, begin in read-only assessment mode unless explicitly told to implement changes.
- Do not modify files, create commits, install dependencies, or run destructive commands during assessment-only work.
- When implementation is requested, make the smallest safe change that satisfies the task.
- If the task is ambiguous, make a reasonable assumption, state it, and proceed safely.
- Never expand scope opportunistically. Document follow-up work instead.

## Engineering priorities

Prioritize in this order:

1. Correctness, data integrity, security, and privacy.
2. Production reliability and graceful failure.
3. Behavior preservation and regression prevention.
4. Testability and maintainability.
5. Simplicity and readability.
6. Performance on proven or plausible hot paths.
7. Developer experience and documentation.

Style-only cleanup is lower priority than bugs, security risks, missing tests, confusing boundaries, or brittle architecture.

## Repository discovery

Before making material recommendations or changes, inspect the relevant project context:

- README and project documentation.
- Package/build/test configuration.
- CI/CD configuration.
- Lint, typecheck, format, and test setup.
- Existing architecture, module boundaries, and naming conventions.
- Existing tests and fixtures around the target code.
- Recent git status and changed files when relevant.
- Existing agent instructions in nested `AGENTS.md`, `CLAUDE.md`, `.github/copilot-instructions.md`, `.github/instructions/`, `.claude/rules/`, or similar files.

Prefer existing project commands over invented commands. If a command cannot be run, report why and say what should be run by a maintainer.

## Refactoring rules

- Preserve external behavior unless the task explicitly allows or requires a behavior change.
- Label any behavior-changing, API-changing, schema-changing, config-changing, auth-changing, or deployment-changing work clearly.
- Add characterization tests before refactoring unclear, legacy, or risky behavior.
- Keep refactors incremental and reviewable.
- Separate mechanical changes from functional changes.
- Separate test additions from risky production changes when possible.
- Prefer extracting, isolating, or simplifying existing behavior over introducing new abstractions.
- Do not introduce speculative abstractions for hypothetical future requirements.
- Do not add new dependencies without explicit approval.
- Do not change public APIs, database schemas, authentication/authorization behavior, deployment configuration, or data contracts without explicit approval.
- Prefer existing project patterns over new patterns unless the existing pattern is directly causing the problem.
- When touching security-, auth-, payment-, privacy-, compliance-, or data-integrity-sensitive code, explicitly call out the risk and verification plan.
- If a safer smaller refactor unlocks the requested larger refactor, do the smaller one first and report the recommended sequence.

## Code review rules

When reviewing code, prioritize findings as:

1. Required: correctness, security, data integrity, reliability, or broken contract.
2. Recommended: meaningful maintainability, testability, performance, or observability improvement.
3. Optional: cleanup that improves clarity but is not urgent.
4. Nit: minor style or wording.

For every material finding, include:

- File path and symbol.
- Line number when available.
- What was observed.
- Why it matters.
- Suggested fix.
- Confidence: Confirmed, Likely, or Hypothesis.
- Verification needed.

Avoid vague review comments such as “clean this up” or “improve error handling.” Be specific about where, why, how, and how to validate.

## Testing and verification

- Run the most focused relevant tests first.
- Add or update tests for behavior that changes or behavior that must be preserved.
- Prefer regression tests for bug fixes.
- Prefer characterization tests before refactoring behavior that is not already covered.
- Run lint, typecheck, build, or broader test suites when relevant and feasible.
- If tests are slow, unavailable, flaky, or blocked by environment limitations, report that clearly.
- Do not claim tests passed unless they were actually run.
- Include the exact commands run and summarize results.
- Include commands that should be run by a maintainer when local validation is incomplete.

## Security and privacy rules

- Never commit secrets, tokens, credentials, private keys, or sensitive environment values.
- Do not print secrets in logs, test output, errors, or summaries.
- Validate and sanitize untrusted input at trust boundaries.
- Preserve authorization checks and tenant/user isolation.
- Avoid unsafe deserialization, path traversal, injection-prone string construction, and overbroad permissions.
- Treat dependency, supply-chain, and configuration changes as security-relevant.
- When logging, prefer structured, contextual logs that avoid PII and secrets.

## Reliability and operations rules

- Preserve or improve timeout, retry, cancellation, and backoff behavior.
- Do not swallow errors silently.
- Avoid broad catch blocks unless they rethrow, log safely, or convert to a well-defined error type.
- Preserve transaction boundaries and resource cleanup.
- Consider partial failure, concurrency, idempotency, and rollback behavior.
- For production-facing changes, note observability expectations: logs, metrics, traces, alerts, or dashboards where applicable.

## Performance rules

- Do not optimize prematurely.
- Prioritize performance work on measured hot paths, obvious N+1 patterns, unbounded loops, blocking I/O, excessive allocations, or repeated expensive calls.
- Preserve correctness and readability while improving performance.
- Include benchmark, profiling, or before/after validation guidance when performance is the primary goal.

## Dependency rules

- Do not add runtime dependencies unless explicitly approved.
- Prefer standard library or existing dependencies when suitable.
- If a dependency change is necessary, explain why, identify alternatives considered, and note security/maintenance implications.
- Do not update lockfiles unless dependency changes are part of the requested task.
- Do not vendor external code without explicit approval.

## Implementation workflow

When asked to implement a change:

1. Restate the goal and constraints briefly.
2. Inspect relevant files and tests.
3. Identify the smallest safe change.
4. Add or update tests first when practical.
5. Make focused code changes.
6. Run focused validation.
7. Run broader validation if appropriate.
8. Summarize files changed, behavior impact, tests run, and remaining risks.

## PR and commit discipline

- Keep changes small and cohesive.
- Do not mix unrelated fixes in one PR.
- Do not include generated, vendored, build artifact, or lockfile changes unless required.
- Write summaries that help a reviewer understand risk quickly.
- Include rollback guidance for risky changes.
- Preserve existing formatting conventions and let project tooling handle formatting.

## Refactoring assessment output

When asked for a repository-wide refactoring assessment, produce:

1. Assessment scope and confidence.
2. Executive summary with top risks, quick wins, foundational refactors, biggest testing gap, biggest architectural risk, and recommended first PR.
3. Architecture map with major modules, data/control flow, cross-cutting concerns, and hotspots.
4. Critical findings with severity, confidence, evidence, suggested action, tests, and whether they block refactoring.
5. Prioritized refactor backlog with scores for risk, blast radius, frequency, confidence, payoff, and effort.
6. Suggested PR slicing plan.
7. Recommended first PR with exact implementation prompt.
8. Recommended durable repo instructions for future agents.
9. Open questions and assumptions.
10. Do-not-do list for risky or premature refactors.

Each backlog item should include:

- Category.
- Location.
- Evidence.
- Proposed target state.
- Step-by-step plan.
- Tests to add or update.
- Verification commands.
- Observability checks where relevant.
- Rollback plan.
- Copy/paste-ready implementation prompt.

## Communication style

- Be concise, specific, and evidence-based.
- State uncertainty explicitly.
- Distinguish observed facts from inferences.
- Do not invent line numbers, command results, metrics, or runtime behavior.
- Do not bury critical findings in prose.
- Use tables for prioritized lists when they improve scanability.

## Do not do

- Do not make broad rewrites without explicit approval.
- Do not change behavior silently.
- Do not remove tests to make a build pass.
- Do not weaken validation, authentication, authorization, or error handling.
- Do not ignore failing tests unless the failure is unrelated and documented.
- Do not introduce global mutable state unless explicitly justified.
- Do not add hidden background processes, telemetry, or network calls.
- Do not use destructive commands such as `rm -rf`, `git reset --hard`, or force-push unless explicitly instructed and the risk is acknowledged.

## Repository facts for agents

### Canonical commands

| Purpose | Command | Notes |
| --- | --- | --- |
| Build | `go build ./...` | |
| Static check | `go vet ./...` | |
| Go tests | `go test ./... -count=1` | About two minutes. The root package autoloads `.env`; run it in CI, or locally only with a scratch `.env` or in a copy without one. |
| Focused Go test | `go test ./service/ -run 'TestName' -count=1` | |
| Race detector | `go test -race ./internal/transcription/... ./internal/whisperxworker/... ./service/...` | Needs cgo. |
| Frontend | `npm --prefix frontend run test:unit` then `npm --prefix frontend run build` | Node 24. |
| Python | `just ci-python` | Needs `uv`; tooling targets Python 3.14, shipped scripts run on 3.12. |
| Packaging | `uv run pytest tests/test_packaging.py` | Needs pwsh; the compose rendering tests need Docker. |

- On Windows, Go is at `C:\Program Files\Go\bin` and may not be on the Git Bash `PATH`.
- With `CGO_ENABLED=0`, Go tests fall back to the pure-Go SQLite driver. CI and
  production use the cgo driver. Say which one you ran.
- `gofmt -l` is noisy on Windows checkouts (CRLF). Do not reformat files you did not edit.

### Data-safety rules

- Never call `os.RemoveAll` or `os.Remove` on a path built from a podcast or episode
  title without a test that proves no other podcast's files are affected.
- Treat only `fs.ErrNotExist` as "file is gone". Any other stat error means "unknown":
  do not change database state because of it.
- Write episode state with `db.UpdatePodcastItemColumns`, naming only the columns the code
  owns. The whole-row `db.UpdatePodcastItem` was removed: it overwrote concurrent changes and
  could recreate a deleted episode. Tests set up rows with `savePodcastItemFixture`.
- Start background goroutines with `service.SafeGo`. Clear any "running" flag in a
  `defer`, because a contained panic skips ordinary cleanup code.
- New state-changing routes use POST, PATCH or DELETE. Do not add GET routes with side
  effects.
- Every ID placed in a frontend request path goes through `encodeURIComponent`.

### Change-control rules

- Compose defaults, `Dockerfile`, migrations, public routes and anything under `scripts/`
  that deploys or deletes are approval-required changes. Label them in the PR title.
- One concern per PR. Mechanical moves, behaviour changes and test-only changes are
  separate PRs.
- A migration that deletes or dedupes rows needs a verified backup step and a note on
  how to restore.

### Do not touch without an explicit request

- `internal/sanitize` (vendored fork).
- `internal/whisperxworker` journal, checkpoint and guardian code (fail-closed by design).
- `RELEASE_RESET.ps1`, `RELEASE_RUN.ps1`, `RELEASE_BUILD.ps1` (legacy and destructive;
  never run them).
- `.env`, `secrets/`, `config/`, `assets/`, `logs/`, `worker-state/`, `worker-cache/`.

### Local conventions to preserve

- Handlers bind into small request structs and answer with `gin.H{"error": ...}`.
- Logging is zap with key-value pairs and IDs (`podcast_id`, `podcast_item_id`); never
  log transcripts, prompts, tokens or database URLs.
- All outbound HTTP goes through `validateOutboundURL` and `outboundHTTPTransport`.
- Tests use `t.TempDir()`, `t.Setenv`, `httptest` servers and temp-file SQLite.
- Raw-SQL migrations go in `db/migrations.go` with a dated, unique name and must be
  safe to run on both SQLite and Postgres.
