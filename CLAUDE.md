@AGENTS.md

# CLAUDE.md

These Claude Code instructions extend the shared `AGENTS.md` instructions above. Treat `AGENTS.md` as the canonical cross-agent engineering contract for this repository.

## Claude Code operating mode

- Use Plan mode for codebase-wide reviews, architecture reviews, refactoring assessments, risky changes, and any task touching security, auth, payments, privacy, data integrity, database schema, deployment configuration, or public APIs.
- In Plan mode, produce the plan first and wait for approval before editing.
- In normal implementation mode, request permission before running commands that install packages, modify generated files, change environment configuration, run migrations, or perform destructive filesystem/git operations.
- Prefer focused edits with clear diffs over broad rewrites.
- Keep explanations short unless the task asks for a deep review.

## Memory and instruction hygiene

- Follow these instructions in every session unless more specific project instructions override them.
- If these instructions conflict with nested `CLAUDE.md`, `.claude/rules/`, or maintainer-provided task instructions, prefer the more specific instruction and state the conflict.
- Do not add noisy or one-off details to persistent memory.
- Add durable lessons to `CLAUDE.md` only when they would help in many future sessions.
- Use `CLAUDE.local.md` for private personal preferences that should not be committed.

## Claude-specific review behavior

When reviewing code or refactors:

- Lead with correctness, security, reliability, and test gaps.
- Avoid excessive nits.
- Label findings as Required, Recommended, Optional, or Nit.
- For claims about behavior, cite concrete source evidence.
- When uncertain, ask for a test or maintainer confirmation rather than presenting speculation as fact.

## Claude-specific implementation behavior

Before editing:

1. Identify the smallest safe change.
2. Identify files likely to change.
3. Identify tests that should protect the change.
4. Identify commands to run.
5. Identify rollback strategy for risky changes.

After editing, report:

- Files changed.
- Behavior preserved or changed.
- Tests added or updated.
- Commands run and results.
- Commands not run and why.
- Remaining risks or follow-ups.

## Tool and command caution

- Use read-only inspection commands before edit commands.
- Prefer project scripts from package/build configuration.
- Do not install dependencies unless explicitly approved.
- Do not run migrations, seed scripts, deploy scripts, or destructive git commands unless explicitly approved.
- Do not use auto-accept mode for risky tasks.