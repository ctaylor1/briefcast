# Migration inventory — redacted

Prepared 2026-10-03 from committed implementation through Stage G (`bfea2562d95462b2441da8729f3e8e95e00a211c`) and the source files referenced below. This inventory prepares future relocation; **no host migration or split production cutover has occurred**. [Stage G](STAGE_G.md) retains the failed strict CUDA quality comparison and incomplete coherent production media recovery gate. Actual host paths, database addresses/passwords, tokens, primary/advanced env copies and backup locations belong in the private operations manifest.

## Deployment and artifact identity

| Logical component | Verified local identity / provenance |
| --- | --- |
| Archived production monolith | `sha256:a7542b94500a5ac7e00271451ef2a51db233bd5e2bec92c615b240302b06d052`; app label 1.9.4, original image source revision remains unverified |
| Lean app 1.9.4 | `sha256:eadbf9df0f3418328d5824739b08e63abe9b4e1f43d0b5743ef4242acea6e2dc` |
| CPU worker 1.0.0/API v1 | `sha256:b032e151f70d8b1f1236a50864405677cb74da8aec938c90a89e81bd670bace0`; `baseline-wx3.8.6-cpu-v1` |
| CUDA worker 1.0.0/API v1 | `sha256:64be5c42e52b03f6371a6183acd08ec2925ef312ca5b06f6f00aa30dde520fba`; `baseline-wx3.8.6-cu128-v1` |
| Split runtime build source | `7d8545f6c0a63ecbb182f318c5deb0b4b63b02bb`; later Stage G/H tools/docs have separate source commits |
| Pipeline | `briefcast-script-v1`, SHA256 `7aa01ea856e94438e728f944b8e71aee79a6843470afc67a0a3334e5d2c14520` |
| Worker persistence | Schema 1, UUID worker instance in journal; checkpoint `briefcast-whisperx-chunks-v1` |
| App remote persistence | Envelope version 1 in the existing `transcript_checkpoint_json` DB column; stable deployment UUID in `/config/whisperx-deployment-id` |

These `sha256:` values are **local Docker image IDs**, not published multi-platform registry manifest digests. Verified archive names/checksums and attestations are in [Stage F](STAGE_F.md); source, restore and replacement evidence are in [Stage G](STAGE_G.md). Capture the destination's loaded image identity and source/version/package manifests before accepting relocation. Save exact old images independently of mutable tags. App and worker versions do not advance together.

## Persistent locations and ownership

