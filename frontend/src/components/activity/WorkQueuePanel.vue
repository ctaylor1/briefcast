<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import UiAlert from "../ui/UiAlert.vue";
import UiBadge from "../ui/UiBadge.vue";
import UiButton from "../ui/UiButton.vue";
import UiCard from "../ui/UiCard.vue";
import { getErrorMessage, settingsApi } from "../../lib/api";
import type { RepairWorkResponse, WorkQueueItem } from "../../types/api";

// The transcription and summary work queue, with the repair job. Moved from Settings to
// Activity; it refreshes itself while repair work runs.
const isLoadingWorkQueue = ref(false);
const isStartingRepairWork = ref(false);
const workQueuePayload = ref<RepairWorkResponse | null>(null);
const workQueueErrorMessage = ref("");
let workQueuePollTimer: number | undefined;

const workQueue = computed(() => workQueuePayload.value?.queue ?? null);
const workQueueItems = computed(() => workQueue.value?.items ?? []);
const workQueueRunning = computed(() => workQueuePayload.value?.running ?? false);
const lastRepairRun = computed(() => workQueuePayload.value?.lastRun ?? null);

function clearWorkQueuePoll(): void {
  if (workQueuePollTimer !== undefined) {
    window.clearTimeout(workQueuePollTimer);
    workQueuePollTimer = undefined;
  }
}

function scheduleWorkQueuePoll(): void {
  clearWorkQueuePoll();
  if (!workQueuePayload.value?.running) {
    return;
  }
  workQueuePollTimer = window.setTimeout(() => {
    void loadWorkQueue({ silent: true });
  }, 5000);
}

async function loadWorkQueue(options: { silent?: boolean } = {}): Promise<void> {
  if (!options.silent) {
    isLoadingWorkQueue.value = true;
  }
  workQueueErrorMessage.value = "";
  try {
    workQueuePayload.value = await settingsApi.getRepairWork(50);
    scheduleWorkQueuePoll();
  } catch (error) {
    workQueueErrorMessage.value = getErrorMessage(error, "Failed to load work queue.");
    clearWorkQueuePoll();
  } finally {
    if (!options.silent) {
      isLoadingWorkQueue.value = false;
    }
  }
}

async function startRepairWork(): Promise<void> {
  isStartingRepairWork.value = true;
  workQueueErrorMessage.value = "";
  try {
    workQueuePayload.value = await settingsApi.startRepairWork(50);
    scheduleWorkQueuePoll();
  } catch (error) {
    workQueueErrorMessage.value = getErrorMessage(error, "Failed to start repair work.");
  } finally {
    isStartingRepairWork.value = false;
  }
}

function workQueueTone(item: WorkQueueItem): "neutral" | "info" | "success" | "danger" | "warning" {
  switch (item.category) {
    case "active":
      return "info";
    case "failed":
    case "blocked":
      return "danger";
    case "retry":
    case "queued":
      return "warning";
    default:
      return "neutral";
  }
}

function formatLogTimestamp(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return iso;
  }
  return date.toLocaleString();
}

function formatOptionalTimestamp(iso?: string | null): string {
  if (!iso) {
    return "";
  }
  return formatLogTimestamp(iso);
}

onMounted(() => void loadWorkQueue());
onUnmounted(clearWorkQueuePoll);
</script>

