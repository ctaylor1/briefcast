<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import UiAlert from "../ui/UiAlert.vue";
import UiBadge from "../ui/UiBadge.vue";
import UiButton from "../ui/UiButton.vue";
import UiCard from "../ui/UiCard.vue";
import { getErrorMessage, settingsApi } from "../../lib/api";
import type { AppLogEntry, AppLogsResponse } from "../../types/api";

// Recent application logs, with the entries that affect users first. Moved from Settings
// to Activity.
const isLoadingLogs = ref(false);
const logsPayload = ref<AppLogsResponse | null>(null);
const logsErrorMessage = ref("");

const logEntries = computed(() => logsPayload.value?.entries ?? []);
const impactLogEntries = computed(() => logsPayload.value?.impactEntries ?? []);
const logSources = computed(() => logsPayload.value?.sources ?? []);
const logReadErrors = computed(() => logsPayload.value?.readErrors ?? []);

async function loadLogs(): Promise<void> {
  isLoadingLogs.value = true;
  logsErrorMessage.value = "";
  try {
    logsPayload.value = await settingsApi.getLogs(200);
  } catch (error) {
    logsErrorMessage.value = getErrorMessage(error, "Failed to load application logs.");
  } finally {
    isLoadingLogs.value = false;
  }
}

function logLevelTone(level: AppLogEntry["level"]): "neutral" | "info" | "success" | "danger" | "warning" {
  switch (level) {
    case "fatal":
    case "error":
      return "danger";
    case "warn":
      return "warning";
    case "debug":
      return "neutral";
    default:
      return "info";
  }
}

function formatLogTimestamp(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return iso;
  }
  return date.toLocaleString();
}

function formatBytes(sizeBytes: number): string {
  if (!Number.isFinite(sizeBytes) || sizeBytes <= 0) {
    return "0 B";
  }
  const units = ["B", "KB", "MB", "GB"];
  let value = sizeBytes;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }
  return `${value.toFixed(unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`;
}

function formatLogFields(fields: AppLogEntry["fields"]): string {
  if (!fields || Object.keys(fields).length === 0) {
    return "";
  }
  return JSON.stringify(fields, null, 2);
}

onMounted(() => void loadLogs());
</script>