| Logical data | Container path / host setting | Access, ownership and recovery requirement |
| --- | --- | --- |
| Application DB | Actual private `DATABASE_URL` and `DB_DRIVER`/`DATABASE_DRIVER`; default SQLite `/config/briefcast.db` | App read/write only. PostgreSQL is external on the observed host; backing up `/config` does not include it. Preserve role/ownership requirements and DB consistency checkpoint |
| App config and identity | `/config` ← `HOST_CONFIG_DIR` | App writable; carry `whisperx-deployment-id` with the matching DB/envelopes. Retain optional historical caches/config/backups; do not regenerate identity to hide missing jobs |
| Media and exports | App `/assets`, worker `/media` ← the **same** `HOST_ASSETS_DIR` | App read/write, worker read-only; preserve relative layout, names, hashes, audio/artwork/text/markdown and `.briefcast.whisperx.resume.json` legacy sidecars and their referenced segment files |
| App logs | `/logs` ← `HOST_LOGS_DIR` | App writable, `PUID`/`PGID` affect created-file ownership. Logs can contain private identifiers; protect separately with the operator's retention policy |
| Worker journal/artifacts | `/state` ← `HOST_WHISPERX_STATE_DIR` | Worker writable, default UID/GID 1000:1000. Retain `journal.sqlite` including WAL/SHM where present, `owner.lock`, metadata and the complete `jobs/<UUID>` trees with results/checkpoints/private diagnostics. Exclusive single owner; stop before full snapshot |
| Worker cache root | `/cache` ← `HOST_WHISPERX_CACHE_DIR` | Worker writable with tested UID/GID; copy baseline caches, retain original source. A state snapshot does not include cache |
| HF models | `/cache/huggingface`, `/cache/huggingface/hub` | `HF_HOME`/`HUGGINGFACE_HUB_CACHE`; preserve blobs, snapshots and pinned refs/link targets. Baseline medium.en revision `a29b04bd15381511a9af671baec01072039215e3` |
| Alignment/Torch cache | `/cache/torch` | `TORCH_HOME`; baseline wav2vec2 artifact SHA256 `488fd4f16de84438ffc945334278c1b9fb9b7159a806c1080b16111a958c945d` (377664473 bytes) |
| NLTK data | `/cache/nltk` | `NLTK_DATA`; retain `punkt_tab` and required language resources. Original monolith caches can also exist outside mounted `/config`; inspect private writable-layer inventory |
| Library/home caches | `/cache/home` | `HOME`; inventory any needed additional cached artifacts instead of assuming all are in HF_HOME |
| Service bearer file | `/run/secrets/whisperx-service-token` ← `HOST_WHISPERX_TOKEN_FILE` | Read-only shared app/worker file, >=32 non-whitespace characters. Restrict host ACLs/modes and grant only required container readers; back up privately |
| HF access file | `/run/secrets/whisperx-hf-token` ← `HOST_WHISPERX_HF_TOKEN_FILE` | Read-only worker-only file; empty allowed with no gated models. Gated authorization/model acceptance still needs separate validation |
| Deployment configuration | Primary/advanced dotenv, ordered Compose files, project/container identity, release/rollback manifests | Private backups, outside build context and scheduled pruning. App dotenv includes DB/LLM secrets and must not become worker `env_file` |
| Optional Compose PostgreSQL data | `postgres_data` named volume, only for explicitly selected `postgres` profile | Not the observed external production DB. Include its own compatible DB/volume recovery if this profile is selected later; do not accidentally start it as a migration shortcut |
| Recovery artifacts | Private operator storage outside repository and `$CONFIG/backups` | DB dump, image/config/state archives, checked hashes, source bundle, restore/canary evidence, private envelope-reset backups; retain until review/acceptance and retention requirements are met |

Scratch `/tmp` and `/cache/tmp` are 512 MiB tmpfs mounts, recreated at startup; Python `TMPDIR` uses `/cache/tmp`. They are not checkpoint or model storage. Worker root is read-only and holds pinned executable/script/runtime manifests in the image. Do not migrate process memory or a running SQLite journal by copying only its main file.

The app also stages bounded result transfers in temporary `briefcast-worker-result-*` files before validation/DB commit, removing them on normal request cleanup. These are not durable receipts or a recovery source. Preserve the DB envelope and worker result instead. The destination filesystem must support tested ownership locks, SQLite durability and atomic writes; app identity publication also uses a hard link. Do not assume an untested network share behaves like the rehearsed Docker Desktop bind mounts.

Pre-provision bind sources; worker Compose refuses to create missing media/state/cache/token host paths. Default worker ownership is 1000:1000. Current `WorkerRecovery.ps1` also runs as 1000:1000 even for a customized worker, so verify recovery access before adopting another identity. App `PUID`/`PGID` do not configure worker identity. Linux mounts require correct numeric ownership and file access; Windows ACLs/Docker Desktop behavior must be tested, not inferred from Unix modes. No Docker socket mount is needed.

## Connectivity and runtime assumptions

