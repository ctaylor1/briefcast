<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRouter } from "vue-router";
import { useDownloadQueue } from "../composables/useDownloadQueue";
import { getErrorMessage } from "../lib/api";
import type { PodcastItem } from "../types/api";
import UiAlert from "../components/ui/UiAlert.vue";
import UiBadge from "../components/ui/UiBadge.vue";
import UiButton from "../components/ui/UiButton.vue";
import UiCard from "../components/ui/UiCard.vue";
import UiDialog from "../components/ui/UiDialog.vue";
import UiSelect from "../components/ui/UiSelect.vue";
import { formatDateTime } from "../lib/format";

const infoMessage = ref("");
const actionError = ref("");
const confirmStopAllOpen = ref(false);
const stoppingAllBusy = ref(false);
const undoCancelItem = ref<PodcastItem | null>(null);
const queueSort = ref<DownloadQueueSort>("download_date_desc");
let undoCancelTimer: number | undefined;
const DOWNLOAD_STATUS_QUEUED = 0;
const DOWNLOAD_STATUS_DOWNLOADING = 1;
const DOWNLOAD_STATUS_DOWNLOADED = 2;
const DOWNLOAD_STATUS_PAUSED = 4;
const QUEUE_REFRESH_MS = 15000;
type BadgeTone = "neutral" | "info" | "success" | "warning" | "danger";
type DownloadQueueSort =
  | "download_date_desc"
  | "download_date_asc"
  | "title_asc"
  | "title_desc"
  | "podcast_asc"
  | "status"
  | "progress_desc"
  | "progress_asc";
const router = useRouter();

const {
  queueItems,
  queueLoading,
  queueError,
  queueCounts,
  fetchQueue,
  pauseDownloads,
  resumeDownloads,
  cancelAllDownloads: cancelAllQueuedDownloads,
  cancelEpisodeDownload,
  resumeEpisodeDownload,
  queueProgressPercent,
  queueProgressLabel,
  queueProgressRemainingLabel,
  queueHasKnownTotal,
} = useDownloadQueue();

let queueInterval: number | undefined;

function isPaused(item: PodcastItem): boolean {
  return item.DownloadStatus === DOWNLOAD_STATUS_PAUSED;
}

function isDownloading(item: PodcastItem): boolean {
  return item.DownloadStatus === DOWNLOAD_STATUS_DOWNLOADING;
}

function isDownloaded(item: PodcastItem): boolean {
  return item.DownloadStatus === DOWNLOAD_STATUS_DOWNLOADED;
}

function queueStatusLabel(item: PodcastItem): string {
  if (isDownloaded(item)) {
    return "Download Completed";
  }
  if (isDownloading(item)) {
    return "Downloading";
  }
  return "Queued";
}

function queueStatusTone(item: PodcastItem): "info" | "neutral" | "success" {
  if (isDownloaded(item)) {
    return "success";
  }
  if (isDownloading(item)) {
    return "info";
  }
  return "neutral";
}

function transcriptQueueBadge(item: PodcastItem): { visible: boolean; label: string; tone: BadgeTone } {
  if (!isDownloaded(item)) {
    return { visible: false, label: "", tone: "neutral" };
  }
  const status = String(item.TranscriptStatus || "").trim().toLowerCase();
  if (status === "processing") {
    return { visible: true, label: "Transcription in progress", tone: "info" };
  }
  if (status.startsWith("pending_")) {
    return { visible: true, label: "Transcription Queued", tone: "warning" };
  }
  if (status === "failed") {
    return { visible: true, label: "Transcription failed", tone: "danger" };
  }
  if (status === "available") {
    return { visible: true, label: "Transcription ready", tone: "success" };
  }
  return { visible: false, label: "", tone: "neutral" };
}

function queueSortPriority(item: PodcastItem): number {
  if (isDownloading(item)) {
    return 0;
  }
  if (item.DownloadStatus === DOWNLOAD_STATUS_QUEUED) {
    return 1;
  }
  if (isPaused(item)) {
    return 2;
  }
  return 3;
}

