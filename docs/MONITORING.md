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
| `briefcast_summary_attempts_total` | counter | `outcome` | LLM summary attempts: `succeeded`, `failed`, `timed_out`, `token_limit`, `panicked`, `discarded` (the transcript changed during the call) |
| `briefcast_summary_duration_seconds` | histogram | | Time per summary attempt, any outcome |
| `briefcast_notifications_total` | counter | `outcome` | Failure notifications: `sent`, `failed`, `suppressed` (hourly limit) |

Standard `go_*` and `process_*` metrics are included.

`GET /settings/monitoring` (the panel's data) also reports, from memory since the app started:

- `summaries`: the LLM setup (provider, model, `host` as host:port only, `reasoning_effort`,
  `max_tokens`, timeout), the summary job, attempts in flight, and the last 20 attempts with
  outcome, duration, finish reason, token counts and a shortened error.
- `jobs`: per background job, the last run, its outcome and error, consecutive failures and
  since when it has been failing.
- `repair`: whether Repair is running and its last run.
- `queue.summaries_by_state` and each window's `summaries`, from the same cached scan as the
  transcript counts (status and date columns only). `waiting` counts transcribed episodes
  that were never summarized.

None of it includes transcript, prompt or summary text, or keys.

`GET /settings/monitoring?checkLLM=1` (sent by the Monitoring panel only) also checks the
LLM server and adds `summaries.server`: reachable, latency, whether `GET /models` lists the
summary model, and an error class (`timeout`, `refused`, `unauthorized`, `not_found`, ...).
For `LLM_PROVIDER=ollama` it also reads Ollama's `GET /api/ps`: whether the model is loaded,
its context length, and `gpu_share` (the part of the model in GPU memory; below 1 means part
of it runs on the CPU, which is much slower). The result is cached for 60 seconds and goes
through the same outbound URL policy as summaries.

The database-backed gauges (`briefcast_episodes`, `briefcast_downloaded_episode_transcripts`,
`briefcast_transcript_backlog_oldest_seconds`) and the panel's completion windows come
from one grouped scan of the episodes table, cached for 60 seconds. Episode rows store
full transcripts inline, so on a large library that scan takes several seconds; a stale
value is served immediately while one background refresh runs.

## Notifications

With `BRIEFCAST_NOTIFY_URL` set, the app checks once a minute and pushes a short message when:

| Condition | When |
| --- | --- |
| Summaries failing | 3 summary attempts in a row failed (the message gives the last reason, for example the token limit) |
| Summaries close to their limit | The last 3 successful summaries each took over 80% of `LLM_TIMEOUT_SECONDS` |
| A job is failing | A background job failed 3 runs in a row, or has failed for an hour (with its last error) |
| LLM server unreachable | For 10 minutes, while summarization is on |
| WhisperX worker down | Unreachable or unhealthy for 10 minutes |
| Low disk space | Under 10 GB free on the audio and export volume |

Each condition sends one message when it starts, a reminder at most every 6 hours while it
lasts, and one "Resolved" message. At most 10 messages are sent per hour. The state is kept in
memory, so after a restart a condition is reported again only once it holds again.

Messages contain the condition, episode titles, counts and shortened errors; never transcript,
prompt or summary text, keys, or the notification URL. Any link inside an error is cut to its
scheme and host (`https://www.patreon.com/…`), because a private feed URL can carry an access token. Delivery has a 10-second timeout and two
retries; failures are logged as `notification_failed` and counted in
`briefcast_notifications_total{outcome}` (`sent`, `failed`, `suppressed`).

Example with [ntfy](https://ntfy.sh) (free phone app; pick a hard-to-guess topic name):

```text
BRIEFCAST_NOTIFY_URL=https://ntfy.sh/briefcast-alerts-<random>
```

Then **Activity → Monitoring → Notifications → Send test notification**
(`POST /settings/notifications/test`: 202 when sent, 409 when notifications are off).
For a ntfy server on your LAN also set `BRIEFCAST_ALLOW_PRIVATE_NOTIFY_URLS=true`.

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
| app | `summary_queued_after_transcription` | `podcast_item_id`, `replace_existing` (the summary runs in the summary job, not in transcription) |
| app | `summary backfill starting` / `summary backfill finished` | `episodes`, `limit`, `requested_episodes`, `replacing`; `succeeded`, `failed`, `total` |
| app | `summary failed` / `summary complete` | `episode_id`, `title`, `error` |
| app | `summary_attempt_finished` | `podcast_item_id`, `source`, `outcome`, `duration_ms`, `model`, `finish_reason`, `prompt_tokens`, `completion_tokens`, `thinking_chars`, `error` |
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
