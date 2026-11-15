import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount, type VueWrapper } from "@vue/test-utils";
import MonitoringPanel from "./MonitoringPanel.vue";
import { fakeServer, flushPromises, type FakeServer } from "../../test-support/http";
import type { MonitoringSnapshot } from "../../types/api";

let server: FakeServer | null = null;
let wrapper: VueWrapper | null = null;
afterEach(() => {
  wrapper?.unmount();
  server?.restore();
  document.body.innerHTML = "";
});

function snapshot(): MonitoringSnapshot {
  const now = new Date();
  const minutesAgo = (m: number) => new Date(now.getTime() - m * 60_000).toISOString();
  return {
    generated_at: now.toISOString(),
    queue: {
      episodes_by_download_state: { downloaded: 10 },
      transcripts_by_state: { available: 8 },
      oldest_pending_seconds: 0,
      summaries_by_state: { available: 3, processing: 1, failed: 4, waiting: 95 },
    },
    windows: [{ label: "Last 24 hours", hours: 24, downloads: 2, transcripts: 2, transcribed_audio_seconds: 3600, summaries: 1 }],
    since_start: {
      started_at: minutesAgo(600),
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
      summary_attempts: { succeeded: 1, token_limit: 3 },
      avg_summary_seconds: 700,
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
    summaries: {
      enabled: true,
      llm_enabled: true,
      api_key_configured: true,
      provider: "ollama",
      model: "qwen3.6:27b",
      host: "192.168.1.50:11434",
      max_tokens: 2048,
      timeout_seconds: 900,
      concurrency: 1,
      job: { running: true, total: 4, completed: 1, failed: 1 },
      queued: 2,
      server: {
        checked_at: now.toISOString(),
        host: "192.168.1.50:11434",
        reachable: true,
        latency_ms: 12,
        model_listed: true,
        ollama: { loaded: true, context_length: 65536, gpu_share: 0.8 },
      },
      in_flight: [{ episode_id: "e9", title: "Washington Is Taking Control of AI", source: "automatic", started_at: minutesAgo(3), duration_ms: 0 }],
      recent: [
        {
          episode_id: "e1",
          title: "7 Things AI-Native Companies Do Differently",
          source: "backfill",
          started_at: minutesAgo(20),
          finished_at: minutesAgo(7),
          duration_ms: 780_000,
          outcome: "token_limit",
          prompt_tokens: 7000,
          completion_tokens: 2048,
          thinking_chars: 9000,
          error: "LLM response reached the output token limit (max_tokens=2048)",
        },
      ],
      outcomes: { token_limit: 3, succeeded: 1 },
      consecutive_failures: 3,
    },
    jobs: [
      {
        name: "TranscribePendingEpisodes",
        running: false,
        last_finished_at: minutesAgo(1),
        last_duration_ms: 40,
        last_outcome: "failed",
        last_error: "INVALID_CONFIG: Worker token file unavailable",
        consecutive_failures: 5,
        failing_since: minutesAgo(5),
        runs: 5,
        failures: 5,
      },
    ],
  };
}

async function mountPanel(): Promise<void> {
  server = fakeServer({
    "GET /settings/monitoring": () => snapshot(),
    "POST /settings/backfill-summaries/stop": () => ({ message: "summary job stopping", status: { running: true, stopping: true } }),
  });
  wrapper = mount(MonitoringPanel, { attachTo: document.body });
  await flushPromises();
}

describe("MonitoringPanel summaries", () => {
  it("asks for the LLM server check and shows summary activity, results and jobs", async () => {
    await mountPanel();
    const first = server!.requests.find((r) => r.url === "/settings/monitoring");
    assert.deepEqual(first?.params, { checkLLM: 1 });

    const text = wrapper!.text();
    for (const expected of [
      "AI summaries",
      "Washington Is Taking Control of AI",
      "3 m",
      "of 15 m",
      "Summary job: 1 of 4 done, 1 failed, 2 queued next",
      "ollama · qwen3.6:27b",
      "192.168.1.50:11434 · 12 ms",
      "80% · context 65,536",
      "Model default",
      "2,048 tokens · 15 m",
      "95 · 4",
      "Recent summaries",
      "7 Things AI-Native Companies Do Differently",
      "Hit token limit",
      "7,000 → 2,048 tokens",
      "Summarize waiting",
      "Background jobs",
      "5 in a row since",
      "INVALID_CONFIG: Worker token file unavailable",
    ]) {
      assert.ok(text.includes(expected), `missing "${expected}"`);
    }
    const attention = wrapper!.get('[aria-label="Needs attention"]').text();
    assert.match(attention, /TranscribePendingEpisodes failing/);
    assert.match(attention, /20% of the model runs on the CPU/);
    assert.match(attention, /3 summaries failed in a row/);
    assert.match(attention, /LLM_REASONING_EFFORT=none/);
    const headers = wrapper!.findAll(".monitoring-table thead th").map((th) => th.text());
    assert.ok(headers.includes("Summaries"));
  });

  it("stops the summary job from Monitoring", async () => {
    await mountPanel();
    const stop = wrapper!.findAll("button").find((b) => b.text().trim() === "Stop");
    assert.ok(stop, "Stop button");
    await stop!.trigger("click");
    await flushPromises();
    assert.ok(server!.requests.some((r) => r.method === "POST" && r.url === "/settings/backfill-summaries/stop"));
  });
});
