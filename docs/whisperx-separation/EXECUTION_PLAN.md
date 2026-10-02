# WhisperX separation execution plan

This is the implementation and operations handoff for [the service specification](SPEC.md). Execute on the current host first. Host migration is explicitly deferred. The commands below are instructions for future execution; none of the snapshots, commits, image builds or deployments described here were performed while writing this plan.

## 1 Execution rules and success criteria

Read root and nested agent instructions before modifying code. Treat this plan as a bounded implementation project, not permission to rewrite unrelated code. Do not add external dependencies, vendor third-party code, change public application APIs, or alter the production database schema without a separate explicit decision. The proposed private worker API, worker-local database, internal checkpoint envelope and Docker deployment changes are intentional parts of this design.

Do not start by upgrading WhisperX. First separate the installed engine with equivalent settings. No production acceptance with only stub tests. No production cutover before the recovery package and rollback rehearsal exist. No host migration, database relocation, production data cleanup, forced Git operations, broad `git add .`, `git clean -fdX`, or `docker compose down -v`.

Complete when all specification requirements and the acceptance matrix below pass, the split services have run through the observation window, independent worker replacement and rollback have been demonstrated, source commits and image digests are recorded, and a future-migration inventory is delivered. A documentation-only handoff or passing unit tests is not implementation completion.

## 2 Source and branch protection

At assessment time the source tree was clean at `62ecec0` on `main`, one commit ahead of the locally recorded remote tracking branch. The new files under `docs/whisperx-separation/` are expected uncommitted documentation from this planning task. Recheck status; another person may have changed the repository since then.

1. Inspect `git status --short --branch`, `git diff --stat`, staged changes and recent commits. Never overwrite, stash or absorb unrelated work automatically. If unrelated work appears, use an isolated checkout/worktree from the agreed source baseline, or finish reviewing the relevant work before continuing.
2. Record the complete source SHA. Make an annotated source-baseline tag with a unique date/run suffix. This tag means source checkpoint, not proof that the commit produced the running image.
3. Create `feat/whisperx-service` from that baseline. Use sequential, reviewable commits; an isolated worktree is optional, not required with a clean tree. Never launch a worktree's default Compose stack against production paths.
4. Explicitly commit these two handoff documents as the first branch commit. Preserve the already-existing OpenRouter/summary changes; do not amend their commit or mix them into this feature.
5. Back up the source with `git bundle create <private-path>/source.bundle --all` and verify it with `git bundle verify`. A bundle captures committed history, not env files or uncommitted changes.
6. Push the feature branch and source tag to the configured repository when remote publication is authorized for implementation. Verify the destination before pushing. If credentials or access are unavailable, keep local commits/bundle and report publication incomplete; do not force-push or merge to main prematurely.

Illustrative commands after inspection, using unused branch/tag names:

```powershell
git rev-parse HEAD
git tag -a baseline/whisperx-split-20261002 -m "Source checkpoint before WhisperX service split"
git switch -c feat/whisperx-service
git add -- docs/whisperx-separation/SPEC.md docs/whisperx-separation/EXECUTION_PLAN.md
git diff --cached --check
git commit -m "docs: specify WhisperX service separation and rollout"
```

If names already exist, inspect them and select a new suffix; do not move existing tags. Check every command exit code. Baseline tests must identify the exact source SHA and must not be represented as tests of the deployed binary unless they actually used that image.

## 3 Recovery package before implementation

Create a restricted backup directory outside the repository and build context, for example `C:\DevRepo\Backups\briefcast\whisperx-split-<timestamp>`. Verify free space for the approximately 13.3 GB image, database, model caches, media backup/snapshot and rehearsal copies. Do not put sensitive recovery files under tracked docs or an image build context.

The recovery package has five separate components:

| Component | Required contents and verification |
| --- | --- |
| Source | Full SHA, source tag, Git bundle, clean/dirty inventory and reviewed source documentation |
| Runtime image | Saved exact running image by immutable ID, independent snapshot tag, archive checksum and load/smoke-test evidence |
| Configuration | Actual Compose files used, env files, secret-file locations, effective settings and image references; raw copies private, redacted manifest safe for reports |
| Application state | PostgreSQL dump and restoration evidence; config; consistent media/filesystem backup or existing verified snapshot; optional logs |
| Runtime additions | Full `docker diff` inventory, model/cache paths, selected runtime caches not in mounted state, package/version and runtime fingerprints |

