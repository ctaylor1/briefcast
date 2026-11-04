# Monitoring

Briefcast records telemetry locally. Nothing is sent to an external service:
metrics are only available to whoever can reach the app, and logs stay in the
container output and `LOG_OUTPUT` files.

## Where to look

| What | Where |
| --- | --- |
| At-a-glance health, queue, throughput, GPU | **Settings → Monitoring** in the app (refreshes every 15 s) |
| App metrics (Prometheus text format) | `GET /metrics` |
| WhisperX worker metrics | `GET /metrics/worker` (the app relays the worker's private `GET /v1/metrics`) |
| Per-episode events | Structured JSON logs (app and worker) |

`/metrics` and `/metrics/worker` use the same access control as the rest of the
app (HTTP basic auth when `PASSWORD` is set; the default deployment binds the
app to localhost only). The worker endpoint itself requires the worker bearer
token and is not published on a host port.

Metric labels are deliberately low-cardinality (outcome, state, stage, job name,
error code). Episode, podcast and job identifiers appear only in log events.

## App metrics (`/metrics`)

| Metric | Type | Labels | Meaning |
| --- | --- | --- | --- |
| `briefcast_info` | gauge | `version` | Running version |
| `briefcast_episodes` | gauge | `download_state` | Library episodes by download state |
| `briefcast_downloaded_episode_transcripts` | gauge | `state` | Downloaded episodes by transcript state (`available`, `pending_whisperx`, `processing`, `failed`, `blocked`) |
| `briefcast_transcript_backlog_oldest_seconds` | gauge | | Age of the oldest downloaded episode waiting for a transcript |
| `briefcast_assets_free_bytes` | gauge | | Free space on the audio/export volume |
| `briefcast_episode_downloads_total` | counter | `outcome` | `success`, `failed`, `paused`, `cancelled` |
| `briefcast_episode_download_duration_seconds` | histogram | | Successful download time |
| `briefcast_episode_download_bytes_total` | counter | | Bytes downloaded |
| `briefcast_feed_refreshes_total` | counter | `outcome` | Per-podcast feed refreshes |
| `briefcast_transcriptions_total` | counter | `outcome` | `success` (committed), `failed`, `blocked`, `retrying` |
| `briefcast_transcription_worker_queue_seconds` | histogram | | Wait in the worker queue before starting |
| `briefcast_transcription_processing_seconds` | histogram | | Worker processing time (first start to finish) |
| `briefcast_transcription_end_to_end_seconds` | histogram | | App hand-off to committed transcript |
| `briefcast_transcription_audio_seconds_total` | counter | | Audio seconds in committed transcripts |
| `briefcast_transcription_speed_ratio` | histogram | | Audio seconds per processing second |
| `briefcast_transcript_exports_total` | counter | `outcome` | Transcript export writes |
| `briefcast_scheduled_job_runs_total` | counter | `job`, `outcome` | Scheduled job runs |
| `briefcast_scheduled_job_duration_seconds` | histogram | `job` | Scheduled job run time |

Standard `go_*` and `process_*` metrics are included.

## Worker metrics (`/metrics/worker`)

| Metric | Type | Labels | Meaning |
| --- | --- | --- | --- |
| `whisperx_worker_info` | gauge | `worker_version`, `engine_profile`, `pipeline_version`, `device`, `compute_type` | Static configuration |
| `whisperx_worker_healthy`, `whisperx_worker_accepting` | gauge | | 1 when healthy / accepting jobs |
| `whisperx_worker_capacity` | gauge | `kind` | Configured `running` and `waiting` slots |
| `whisperx_jobs` | gauge | `state` | `queued`, `running`, `unacknowledged` |
| `whisperx_jobs_accepted_total` | counter | | Jobs accepted |
| `whisperx_job_runs_started_total` | counter | | Runs started (a resume starts a new run) |
| `whisperx_jobs_finished_total` | counter | `state`, `error_code` | Terminal runs |
| `whisperx_job_runs_interrupted_total` | counter | | Runs requeued by shutdown (resume from checkpoint) |
| `whisperx_job_queue_wait_seconds` | histogram | | Acceptance to first start |
| `whisperx_job_run_seconds` | histogram | `state` | Run wall time |
| `whisperx_job_stage_seconds` | histogram | `stage` | Time in each stage until the next progress event (`starting` includes model load) |
| `whisperx_transcribed_audio_seconds_total` | counter | | Audio processed by successful runs (resumed portions excluded) |
| `whisperx_job_speed_ratio` | histogram | | Audio seconds per wall second, successful runs |
| `whisperx_state_free_bytes` | gauge | | Free space on the worker state volume |
| `whisperx_gpu_memory_used_bytes`, `whisperx_gpu_memory_total_bytes`, `whisperx_gpu_utilization_ratio` | gauge | `gpu` | Whole-device readings from `nvidia-smi` (cached 5 s; includes other processes) |

Worker counters reset when the worker restarts; app counters reset when the app
restarts. Gauges derived from the database or journal do not.

## Log events

Both services log one JSON object per line. Useful event names (`msg` field):

| Service | Event | Key fields |
| --- | --- | --- |
| app | `episode_download_completed` | `podcast_item_id`, `podcast_id`, `bytes`, `duration_ms`, `host` |
| app | `failed to download episode` | `podcast_item_id`, `url`, `error` |
| app | `transcription_started` | `podcast_item_id`, `job_id`, `attempt`, `stage` |
| app | `transcription_completed` | `audio_seconds`, `worker_queue_ms`, `processing_ms`, `end_to_end_ms`, `speed_x`, `resumed_from_checkpoint` |
| app | `transcription_retry_scheduled` / `transcription_failed` | `outcome`, `code`, `retryable`, `http_status`, `retry_count`, `next_attempt` |
| app | `transcript_exported` | `podcast_item_id` |
| app | `job_started` / `job_completed` / `job_failed` | `job_name`, `duration_ms` |
| worker | `worker_job_accepted` | `job_id`, `attempt`, `input_bytes`, `resume_from_checkpoint` |
| worker | `worker_job_started` | `queue_wait_ms`, `first_run`, `resume_offset_seconds` |
| worker | `worker_job_stage` | `stage`, `previous_stage`, `previous_stage_ms`, `percent` |
| worker | `worker_job_finished` | `state`, `error_code`, `run_ms`, `audio_seconds`, `processed_audio_seconds`, `speed_x` |
| worker | `worker_job_interrupted` | `run_ms`, `stage` |

Example: transcription speed of the last 20 jobs.

```powershell
docker logs briefcast-prod 2>&1 | Select-String '"transcription_completed"' |
  Select-Object -Last 20 | ForEach-Object { $_.Line | ConvertFrom-Json } |
  Format-Table ts, podcast_item_id, audio_seconds, processing_ms, speed_x
```

## Prometheus queries

```promql
# Transcription throughput (audio hours per hour)
rate(briefcast_transcription_audio_seconds_total[1h])
# Failure ratio
sum(rate(briefcast_transcriptions_total{outcome=~"failed|blocked"}[6h])) / sum(rate(briefcast_transcriptions_total[6h]))
# Backlog stuck
briefcast_transcript_backlog_oldest_seconds > 6 * 3600
# Average time per stage
rate(whisperx_job_stage_seconds_sum[1d]) / rate(whisperx_job_stage_seconds_count[1d])
# GPU memory headroom
whisperx_gpu_memory_total_bytes - whisperx_gpu_memory_used_bytes
```
