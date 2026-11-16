<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { getErrorMessage, settingsApi } from "../../lib/api";
import {
  attentionItems,
  elapsedSeconds,
  formatBytes,
  formatCount,
  formatDuration,
  formatHours,
  formatSpeed,
  formatTokens,
  jobStatus,
  percent,
  stageTimings,
  summaryOutcomeLabel,
  summaryOutcomeTone,
  summarySourceLabel,
  summaryStatus,
  workerStatus,
  type HealthTone,
} from "../../lib/monitoring";
import type { MonitoringSnapshot, SummaryAttempt } from "../../types/api";
import UiAlert from "../ui/UiAlert.vue";
import UiBadge from "../ui/UiBadge.vue";
import UiButton from "../ui/UiButton.vue";
import UiCard from "../ui/UiCard.vue";

const REFRESH_MS = 15_000;

const snapshot = ref<MonitoringSnapshot | null>(null);
const loading = ref(false);
const errorMessage = ref("");
const stopMessage = ref("");
const isStoppingSummaries = ref(false);
const isSendingTest = ref(false);
const testMessage = ref<{ tone: "success" | "danger"; text: string } | null>(null);
// Ticks every second while a summary is in progress, so its elapsed time moves
// between the 15-second refreshes without extra requests.
const now = ref(Date.now());
let timer: number | undefined;
let ticker: number | undefined;

const badgeTone: Record<HealthTone, "success" | "warning" | "danger" | "neutral"> = {
  ok: "success",
  warn: "warning",
  bad: "danger",
  idle: "neutral",
};

const status = computed(() => (snapshot.value ? workerStatus(snapshot.value) : null));
const attention = computed(() => (snapshot.value ? attentionItems(snapshot.value, now.value) : []));
const summaries = computed(() => snapshot.value?.summaries);
const summary = computed(() => (snapshot.value ? summaryStatus(snapshot.value) : null));
const summaryCounts = computed(() => snapshot.value?.queue.summaries_by_state ?? {});
const recentSummaries = computed(() => summaries.value?.recent ?? []);
const jobs = computed(() => snapshot.value?.jobs ?? []);
const ollama = computed(() => summaries.value?.server?.ollama);
const serverLabel = computed(() => {
  const s = summaries.value;
  if (!s) {
    return "—";
  }
  const server = s.server;
  if (!server) {
    return s.host || "—";
  }
  if (!server.reachable) {
    return `${server.host ?? s.host ?? ""} · unreachable`;
  }
  const model = server.model_listed === false ? " · model missing" : "";
  return `${server.host ?? s.host ?? ""} · ${formatCount(server.latency_ms)} ms${model}`;
});

function attemptElapsed(attempt: SummaryAttempt): number {
  return elapsedSeconds(attempt.started_at, now.value);
}

function attemptTime(attempt: SummaryAttempt): string {
  const at = attempt.finished_at ?? attempt.started_at;
  const date = new Date(at);
  return Number.isNaN(date.getTime()) ? "" : date.toLocaleString();
}

function timeLabel(iso: string | undefined): string {
  if (!iso) {
    return "—";
  }
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleString();
}

const notifications = computed(() => snapshot.value?.notifications);

async function sendTestNotification(): Promise<void> {
  isSendingTest.value = true;
  testMessage.value = null;
  try {
    await settingsApi.sendTestNotification();
    testMessage.value = { tone: "success", text: "Test notification sent. Check that it arrived." };
  } catch (error) {
    testMessage.value = { tone: "danger", text: getErrorMessage(error, "Test notification failed.") };
  } finally {
    isSendingTest.value = false;
  }
}

async function stopSummaries(): Promise<void> {
  isStoppingSummaries.value = true;
  stopMessage.value = "";
  try {
    await settingsApi.stopSummaryJob();
    await refresh();
  } catch (error) {
    stopMessage.value = getErrorMessage(error, "Failed to stop the summary job.");
  } finally {
    isStoppingSummaries.value = false;
  }
}

