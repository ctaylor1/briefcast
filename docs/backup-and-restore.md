# Backups and restore verification

This runbook describes the implemented application/worker backup boundaries. It does not redesign scheduled backups or authorize a production restore. Use the active database settings and reviewed private release record. [Stage G evidence](whisperx-separation/STAGE_G.md) records the actual image/database/config restore rehearsal and its unresolved media consistency gate.

## What scheduled backups contain

`service/fileService.go:CreateBackup` runs every 48 hours and writes a tar.gz under `$CONFIG/backups`. It reads only the fixed `$CONFIG/briefcast.db` filename and retains at most five files through the existing pruning code.

| Database | Current scheduled behavior | Required recovery method |
| --- | --- | --- |
| SQLite at the default filename | Copies the main file while the app may be writing. No online backup transaction, WAL/journal capture or restore verification | Stop all writers and protect the actual configured database plus any journal files, or use a separately verified consistent SQLite backup mechanism |
| SQLite at another configured filename | Still reads `briefcast.db`, so may fail or archive an unrelated old file | Locate the actual `DATABASE_URL`/`CONFIG` database and verify its snapshot |
| PostgreSQL | Does not invoke `pg_dump` or inspect the active backend. May fail because `briefcast.db` is missing or archive an irrelevant leftover SQLite file | Compatible PostgreSQL logical dump, role/ownership plan and isolated `pg_restore` verification |

The scheduled tar contains no media, configuration tree, stable app deployment ID, worker journal, model cache or tokens. File existence, successful creation and the UI's backup listing do not prove recoverability. Keep operational recovery artifacts **outside** `$CONFIG/backups`, where scheduled pruning can remove files, and outside the repository/build context. Preserve checksums and at least one separate storage copy according to the operator's retention policy.

## Recovery package

Protect these components together; [migration-inventory.md](whisperx-separation/migration-inventory.md) lists logical paths and ownership:

- Actual app database, app `/config` including `/config/whisperx-deployment-id`, and `/assets` including source audio, artwork, exports and legacy resume sidecars.
- Complete stopped worker `/state`, including SQLite WAL and all referenced job artifacts. Include model caches, HF/Torch/NLTK directories and private service/HF token files separately; `WorkerRecovery.ps1` archives state, not cache or tokens.
- Actual Compose definitions, primary/advanced dotenv files and effective settings, saved privately. Preserve original database connectivity and app internal paths.
- Exact running image archive and load/version evidence; worker/app image manifests, source SHA and verified source bundle. A source bundle covers committed history. An image archive covers image layers, not mounts, database, process memory or writable-container changes; preserve and review the captured `docker diff` inventory.

For the final consistency checkpoint, arrange maintenance, finish accepted work, stop the app and worker and stop other writers. Protect database/config/media at that checkpoint and record times/checksums. Preserve legacy sidecars even when unfinished work must restart from zero. A dump taken during live downloads and an unrelated filesystem copy are not automatically one coherent restore point. Starting a second normal app against production is unsafe: startup initializes the database, runs migrations and unlocks jobs.

## PostgreSQL capture and isolated rehearsal

Run from repository root with the existing development environment. Assign `$RecoveryDir` to a **new absolute private directory outside the repository**, `$BaselineContainer` to the inspected monolith and `$PostgresClientImage` to a locally available compatible PostgreSQL client image. Never supply a connection URL/password as an argument. Capture reads the inspected container's PostgreSQL URL into restricted env/`.pgpass` files; only `sslmode` URL options are supported by the current helper. Other options require a reviewed recipe. Select a client appropriate for the server version; Stage G used PostgreSQL 17 to dump/restore the observed PostgreSQL 16.11 database.

```powershell
uv run python scripts/briefcast_recovery.py capture --container $BaselineContainer --destination $RecoveryDir --postgres-image $PostgresClientImage --config-file .env --config-file .env.whisperx --config-file docker-compose.legacy.yml --config-file docker-compose.legacy-gpu.yml
uv run python scripts/briefcast_recovery.py verify-database --destination $RecoveryDir
uv run python scripts/briefcast_recovery.py verify-app --destination $RecoveryDir
```

