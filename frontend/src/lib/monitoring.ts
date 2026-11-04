import type { MonitoringSnapshot } from "../types/api";

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

/** Conditions worth attention, most serious first. Empty when everything looks normal. */
export function attentionItems(snapshot: MonitoringSnapshot): StatusItem[] {
  const items: StatusItem[] = [];
  const worker = workerStatus(snapshot);
  if (worker.tone === "bad") {
    items.push({ tone: "bad", label: `Worker ${worker.label.toLowerCase()}`, detail: worker.detail });
  }
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
  const jobFailures = Object.entries(snapshot.since_start.scheduled_job_failures ?? {}).filter(([, n]) => n > 0);
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
