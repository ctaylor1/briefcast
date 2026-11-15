import { describe, it } from "node:test";
import assert from "node:assert/strict";
import {
  attentionItems,
  elapsedSeconds,
  failingJobs,
  formatBytes,
  formatDuration,
  formatHours,
  formatSpeed,
  formatTokens,
  jobStatus,
  percent,
  stageTimings,
  summaryOutcomeLabel,
  summaryStatus,
  workerStatus,
} from "./monitoring.ts";
import type { JobRunSummary, MonitoringSnapshot, MonitoringSummaries, SummaryAttempt } from "../types/api";

function snapshot(overrides: Partial<MonitoringSnapshot> = {}): MonitoringSnapshot {
  return {
    generated_at: "2026-11-04T12:00:00Z",
    queue: { episodes_by_download_state: {}, transcripts_by_state: { available: 3 }, oldest_pending_seconds: 0 },
    windows: [],
    since_start: {
      started_at: "2026-11-04T10:00:00Z",
      downloads: {},
      download_bytes: 0,
      avg_download_seconds: 0,
      transcriptions: {},
      transcribed_audio_seconds: 0,
      avg_processing_seconds: 0,
      avg_end_to_end_seconds: 0,
      avg_worker_queue_seconds: 0,
      avg_speed_ratio: 0,
      scheduled_job_failures: {},
      transcript_export_failures: 0,
    },
    worker: {
      configured: true,
      reachable: true,
      healthy: true,
      accepting: true,
      running: 0,
      queued: 0,
      max_running: 1,
      state_free_bytes: 0,
      jobs_finished: {},
      avg_speed_ratio: 0,
      avg_stage_seconds: {},
      transcribed_audio_seconds: 0,
      gpus: [],
    },
    assets_free_bytes: 500 * 1024 ** 3,
    ...overrides,
  };
}

describe("Monitoring formatting", () => {
  it("formats bytes, durations, hours, speed and percentages", () => {
    assert.equal(formatBytes(512), "512 B");
    assert.equal(formatBytes(1536), "1.5 KB");
    assert.equal(formatBytes(12282 * 1024 * 1024), "12.0 GB");
    assert.equal(formatBytes(null), "—");
    assert.equal(formatDuration(0.3), "<1 s");
    assert.equal(formatDuration(45), "45 s");
    assert.equal(formatDuration(200), "3 m 20 s");
    assert.equal(formatDuration(7500), "2 h 5 m");
    assert.equal(formatDuration(3 * 86400 + 4 * 3600), "3 d 4 h");
    assert.equal(formatDuration(0), "—");
    assert.equal(formatHours(5.4 * 3600), "5.4 h");
    assert.equal(formatHours(25 * 3600), "25 h");
    assert.equal(formatSpeed(38.7), "39× real time");
    assert.equal(formatSpeed(2.25), "2.3× real time");
    assert.equal(formatSpeed(0), "—");
    assert.equal(percent(9, 12), 75);
    assert.equal(percent(1, 0), 0);
  });
});

describe("Monitoring status", () => {
  it("reports worker states from configuration to activity", () => {
    assert.equal(workerStatus(snapshot({ worker: { ...snapshot().worker, configured: false } })).tone, "idle");
    assert.equal(workerStatus(snapshot({ worker: { ...snapshot().worker, reachable: false, error: "x" } })).label, "Unreachable");
    assert.equal(workerStatus(snapshot({ worker: { ...snapshot().worker, healthy: false } })).tone, "bad");
    assert.equal(workerStatus(snapshot({ worker: { ...snapshot().worker, accepting: false } })).label, "Draining");
    const busy = workerStatus(snapshot({ worker: { ...snapshot().worker, running: 1, queued: 2 } }));
    assert.equal(busy.label, "Transcribing");
    assert.equal(busy.detail, "1 of 1 slots busy, 2 waiting");
    assert.equal(workerStatus(snapshot()).label, "Idle");
  });

  it("raises attention items only for real problems", () => {
    assert.deepEqual(attentionItems(snapshot()), []);
    const items = attentionItems(
      snapshot({
        queue: { episodes_by_download_state: {}, transcripts_by_state: { blocked: 2, failed: 1 }, oldest_pending_seconds: 8 * 3600 },
        since_start: { ...snapshot().since_start, downloads: { failed: 6 }, scheduled_job_failures: { RefreshEpisodes: 1, Backup: 0 } },
        worker: { ...snapshot().worker, gpus: [{ index: "0", memory_used_bytes: 11.9e9, memory_total_bytes: 12e9, utilization: 0.9 }] },
        assets_free_bytes: 2 * 1024 ** 3,
      }),
    );
    assert.deepEqual(
      items.map((i) => i.label),
      [
        "2 transcript(s) blocked",
        "1 transcript(s) failed",
        "Transcription backlog is aging",
        "6 download failure(s) since restart",
        "Scheduled job failures since restart",
        "Low disk space for audio",
        "GPU 0 memory nearly full",
      ],
    );
    assert.equal(items[0].tone, "bad");
    assert.equal(items[4].detail, "RefreshEpisodes ×1");
  });

  it("orders stage timings by pipeline order and drops unknown stages", () => {
    assert.deepEqual(
      stageTimings({ aligning: 12, transcribing: 300, starting: 9, succeeded: 1, other: 4, queued: 2 }).map((s) => s.label),
      ["Start-up & model load", "Transcribing", "Aligning words"],
    );
  });
});