<template>
  <div class="stack-4">
    <UiCard padding="lg" class="stack-4">
      <div class="surface-row surface-row--between">
        <div class="stack-2">
          <h3 class="settings-section-title">Latest Logs</h3>
          <p class="section-subtitle">
            Recent application, job, download, summary, and transcript logs.
          </p>
        </div>
        <UiButton variant="secondary" :disabled="isLoadingLogs" @click="loadLogs">
          {{ isLoadingLogs ? "Refreshing..." : "Refresh" }}
        </UiButton>
      </div>

      <UiAlert v-if="logsErrorMessage" tone="danger">
        {{ logsErrorMessage }}
      </UiAlert>

      <div v-if="isLoadingLogs" class="log-loading stack-2">
        <span class="skeleton settings-skeleton-line settings-skeleton-line--title"></span>
        <span class="skeleton settings-skeleton-line"></span>
        <span class="skeleton settings-skeleton-line settings-skeleton-line--short"></span>
      </div>

      <template v-else>
        <div v-if="logsPayload" class="log-summary-grid">
          <div class="log-summary-item">
            <span class="meta-text">User-impacting</span>
            <strong>{{ impactLogEntries.length }}</strong>
          </div>
          <div class="log-summary-item">
            <span class="meta-text">Entries shown</span>
            <strong>{{ logEntries.length }}</strong>
          </div>
          <div class="log-summary-item">
            <span class="meta-text">Sources</span>
            <strong>{{ logSources.length }}</strong>
          </div>
        </div>

        <section v-if="impactLogEntries.length" class="stack-2" aria-labelledby="impact-log-heading">
          <h4 id="impact-log-heading" class="settings-section-subtitle">Needs Attention</h4>
          <div class="log-list log-list--compact">
            <article
              v-for="entry in impactLogEntries"
              :key="`impact-${entry.id}`"
              class="log-entry log-entry--impact"
            >
              <div class="log-entry-header">
                <div class="log-entry-badges">
                  <UiBadge :tone="logLevelTone(entry.level)">{{ entry.level.toUpperCase() }}</UiBadge>
                  <UiBadge tone="warning">{{ entry.category }}</UiBadge>
                </div>
                <time class="log-entry-time">{{ formatLogTimestamp(entry.timestamp) }}</time>
              </div>
              <p class="log-entry-message">{{ entry.humanMessage }}</p>
              <p class="log-entry-meta">
                {{ entry.source }}<template v-if="entry.caller"> | {{ entry.caller }}</template>
              </p>
            </article>
          </div>
        </section>

        <UiAlert v-else-if="logsPayload" tone="success">
          No user-impacting errors were found in the latest log entries.
        </UiAlert>

        <UiAlert v-if="logReadErrors.length" tone="warning">
          {{ logReadErrors.length }} log source{{ logReadErrors.length === 1 ? "" : "s" }} could not be read.
        </UiAlert>

        <div v-if="logSources.length" class="log-source-list">
          <span v-for="source in logSources" :key="source.name" class="log-source-chip">
            {{ source.name }} ({{ formatBytes(source.sizeBytes) }})
          </span>
        </div>

        <section class="stack-2" aria-labelledby="all-log-heading">
          <div class="surface-row surface-row--between">
            <h4 id="all-log-heading" class="settings-section-subtitle">All Recent Entries</h4>
            <p v-if="logsPayload && logsPayload.totalDiscovered > logEntries.length" class="meta-text">
              Showing {{ logEntries.length }} of {{ logsPayload.totalDiscovered }} discovered entries.
            </p>
          </div>

          <div v-if="logEntries.length" class="log-list">
            <article
              v-for="entry in logEntries"
              :key="entry.id"
              class="log-entry"
              :class="{ 'log-entry--impact': entry.userImpact }"
            >
              <div class="log-entry-header">
                <div class="log-entry-badges">
                  <UiBadge :tone="logLevelTone(entry.level)">{{ entry.level.toUpperCase() }}</UiBadge>
                  <UiBadge :tone="entry.userImpact ? 'warning' : 'neutral'">{{ entry.category }}</UiBadge>
                </div>
                <time class="log-entry-time">{{ formatLogTimestamp(entry.timestamp) }}</time>
              </div>
              <p class="log-entry-message">{{ entry.humanMessage }}</p>
              <p class="log-entry-meta">
                {{ entry.source }}<template v-if="entry.service"> | {{ entry.service }}</template><template v-if="entry.caller"> | {{ entry.caller }}</template>
              </p>
              <details v-if="formatLogFields(entry.fields) || entry.raw" class="log-entry-details">
                <summary>Details</summary>
                <pre v-if="formatLogFields(entry.fields)" class="log-entry-pre">{{ formatLogFields(entry.fields) }}</pre>
                <pre v-if="entry.raw" class="log-entry-pre">{{ entry.raw }}</pre>
              </details>
            </article>
          </div>
          <div v-else class="empty-state">
            No file-backed logs were found.
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

.log-summary-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: var(--space-3);
}

.log-summary-item {
  display: grid;
  gap: var(--space-1);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
  padding: var(--space-3);
}

.log-summary-item strong {
  color: var(--color-text-primary);
  font-size: var(--font-section-size);
  line-height: var(--font-section-line-height);
}

.log-list {
  display: grid;
  gap: var(--space-3);
}

.log-list--compact {
  gap: var(--space-2);
}

.log-entry {
  min-width: 0;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-3);
  background: var(--color-bg-primary);
  padding: var(--space-4);
}

.log-entry--impact {
  border-color: rgba(203, 145, 47, 0.55);
  background: rgba(203, 145, 47, 0.08);
}

.log-entry-header {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
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

.log-entry-details {
  margin-top: var(--space-3);
}

.log-entry-details summary {
  color: var(--color-accent);
  cursor: pointer;
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
}

.log-entry-pre {
  max-height: 260px;
  overflow: auto;
  margin: var(--space-2) 0 0;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
  color: var(--color-text-primary);
  padding: var(--space-3);
  font-family: ui-monospace, SFMono-Regular, Consolas, "Liberation Mono", monospace;
  font-size: 12px;
  line-height: 18px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.log-source-list {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.log-source-chip {
  max-width: 100%;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
  color: var(--color-text-secondary);
  padding: var(--space-1) var(--space-2);
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
  .log-summary-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}
</style>