watch(
  () => summaries.value?.in_flight.length ?? 0,
  (inFlight) => {
    if (inFlight > 0 && ticker === undefined) {
      now.value = Date.now();
      ticker = window.setInterval(() => {
        now.value = Date.now();
      }, 1000);
    } else if (inFlight === 0 && ticker !== undefined) {
      window.clearInterval(ticker);
      ticker = undefined;
    }
  },
);
const stages = computed(() => stageTimings(snapshot.value?.worker.avg_stage_seconds));
const transcripts = computed(() => snapshot.value?.queue.transcripts_by_state ?? {});
const episodes = computed(() => snapshot.value?.queue.episodes_by_download_state ?? {});
const since = computed(() => snapshot.value?.since_start);
const updatedLabel = computed(() =>
  snapshot.value ? new Date(snapshot.value.generated_at).toLocaleTimeString() : "",
);
const sinceLabel = computed(() => (since.value ? new Date(since.value.started_at).toLocaleString() : ""));
const finishedJobs = computed(() => {
  const jobs = snapshot.value?.worker.jobs_finished ?? {};
  return { succeeded: jobs.succeeded ?? 0, failed: jobs.failed ?? 0, cancelled: jobs.cancelled ?? 0 };
});

async function refresh(): Promise<void> {
  if (loading.value) {
    return;
  }
  loading.value = true;
  try {
    snapshot.value = await settingsApi.getMonitoring({ checkLLM: true });
    now.value = Date.now();
    errorMessage.value = "";
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Failed to load monitoring data.");
  } finally {
    loading.value = false;
  }
}

function schedule(): void {
  timer = window.setTimeout(async () => {
    if (document.visibilityState === "visible") {
      await refresh();
    }
    schedule();
  }, REFRESH_MS);
}

onMounted(async () => {
  await refresh();
  schedule();
});

onBeforeUnmount(() => {
  if (timer !== undefined) {
    window.clearTimeout(timer);
  }
  if (ticker !== undefined) {
    window.clearInterval(ticker);
    ticker = undefined;
  }
});
</script>