Do not dump `docker inspect` environment arrays, `docker compose config` output, connection URLs or `pip freeze` with private URLs into a public log. Capture raw sensitive artifacts only in restricted storage. Track only a redacted manifest in Git.

### Save the exact image

Use the container's `.Image` ID rather than trusting the mutable `briefcast:1.9.4` tag. A representative PowerShell sequence is below; implement it as an error-checking script with explicit destination/space checks and secure permissions. `<private-backup-directory>` is a placeholder, not a runnable path.

```powershell
$snapshotDir = '<private-backup-directory>'
$snapshotImage = 'briefcast:pre-whisperx-split-20261002'
$runningImageId = (docker inspect --format '{{.Image}}' briefcast).Trim()
if ($LASTEXITCODE -ne 0 -or -not $runningImageId.StartsWith('sha256:')) {
    throw 'Cannot resolve the running image'
}
docker image tag $runningImageId $snapshotImage
if ($LASTEXITCODE -ne 0) { throw 'Image tag failed' }
$imageArchive = Join-Path $snapshotDir 'briefcast-running-image.tar'
docker image save --output $imageArchive $snapshotImage
if ($LASTEXITCODE -ne 0) { throw 'Image archive failed' }
Get-FileHash -LiteralPath $imageArchive -Algorithm SHA256
docker image load --input $imageArchive
if ($LASTEXITCODE -ne 0) { throw 'Image archive load failed' }
docker image inspect $snapshotImage --format '{{.Id}}'
docker run --rm --network none --entrypoint /api/app $snapshotImage --version
if ($LASTEXITCODE -ne 0) { throw 'Archived image smoke test failed' }
```

Compare the reloaded ID to the recorded ID and save the checksum. This smoke test must use `--version`, which exits before database initialization; do not accidentally start another normal app connected to production. Loading on the same Docker daemon verifies archive readability but is not a full restore rehearsal. Later run the archived image against a separate restored test database and copied assets.

