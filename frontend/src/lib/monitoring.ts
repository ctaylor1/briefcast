import type { JobRunSummary, MonitoringSnapshot, MonitoringSummaries, SummaryAttempt } from "../types/api";

export type HealthTone = "ok" | "warn" | "bad" | "idle";

export interface StatusItem {
  tone: HealthTone;
  label: string;
  detail?: string;
}

const numberFormat = new Intl.NumberFormat("en-US");

export function formatCount(value: number | undefined | null): string {
  return numberFormat.format(Math.round(value ?? 0));
}

export function formatBytes(bytes: number | null | undefined): string {
  if (bytes == null || !Number.isFinite(bytes) || bytes < 0) {
    return "—";
  }
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value >= 100 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`;
}

/** Compact duration: "45 s", "3 m 20 s", "2 h 5 m", "3 d 4 h". */
export function formatDuration(seconds: number | null | undefined): string {
  if (seconds == null || !Number.isFinite(seconds) || seconds <= 0) {
    return "—";
  }
  if (seconds < 1) {
    return "<1 s";
  }
  const s = Math.round(seconds);
  if (s < 60) {
    return `${s} s`;
  }
  const m = Math.floor(s / 60);
  if (m < 60) {
    return s % 60 ? `${m} m ${s % 60} s` : `${m} m`;
  }
  const h = Math.floor(m / 60);
  if (h < 48) {
    return m % 60 ? `${h} h ${m % 60} m` : `${h} h`;
  }
  const d = Math.floor(h / 24);
  return h % 24 ? `${d} d ${h % 24} h` : `${d} d`;
}

export function formatHours(seconds: number | null | undefined): string {
  const hours = (seconds ?? 0) / 3600;
  return hours >= 10 ? `${Math.round(hours)} h` : `${hours.toFixed(1)} h`;
}

export function formatSpeed(ratio: number | null | undefined): string {
  if (!ratio || !Number.isFinite(ratio) || ratio <= 0) {
    return "—";
  }
  return `${ratio >= 10 ? Math.round(ratio) : ratio.toFixed(1)}× real time`;
}

export function percent(part: number, whole: number): number {
  if (!whole || whole <= 0) {
    return 0;
  }
  return Math.min(100, Math.max(0, Math.round((part / whole) * 100)));
}

/** Worker status for the header badge. */
export function workerStatus(snapshot: MonitoringSnapshot): StatusItem {
  const w = snapshot.worker;
  if (!w.configured) {
    return { tone: "idle", label: "Not configured", detail: "Transcription runs without the separate WhisperX worker." };
  }
  if (!w.reachable) {
    return { tone: "bad", label: "Unreachable", detail: w.error || "The app cannot read worker metrics." };
  }
  if (!w.healthy) {
    return { tone: "bad", label: "Unhealthy", detail: "The worker reported a fault or is stopping." };
  }
  if (!w.accepting) {
    return { tone: "warn", label: "Draining", detail: "The worker is not accepting new jobs." };
  }
  if (w.running > 0) {
    return { tone: "ok", label: "Transcribing", detail: `${formatCount(w.running)} of ${formatCount(w.max_running)} slots busy, ${formatCount(w.queued)} waiting` };
  }
  return { tone: "ok", label: "Idle", detail: "Healthy and ready for work." };
}

const BACKLOG_WARN_SECONDS = 6 * 3600;
const LOW_DISK_BYTES = 10 * 1024 ** 3;
/** Failures in a row before summaries or a job count as failing. */
export const FAILURE_STREAK = 3;
const LONG_RUN_SHARE = 0.8;
const CPU_OFFLOAD_SHARE = 0.99;

const OUTCOME_LABELS: Record<string, string> = {
  succeeded: "Done",
  failed: "Failed",
  timed_out: "Timed out",
  token_limit: "Hit token limit",
  panicked: "Crashed",
  discarded: "Discarded",
};

const OUTCOME_TONES: Record<string, HealthTone> = {
  succeeded: "ok",
  failed: "bad",
  timed_out: "bad",
  token_limit: "bad",
  panicked: "bad",
  discarded: "idle",
};

const SOURCE_LABELS: Record<string, string> = {
  automatic: "Automatic",
  backfill: "Summarize waiting",
  resummarize: "Re-summarize",
  direct: "Direct",
};

export function summaryOutcomeLabel(outcome: string | undefined): string {
  return outcome ? (OUTCOME_LABELS[outcome] ?? outcome) : "In progress";
}

export function summaryOutcomeTone(outcome: string | undefined): HealthTone {
  return outcome ? (OUTCOME_TONES[outcome] ?? "warn") : "ok";
}

export function summarySourceLabel(source: string): string {
  return SOURCE_LABELS[source] ?? source;
}

/** Seconds from an ISO time to nowMs, never negative. */
export function elapsedSeconds(startedAt: string, nowMs: number): number {
  const started = Date.parse(startedAt);
  return Number.isFinite(started) ? Math.max(0, (nowMs - started) / 1000) : 0;
}

/** "12,000 → 900 tokens", or "" when the provider reported none. */
export function formatTokens(attempt: SummaryAttempt): string {
  if (!attempt.prompt_tokens && !attempt.completion_tokens) {
    return "";
  }
  return `${formatCount(attempt.prompt_tokens)} → ${formatCount(attempt.completion_tokens)} tokens`;
}

function summariesMisconfigured(s: MonitoringSummaries): string {
  if (!s.llm_enabled) {
    return "Summarization is on in Settings, but LLM_ENABLED is not true in the app environment.";
  }
  if (!s.api_key_configured) {
    return "Summarization is on in Settings, but LLM_API_KEY is empty. Any non-empty value works for Ollama.";
  }
  return "";
}

/** The last streak of failed attempts, newest first. */
function failureStreak(s: MonitoringSummaries): SummaryAttempt[] {
  const streak: SummaryAttempt[] = [];
  for (const attempt of s.recent) {
    if (attempt.outcome === "succeeded") {
      break;
    }
    if (attempt.outcome && attempt.outcome !== "discarded") {
      streak.push(attempt);
    }
  }
  return streak;
}

function failureAdvice(s: MonitoringSummaries, streak: SummaryAttempt[]): string {
  const last = streak[0];
  if (!last) {
    return "";
  }
  if (streak.every((a) => a.outcome === "token_limit")) {
    const thinking = streak.some((a) => (a.thinking_chars ?? 0) > 0);
    return thinking && !s.reasoning_effort
      ? `The model spent its ${formatCount(s.max_tokens)}-token output limit thinking. Set LLM_REASONING_EFFORT=none to turn thinking off.`
      : `Answers stop at the ${formatCount(s.max_tokens)}-token output limit (LLM_MAX_TOKENS).`;
  }
  if (streak.every((a) => a.outcome === "timed_out")) {
    return `The LLM did not answer within ${formatDuration(s.timeout_seconds)} (LLM_TIMEOUT_SECONDS). The server may be too slow for this model.`;
  }
  return `Last error: ${last.error || summaryOutcomeLabel(last.outcome)}`;
}

/** Summary pipeline status for the card badge. */
export function summaryStatus(snapshot: MonitoringSnapshot): StatusItem {
  const s = snapshot.summaries;
  if (!s) {
    return { tone: "idle", label: "Not reported", detail: "This app version does not report summary activity." };
  }
  if (!s.enabled) {
    return { tone: "idle", label: "Off", detail: "Enable summarization in Settings → AI summaries." };
  }
  const misconfigured = summariesMisconfigured(s);
  if (misconfigured) {
    return { tone: "bad", label: "Misconfigured", detail: misconfigured };
  }
  if (s.server && !s.server.reachable) {
    return { tone: "bad", label: "Server unreachable", detail: s.server.error || "The LLM server did not answer." };
  }
  if (s.consecutive_failures >= FAILURE_STREAK) {
    return { tone: "bad", label: "Failing", detail: `${formatCount(s.consecutive_failures)} attempts in a row failed.` };
  }
  if (s.in_flight.length) {
    const queued = s.queued ? `, ${formatCount(s.queued)} queued` : "";
    return { tone: "ok", label: "Summarizing", detail: `${formatCount(s.in_flight.length)} in progress${queued}` };
  }
  return { tone: "ok", label: "Idle", detail: "Ready. New transcripts are summarized automatically." };
}

function summaryAttention(snapshot: MonitoringSnapshot, nowMs: number): StatusItem[] {
  const s = snapshot.summaries;
  if (!s || !s.enabled) {
    return [];
  }
  const items: StatusItem[] = [];
  const misconfigured = summariesMisconfigured(s);
  if (misconfigured) {
    return [{ tone: "bad", label: "Summaries cannot run", detail: misconfigured }];
  }
  const server = s.server;
  if (server && !server.reachable) {
    items.push({ tone: "bad", label: "LLM server unreachable", detail: `${server.host ?? "The server"}: ${server.error ?? "no answer"}` });
  } else if (server?.model_listed === false) {
    items.push({ tone: "bad", label: "Summary model not on the LLM server", detail: `${s.model} is not in the server's model list. Pull it or change the model.` });
  }
  const ollama = server?.ollama;
  if (ollama?.loaded && ollama.gpu_share != null && ollama.gpu_share < CPU_OFFLOAD_SHARE) {
    items.push({
      tone: "warn",
      label: `${percent(1 - ollama.gpu_share, 1)}% of the model runs on the CPU`,
      detail: "It does not fit in GPU memory, so summaries are slow. Use a smaller model, or give the GPU more memory.",
    });
  }
  const streak = failureStreak(s);
  if (s.consecutive_failures >= FAILURE_STREAK) {
    items.push({ tone: "bad", label: `${formatCount(s.consecutive_failures)} summaries failed in a row`, detail: failureAdvice(s, streak) });
  }
  if (s.timeout_seconds > 0) {
    for (const attempt of s.in_flight) {
      const elapsed = elapsedSeconds(attempt.started_at, nowMs);
      if (elapsed > s.timeout_seconds * LONG_RUN_SHARE) {
        items.push({
          tone: "warn",
          label: "Summary close to its time limit",
          detail: `"${attempt.title}" has run ${formatDuration(elapsed)} of ${formatDuration(s.timeout_seconds)}.`,
        });
      }
    }
  }
  const processing = snapshot.queue.summaries_by_state?.processing ?? 0;
  if (processing > 0 && !s.in_flight.length && !s.job.running) {
    items.push({
      tone: "warn",
      label: `${formatCount(processing)} summary(s) stuck in processing`,
      detail: "Nothing is summarizing them. Summarize waiting episodes picks them up once they are stale.",
    });
  }
  return items;
}