- One app and one worker per state directory are supported. No multi-app scheduling/active-active worker failover or cross-host storage architecture was accepted.
- App resolves `whisperx` over the same Compose network at `http://whisperx:8090`. Default worker has no host-published port; the app exposes its existing configured HTTP port. HTTP bearer auth is intended for this private network. Exposing it across hosts needs a separate transport/network design.
- The observed DB uses `host.docker.internal`; Compose maps it to the host gateway. Destination DNS/routing/server placement must be decided and tested before changing the private DB URL. The optional Compose PostgreSQL profile is not an automatic replacement for that DB.
- App still needs feed/media/artwork and optional summary-provider egress. Worker network permits model downloads when `WHISPERX_OFFLINE=false`; default offline mode relies on copied cached weights. Capability `model_ready` does not prove every weight/gated credential is usable.
- Linux/amd64 CPU and CUDA images are the supported built variants. CUDA reserves GPU only for worker and needs a compatible destination NVIDIA/Docker runtime. Stage G used Windows Docker Desktop Linux containers and RTX 5070 Ti; standalone Linux host ownership and alternate GPU/hardware acceptance must be recorded separately. Lean app packaging does not prove ARM support.
- Baseline FFmpeg/system/Python dependencies are pinned. Archive mirror availability may affect rebuilds; preserve verified image archives. Optional CUDA TorchCodec missing `libnppicc.so.12` and unverified positive diarization remain baseline limitations, not reasons to upgrade packages silently.

## Existing recovery evidence

Stage G archived-image load/version smoke, isolated PostgreSQL restore, copied config/cache restore and archived-app catalog/settings boot passed. Baseline recovery identifiers:

| Artifact | SHA256 |
| --- | --- |
| Monolith image archive | `97a4d7f7d041fa4503bdcf5647ec095c6794dc3cb31bb5245bdf461ac72c03a5` |
| PostgreSQL custom dump | `5003e3789b975dd8d739c443c8f7c5bb433560179f48b6ffc73c0a37556c2a86` |
| Config/cache tar | `9332b4c215c503d4bc3dd5d0294b488089e54c1b3df7f9ebc38dbbf7346962f6` |

Initial source bundle captured Stage F source with a dirty working-tree inventory; separate verified Stage G committed-source bundle exists. Actual locations, source bundle checksums, settings, stopped rehearsal volumes/container identities and command arrays are in private manifests referenced by Stage G. Do not publish their raw contents. A future Stage H documentation bundle is a source checkpoint, not a replacement production database/media checkpoint.

Source DB was PostgreSQL 16.11; isolated client/restore used PostgreSQL 17 with new test-role ownership and no original roles/ACLs. Restored catalog counts and app boot are reported in Stage G. Production media was not copied/restored. These archives do not form a final coherent database/config/media restore point, and writes after their capture must be reconciled. Worker full-state restore/replay and independent compatible replacement/rollback passed on isolated CPU/CUDA projects while preserving app image/container identity; no upstream worker/state-schema downgrade or production rollback was performed.

## Decisions required before a future move

1. Destination OS/CPU architecture, GPU/driver/runtime and storage capacity; acceptance results for that hardware, including long audio and permissions. Preserve the baseline engine first.
2. DB server placement/version/client, hostname replacement for `host.docker.internal`, authentication/TLS, original role/ownership recovery and a verified isolated representative restore.
3. Durable config/media/state/cache/secret locations, app/worker ownership, snapshot/backup retention, disk headroom and an independent backup storage copy. Preserve stable deployment/worker identity and legacy sidecars.
4. Maintenance interval, final coherent DB/config/media checkpoint, private config/image/source record, rollback target and post-snapshot write reconciliation. Test both worker-only and monolith rollback before moving live state.
5. Exact runtime/Compose/env/project selection and immutable image references; private token handling, app feed/provider connectivity and worker offline/gated model policy.
6. Resolve Stage G quality/media/matrix gates, then record one transcript/export/summary canary, long episode, scheduling interval and 24-hour normal-use acceptance. Current rehearsal evidence alone does not clear production rollout or relocation.

Use [operations](OPERATIONS.md) and [backup/restore](../backup-and-restore.md). Broad backup subsystem redesign, database relocation and actual host migration remain separate work.