<template>
  <div class="stack-4">
    <UiCard padding="lg" class="stack-3">
      <div class="surface-row surface-row--between monitoring-header">
        <div class="stack-1">
          <h3 class="settings-section-title">Pipeline monitoring</h3>
          <p class="section-subtitle">
            Downloads, transcription, summaries and worker health. Refreshes every 15 seconds{{ updatedLabel ? ` · updated ${updatedLabel}` : "" }}.
          </p>
        </div>
        <UiButton size="sm" variant="outline" :disabled="loading" @click="refresh">
          {{ loading ? "Refreshing..." : "Refresh" }}
        </UiButton>
      </div>

      <UiAlert v-if="errorMessage" tone="danger">{{ errorMessage }}</UiAlert>
      <p v-else-if="!snapshot" class="meta-text" role="status">Loading monitoring data...</p>

      <template v-if="snapshot">
        <ul v-if="attention.length" class="monitoring-attention" aria-label="Needs attention">
          <li v-for="item in attention" :key="item.label" :class="`monitoring-attention__item monitoring-attention__item--${item.tone}`">
            <strong>{{ item.label }}</strong>
            <span v-if="item.detail" class="meta-text">{{ item.detail }}</span>
          </li>
        </ul>
        <p v-else class="meta-text monitoring-allclear">No problems detected.</p>
      </template>
    </UiCard>

    <template v-if="snapshot && status && since">
      <div class="monitoring-grid">
        <UiCard padding="lg" class="stack-2">
          <div class="surface-row surface-row--between">
            <h4 class="monitoring-card__title">WhisperX worker</h4>
            <UiBadge :tone="badgeTone[status.tone]">{{ status.label }}</UiBadge>
          </div>
          <p class="meta-text">{{ status.detail }}</p>
          <dl v-if="snapshot.worker.reachable" class="monitoring-stats">
            <div><dt>Engine</dt><dd>{{ snapshot.worker.engine_profile || "—" }} ({{ snapshot.worker.device || "?" }})</dd></div>
            <div><dt>Jobs finished</dt><dd>{{ formatCount(finishedJobs.succeeded) }} ok · {{ formatCount(finishedJobs.failed) }} failed</dd></div>
            <div><dt>Average speed</dt><dd>{{ formatSpeed(snapshot.worker.avg_speed_ratio) }}</dd></div>
            <div><dt>Audio transcribed</dt><dd>{{ formatHours(snapshot.worker.transcribed_audio_seconds) }}</dd></div>
            <div><dt>State volume free</dt><dd>{{ formatBytes(snapshot.worker.state_free_bytes) }}</dd></div>
          </dl>
          <p v-if="snapshot.worker.reachable" class="meta-text">Worker counts reset when the worker restarts.</p>
        </UiCard>

        <UiCard v-if="summaries && summary" padding="lg" class="stack-2">
          <div class="surface-row surface-row--between">
            <h4 class="monitoring-card__title">AI summaries</h4>
            <UiBadge :tone="badgeTone[summary.tone]">{{ summary.label }}</UiBadge>
          </div>
          <p class="meta-text">{{ summary.detail }}</p>
          <div v-for="attempt in summaries.in_flight" :key="`${attempt.episode_id}-${attempt.started_at}`" class="stack-1">
            <div class="monitoring-meter-label">
              <span class="monitoring-ellipsis" :title="attempt.title">{{ attempt.title }}</span>
              <span>{{ formatDuration(attemptElapsed(attempt)) }} of {{ formatDuration(summaries.timeout_seconds) }}</span>
            </div>
            <div
              class="monitoring-meter"
              role="meter"
              :aria-valuenow="percent(attemptElapsed(attempt), summaries.timeout_seconds)"
              aria-valuemin="0"
              aria-valuemax="100"
              :aria-label="`Time used summarizing ${attempt.title}`"
            >
              <span :style="{ width: `${percent(attemptElapsed(attempt), summaries.timeout_seconds)}%` }"></span>
            </div>
          </div>
          <div v-if="summaries.job.running" class="surface-row surface-row--between monitoring-job-row">
            <span class="meta-text" role="status">
              {{ summaries.job.stopping ? "Stopping after the episode in progress." : "Summary job:" }}
              {{ formatCount(summaries.job.completed) }} of {{ formatCount(summaries.job.total) }} done,
              {{ formatCount(summaries.job.failed) }} failed{{ summaries.queued ? `, ${formatCount(summaries.queued)} queued next` : "" }}
            </span>
            <UiButton size="sm" variant="outline" :disabled="summaries.job.stopping || isStoppingSummaries" @click="stopSummaries">
              {{ summaries.job.stopping || isStoppingSummaries ? "Stopping..." : "Stop" }}
            </UiButton>
          </div>
          <UiAlert v-if="stopMessage" tone="warning">{{ stopMessage }}</UiAlert>
          <dl class="monitoring-stats">
            <div><dt>Model</dt><dd>{{ summaries.provider }} · {{ summaries.model }}</dd></div>
            <div><dt>LLM server</dt><dd>{{ serverLabel }}</dd></div>
            <div v-if="ollama?.loaded">
              <dt>In GPU memory</dt>
              <dd>{{ percent(ollama.gpu_share ?? 0, 1) }}% · context {{ formatCount(ollama.context_length) }}</dd>
            </div>
            <div><dt>Thinking</dt><dd>{{ summaries.reasoning_effort ? `reasoning_effort=${summaries.reasoning_effort}` : "Model default" }}</dd></div>
            <div><dt>Output limit · time limit</dt><dd>{{ formatCount(summaries.max_tokens) }} tokens · {{ formatDuration(summaries.timeout_seconds) }}</dd></div>
            <div><dt>Waiting · failed</dt><dd>{{ formatCount(summaryCounts.waiting) }} · {{ formatCount(summaryCounts.failed) }}</dd></div>
            <div><dt>Summarized</dt><dd>{{ formatCount(summaryCounts.available) }}</dd></div>
            <div v-if="since?.summary_attempts">
              <dt>Since restart: done · failed</dt>
              <dd>
                {{ formatCount(since.summary_attempts.succeeded) }} ·
                {{ formatCount((since.summary_attempts.failed ?? 0) + (since.summary_attempts.timed_out ?? 0) + (since.summary_attempts.token_limit ?? 0) + (since.summary_attempts.panicked ?? 0)) }}
                (avg {{ formatDuration(since.avg_summary_seconds) }})
              </dd>
            </div>
          </dl>
        </UiCard>

        <UiCard v-if="recentSummaries.length" padding="lg" class="stack-2">
          <h4 class="monitoring-card__title">Recent summaries</h4>
          <ul class="monitoring-list" aria-label="Recent summaries">
            <li v-for="attempt in recentSummaries" :key="`${attempt.episode_id}-${attempt.started_at}`" class="monitoring-list__item">
              <div class="surface-row surface-row--between monitoring-list__head">
                <span class="monitoring-ellipsis monitoring-list__title" :title="attempt.title">{{ attempt.title }}</span>
                <UiBadge :tone="badgeTone[summaryOutcomeTone(attempt.outcome)]">{{ summaryOutcomeLabel(attempt.outcome) }}</UiBadge>
              </div>
              <span class="meta-text">
                {{ attemptTime(attempt) }} · {{ formatDuration(attempt.duration_ms / 1000) }} · {{ summarySourceLabel(attempt.source) }}{{ formatTokens(attempt) ? ` · ${formatTokens(attempt)}` : "" }}
              </span>
              <span v-if="attempt.error" class="meta-text monitoring-list__error">{{ attempt.error }}</span>
            </li>
          </ul>
          <p class="meta-text">Since the app started; the newest {{ formatCount(recentSummaries.length) }}.</p>
        </UiCard>

        <UiCard v-if="snapshot.worker.gpus.length" padding="lg" class="stack-2">
          <h4 class="monitoring-card__title">GPU</h4>
          <div v-for="gpu in snapshot.worker.gpus" :key="gpu.index" class="stack-2">
            <div class="monitoring-meter-label">
              <span>GPU {{ gpu.index }} memory</span>
              <span>{{ formatBytes(gpu.memory_used_bytes) }} / {{ formatBytes(gpu.memory_total_bytes) }}</span>
            </div>
            <div
              class="monitoring-meter"
              role="meter"
              :aria-valuenow="percent(gpu.memory_used_bytes, gpu.memory_total_bytes)"
              aria-valuemin="0"
              aria-valuemax="100"
              :aria-label="`GPU ${gpu.index} memory used`"
            >
              <span :style="{ width: `${percent(gpu.memory_used_bytes, gpu.memory_total_bytes)}%` }"></span>
            </div>
            <div class="monitoring-meter-label">
              <span>Utilization</span>
              <span>{{ percent(gpu.utilization, 1) }}%</span>
            </div>
            <div class="monitoring-meter" role="meter" :aria-valuenow="percent(gpu.utilization, 1)" aria-valuemin="0" aria-valuemax="100" :aria-label="`GPU ${gpu.index} utilization`">
              <span :style="{ width: `${percent(gpu.utilization, 1)}%` }"></span>
            </div>
          </div>
          <p class="meta-text">Includes memory used by other apps on this GPU.</p>
        </UiCard>

        <UiCard padding="lg" class="stack-2">
          <h4 class="monitoring-card__title">Queue now</h4>
          <dl class="monitoring-stats">
            <div><dt>Waiting for transcript</dt><dd>{{ formatCount(transcripts.pending_whisperx) }}</dd></div>
            <div><dt>Transcribing</dt><dd>{{ formatCount(transcripts.processing) }}</dd></div>
            <div><dt>Failed (will retry)</dt><dd>{{ formatCount(transcripts.failed) }}</dd></div>
            <div><dt>Blocked</dt><dd>{{ formatCount(transcripts.blocked) }}</dd></div>
            <div><dt>Oldest waiting</dt><dd>{{ formatDuration(snapshot.queue.oldest_pending_seconds) }}</dd></div>
            <div><dt>Downloads in progress</dt><dd>{{ formatCount(episodes.downloading) }}</dd></div>
            <div><dt>Transcripts available</dt><dd>{{ formatCount(transcripts.available) }}</dd></div>
          </dl>
        </UiCard>

        <UiCard padding="lg" class="stack-2">
          <h4 class="monitoring-card__title">Completed</h4>
          <div class="monitoring-table-wrap">
          <table class="monitoring-table">
            <thead>
              <tr><th scope="col"></th><th scope="col">Downloads</th><th scope="col">Transcripts</th><th scope="col">Audio</th><th scope="col">Summaries</th></tr>
            </thead>
            <tbody>
              <tr v-for="window in snapshot.windows" :key="window.label">
                <th scope="row">{{ window.label }}</th>
                <td>{{ formatCount(window.downloads) }}</td>
                <td>{{ formatCount(window.transcripts) }}</td>
                <td>{{ formatHours(window.transcribed_audio_seconds) }}</td>
                <td>{{ formatCount(window.summaries) }}</td>
              </tr>
            </tbody>
          </table>
          </div>
          <p class="meta-text">Audio hours use each episode's feed duration.</p>
        </UiCard>

        <UiCard padding="lg" class="stack-2">
          <h4 class="monitoring-card__title">Since app restart</h4>
          <p class="meta-text">{{ sinceLabel }}</p>
          <dl class="monitoring-stats">
            <div><dt>Transcripts committed</dt><dd>{{ formatCount(since.transcriptions.success) }}</dd></div>
            <div><dt>Failed · blocked · retried</dt><dd>{{ formatCount(since.transcriptions.failed) }} · {{ formatCount(since.transcriptions.blocked) }} · {{ formatCount(since.transcriptions.retrying) }}</dd></div>
            <div><dt>Average speed</dt><dd>{{ formatSpeed(since.avg_speed_ratio) }}</dd></div>
            <div><dt>Avg processing</dt><dd>{{ formatDuration(since.avg_processing_seconds) }}</dd></div>
            <div><dt>Avg wait in worker queue</dt><dd>{{ formatDuration(since.avg_worker_queue_seconds) }}</dd></div>
            <div><dt>Avg hand-off to saved</dt><dd>{{ formatDuration(since.avg_end_to_end_seconds) }}</dd></div>
            <div><dt>Downloads ok · failed</dt><dd>{{ formatCount(since.downloads.success) }} · {{ formatCount(since.downloads.failed) }}</dd></div>
            <div><dt>Downloaded</dt><dd>{{ formatBytes(since.download_bytes) }} (avg {{ formatDuration(since.avg_download_seconds) }})</dd></div>
          </dl>
        </UiCard>

        <UiCard v-if="stages.length" padding="lg" class="stack-2">
          <h4 class="monitoring-card__title">Average time per stage</h4>
          <dl class="monitoring-stats">
            <div v-for="stage in stages" :key="stage.stage"><dt>{{ stage.label }}</dt><dd>{{ formatDuration(stage.seconds) }}</dd></div>
          </dl>
        </UiCard>

        <UiCard v-if="jobs.length" padding="lg" class="stack-2">
          <h4 class="monitoring-card__title">Background jobs</h4>
          <ul class="monitoring-list" aria-label="Background jobs">
            <li v-for="job in jobs" :key="job.name" class="monitoring-list__item">
              <div class="surface-row surface-row--between monitoring-list__head">
                <span class="monitoring-ellipsis monitoring-list__title">{{ job.name }}</span>
                <UiBadge :tone="badgeTone[jobStatus(job).tone]">{{ jobStatus(job).label }}</UiBadge>
              </div>
              <span class="meta-text">
                Last run {{ timeLabel(job.last_finished_at ?? job.last_started_at) }} · {{ formatDuration(job.last_duration_ms / 1000) }} ·
                {{ formatCount(job.runs) }} runs, {{ formatCount(job.failures) }} failed
              </span>
              <span v-if="job.consecutive_failures > 0 && job.last_error" class="meta-text monitoring-list__error">
                {{ formatCount(job.consecutive_failures) }} in a row since {{ timeLabel(job.failing_since) }}: {{ job.last_error }}
              </span>
            </li>
          </ul>
          <p class="meta-text">Since the app started.</p>
        </UiCard>

        <UiCard v-if="notifications" padding="lg" class="stack-2">
          <div class="surface-row surface-row--between">
            <h4 class="monitoring-card__title">Notifications</h4>
            <UiBadge :tone="notifications.enabled ? 'success' : notifications.error ? 'danger' : 'neutral'">
              {{ notifications.enabled ? "On" : notifications.error ? "Misconfigured" : "Off" }}
            </UiBadge>
          </div>
          <p v-if="notifications.enabled" class="meta-text">
            Problems are pushed to {{ notifications.host }} ({{ notifications.format }}): failing summaries or jobs, an
            unreachable LLM server or worker, and low disk space. Repeats are limited to one reminder every 6 hours.
          </p>
          <p v-else-if="notifications.error" class="meta-text">{{ notifications.error }}</p>
          <p v-else class="meta-text">
            Set BRIEFCAST_NOTIFY_URL in the app environment (for example an ntfy topic URL) and restart to get a push
            message when something here needs attention.
          </p>
          <dl v-if="notifications.enabled" class="monitoring-stats">
            <div><dt>Active alerts</dt><dd>{{ notifications.active.length ? notifications.active.map((a) => a.title).join(", ") : "None" }}</dd></div>
            <div><dt>Last sent</dt><dd>{{ timeLabel(notifications.last_sent_at) }}</dd></div>
          </dl>
          <UiAlert v-if="notifications.last_error" tone="warning">Last delivery failed: {{ notifications.last_error }}</UiAlert>
          <div v-if="notifications.enabled" class="surface-row">
            <UiButton size="sm" variant="outline" :disabled="isSendingTest" @click="sendTestNotification">
              {{ isSendingTest ? "Sending..." : "Send test notification" }}
            </UiButton>
          </div>
          <UiAlert v-if="testMessage" :tone="testMessage.tone">{{ testMessage.text }}</UiAlert>
        </UiCard>

        <UiCard padding="lg" class="stack-2">
          <h4 class="monitoring-card__title">Storage</h4>
          <dl class="monitoring-stats">
            <div><dt>Audio &amp; exports volume free</dt><dd>{{ formatBytes(snapshot.assets_free_bytes) }}</dd></div>
            <div><dt>Episodes downloaded</dt><dd>{{ formatCount(episodes.downloaded) }}</dd></div>
          </dl>
          <p class="meta-text">
            Prometheus endpoints: <code>/metrics</code> (app) and <code>/metrics/worker</code> (WhisperX worker).
          </p>
        </UiCard>
      </div>
    </template>
  </div>
