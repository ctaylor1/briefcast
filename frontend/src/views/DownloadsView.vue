<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";
import { useDownloadQueue } from "../composables/useDownloadQueue";
import { getErrorMessage } from "../lib/api";
import type { PodcastItem } from "../types/api";
import UiAlert from "../components/ui/UiAlert.vue";
import UiBadge from "../components/ui/UiBadge.vue";
import UiButton from "../components/ui/UiButton.vue";
import UiCard from "../components/ui/UiCard.vue";

const infoMessage = ref("");
const actionError = ref("");

const {
  queueItems,
  queueLoading,
  queueError,
  queueCounts,
  downloadsPaused,
  fetchQueue,
  pauseDownloads,
  resumeDownloads,
  cancelAllDownloads: cancelAllQueuedDownloads,
  cancelEpisodeDownload,
  queueProgressPercent,
  queueProgressLabel,
  queueHasKnownTotal,
} = useDownloadQueue();

let queueInterval: number | undefined;

async function toggleDownloadsPause(): Promise<void> {
  infoMessage.value = "";
  actionError.value = "";
  try {
    if (downloadsPaused.value) {
      await resumeDownloads();
      infoMessage.value = "Downloads resumed.";
    } else {
      await pauseDownloads();
      infoMessage.value = "Downloads paused.";
    }
    await fetchQueue();
  } catch (error) {
    actionError.value = getErrorMessage(error, "Could not update download pause.");
  }
}

async function cancelAllDownloads(): Promise<void> {
  infoMessage.value = "";
  actionError.value = "";
  try {
    await cancelAllQueuedDownloads();
    infoMessage.value = "All queued downloads cancelled.";
    await fetchQueue();
  } catch (error) {
    actionError.value = getErrorMessage(error, "Could not cancel downloads.");
  }
}

async function cancelDownload(item: PodcastItem): Promise<void> {
  infoMessage.value = "";
  actionError.value = "";
  try {
    await cancelEpisodeDownload(item.ID);
    infoMessage.value = "Download cancelled.";
    await fetchQueue();
  } catch (error) {
    actionError.value = getErrorMessage(error, "Could not cancel download.");
  }
}

function openPlayer(item: PodcastItem): void {
  const target = `/app/#/player?itemIds=${encodeURIComponent(item.ID)}`;
  window.open(target, "briefcast_player");
}

onMounted(() => {
  void fetchQueue();
  queueInterval = window.setInterval(() => {
    void fetchQueue();
  }, 5000);
});

onUnmounted(() => {
  if (queueInterval) {
    window.clearInterval(queueInterval);
  }
});
</script>

<template>
  <section class="downloads-page stack-4">
    <header class="page-header">
      <h2 class="section-title">Download queue</h2>
      <p class="section-subtitle">
        Track queue progress, pause downloads, and stop individual jobs.
      </p>
    </header>

    <UiAlert v-if="infoMessage" tone="success">
      {{ infoMessage }}
    </UiAlert>
    <UiAlert v-if="actionError" tone="danger">
      {{ actionError }}
    </UiAlert>

    <UiCard padding="lg" class="stack-3">
      <div class="surface-row surface-row--between">
        <div class="stack-1">
          <h3 class="section-title">Queue status</h3>
          <p class="meta-text">
            Queued {{ queueCounts.queued }} • Downloading {{ queueCounts.downloading }} • Paused {{ queueCounts.paused }} • Downloaded {{ queueCounts.downloaded }}
          </p>
        </div>
        <div class="queue-toolbar">
          <UiButton size="sm" variant="outline" @click="toggleDownloadsPause">
            {{ downloadsPaused ? "Resume downloads" : "Pause downloads" }}
          </UiButton>
          <UiButton
            size="sm"
            variant="danger"
            :disabled="queueCounts.queued === 0 && queueCounts.downloading === 0"
            @click="cancelAllDownloads"
          >
            Stop all
          </UiButton>
          <UiButton size="sm" variant="ghost" @click="fetchQueue">
            Refresh
          </UiButton>
        </div>
      </div>

      <UiAlert v-if="queueError" tone="danger">
        {{ queueError }}
      </UiAlert>

      <div v-if="queueLoading && queueItems.length === 0" class="queue-skeleton">
        <div v-for="index in 4" :key="index" class="queue-skeleton__row">
          <span class="skeleton queue-skeleton__line queue-skeleton__line--title"></span>
          <span class="skeleton queue-skeleton__line"></span>
          <span class="skeleton queue-skeleton__line queue-skeleton__line--short"></span>
        </div>
      </div>

      <UiCard v-else-if="queueItems.length === 0" padding="md" tone="subtle" class="empty-state">
        <p class="empty-state__title">No queued downloads</p>
        <p class="empty-state__copy">
          Queue episodes from the Episodes page and progress will appear here.
        </p>
      </UiCard>

      <ul v-else class="queue-list">
        <li v-for="item in queueItems" :key="item.ID" class="queue-list__row">
          <div class="queue-list__main">
            <p class="queue-list__title">{{ item.Title }}</p>
            <p class="meta-text">{{ item.Podcast?.Title || "Unknown podcast" }}</p>
            <div class="queue-list__progress-track">
              <div
                class="queue-list__progress-fill"
                :class="!queueHasKnownTotal(item) && 'queue-list__progress-fill--unknown'"
                :style="queueHasKnownTotal(item) ? { width: `${queueProgressPercent(item)}%` } : undefined"
              />
            </div>
            <p class="meta-text">{{ queueProgressLabel(item) }}</p>
          </div>
          <div class="queue-list__actions">
            <UiBadge
              :tone="item.DownloadStatus === 1 ? 'info' : item.DownloadStatus === 4 ? 'warning' : 'neutral'"
            >
              {{
                item.DownloadStatus === 1
                  ? "Downloading"
                  : item.DownloadStatus === 4
                    ? "Paused"
                    : "Queued"
              }}
            </UiBadge>
            <UiButton size="sm" variant="outline" @click="openPlayer(item)">
              Play
            </UiButton>
            <UiButton size="sm" variant="danger" @click="cancelDownload(item)">
              Stop
            </UiButton>
          </div>
        </li>
      </ul>
    </UiCard>
  </section>
</template>

<style scoped>
.queue-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.queue-skeleton {
  display: grid;
  gap: var(--space-3);
}

.queue-skeleton__row {
  display: grid;
  gap: var(--space-2);
}

.queue-skeleton__line {
  height: 12px;
}

.queue-skeleton__line--title {
  width: 64%;
  height: 18px;
}

.queue-skeleton__line--short {
  width: 42%;
}

.queue-list {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: var(--space-2);
}

.queue-list__row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
  padding: var(--space-3);
}

.queue-list__main {
  min-width: min(100%, 240px);
  flex: 1;
}

.queue-list__title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
}

.queue-list__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
}

.queue-list__progress-track {
  margin-top: var(--space-2);
  width: 100%;
  height: 6px;
  border-radius: 999px;
  background: var(--color-hover);
  overflow: hidden;
}

.queue-list__progress-fill {
  height: 100%;
  border-radius: inherit;
  background: var(--color-accent);
}

.queue-list__progress-fill--unknown {
  width: 50%;
  animation: pulse-track 1.2s infinite ease-in-out;
}

.empty-state__title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
}

.empty-state__copy {
  margin: var(--space-2) auto 0;
  max-width: 46ch;
}

@keyframes pulse-track {
  0%,
  100% {
    opacity: 0.35;
  }
  50% {
    opacity: 0.85;
  }
}
</style>