All listed config files must exist and must be the ones actually used; omit absent optional files or supply their real private paths. Check each exit code. Capture requires a new destination, free space and private permissions, records the inspected immutable image ID and server version, and preserves the config tree as binary tar so Linux cache symlinks survive Windows capture. `--skip-image` is partial capture and cannot satisfy the image gate. After an interrupted capture, `finish-capture` can resume only from its preserved manifest/dump with matching runtime identity; it refuses to overwrite an incomplete image archive. Inspect diagnostics privately before choosing a new capture.

`verify-database` checks the dump checksum, creates a unique isolated PostgreSQL container without network/host ports, restores with `--no-owner --no-privileges`, verifies tables/counts and stops its server. It preserves its private volume. `verify-app` uses only that verified private volume with one active server, a private internal network, restored config and copied synthetic assets. It boots the archived monolith with transcription/LLM disabled and serves catalog/settings. It stops only its own containers; it does not restore into production.

Original roles/ACLs are **not** restored by this helper. A production recovery needs a separately reviewed ownership/global-role plan. This rehearsal does not prove production audio/playback, complete transcript/summary recovery or a coherent media snapshot. Capture always marks `coherent_media_checkpoint=false`; pair a reviewed final database dump with independently verified asset protection before clearing that gate in the private cutover package. Never set a success flag merely because the dump command exited successfully.

## SQLite application recovery

Resolve the actual SQLite filename from the private effective configuration; it may be outside `/config` or use a historical name. Stop all app/database writers before a filesystem snapshot. Preserve the entire containing database/journal set and `/config`/media at the maintenance checkpoint; copying only a live main file can miss WAL data. Use new restored paths and a fresh isolated database connection for verification. Do not start a restore test against the production SQLite file.

The PostgreSQL capture helper explicitly requires PostgreSQL and is not a SQLite backup command. The current scheduled tar is not a substitute for this stopped/consistent procedure. Check restored subscriptions, episodes, settings, transcript/summary output, representative media playback and app deployment identity before trusting a restore. Keep original files intact until verification and retention requirements are satisfied.

## Worker snapshots and restoration

Drain acceptance, wait for queued/running work and app receipts, then stop only the worker using the **same** env/runtime/project selection. Provision a private snapshot parent that the recovery container can write. `$WorkerImageId` must be the immutable **local** `sha256:` ID; `$StateDir` is the inspected state bind source. State must be accessible to UID/GID 1000:1000 because the current PowerShell recovery wrapper uses that identity even if the running worker UID is customized.

```powershell
pwsh -File scripts/WorkerRecovery.ps1 -Operation snapshot -Image $WorkerImageId -StateDir $StateDir -SnapshotDir $SnapshotDir
pwsh -File scripts/WorkerRecovery.ps1 -Operation verify -Image $WorkerImageId -SnapshotDir $SnapshotDir
pwsh -File scripts/WorkerRecovery.ps1 -Operation restore -Image $WorkerImageId -SnapshotDir $SnapshotDir -StateDir $NewRestoredStateDir
```

Snapshot refuses a running state owner, undrained or nonterminal work, corrupt journal/references, symlinks/special files, insufficient space or an existing destination. It archives the whole state tree with per-file hashes and a state/image manifest. Restore validates safe relative regular archive members and writes only a new directory; it does not select that directory in Compose. Boot a compatible worker against the isolated restoration and verify instance identity, results and receipts.

Before selecting an older restored journal in a live deployment, preserve candidate-era state and reconcile every persisted app envelope against the restored job inventory. Newer successful jobs/receipts must not disappear silently. Keep completed app outputs and source media. [Operations rollback](whisperx-separation/OPERATIONS.md#rollback) distinguishes worker rollback from rollback to the monolith.

## Verification and observability

Record source/image IDs, archive/per-file SHA256 values, DB server/client versions, schema/checkpoint compatibility, capture/restore times, data counts, permissions and command results in private manifests. Logs/inspection/dumps can contain credentials or private transcript text; only redacted metadata belongs in Git/CI reports. Monitor `CreateBackup` failures and verify operator-managed database/media/state backups independently. General scheduled-backup correctness and retention redesign remain follow-up work.