const NOW = Date.parse("2026-11-15T12:00:00Z");

function summaries(overrides: Partial<MonitoringSummaries> = {}): MonitoringSummaries {
  return {
    enabled: true,
    llm_enabled: true,
    api_key_configured: true,
    provider: "ollama",
    model: "qwen3.6:27b",
    host: "192.168.1.50:11434",
    max_tokens: 2048,
    timeout_seconds: 900,
    concurrency: 1,
    job: { running: false, total: 0, completed: 0, failed: 0 },
    queued: 0,
    in_flight: [],
    recent: [],
    outcomes: {},
    consecutive_failures: 0,
    ...overrides,
  };
}

function attempt(overrides: Partial<SummaryAttempt> = {}): SummaryAttempt {
  return { episode_id: "e1", title: "Episode", source: "automatic", started_at: "2026-11-15T11:50:00Z", duration_ms: 1000, ...overrides };
}

function job(overrides: Partial<JobRunSummary> = {}): JobRunSummary {
  return { name: "TranscribePendingEpisodes", running: false, last_duration_ms: 10, consecutive_failures: 0, runs: 1, failures: 0, ...overrides };
}

describe("Summary monitoring", () => {
  it("reports the summary pipeline state from configuration to activity", () => {
    assert.equal(summaryStatus(snapshot()).label, "Not reported");
    assert.equal(summaryStatus(snapshot({ summaries: summaries({ enabled: false }) })).label, "Off");
    const noKey = summaryStatus(snapshot({ summaries: summaries({ api_key_configured: false }) }));
    assert.equal(noKey.label, "Misconfigured");
    assert.match(noKey.detail ?? "", /LLM_API_KEY/);
    assert.equal(summaryStatus(snapshot({ summaries: summaries({ llm_enabled: false }) })).tone, "bad");
    const unreachable = summaries({ server: { checked_at: "", reachable: false, latency_ms: 5000, error: "connection refused" } });
    assert.equal(summaryStatus(snapshot({ summaries: unreachable })).label, "Server unreachable");
    assert.equal(summaryStatus(snapshot({ summaries: summaries({ consecutive_failures: 3 }) })).label, "Failing");
    const busy = summaryStatus(snapshot({ summaries: summaries({ in_flight: [attempt()], queued: 2 }) }));
    assert.equal(busy.label, "Summarizing");
    assert.equal(busy.detail, "1 in progress, 2 queued");
    assert.equal(summaryStatus(snapshot({ summaries: summaries() })).label, "Idle");
  });

  it("explains a token-limit streak caused by thinking", () => {
    const streak = [1, 2, 3].map((n) => attempt({ episode_id: `e${n}`, outcome: "token_limit", thinking_chars: 9000, error: "limit" }));
    const items = attentionItems(snapshot({ summaries: summaries({ recent: streak, consecutive_failures: 3 }) }), NOW);
    const failing = items.find((i) => i.label === "3 summaries failed in a row");
    assert.ok(failing);
    assert.equal(failing.tone, "bad");
    assert.match(failing.detail ?? "", /LLM_REASONING_EFFORT=none/);

    const withEffort = attentionItems(snapshot({ summaries: summaries({ recent: streak, consecutive_failures: 3, reasoning_effort: "none" }) }), NOW);
    assert.doesNotMatch(withEffort.find((i) => i.label.includes("failed in a row"))?.detail ?? "", /LLM_REASONING_EFFORT/);
  });

  it("explains a timeout streak and flags long, stuck and slow summaries", () => {
    const timeouts = [1, 2, 3].map((n) => attempt({ episode_id: `e${n}`, outcome: "timed_out" }));
    const items = attentionItems(
      snapshot({
        queue: { episodes_by_download_state: {}, transcripts_by_state: {}, oldest_pending_seconds: 0, summaries_by_state: { processing: 2 } },
        summaries: summaries({
          recent: timeouts,
          consecutive_failures: 3,
          in_flight: [attempt({ title: "Long one", started_at: "2026-11-15T11:46:00Z" })],
          server: { checked_at: "", reachable: true, latency_ms: 20, model_listed: true, ollama: { loaded: true, gpu_share: 0.8 } },
        }),
      }),
      NOW,
    );
    assert.deepEqual(
      items.map((i) => i.label),
      ["20% of the model runs on the CPU", "3 summaries failed in a row", "Summary close to its time limit"],
    );
    assert.match(items[1].detail ?? "", /15 m/);
    assert.match(items[2].detail ?? "", /"Long one" has run 14 m of 15 m/);

    const stuck = attentionItems(
      snapshot({
        queue: { episodes_by_download_state: {}, transcripts_by_state: {}, oldest_pending_seconds: 0, summaries_by_state: { processing: 2 } },
        summaries: summaries(),
      }),
      NOW,
    );
    assert.deepEqual(stuck.map((i) => i.label), ["2 summary(s) stuck in processing"]);
  });

  it("reports an unreachable server, a missing model and misconfiguration", () => {
    const down = attentionItems(snapshot({ summaries: summaries({ server: { checked_at: "", host: "h:1", reachable: false, latency_ms: 0, error: "connection refused" } }) }), NOW);
    assert.deepEqual(down.map((i) => [i.label, i.detail]), [["LLM server unreachable", "h:1: connection refused"]]);
    const missing = attentionItems(snapshot({ summaries: summaries({ server: { checked_at: "", reachable: true, latency_ms: 1, model_listed: false } }) }), NOW);
    assert.equal(missing[0].label, "Summary model not on the LLM server");
    const off = attentionItems(snapshot({ summaries: summaries({ llm_enabled: false, consecutive_failures: 9 }) }), NOW);
    assert.deepEqual(off.map((i) => i.label), ["Summaries cannot run"]);
    assert.deepEqual(attentionItems(snapshot({ summaries: summaries({ enabled: false, consecutive_failures: 9 }) }), NOW), []);
  });

  it("names a failing job with its reason once", () => {
    const jobs = [
      job({ consecutive_failures: 1478, failing_since: "2026-11-13T01:58:00Z", last_error: "INVALID_CONFIG: Worker token file unavailable" }),
      job({ name: "RefreshEpisodes", consecutive_failures: 1 }),
    ];
    assert.deepEqual(failingJobs(jobs).map((j) => j.name), ["TranscribePendingEpisodes"]);
    const items = attentionItems(
      snapshot({ jobs, since_start: { ...snapshot().since_start, scheduled_job_failures: { TranscribePendingEpisodes: 1478, RefreshEpisodes: 1 } } }),
      NOW,
    );
    assert.equal(items[0].label, "TranscribePendingEpisodes failing");
    assert.equal(items[0].tone, "bad");
    assert.match(items[0].detail ?? "", /^1,478 runs in a row since .+: INVALID_CONFIG: Worker token file unavailable$/);
    const sinceRestart = items.find((i) => i.label === "Scheduled job failures since restart");
    assert.equal(sinceRestart?.detail, "RefreshEpisodes ×1");
  });

  it("labels job runs", () => {
    assert.equal(jobStatus(job({ running: true })).label, "Running");
    assert.equal(jobStatus(job({ last_outcome: "succeeded" })).tone, "ok");
    assert.equal(jobStatus(job({ last_outcome: "failed", consecutive_failures: 1 })).label, "Failed");
    assert.equal(jobStatus(job({ last_outcome: "failed", consecutive_failures: 3 })).label, "Failing");
    assert.equal(jobStatus(job({ last_outcome: "panicked" })).tone, "bad");
  });

  it("formats attempt details", () => {
    assert.equal(summaryOutcomeLabel("token_limit"), "Hit token limit");
    assert.equal(summaryOutcomeLabel(undefined), "In progress");
    assert.equal(formatTokens(attempt({ prompt_tokens: 12000, completion_tokens: 900 })), "12,000 → 900 tokens");
    assert.equal(formatTokens(attempt()), "");
    assert.equal(elapsedSeconds("2026-11-15T11:59:00Z", NOW), 60);
    assert.equal(elapsedSeconds("not a date", NOW), 0);
  });
});