function parseQueueDate(value?: string | null): number | null {
  if (!value) {
    return null;
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime()) || parsed.getUTCFullYear() <= 1) {
    return null;
  }
  return parsed.getTime();
}

function queueActivityDate(item: PodcastItem): number | null {
  return (
    parseQueueDate(item.DownloadDate) ??
    parseQueueDate(item.UpdatedAt) ??
    parseQueueDate(item.CreatedAt) ??
    parseQueueDate(item.PubDate)
  );
}

function compareDates(left: PodcastItem, right: PodcastItem, direction: "asc" | "desc"): number {
  const leftDate = queueActivityDate(left);
  const rightDate = queueActivityDate(right);
  if (leftDate === null && rightDate === null) {
    return 0;
  }
  if (leftDate === null) {
    return 1;
  }
  if (rightDate === null) {
    return -1;
  }
  return direction === "asc" ? leftDate - rightDate : rightDate - leftDate;
}

function compareStrings(left: string | undefined, right: string | undefined, direction: "asc" | "desc"): number {
  const result = (left || "").localeCompare(right || "", undefined, {
    sensitivity: "base",
    numeric: true,
  });
  return direction === "asc" ? result : -result;
}

function compareProgress(left: PodcastItem, right: PodcastItem, direction: "asc" | "desc"): number {
  const leftProgress = queueHasKnownTotal(left) ? queueProgressPercent(left) : 0;
  const rightProgress = queueHasKnownTotal(right) ? queueProgressPercent(right) : 0;
  return direction === "asc" ? leftProgress - rightProgress : rightProgress - leftProgress;
}

function tieBreakQueueSort(left: PodcastItem, right: PodcastItem): number {
  return (
    queueSortPriority(left) - queueSortPriority(right) ||
    compareDates(left, right, "desc") ||
    compareStrings(left.Title, right.Title, "asc")
  );
}

const sortedQueueItems = computed(() =>
  [...queueItems.value].sort((left, right) => {
    let result = 0;
    switch (queueSort.value) {
      case "download_date_asc":
        result = compareDates(left, right, "asc");
        break;
      case "title_asc":
        result = compareStrings(left.Title, right.Title, "asc");
        break;
      case "title_desc":
        result = compareStrings(left.Title, right.Title, "desc");
        break;
      case "podcast_asc":
        result = compareStrings(left.Podcast?.Title, right.Podcast?.Title, "asc");
        break;
      case "status":
        result = queueSortPriority(left) - queueSortPriority(right);
        break;
      case "progress_desc":
        result = compareProgress(left, right, "desc");
        break;
      case "progress_asc":
        result = compareProgress(left, right, "asc");
        break;
      case "download_date_desc":
      default:
        result = compareDates(left, right, "desc");
        break;
    }
    if (result !== 0) {
      return result;
    }
    return tieBreakQueueSort(left, right);
  }),
);

function downloadDateLabel(item: PodcastItem): string {
  if (parseQueueDate(item.DownloadDate) !== null) {
    return formatDateTime(item.DownloadDate);
  }
  const activityDate = queueActivityDate(item);
  if (activityDate !== null) {
    return formatDateTime(new Date(activityDate).toISOString());
  }
  return "Unknown";
}

async function pauseAllDownloads(): Promise<void> {
  infoMessage.value = "";
  actionError.value = "";
  try {
    await pauseDownloads();
    infoMessage.value = "Downloads paused.";
    await fetchQueue();
  } catch (error) {
    actionError.value = getErrorMessage(error, "Could not update download pause.");
  }
}

async function resumeAllDownloads(): Promise<void> {
  infoMessage.value = "";
  actionError.value = "";
  try {
    await resumeDownloads();
    infoMessage.value = "Downloads resumed.";
    await fetchQueue();
  } catch (error) {
    actionError.value = getErrorMessage(error, "Could not resume downloads.");
  }
}

