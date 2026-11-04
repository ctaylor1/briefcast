<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { getErrorMessage, settingsApi } from "../../lib/api";
import {
  attentionItems,
  formatBytes,
  formatCount,
  formatDuration,
  formatHours,
  formatSpeed,
  percent,
  stageTimings,
  workerStatus,
  type HealthTone,
} from "../../lib/monitoring";
import type { MonitoringSnapshot } from "../../types/api";
import UiAlert from "../ui/UiAlert.vue";
import UiBadge from "../ui/UiBadge.vue";
import UiButton from "../ui/UiButton.vue";
import UiCard from "../ui/UiCard.vue";

const REFRESH_MS = 15_000;

const snapshot = ref<MonitoringSnapshot | null>(null);
const loading = ref(false);
const errorMessage = ref("");
let timer: number | undefined;

const badgeTone: Record<HealthTone, "success" | "warning" | "danger" | "neutral"> = {
  ok: "success",
  warn: "warning",
  bad: "danger",
  idle: "neutral",
};

const status = computed(() => (snapshot.value ? workerStatus(snapshot.value) : null));
const attention = computed(() => (snapshot.value ? attentionItems(snapshot.value) : []));
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
    snapshot.value = await settingsApi.getMonitoring();
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
});
</script>

<template>
  <div class="stack-4">
    <UiCard padding="lg" class="stack-3">
      <div class="surface-row surface-row--between monitoring-header">
        <div class="stack-1">
          <h3 class="settings-section-title">Pipeline monitoring</h3>
          <p class="section-subtitle">
            Downloads, transcription and worker health. Refreshes every 15 seconds{{ updatedLabel ? ` · updated ${updatedLabel}` : "" }}.
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
          <table class="monitoring-table">
            <thead>
              <tr><th scope="col"></th><th scope="col">Downloads</th><th scope="col">Transcripts</th><th scope="col">Audio</th></tr>
            </thead>
            <tbody>
              <tr v-for="window in snapshot.windows" :key="window.label">
                <th scope="row">{{ window.label }}</th>
                <td>{{ formatCount(window.downloads) }}</td>
                <td>{{ formatCount(window.transcripts) }}</td>
                <td>{{ formatHours(window.transcribed_audio_seconds) }}</td>
              </tr>
            </tbody>
          </table>
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
  font-size: var(--font-caption-size);
  color: var(--color-text-secondary);
  font-variant-numeric: tabular-nums;
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