</template>

<style scoped>
.monitoring-header {
  align-items: flex-start;
  gap: var(--space-3);
}

.monitoring-grid {
  display: grid;
  gap: var(--space-4);
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 320px), 1fr));
}

/* Grid items default to min-width: auto, so a card with a long one-line
   title would refuse to shrink and push the page wider than a phone. */
.monitoring-grid > * {
  min-width: 0;
}

.monitoring-card__title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
}

.monitoring-stats {
  margin: 0;
  display: grid;
  gap: var(--space-1);
}

.monitoring-stats > div {
  display: flex;
  justify-content: space-between;
  gap: var(--space-3);
  border-bottom: 1px solid var(--color-border);
  padding-block: var(--space-1);
}

.monitoring-stats dt {
  color: var(--color-text-secondary);
}

.monitoring-stats dd {
  margin: 0;
  color: var(--color-text-primary);
  font-variant-numeric: tabular-nums;
  text-align: right;
}

.monitoring-attention {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: var(--space-2);
}

.monitoring-attention__item {
  display: grid;
  gap: 2px;
  border-left: 3px solid var(--color-border);
  padding: var(--space-1) var(--space-3);
}

.monitoring-attention__item--bad {
  border-left-color: var(--color-danger, #c0392b);
}

.monitoring-attention__item--warn {
  border-left-color: var(--color-warning, #d68910);
}

.monitoring-allclear {
  margin: 0;
}

.monitoring-meter-label {
  display: flex;
  justify-content: space-between;
  gap: var(--space-2);
  font-size: var(--font-caption-size);
  color: var(--color-text-secondary);
  font-variant-numeric: tabular-nums;
}

/* The value ("12 s of 15 m") stays on one line; a long title is shortened instead. */
.monitoring-meter-label > span:last-child {
  flex: 0 0 auto;
  white-space: nowrap;
}

.monitoring-meter {
  height: 8px;
  border-radius: 999px;
  background: var(--color-hover);
  overflow: hidden;
}

.monitoring-meter > span {
  display: block;
  height: 100%;
  background: var(--color-accent-hover);
  border-radius: inherit;
}

.monitoring-ellipsis {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.monitoring-job-row {
  gap: var(--space-2);
  flex-wrap: wrap;
}

.monitoring-list {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: var(--space-2);
}

.monitoring-list__item {
  display: grid;
  gap: 2px;
  min-width: 0;
  border-bottom: 1px solid var(--color-border);
  padding-bottom: var(--space-2);
}

.monitoring-list__head {
  gap: var(--space-2);
  flex-wrap: nowrap;
  min-width: 0;
}

.monitoring-list__title {
  color: var(--color-text-primary);
  font-weight: 500;
}

.monitoring-list__error {
  overflow-wrap: anywhere;
}

/* Five columns do not fit a phone-width card; the table scrolls inside it. */
.monitoring-table-wrap {
  overflow-x: auto;
}

.monitoring-table {
  width: 100%;
  border-collapse: collapse;
  font-variant-numeric: tabular-nums;
}

.monitoring-table th,
.monitoring-table td {
  padding: var(--space-1) var(--space-2);
  text-align: right;
  border-bottom: 1px solid var(--color-border);
}

.monitoring-table th[scope="row"],
.monitoring-table thead th:first-child {
  white-space: nowrap;
  text-align: left;
  color: var(--color-text-secondary);
  font-weight: 500;
}
</style>