/** Badge for one background job's latest run. */
export function jobStatus(job: JobRunSummary): StatusItem {
  if (job.running) {
    return { tone: "ok", label: "Running" };
  }
  switch (job.last_outcome) {
    case "succeeded":
      return { tone: "ok", label: "OK" };
    case "panicked":
      return { tone: "bad", label: "Crashed" };
    case "failed":
      return job.consecutive_failures >= FAILURE_STREAK ? { tone: "bad", label: "Failing" } : { tone: "warn", label: "Failed" };
    default:
      return { tone: "idle", label: "Not run" };
  }
}

/** Jobs that failed FAILURE_STREAK or more times in a row. */
export function failingJobs(jobs: JobRunSummary[] | undefined): JobRunSummary[] {
  return (jobs ?? []).filter((job) => job.consecutive_failures >= FAILURE_STREAK);
}

function formatSince(iso: string | undefined): string {
  if (!iso) {
    return "";
  }
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? "" : date.toLocaleString();
}

/** Conditions worth attention, most serious first. Empty when everything looks normal. */
export function attentionItems(snapshot: MonitoringSnapshot, nowMs = Date.now()): StatusItem[] {
  const items: StatusItem[] = [];
  const worker = workerStatus(snapshot);
  if (worker.tone === "bad") {
    items.push({ tone: "bad", label: `Worker ${worker.label.toLowerCase()}`, detail: worker.detail });
  }
  const failing = failingJobs(snapshot.jobs);
  for (const job of failing) {
    const since = formatSince(job.failing_since);
    items.push({
      tone: "bad",
      label: `${job.name} failing`,
      detail: `${formatCount(job.consecutive_failures)} runs in a row${since ? ` since ${since}` : ""}${job.last_error ? `: ${job.last_error}` : ""}`,
    });
  }
  items.push(...summaryAttention(snapshot, nowMs));
  const transcripts = snapshot.queue.transcripts_by_state ?? {};
  if ((transcripts.blocked ?? 0) > 0) {
    items.push({ tone: "bad", label: `${formatCount(transcripts.blocked)} transcript(s) blocked`, detail: "Will not retry automatically; see Work Queue." });
  }
  if ((transcripts.failed ?? 0) > 0) {
    items.push({ tone: "warn", label: `${formatCount(transcripts.failed)} transcript(s) failed`, detail: "Scheduled for retry." });
  }
  if (snapshot.queue.oldest_pending_seconds > BACKLOG_WARN_SECONDS) {
    items.push({ tone: "warn", label: "Transcription backlog is aging", detail: `Oldest waiting episode: ${formatDuration(snapshot.queue.oldest_pending_seconds)}` });
  }
  const downloadFailures = snapshot.since_start.downloads?.failed ?? 0;
  if (downloadFailures > 0) {
    items.push({ tone: "warn", label: `${formatCount(downloadFailures)} download failure(s) since restart`, detail: "Check Logs for the host and error." });
  }
  // Jobs already reported as failing above are left out.
  const failingNames = new Set(failing.map((job) => job.name));
  const jobFailures = Object.entries(snapshot.since_start.scheduled_job_failures ?? {}).filter(([job, n]) => n > 0 && !failingNames.has(job));
  if (jobFailures.length) {
    items.push({ tone: "warn", label: "Scheduled job failures since restart", detail: jobFailures.map(([job, n]) => `${job} ×${formatCount(n)}`).join(", ") });
  }
  if (snapshot.assets_free_bytes != null && snapshot.assets_free_bytes < LOW_DISK_BYTES) {
    items.push({ tone: "warn", label: "Low disk space for audio", detail: `${formatBytes(snapshot.assets_free_bytes)} free` });
  }
  for (const gpu of snapshot.worker.gpus ?? []) {
    if (gpu.memory_total_bytes > 0 && gpu.memory_used_bytes / gpu.memory_total_bytes > 0.95) {
      items.push({ tone: "warn", label: `GPU ${gpu.index} memory nearly full`, detail: `${formatBytes(gpu.memory_used_bytes)} of ${formatBytes(gpu.memory_total_bytes)}` });
    }
  }
  return items;
}

const STAGE_ORDER = ["queued", "starting", "model_loaded", "audio_loaded", "resuming", "transcribing", "aligning", "diarizing"];
// Each stage's time runs until the next progress event, so "starting" covers
// process start-up and model loading, "model_loaded" covers reading the audio.
const STAGE_LABELS: Record<string, string> = {
  starting: "Start-up & model load",
  model_loaded: "Loading audio",
  audio_loaded: "Preparing transcription",
  resuming: "Resuming checkpoint",
  transcribing: "Transcribing",
  aligning: "Aligning words",
  diarizing: "Speaker diarization",
};

/** Average time per pipeline stage, in pipeline order, excluding terminal stages. */
export function stageTimings(avg: Record<string, number> | undefined): { stage: string; label: string; seconds: number }[] {
  return Object.entries(avg ?? {})
    .filter(([stage, seconds]) => seconds > 0 && STAGE_ORDER.includes(stage) && stage !== "queued")
    .sort(([a], [b]) => STAGE_ORDER.indexOf(a) - STAGE_ORDER.indexOf(b))
    .map(([stage, seconds]) => ({ stage, label: STAGE_LABELS[stage] ?? stage, seconds }));
}
