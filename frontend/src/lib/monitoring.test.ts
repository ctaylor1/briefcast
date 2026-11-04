import { describe, it } from "node:test";
import assert from "node:assert/strict";
import {
  attentionItems,
  formatBytes,
  formatDuration,
  formatHours,
  formatSpeed,
  percent,
  stageTimings,
  workerStatus,
} from "./monitoring.ts";
import type { MonitoringSnapshot } from "../types/api";

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