async function cancelAllDownloads(): Promise<void> {
  infoMessage.value = "";
  actionError.value = "";
  stoppingAllBusy.value = true;
  try {
    await cancelAllQueuedDownloads();
    infoMessage.value = "All queued downloads cancelled.";
    await fetchQueue();
    clearUndoCancel();
  } catch (error) {
    actionError.value = getErrorMessage(error, "Could not cancel downloads.");
  } finally {
    stoppingAllBusy.value = false;
  }
}

function requestCancelAllDownloads(): void {
  confirmStopAllOpen.value = true;
}

async function confirmCancelAllDownloads(): Promise<void> {
  confirmStopAllOpen.value = false;
  await cancelAllDownloads();
}

function clearUndoCancel(): void {
  if (undoCancelTimer) {
    window.clearTimeout(undoCancelTimer);
    undoCancelTimer = undefined;
  }
  undoCancelItem.value = null;
}

function armUndoCancel(item: PodcastItem): void {
  clearUndoCancel();
  undoCancelItem.value = item;
  undoCancelTimer = window.setTimeout(() => {
    undoCancelTimer = undefined;
    undoCancelItem.value = null;
  }, 5000);
}

async function undoCancelDownload(): Promise<void> {
  const item = undoCancelItem.value;
  if (!item) {
    return;
  }
  infoMessage.value = "";
  actionError.value = "";
  clearUndoCancel();
  try {
    await resumeEpisodeDownload(item.ID);
    infoMessage.value = "Download resumed.";
    await fetchQueue();
  } catch (error) {
    actionError.value = getErrorMessage(error, "Could not resume download.");
  }
}

async function cancelDownload(item: PodcastItem): Promise<void> {
  infoMessage.value = "";
  actionError.value = "";
  try {
    await cancelEpisodeDownload(item.ID);
    infoMessage.value = "Download cancelled.";
    armUndoCancel(item);
    await fetchQueue();
  } catch (error) {
    actionError.value = getErrorMessage(error, "Could not cancel download.");
  }
}

async function resumeDownload(item: PodcastItem): Promise<void> {
  infoMessage.value = "";
  actionError.value = "";
  try {
    await resumeEpisodeDownload(item.ID);
    infoMessage.value = "Download resumed.";
    await fetchQueue();
  } catch (error) {
    actionError.value = getErrorMessage(error, "Could not resume download.");
  }
}

function refreshQueue(): void {
  void fetchQueue();
}

function queueProgressAriaValue(item: PodcastItem): number | undefined {
  if (!queueHasKnownTotal(item)) {
    return undefined;
  }
  return queueProgressPercent(item);
}

function queueProgressAriaText(item: PodcastItem): string {
  return `${queueProgressLabel(item)} ${queueProgressRemainingLabel(item)}`;
}

function openPlayer(item: PodcastItem): void {
  void router.push({
    path: "/player",
    query: {
      itemIds: item.ID,
    },
  });
}

onMounted(() => {
  void fetchQueue();
  queueInterval = window.setInterval(() => {
    void fetchQueue();
  }, QUEUE_REFRESH_MS);
});