[Docker image save](https://docs.docker.com/reference/cli/docker/image/save/) archives image layers and tags. It does not capture bind mounts, PostgreSQL, live process memory, or modifications in the container writable layer. A `docker commit` is not the normal backup strategy: it excludes mounted data, can pause the container, and can carry sensitive runtime settings into an image. See [Docker container commit](https://docs.docker.com/reference/cli/docker/container/commit/). Inspect the diff and preserve needed caches/files separately; unexpected installed-code changes require a reviewed private capture/reproduction plan before relying on rollback.

### Protect PostgreSQL and files

The active PostgreSQL connection currently targets `host.docker.internal`. Copying `config/briefcast.db` is insufficient. Determine the actual PostgreSQL server version and backup method without exposing credentials. Use a compatible `pg_dump` client, a secure password file/secret mechanism, and custom-format output through `--file`, avoiding binary shell redirection surprises on Windows. Include relevant role/ownership requirements in the manifest; a database dump alone does not include global roles.

Restore the dump into a new isolated database and verify tables, episode/subscription counts, settings, transcript and summary counts, and representative records. Never use the production database as a restore test. PostgreSQL supports consistent logical dumps and custom-format restoration with `pg_restore`; [official backup guidance](https://www.postgresql.org/docs/current/backup-dump.html).

For the initial safety baseline, record the dump's time and the media snapshot's time. Before production cutover, stop app writes and take a final consistent database/files snapshot or final incremental media sync. The current application does not provide the future worker's drain endpoint: wait for active work where practical, then use a planned maintenance stop and retain existing sidecars. A backup taken while downloads change files is not automatically a coherent restore point.

Copy `.env`, `.env.whisperx`, the actual Compose files and `/config` privately. Protect `E:\Podcasts`, including audio, artwork, exports and `.briefcast.whisperx.resume.json` sidecars. Export required caches outside `/config`, especially current NLTK data. Preserve permissions, filenames and timestamps as applicable; document whether a cache can safely be regenerated. Keep the baseline image/configuration package and backup off the only disk holding production data where available.

Gate: do not start production-affecting work until the exact image archive is verified, the database restore succeeds, assets/config protection is documented, and the baseline can be started in isolation. Baseline code tests may proceed without stopping production.

## 4 Baseline tests and isolated rehearsal environment

Create a separate Compose project such as `briefcast-whisperx-test`, unique container names, different host port, separate application database, separate worker state/cache, and copied fixtures. Remove fixed container names from test definitions or override them explicitly. Use a test env file with no production DB URL, API keys or asset directories. The harness must reject configurations that resolve to production mounts/database endpoints.

A second app instance must never share the production database. `main.go` calls `UnlockAllJobs` and starts scheduled jobs automatically. Prefer a small synthetic database for automated tests and a separately restored real database for manual acceptance; use copied media. Disable LLM requests or point them to a stub during tests. Use the real LLM only in a separately authorized final verification if necessary.

Before changing code, run the existing relevant source tests in this isolated environment:

```text
go test ./service -run 'Test(LoadWhisperX|ResolveWhisperX|RunWhisperX|WhisperX|ApplyWhisperX|ReadWhisperX|ScheduleTranscript|StartJobLock|CollectWhisperX|Characterize_Transcript|IntegrationFeedDownloadWhisperX)' -count=1
go test ./...
npm --prefix frontend run test
uv run pytest
uv run ruff check .
uv run ruff format --check .
uv run mypy src
```

These are existing commands, not claims they passed during planning. Inspect skipped tests, interpreter availability and CGO prerequisites. Do not suppress failures. Record unrelated pre-existing failures with evidence and fix only what blocks this scope. Avoid package installation unless already authorized; never run a test configured with a live `BRIEFCAST_INTEGRATION_DB_URL`.

The current real test is opt-in (`just test-whisperx-real`), defaults to short synthetic audio, and is insufficient as the sole quality check. Add a private or redistributable speech corpus: a normal episode excerpt, multi-chunk speech longer than 240 seconds, two speakers, silence, an unusual supported audio format, and malformed audio. Use fixed model/options and the exact baseline image; remove timestamps/provenance before exact structural comparisons. Record timing/memory measurements without claiming benchmarks from unrun tests. Avoid running candidate GPU workloads concurrently with production transcription; schedule an idle period or a maintenance window.

## 5 Implementation stages and commits

Stages are ordered. Contract and fixture agreement precedes client/server work. A coding agent may use parallel work only when the execution environment explicitly authorizes it; this plan does not require multiple agents. Each stage ends with focused tests, `git diff --check`, a review of changed files, and a commit containing only that stage's work.

| Stage | Suggested commit | Deliverable and exit gate |
| --- | --- | --- |
| A | `docs: record transcription worker architecture and contract` | Refresh wrapper ADR; freeze OpenAPI, fixtures, compatibility rules and baseline manifest. Confirm the specified wrapper decision or document concrete contrary evidence |
| B | `test: characterize transcription execution and persistence` | Tests for local pipeline, fallback fields, progress/resume, failures and downstream effects; no production behavior change |
| C | `refactor: isolate transcription backend execution` | Small backend seam and shared types; local mode remains default in transitional builds; baseline tests unchanged |
| D | `feat: add durable WhisperX worker service` | Worker API, state store, subprocess supervisor, checkpoints, authentication, bounded resources, recovery, cancellation and maintenance commands |
| E | `feat: connect Briefcast to remote transcription jobs` | HTTP adapter, persisted request identity, safe retries/error classification, conditional DB writes, ACK reconciliation and startup/outage behavior |
| F | `build: package independent app and WhisperX images` | Lean app; pinned baseline CPU/CUDA worker locks/images; explicit Compose layers; env and release tooling updates |
| G | `test: verify split transcription recovery and upgrades` | Container acceptance suite, real-audio report, replacement/rollback demonstration, safe backup/restore and rollout tools |
| H | `docs: document independent WhisperX operations and migration inventory` | Operator runbook, tested commands, compatibility matrix, release evidence and future-host inventory |

Splitting a large stage into multiple cohesive commits is encouraged. Do not combine the first real WhisperX version upgrade with stages C through F.

### Stage A implementation prompt

Read both files in `docs/whisperx-separation`, the current agent instructions and transcription/release code. Reconfirm the running image and source baseline without printing secrets. Record dated, commit-pinned evidence for the four wrapper candidates and select the Briefcast worker unless an external candidate demonstrably meets the requirements. Write `docs/adr/whisperx-worker.md`, an OpenAPI v1 contract and request/status/result/error fixtures. Specify every existing configuration field, limits, idempotency, state transitions and checkpoint compatibility. Do not modify production code or deploy anything in this stage.

### Stage B implementation prompt

Add characterization tests around `RunWhisperXWithProgress`, `TranscribePendingEpisodes`, resume sidecars, best-effort alignment/diarization metadata, canonical transcript generation, retry bookkeeping, transcript persistence, exports and summary invocation. Cover valid and invalid checkpoints and local subprocess errors. Record known behavior defects separately from preserved behavior. Use isolated temporary state and stub external endpoints. Run the focused existing suite and added tests; production changes belong in later commits.

### Stage C implementation prompt

Introduce one small transcription backend interface and shared contract types without changing local transcription behavior. Move only execution concerns out of orchestration. Keep application DB ownership, existing queue selection and downstream work in service code. Keep the Python script as the single pipeline implementation during transition. Ensure backend-specific preflight means HTTP mode will not require local WhisperX/Python/script resolution. Run characterization and integration tests, review dependency direction and commit the extraction separately from HTTP behavior.

### Stage D implementation prompt

Implement `cmd/whisperx-worker` and its internal packages using Go HTTP, existing SQLite dependencies and the current Python script. Implement the specification's authenticated async job contract, atomic acceptance, persisted status/results, duplicate handling, bounded queue and output, read-only rooted media validation, state ownership lock, process-group cleanup, checkpoints, maintenance drain/resume and idempotent ACK. Fail safely on storage errors. Move HTTP-job checkpoint cleanup after durable result persistence. Add deterministic crash-boundary and malicious-path tests using a stub child; then prove a real baseline transcription in the worker image. No application database access is allowed.

### Stage E implementation prompt

Implement the app HTTP backend with durable submission identity in the existing checkpoint column, backward-compatible legacy checkpoint parsing, purpose-specific private-network transport, bounded requests and typed errors. Reconnect to accepted jobs across HTTP failures/restarts. Preserve local audio on remote infrastructure failures. Commit only transcript-owned columns after generation/input validation; do not persist stale full episode records. Do not ACK/export/summarize if DB persistence fails. Reconcile committed-but-unacknowledged results. Preserve manual repair behavior and avoid permanent-error queue starvation. Test lease loss, stale progress, cancellation and app startup without a worker.

### Stage F implementation prompt

Build the lean app image with its non-transcription Python helpers and a separate baseline worker image for CPU and CUDA. Use independent pinned runtime locks, model/cache/state paths, version manifests and digest-pinned release artifacts. Remove app GPU reservations and the implicit GPU override from normal deployment. Fix hardcoded configuration precedence while keeping current production behavior at cutover. Update Compose, `justfile`, release scripts/config and CI to use identical explicit file selection and independent worker versions. Keep source builds distinct from image deployment. Add Compose matrix and image-content checks. Do not switch production yet.

### Stage G implementation prompt

Implement a reproducible isolated container test harness and the failure/compatibility matrix in this plan. Add safe capture/restore-verification, deployment selection, worker drain/resume and rollback helpers using existing tools. Prove the baseline real corpus, resume across worker/app restarts, durable results, audio preservation, cache persistence and independent replacement with unchanged app image ID. Test the exact supported CPU/GPU commands. Produce test evidence and a concrete production cutover/rollback package. Do not use live data for destructive tests or claim a newer upstream release was tested if none exists.

### Stage H implementation prompt

Document the final implemented contract, supported versions and operational commands. Remove the destructive README migration cleanup advice and replace it with safe reference to data backups. Correct the backup documentation to distinguish PostgreSQL from SQLite. Record all persistent paths including worker state, model cache, tokens, NLTK and deployment identity, permissions, image digests and database connectivity assumptions. Include rollback and future-machine inventory. Keep actual host migration and broad backup subsystem redesign out of this change.

## 6 Acceptance test matrix

| Area | Required cases and pass criteria |
| --- | --- |
| Baseline behavior | Same normalized schema, timestamps, text and speaker handling under the same engine/options. Pin exact equality where deterministic; predeclare justified tolerances for GPU nondeterminism and review differences rather than weakening assertions after failures |
| Existing features | Feed refresh, audio download/playback, images, feed-provided transcripts, canonical text, exports, summary queue, repairs, bookmarks and play status remain functional |
| Cold/warm runtime | Clean cache downloads or reports model access failure explicitly; warm restart reuses cache; model readiness distinct from liveness; offline warmed test where supported |
| Long recordings | Multi-chunk real speech resumes without omitted or duplicated earlier segments; final alignment/diarization reruns safely; memory/output bounds are enforced |
| Request uncertainty | Drop connection before and after durable PUT acceptance; repeated UUID executes at most one active attempt; mismatched request is 409 |
| App restart | Kill app while worker runs, after worker succeeds, and after DB commit before ACK; reconnect and finish without duplicate inference or early summary |
| Worker restart | Kill child/supervisor/container at checkpoint writes and result commits; recover consistent state or visibly restart incompatible work; no false success |
| Result persistence | Inject DB failure and export/summary failure; successful inference remains retrievable and is not repeated merely because downstream work failed |
| Stale data | Concurrent bookmark/play edits survive; regenerated/deleted/changed episode cannot receive an old result; changed file hashes reject stale output |
| Error classification | 401/403, malformed JSON, 404, 409, 429, 5xx, missing mount/model, OOM, timeout and transport failures do not delete valid media |
| Scheduler | Failed permanent entries cannot starve due jobs under max-items limits; retry backoff remains correct; manual retry creates an intentional new attempt |
| Cancellation | Cancel queued/running/already-completed jobs; race cancel with completion; no surviving ffmpeg/Python process group; graceful and forced shutdown tested |
| Storage | Disk full, unwritable state/cache, torn temporary files, missing checkpoint/result, deleted state volume and second worker using the same state all fail safely |
| Security boundaries | Missing/wrong token; secrets absent from worker config/logs; traversal, symlink escapes, Windows drive/UNC paths, special files, body/output limits; no arbitrary URL/script execution |
| Service isolation | App usable while worker absent/draining; GPU assigned only to worker; worker has no app DB/LLM credentials, whole-config mount, or Docker socket |
| Upgrade/downgrade | Compatible new worker image handles app v1 without app replacement; prepared-but-unaccepted old-profile requests are reconciled safely; incompatible major rejected; checkpoint/state incompatibility explicit; old worker rollback succeeds |
| Deployment | Base disabled, CPU, CUDA, and isolated-test configurations render and start as documented; all helper commands select the same files; fresh clone works with documented env files |
| Restores | Archived baseline image + restored isolated PostgreSQL + copied assets boot and serve representative data; worker-state backup restores in split setup |

Add targeted tests for any new behavior that does not fit these cases. Stubs exercise failure deterministically; real CPU/CUDA audio exercises dependencies and output. Neither substitutes for the other.

Add proposed `just` targets for worker unit tests, contract tests, CPU container acceptance, CUDA acceptance and upgrade rehearsal. Their names should be committed with their implementations; do not document imaginary working commands. Standard focused commands after the packages exist include:

```text
go test ./internal/transcription/... ./internal/whisperxworker/... ./service/... -count=1
go test -race ./internal/transcription/... ./internal/whisperxworker/... ./service/...
go test ./...
just test-full
```

Run race tests in supported Linux/CGO CI if unavailable on the host. Add script-focused Python tests/lint under the worker's Python runtime: root Ruff currently excludes `scripts`, and root pytest discovers only `tests`. Merely passing the existing Python gate would miss new worker/pipeline coverage. Build images and inspect installed distributions to verify WhisperX/Torch absence from the app and presence at the expected versions in the worker.

CI runs deterministic contract/recovery tests on every relevant PR. CPU real-audio tests should run in a bounded release job. GPU acceptance can use a separately invoked trusted runner or the operator's current host; missing GPU CI does not excuse skipping the pre-release GPU check. Never expose production secrets to untrusted PR code.

## 7 Release tooling changes

`release.config.json` currently declares only one image and `RELEASE.ps1` deploys a single Compose file and broadly recreates the stack. Extend these deliberately:

- Independent app and worker version inputs and image references; worker-only build/publish/deploy commands must not bump or rebuild the app.
- An ordered Compose file list selected consistently for disabled, CPU and CUDA modes.
- Unique staging tags; immutable recorded digests; source revision and dependency metadata for each image.
- Worker-only deployment uses service-targeted recreation with `--no-deps`, never a full `down`. Explicit drain, state snapshot, capability/readiness verification and resume precede/follow it.
- Rollback restores the prior worker image reference and, only when necessary, the compatible state snapshot. Do not assume old code can read a newer worker store.
- Multi-service first cutover is a separately named operation. Existing `ship`, `deploy` and `rollback` wrappers must not silently discard the GPU file or restart unrelated PostgreSQL.
- Keep registry publication separate from local validation. Publish after review/tests according to repository authorization; use private archives when registry access is not available.

Use structured arguments or proper PowerShell arrays for commands. Do not build shell strings containing secrets. Save multiline Git/PR text to a file when needed. Existing release infrastructure should be extended, not replaced with another task runner.

## 8 Production cutover on this machine

Prepare a concrete release record containing source commit, app image digest, worker image digest, worker profile, full test outcomes, backup identifiers and rollback commands. Protect secrets separately. This is the final reviewable deployment package; do not ask someone to approve an unspecified future change.

1. Complete all gates and an isolated rollback rehearsal. Capture baseline queue counts and current effective runtime settings. Confirm no active app/model upgrade is happening concurrently.
2. Arrange a maintenance interval. Finish active local transcription where feasible; otherwise preserve its checkpoint and verify the legacy importer in rehearsal. Stop the old app before starting the new app against the production database.
3. Take the final PostgreSQL/config/media consistency checkpoint. Preserve old Compose files, env values, image tag and runtime cache exports. Keep `/assets` and `/config` internal app paths unchanged.
4. Create worker state/cache directories and the token file with tested ownership. Copy needed caches without deleting originals. Configure read-only worker media access and mount only the shared token/HF token needed by that worker.
5. Start the pinned baseline CUDA worker. Verify runtime versions, GPU availability, state ownership, token auth, cache access and API compatibility. No dependency upgrade yet.
6. Start the lean app with HTTP backend, concurrency one, batch limit/intervals and diarization settings matching the baseline. Use the new explicit Compose selection. Verify DB connectivity and that the app image has no GPU reservation.
7. Initially limit dispatch to one eligible item using existing controls and record temporary settings. Verify a complete transcript, progress, exported files and summary flow. Then restore the recorded normal limit and observe queue processing.
8. Restart worker and app separately during an agreed test episode or use the isolated fixture environment for disruptive cases; never intentionally kill unrelated production work just to repeat a test already proven in rehearsal.
9. Observe at least one representative long episode and one normal scheduling interval, plus a 24-hour normal-use window before declaring operational acceptance. Review failed/retried jobs, old processing entries, model/cache errors, disk use, audio integrity and downstream summary/export outcomes.
10. Record acceptance and retain recovery artifacts until migration and the operator's backup retention policy no longer require them. Do not prune the baseline image automatically.

In the first split release, production dependency versions must match the baseline. If a direct version match cannot be built, stop that gate and report the precise unavailable artifact; a dependency upgrade needs a separate comparison/review, not a silent substitution.

## 9 Independent worker upgrade procedure

After the split is accepted, the normal operator workflow must be:

1. Select a stable WhisperX candidate and lock a compatible dependency set. Use a separate change/PR. Build only the worker, record its runtime profile and digest, and run the same real-audio/recovery suite.
2. Verify the deployed app supports the worker's wire API major and required capabilities. Confirm state/checkpoint compatibility and model version policy.
3. Put the current worker in drain mode. App remains available; new transcription work stays in the app queue. Wait for accepted jobs and app receipts to settle, or explicitly cancel/preserve them with a tested recovery path.
4. Back up the quiescent worker state using a consistent SQLite backup plus referenced files, or stop the worker cleanly and copy the full state directory. Never hot-copy only the SQLite main file while WAL is active. Clone the cache for candidates if they can mutate incompatible artifacts.
5. Change only `WHISPERX_IMAGE` and approved worker-specific configuration; recreate only the worker. Record app container ID and image ID before and after to prove no app replacement occurred.
6. Verify ready/capabilities and a canary transcription, resume acceptance and monitor. On failure, drain/stop candidate and restore the previous image/state as required. Keep a record of jobs accepted by the candidate so rolling back state cannot silently lose their outcomes.

If no newer stable WhisperX exists, prove replacement with a second worker build that preserves API v1 and the baseline engine. Clearly report that as independent image replacement, not a test of an unreleased WhisperX version. A prerelease may be tested separately but must not become the baseline by accident.

## 10 Rollback procedures

There are two different rollback operations; implement and rehearse both.

**Worker-only rollback:** stop acceptance, settle or inventory outstanding jobs, stop the candidate, restore the previous compatible worker image and state snapshot if required, verify instance/contract/capability reconciliation, and resume. Restoring old state can remove newer job records, so compare the app's persisted job envelopes to the restored journal. Recover or explicitly requeue missing attempts with new identities after proving the candidate process cannot still run. Preserve successful app transcripts.

**Full split rollback to the monolith:** stop new dispatch and stop both new services before starting the archived app. Keep current database/media unless a separate data corruption recovery is necessary. Preserve completed transcripts/summaries. Run a reviewed rollback helper in dry-run mode to identify only pending/failed/processing remote checkpoint envelopes; back them up, then reset those specific items to safe local queued state with zero progress and no remote envelope. Preserve legacy checkpoint files separately; use one only if it is proven compatible, otherwise restart that unfinished episode from zero. Do not feed worker-private `/state` paths to the old local script.

Restore the old env and both original Compose files, including GPU configuration, and deploy the exact snapshot image. Verify DB compatibility on the isolated restore before this operation. No production database schema change is planned, but the starting source and deployed image can already differ; do not assume that makes arbitrary old images compatible. If a database restore is actually required, explicitly reconcile writes after the snapshot rather than discarding them silently.

Rollback helpers must produce a redacted before/after count, operate by inspected IDs/recognized checkpoint version, preserve source audio and completed outputs, and stop on unexpected formats. They must not bulk-reset all episode state, remove volumes, or drop the database. Keep rollback backups private and identify any operation that changes production rows in the final deployment review.

## 11 Migration preparation included now

These changes directly support the split and also reduce later migration work:

- Explicit relative container paths and host-configurable mounts; stable app paths and worker-relative media references.
- Portable documented worker state, caches, identity and secrets instead of relying on a container writable layer.
- Correct PostgreSQL-aware backup/restore runbook and backend detection in capture tooling. Fix the misleading backup claim; a general scheduled-backup subsystem redesign is deferred.
- Named CPU/CUDA deployment modes and documented minimum Docker/Compose versions; no implicit hardware override.
- Image manifests, immutable references, offline image archives and separate source/runtime provenance.
- Tested permissions and disk capacity checks for state/cache/media.
- Removal of dangerous README cleanup commands as a migration recommendation.
- Document `host.docker.internal` as a current-host dependency; do not change the live database address during this work.
- Document platform support honestly. Validate Linux/amd64 CPU/CUDA here. A lean ARM app build or ARM worker is optional follow-up unless the chosen destination requires it; removing the app's ML dependency does not prove ARM support.

Deliver a redacted `migration-inventory.md` listing logical data locations, mount destinations, image/contract versions, backup artifacts, ownership requirements, network dependencies and unresolved target-hardware decisions. Keep machine-specific addresses, credentials, private paths and actual backup locations in the private operations manifest where appropriate. Do not migrate any of them yet.

## 12 Final handoff from the implementing agent

The final implementation report must contain:

- Branch, base SHA, ordered commit list, pushed/unpushed status, and PR link if one was created.
- App and worker image tags/digests, source revisions, dependency profile, test hardware and model revision information.
- Recovery archive checksum, verified image ID, source-bundle verification, database restore evidence and private backup location reference without secrets.
- Exact commands run with passed/failed/skipped results; distinguish stub, real CPU, real CUDA, container, restart and rollback tests.
- Behavior/configuration/internal-contract changes and confirmation that public app APIs and app schema were preserved, or explicit approved exceptions.
- Evidence of one worker-only upgrade/rollback with unchanged app image and container identity.
- Production rollout status and observation-window outcome; if not deployed, clearly state that and supply the complete reviewable deployment package.
- Remaining limitations, including one-app/one-worker-instance operation, same-host mounted audio, and deferred host migration.

Suggested final reviewer prompt: "Review this branch against docs/whisperx-separation/SPEC.md and EXECUTION_PLAN.md. Prioritize data loss, duplicate/lost execution, stale DB writes, false durable success, checkpoint/rollback compatibility, secret exposure, unsafe media deletion, dependency reproducibility and deployment-file drift. Check actual test evidence. Do not approve the split solely because the happy-path transcript works."
