# Stage G acceptance follow-up — bounded resource failures

Implemented and rehearsed 2026-10-04. Stages A–H already exist; this is a remaining Stage G acceptance slice, following the [app crash checks](ACCEPTANCE_FOLLOWUP.md).

## Implementation and isolation

`scripts/whisperx_resource_acceptance.py` runs the shipped worker with a deployment-owned fixture script through its existing `serve --script` option. The Python standard-library child deliberately exhausts a bounded resource, then a successful fixture job proves recovery. It does not import WhisperX, load models or attest inference quality. No worker/app runtime code, API, database schema or dependency changes are required.

Every scenario has a new labelled container, private copied synthetic audio/token/fixture/cache and its own state. Before startup, the harness inspects the resolved image ID, mounts and resource/security settings. Containers have network `none`, no published ports/GPU/device access, a read-only root, UID 1000, all capabilities dropped, no new privileges, init, one CPU, 64 processes, 128 MiB memory and no swap. The API is accessed through authenticated loopback requests inside the worker; bearer values are never command arguments or evidence fields. Host deployment variables are excluded.

Linux ownership provisioning runs as root only against the new private tree. The host retains ownership of its run directory and writable fixture controls; only worker state/cache/token ownership changes. Fixture and media mounts are read-only in the worker. The repository and production directories are never writable container mounts.

The child allocations stop at 256 MiB; disk writes stop at 32 MiB even if a mount was selected incorrectly. Scratch is an 8 MiB tmpfs; the separate acceptance-storage scenario has a 16 MiB state tmpfs. Setting equal Docker memory and memory-swap limits prevents swap. Tmpfs pages count toward cgroup memory. Evidence requires increased local `max`, `oom` and `oom_kill` counters, rather than inferring OOM from an exit status. See [Docker resource constraints](https://docs.docker.com/engine/containers/resource_constraints/) and [Linux cgroup v2 memory controls](https://docs.kernel.org/admin-guide/cgroup-v2.html#memory).

Cleanup inspects ownership labels again, stops every owned test container and retains its artifacts. It preserves the original failure if cleanup also fails, and marks cleanup failure in evidence. No volume/image/network pruning is performed. Unit tests exercise unsafe envelope rejection before startup, refusal to stop unowned containers, bounded writes, inconclusive fault evidence and cleanup after partial failure.

## Actual container results

The final run used CPU worker `sha256:b032e151f70d8b1f1236a50864405677cb74da8aec938c90a89e81bd670bace0`, the unchanged verified Stage F worker. Its version is 1.0.0. The distinct engine profile is `resource-fixture-cpu-v1`, and evidence explicitly sets `real_ml: false`. Synthetic `speech.flac` SHA-256 is `8eda8bbe3417bd1a561b3402aee1437a8941dda386efe96d38d213b3f6eb66c3`.

| Scenario | Observed fault | Verified behavior |
| --- | --- | --- |
| Child OOM | `memory.max=134217728`, `memory.swap.max=0`; local counters changed from zero to `max=21`, `oom=1`, `oom_kill=1` | Worker remained available; durable `failed` / retryable `INTERNAL`; result and ACK refused with 409; child and sleeper descendant reaped; source SHA unchanged |
| Child scratch exhaustion | Actual `errno=28`, zero available bytes on the 8 MiB scratch tmpfs | Same terminal failure/result/ACK/reaping/source checks; removing only `/cache/tmp/fill.bin` allowed recovery |
| State disk full before acceptance | Actual `errno=28`, zero available bytes on the 16 MiB state tmpfs | Readiness 503; submit 503 / retryable `STATE_UNAVAILABLE`; status/result/ACK 404 for the rejected UUID; source SHA unchanged; removing only `/state/fill.bin` restored readiness, acceptance, result and idempotent ACK |

Both child-failure scenarios restart the same container with persistent private bind state. Failed outcomes remain unchanged; resubmission of the same UUID does not re-execute. An explicit new UUID succeeds. A second restart retains both failed and successful outcomes, the successful result and its original idempotent ACK receipt. Parent/descendant checks compare `/proc` PID and start time, avoiding PID-reuse confusion.

The final evidence contains **nine passed checks**, `passed: true` and `cleanup_passed: true`. An earlier complete rehearsal also passed; the final run adds explicit post-restart ACK verification, rejected-job result/ACK refusal and source/fixture/harness hashes. Evidence stays outside Git under the directory named by `%TEMP%/briefcast-resource-directory.txt`. Final evidence SHA-256 is `b9a792ccb309b3158693e98da0ee92e16dcb6c025158b15f7624b1a7e63b0f39`. Raw job artifacts remain private.

All three final fixture containers were stopped. Read-only production inspection retained container `12a316ad6ad8c1b146832bda4581e8a90030e6e0fc938346e2abc46bef33d7ea`, image `sha256:a7542b94500a5ac7e00271451ef2a51db233bd5e2bec92c615b240302b06d052` and start time `2026-10-02T20:40:04.862330437Z`. Only `briefcast` remained running.

## Commands and verification

Run on a Docker Linux engine with cgroup v2 and the existing development environment. Run directories must be new and outside the repository. The fixture needs no cache downloads or model credentials.

```powershell
$env:UV_NO_SYNC = '1'
$resourceRun = Join-Path $env:TEMP ('briefcast-resource-' + [guid]::NewGuid().ToString('N'))
uv run python scripts/whisperx_resource_acceptance.py --worker-image briefcast-whisperx-cpu:stage-f-verified --run-dir $resourceRun --timeout 90
# Equivalent repository command:
just test-whisperx-resources --worker-image $WorkerImage --run-dir $NewPrivateRunDir --timeout 90

uv run pytest tests/test_whisperx_resource_acceptance.py -o addopts='' -q
pwsh -NoLogo -NoProfile -File scripts/TestWorkerPython.ps1 -Image briefcast-whisperx-cpu:stage-f-verified
uv run ruff check .
uv run ruff format --check .
uv run mypy src
uv run pytest -o addopts='' -q
```

Focused host resource tests passed **31 tests**. The shipped Python 3.12/Linux test command passed **87 tests**, including the fixture module. Full host pytest passed **105 tests**, with four existing platform skips. Root lint/format, script lint/format, mypy and CLI help passed. No Go/frontend runtime files changed; their suites and image builds were not repeated for this slice.

The CPU worker release workflow now runs the bounded container harness against the candidate image before real-engine acceptance and uploads only `resource-acceptance.json`. Existing worker-runtime Python validation includes the new resource unit tests. No workflow was dispatched and no image was published. Local rehearsal uses the retained Stage F image, so the CI candidate-image path still requires its normal release run.

## Limits, remaining gates and rollback

These checks establish real cgroup OOM handling for a controlled child, scratch ENOSPC handling, and state-space refusal before acceptance. They do **not** test a Torch/CUDA allocator failure, an OOM that kills the worker itself, persistent storage exhaustion during journal/result commit, or restart durability of the ephemeral state tmpfs. Controlled journal/result-write failures remain covered by existing worker unit tests; a real in-flight storage failure requires a separate bounded scenario.

Strict CUDA score comparison remains failed; no tolerance or assertions changed. Coherent production DB/config/media recovery, positive gated diarization, an upstream/state-schema upgrade/downgrade, production summary/playback canaries and the observation window remain open. The production cutover package remains unready. This follow-up does not clear those gates or authorize production replacement.

Rollback is reverting this test/tooling/docs slice. There is no runtime or state migration to undo. Keep private evidence and existing recovery archives/source bundles.
