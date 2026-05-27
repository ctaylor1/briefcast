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