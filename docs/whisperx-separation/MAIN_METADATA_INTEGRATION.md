# Main Markdown fixes integrated into the split worker

Prepared 2026-10-28. This merges main `aabbcb6` into the WhisperX separation
branch based on `ecbfb6b`. It carries the portable Markdown fixes into the app
and remote worker without deploying the split architecture.

## Integration behavior

- Transcript exports use concise YAML frontmatter followed by plain canonical
  transcript text. Summaries retain their Markdown body. Full provenance and
  exact input snapshots remain in the separate provenance directory, including
  in portable export bundles.
- Local and remote summaries share exact input, prompt, model and completion
  capture. Remote persistence retains its lease predicates and column updates,
  including when library metadata changes during an LLM call. SQL logging for
  those updates omits private transcript and prompt payloads.
- The worker image includes `podcast_provenance.py`. Pipeline fingerprints hash
  both Python modules, separated by a NUL byte; runtime manifests use the same
  calculation. Standalone custom pipelines without the helper retain their
  original single-script fingerprint.
- The worker result envelope retains the accepted request model for protocol
  validation. Actual ASR attribution uses the separate production metadata.
  A checkpoint-only execution does not acquire today's ASR model or time.
  The existing result normalizer retains the added provenance fields; the
  protocol version is unchanged.
- Worker syntax remains compatible with shipped Python 3.12. The system package
  lock uses LF on Windows checkouts so Linux package installation reads the exact
  pinned versions. No package versions or dependency declarations change.

## Validation and operational limits

Integration checks cover remote summary provenance and concurrent edits,
checkpoint-only model attribution, pipeline fingerprint invalidation, existing
workflow characterization, Markdown rendering/export, and worker packaging.
See the merge commit for the complete source change.

Commands and results from the integration working tree:

| Command | Result |
| --- | --- |
| `go test ./...` | Passed; service suite 234.030 seconds |
| `go test -race ./internal/transcription/... ./internal/whisperxworker/... ./service/...` (isolated Linux Go 1.26 image with Python and ffmpeg) | Passed; service suite 249.879 seconds |
| `uv run --no-sync --offline pytest` | 113 passed, 4 skipped |
| `uv run --no-sync --offline ruff check .` | Passed |
| `uv run --no-sync --offline ruff format --check .` | Passed |
| `uv run --no-sync --offline mypy src` | Passed |
| `npm run test:unit` (in `frontend`) | 12 passed |
| `npm run build` (in `frontend`) | Passed |
| `docker build --progress plain -t briefcast-app:main-merge-test --build-arg SOURCE_REVISION=main-integration .` | Passed |
| `pwsh -NoProfile -File scripts/CheckImage.ps1 -Image briefcast-app:main-merge-test -Kind app` | Passed |
| `docker build --progress plain -f docker/whisperx/Dockerfile --build-arg RUNTIME=cpu --build-arg WORKER_VERSION=1.0.1-main-merge --build-arg SOURCE_REVISION=main-integration -t briefcast-whisperx-cpu:main-merge-test .` | Passed |
| `pwsh -NoProfile -File scripts/CheckImage.ps1 -Image briefcast-whisperx-cpu:main-merge-test -Kind cpu` | Passed, including helper imports and combined pipeline fingerprint |
| `pwsh -NoProfile -File scripts/TestWorkerPython.ps1 -Image briefcast-whisperx-cpu:main-merge-test` | 95 passed in shipped Python 3.12; explicit worker lint/format passed |

Local build and test logs are retained under the ignored
`builds/whisperx-main-merge/` directory. Test image tags identify this integration
working tree; they are not commit-pinned release artifacts.

This changes the worker pipeline fingerprint, so rebuild the worker alongside
the app. Do not treat checkpoints from an older fingerprint as compatible.
Retain the prior worker image, state and recovery bundles for rollback; existing
compatibility checks must remain enabled.

Production remains on the main monolith image `briefcast:markdown-9820e36`.
The merge does not perform production cutover, alter the podcast library or
re-export existing files. The earlier strict CUDA quality comparison and other
production acceptance gates in [Stage G](STAGE_G.md),
[the crash follow-up](ACCEPTANCE_FOLLOWUP.md), and
[the resource follow-up](RESOURCE_ACCEPTANCE.md) remain outstanding.