<template>
  <div class="stack-4">
    <UiCard padding="lg" class="stack-4">
      <div class="surface-row surface-row--between">
        <div class="stack-2">
          <h3 class="settings-section-title">Work Queue</h3>
          <p class="section-subtitle">
            Transcript and summary status across completed, queued, failed, and retrying work.
          </p>
        </div>
        <div class="work-queue-actions">
          <UiButton variant="secondary" :disabled="isLoadingWorkQueue" @click="loadWorkQueue()">
            {{ isLoadingWorkQueue ? "Refreshing..." : "Refresh" }}
          </UiButton>
          <UiButton :disabled="isStartingRepairWork || workQueueRunning" @click="startRepairWork">
            {{ isStartingRepairWork || workQueueRunning ? "Repairing..." : "Repair failed/missing work" }}
          </UiButton>
        </div>
      </div>

      <UiAlert v-if="workQueueErrorMessage" tone="danger">
        {{ workQueueErrorMessage }}
      </UiAlert>

      <UiAlert v-if="workQueueRunning" tone="info">
        Repair is running. This page will refresh while the job is active.
      </UiAlert>

      <div v-if="isLoadingWorkQueue" class="log-loading stack-2">
        <span class="skeleton settings-skeleton-line settings-skeleton-line--title"></span>
        <span class="skeleton settings-skeleton-line"></span>
        <span class="skeleton settings-skeleton-line settings-skeleton-line--short"></span>
      </div>

      <template v-else-if="workQueue">
        <div class="work-queue-grid">
          <section class="work-queue-panel stack-3" aria-labelledby="summary-work-heading">
            <div>
              <h4 id="summary-work-heading" class="settings-section-subtitle">Summaries</h4>
              <p class="meta-text">Generated from canonical transcripts.</p>
            </div>
            <div class="work-count-grid">
              <div class="work-count-item">
                <span class="meta-text">Complete</span>
                <strong>{{ workQueue.summary.complete }}</strong>
              </div>
              <div class="work-count-item">
                <span class="meta-text">Processing</span>
                <strong>{{ workQueue.summary.processing }}</strong>
              </div>
              <div class="work-count-item">
                <span class="meta-text">Missing</span>
                <strong>{{ workQueue.summary.missing }}</strong>
              </div>
              <div class="work-count-item">
                <span class="meta-text">Failed</span>
                <strong>{{ workQueue.summary.failed }}</strong>
              </div>
              <div class="work-count-item">
                <span class="meta-text">Repair eligible</span>
                <strong>{{ workQueue.summary.eligibleForBackfill }}</strong>
              </div>
              <div class="work-count-item">
                <span class="meta-text">No transcript</span>
                <strong>{{ workQueue.summary.blockedNoTranscript }}</strong>
              </div>
            </div>
          </section>

          <section class="work-queue-panel stack-3" aria-labelledby="transcript-work-heading">
            <div>
              <h4 id="transcript-work-heading" class="settings-section-subtitle">Transcripts</h4>
              <p class="meta-text">Created by the WhisperX worker after downloads finish.</p>
            </div>
            <div class="work-count-grid">
              <div class="work-count-item">
                <span class="meta-text">Complete</span>
                <strong>{{ workQueue.transcripts.complete }}</strong>
              </div>
              <div class="work-count-item">
                <span class="meta-text">Queued</span>
                <strong>{{ workQueue.transcripts.queued }}</strong>
              </div>
              <div class="work-count-item">
                <span class="meta-text">Processing</span>
                <strong>{{ workQueue.transcripts.processing }}</strong>
              </div>
              <div class="work-count-item">
                <span class="meta-text">Failed</span>
                <strong>{{ workQueue.transcripts.failed }}</strong>
              </div>
              <div class="work-count-item">
                <span class="meta-text">Retry due</span>
                <strong>{{ workQueue.transcripts.retryDue }}</strong>
              </div>
              <div class="work-count-item">
                <span class="meta-text">Retry scheduled</span>
                <strong>{{ workQueue.transcripts.retryScheduled }}</strong>
              </div>
              <div class="work-count-item">
                <span class="meta-text">Blocked</span>
                <strong>{{ workQueue.transcripts.blocked }}</strong>
              </div>
            </div>
          </section>
        </div>

        <div class="work-config-alerts">
          <UiAlert v-if="!workQueue.config.whisperxEnabled" tone="warning">
            WhisperX is disabled. Transcript repair can queue work, but it cannot run transcriptions.
          </UiAlert>
          <UiAlert v-if="!workQueue.config.llmEnabled || !workQueue.config.llmApiKeyConfigured || !workQueue.config.summarizationEnabled" tone="warning">
            Summary repair requires summarization enabled, LLM enabled, and an LLM API key.
          </UiAlert>
        </div>

        <section v-if="lastRepairRun" class="work-queue-panel stack-2" aria-labelledby="last-repair-heading">
          <h4 id="last-repair-heading" class="settings-section-subtitle">Last Repair</h4>
          <p class="meta-text">
            Started {{ formatOptionalTimestamp(lastRepairRun.startedAt) }}<template v-if="lastRepairRun.finishedAt">, finished {{ formatOptionalTimestamp(lastRepairRun.finishedAt) }}</template>.
          </p>
          <div class="work-repair-summary">
            <span>Summaries: {{ lastRepairRun.summary.succeeded }} repaired, {{ lastRepairRun.summary.failed }} failed, {{ lastRepairRun.summary.eligible }} eligible.</span>
            <span>Transcripts: {{ lastRepairRun.transcripts.forcedDue }} retries queued, {{ lastRepairRun.transcripts.queued }} ready now.</span>
          </div>
          <UiAlert v-if="lastRepairRun.error" tone="warning">
            {{ lastRepairRun.error }}
          </UiAlert>
        </section>

        <section class="stack-2" aria-labelledby="work-items-heading">
          <div class="surface-row surface-row--between">
            <h4 id="work-items-heading" class="settings-section-subtitle">Active And Attention Items</h4>
            <p class="meta-text">Showing up to {{ workQueue.limit }} items.</p>
          </div>

          <div v-if="workQueueItems.length" class="work-item-list">
            <article v-for="item in workQueueItems" :key="`${item.kind}-${item.id}`" class="work-item">
              <div class="work-item-header">
                <div class="log-entry-badges">
                  <UiBadge :tone="item.kind === 'summary' ? 'info' : 'neutral'">{{ item.kind }}</UiBadge>
                  <UiBadge :tone="workQueueTone(item)">{{ item.statusLabel }}</UiBadge>
                </div>
                <time class="log-entry-time">{{ formatOptionalTimestamp(item.updatedAt) }}</time>
              </div>
              <p class="log-entry-message">{{ item.title }}</p>
              <p class="log-entry-meta">
                {{ item.podcastTitle || "Unknown podcast" }}
                <template v-if="item.model"> | {{ item.model }}</template>
              </p>
              <div class="work-item-meta">
                <span v-if="item.progressPct">Progress {{ item.progressPct }}%</span>
                <span v-if="item.progressStage">{{ item.progressStage.replace(/_/g, " ") }}</span>
                <span v-if="item.retryCount">Retried {{ item.retryCount }} {{ item.retryCount === 1 ? "time" : "times" }}</span>
                <span v-if="item.nextAttempt">Next attempt {{ formatOptionalTimestamp(item.nextAttempt) }}</span>
              </div>
              <p v-if="item.lastError" class="work-item-error">{{ item.lastError }}</p>
            </article>
          </div>
          <div v-else class="empty-state">
            No active, failed, retrying, or blocked transcript/summary work was found.
          </div>
        </section>
      </template>
    </UiCard>
  </div>