onUnmounted(() => {
  if (queueInterval) {
    window.clearInterval(queueInterval);
  }
  clearUndoCancel();
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
    <UiAlert v-if="undoCancelItem" tone="info">
      <div class="queue-undo">
        <span>Stopped "{{ undoCancelItem.Title }}".</span>
        <UiButton size="sm" variant="secondary" @click="undoCancelDownload">
          Undo
        </UiButton>
      </div>
    </UiAlert>

    <UiCard padding="md" tone="subtle" class="downloads-filters">
      <div class="downloads-filters__row">
        <UiSelect
          :model-value="queueSort"
          label="Sort"
          @update:model-value="queueSort = $event as DownloadQueueSort"
        >
          <option value="download_date_desc">Download Date (Desc)</option>
          <option value="download_date_asc">Download Date (Asc)</option>
          <option value="title_asc">Alphabetical (A-Z)</option>
          <option value="title_desc">Alphabetical Z-A</option>
          <option value="podcast_asc">Podcast (A-Z)</option>
          <option value="status">Status (Active First)</option>
          <option value="progress_desc">Progress (High-Low)</option>
          <option value="progress_asc">Progress (Low-High)</option>
        </UiSelect>
        <div class="downloads-filters__status">
          <span class="ui-label">Queue status</span>
          <p class="meta-text">
            Queued {{ queueCounts.queued }} • Downloading {{ queueCounts.downloading }} • Paused {{ queueCounts.paused }} • Downloaded {{ queueCounts.downloaded }}
          </p>
        </div>
      </div>

      <div class="downloads-filters__footer">
        <span v-if="queueLoading" class="meta-text">Refreshing downloads...</span>
        <span v-else class="meta-text">{{ queueItems.length }} {{ queueItems.length === 1 ? "download" : "downloads" }}</span>
        <div class="downloads-filters__actions">
          <UiButton
            size="sm"
            variant="outline"
            :disabled="queueCounts.queued === 0 && queueCounts.downloading === 0"
            @click="pauseAllDownloads"
          >
            Pause All Downloads
          </UiButton>
          <UiButton size="sm" variant="success" :disabled="queueCounts.paused === 0" @click="resumeAllDownloads">
            Resume All Downloads
          </UiButton>
          <UiButton
            size="sm"
            variant="danger"
            :disabled="queueCounts.queued === 0 && queueCounts.downloading === 0"
            @click="requestCancelAllDownloads"
          >
            Stop all
          </UiButton>
          <UiButton
            size="sm"
            variant="ghost"
            class="downloads-filters__refresh"
            aria-label="Refresh queue"
            title="Refresh queue"
            @click="refreshQueue"
          >
            ↻
          </UiButton>
        </div>
      </div>
    </UiCard>

    <UiAlert v-if="queueError" tone="danger">
      {{ queueError }}
    </UiAlert>

    <UiCard v-if="queueLoading && queueItems.length === 0" padding="md">
      <div class="queue-skeleton">
        <div v-for="index in 4" :key="index" class="queue-skeleton__row">
          <span class="skeleton queue-skeleton__line queue-skeleton__line--title"></span>
          <span class="skeleton queue-skeleton__line"></span>
          <span class="skeleton queue-skeleton__line queue-skeleton__line--short"></span>
        </div>
      </div>
    </UiCard>

    <UiCard v-else-if="queueItems.length === 0" padding="md" tone="subtle" class="empty-state">
      <p class="empty-state__title">No queued downloads</p>
      <p class="empty-state__copy">
        Queue episodes from the Episodes page and progress will appear here.
      </p>
    </UiCard>

    <UiCard v-else padding="none">
      <div class="table-wrap visually-scrollable">
        <table class="data-table downloads-table">
          <thead>
            <tr>
              <th>Episode</th>
              <th>Podcast</th>
              <th>Download Date</th>
              <th>Status</th>
              <th>Progress</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="item in sortedQueueItems"
              :key="item.ID"
              :class="{
                'downloads-table__row--downloading': isDownloading(item),
                'downloads-table__row--paused': isPaused(item),
              }"
            >
              <td>
                <div class="downloads-table__episode">
                  <p class="downloads-table__title">{{ item.Title }}</p>
                </div>
              </td>
              <td class="meta-text">{{ item.Podcast?.Title || "Unknown podcast" }}</td>
              <td class="meta-text">{{ downloadDateLabel(item) }}</td>
              <td>
                <div class="downloads-table__badges">
                  <UiBadge :tone="queueStatusTone(item)">
                    {{ queueStatusLabel(item) }}
                  </UiBadge>
                  <UiBadge
                    v-if="transcriptQueueBadge(item).visible"
                    :tone="transcriptQueueBadge(item).tone"
                  >
                    {{ transcriptQueueBadge(item).label }}
                  </UiBadge>
                </div>
              </td>
              <td>
                <div v-if="!isPaused(item) && !isDownloaded(item)" class="downloads-table__progress">
                  <div
                    class="downloads-table__progress-track"
                    role="progressbar"
                    :aria-label="`Download progress for ${item.Title}`"
                    aria-valuemin="0"
                    aria-valuemax="100"
                    :aria-valuenow="queueProgressAriaValue(item)"
                    :aria-valuetext="queueProgressAriaText(item)"
                  >
                    <div
                      class="downloads-table__progress-fill"
                      :class="!queueHasKnownTotal(item) && 'downloads-table__progress-fill--unknown'"
                      :style="queueHasKnownTotal(item) ? { width: `${queueProgressPercent(item)}%` } : undefined"
                    />
                  </div>
                  <p class="meta-text">{{ queueProgressLabel(item) }}</p>
                  <p class="meta-text">{{ queueProgressRemainingLabel(item) }}</p>
                </div>
                <p v-else-if="isDownloaded(item)" class="downloads-table__completed-note">Download completed.</p>
                <p v-else class="downloads-table__paused-note">Paused. Resume downloads to continue.</p>
              </td>
              <td>
                <div class="downloads-table__actions">
                  <UiButton
                    v-if="isPaused(item)"
                    size="sm"
                    variant="success"
                    @click="resumeDownload(item)"
                  >
                    Resume Download
                  </UiButton>
                  <UiButton size="sm" variant="outline" @click="openPlayer(item)">
                    Play
                  </UiButton>
                  <UiButton v-if="!isDownloaded(item)" size="sm" variant="danger" @click="cancelDownload(item)">
                    Stop
                  </UiButton>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </UiCard>

    <UiDialog
      :open="confirmStopAllOpen"
      tone="danger"
      title="Stop all downloads?"
      description="This will cancel all queued and active downloads. You can resume individual episodes from the queue."
      confirm-label="Stop all"
      cancel-label="Keep downloads running"
      :busy="stoppingAllBusy"
      @close="confirmStopAllOpen = false"
      @confirm="confirmCancelAllDownloads"
    />
  </section>
</template>

<style scoped>
.queue-undo {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
}

.downloads-filters {
  display: grid;
  gap: var(--space-3);
}

.downloads-filters__row {
  display: grid;
  gap: var(--space-3);
  grid-template-columns: 1fr;
}

.downloads-filters__status {
  display: grid;
  align-content: end;
  gap: var(--space-2);
}

.downloads-filters__status .meta-text {
  margin: 0;
}

.downloads-filters__footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
}

.downloads-filters__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: var(--space-2);
}

.downloads-filters__refresh {
  min-width: 40px;
  padding-inline: var(--space-2);
  font-size: 18px;
  line-height: 1;
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

.downloads-table {
  min-width: 1120px;
}

.downloads-table__row--downloading {
  box-shadow: inset 3px 0 0 var(--color-accent);
}

.downloads-table__row--paused {
  background: color-mix(in srgb, var(--color-warning) 8%, var(--color-bg-primary));
}

.downloads-table__episode {
  min-width: 320px;
}

.downloads-table__title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
  overflow-wrap: anywhere;
}

.downloads-table__badges,
.downloads-table__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
}

.downloads-table__actions {
  min-width: 220px;
}

.downloads-table__progress {
  min-width: 180px;
}

.downloads-table__progress-track {
  width: 180px;
  height: 6px;
  border-radius: 999px;
  background: var(--color-hover);
  overflow: hidden;
}

.downloads-table__progress-fill {
  height: 100%;
  border-radius: inherit;
  background: var(--color-accent);
}

.downloads-table__progress-fill--unknown {
  width: 50%;
  animation: pulse-track 1.2s infinite ease-in-out;
}

.downloads-table__paused-note {
  margin: 0;
  color: var(--color-warning);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
  font-weight: 600;
}

.downloads-table__completed-note {
  margin: 0;
  color: var(--color-success);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
  font-weight: 600;
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

@media (min-width: 768px) {
  .downloads-filters__row {
    grid-template-columns: minmax(220px, 0.8fr) minmax(0, 1.2fr);
  }
}
</style>