</template>

<style scoped>
.settings-section-title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-section-size);
  line-height: var(--font-section-line-height);
  font-weight: var(--font-section-weight);
}

.log-loading {
  border: 1px solid var(--color-border);
  border-radius: var(--radius-3);
  background: var(--color-bg-secondary);
  padding: var(--space-4);
}

.log-entry-badges {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
}

.log-entry-time {
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
}

.log-entry-message {
  margin: var(--space-3) 0 0;
  color: var(--color-text-primary);
  font-size: var(--font-body-size);
  line-height: var(--font-body-line-height);
  overflow-wrap: anywhere;
}

.log-entry-meta {
  margin: var(--space-2) 0 0;
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
  overflow-wrap: anywhere;
}

.work-queue-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: var(--space-2);
}

.work-queue-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: var(--space-4);
}

.work-queue-panel {
  min-width: 0;
  border-top: 1px solid var(--color-border);
  padding-top: var(--space-4);
}

.work-count-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-2);
}

.work-count-item {
  display: grid;
  gap: var(--space-1);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
  padding: var(--space-3);
}

.work-count-item strong {
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
}

.work-config-alerts {
  display: grid;
  gap: var(--space-2);
}

.work-repair-summary {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-4);
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
}

.work-item-list {
  display: grid;
  gap: var(--space-3);
}

.work-item {
  min-width: 0;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-3);
  background: var(--color-bg-primary);
  padding: var(--space-4);
}

.work-item-header {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}

.work-item-meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-3);
  margin-top: var(--space-2);
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
}

.work-item-error {
  margin: var(--space-3) 0 0;
  color: var(--color-danger);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
  overflow-wrap: anywhere;
}

.settings-skeleton-line {
  height: 14px;
}

.settings-skeleton-line--title {
  width: 48%;
  height: 20px;
}

.settings-skeleton-line--short {
  width: 60%;
}

.settings-section-subtitle {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
}

@media (min-width: 768px) {
  .work-queue-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
