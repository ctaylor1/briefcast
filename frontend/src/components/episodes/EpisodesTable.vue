<script setup lang="ts">
import { formatBytes, formatDateTime, formatDuration } from "../../lib/format";
import type { PodcastItem } from "../../types/api";
import UiBadge from "../ui/UiBadge.vue";
import UiButton from "../ui/UiButton.vue";
import UiCard from "../ui/UiCard.vue";

defineProps<{
  items: PodcastItem[];
}>();

const emit = defineEmits<{
  (event: "play", item: PodcastItem): void;
  (event: "toggle-played", item: PodcastItem): void;
  (event: "toggle-bookmark", item: PodcastItem): void;
  (event: "queue-download", item: PodcastItem): void;
  (event: "cancel-download", item: PodcastItem): void;
  (event: "open-details", item: PodcastItem, tab?: "overview" | "chapters" | "transcript"): void;
}>();

function isBookmarked(value: string): boolean {
  return value !== "0001-01-01T00:00:00Z";
}

function downloadStatusLabel(status: number): string {
  switch (status) {
    case 0:
      return "Queued";
    case 1:
      return "Downloading";
    case 2:
      return "Downloaded";
    case 3:
      return "Not queued";
    case 4:
      return "Paused";
    default:
      return "Unknown";
  }
}

function downloadStatusTone(status: number): "neutral" | "info" | "success" | "warning" {
  switch (status) {
    case 0:
      return "warning";
    case 1:
      return "info";
    case 2:
      return "success";
    case 3:
      return "neutral";
    case 4:
      return "warning";
    default:
      return "neutral";
  }
}

function canCancel(status: number): boolean {
  return status === 0 || status === 1;
}

function isDownloaded(status: number): boolean {
  return status === 2;
}

function isPaused(status: number): boolean {
  return status === 4;
}

function transcriptPill(status: string): { label: string; tone: "neutral" | "info" | "success" | "warning" | "danger"; visible: boolean } {
  if (status.startsWith("pending_")) {
    return { label: "Transcript pending", tone: "warning", visible: true };
  }
  switch (status) {
    case "available":
      return { label: "Transcript ready", tone: "success", visible: true };
    case "processing":
      return { label: "Transcribing", tone: "info", visible: true };
    case "failed":
      return { label: "Transcript failed", tone: "danger", visible: true };
    default:
      return { label: "Transcript missing", tone: "neutral", visible: false };
  }
}

function progressPercent(item: PodcastItem): number {
  if (item.DownloadTotalBytes <= 0) {
    return 0;
  }
  return Math.min(100, Math.round((item.DownloadedBytes / item.DownloadTotalBytes) * 100));
}

function progressLabel(item: PodcastItem): string {
  if (item.DownloadTotalBytes > 0) {
    return `${progressPercent(item)}% (${formatBytes(item.DownloadedBytes)} / ${formatBytes(item.DownloadTotalBytes)})`;
  }
  if (item.DownloadedBytes > 0) {
    return `${formatBytes(item.DownloadedBytes)} downloaded`;
  }
  return "Downloading...";
}

function hasKnownTotal(item: PodcastItem): boolean {
  return item.DownloadTotalBytes > 0;
}
</script>

<template>
  <UiCard padding="none">
    <div class="table-wrap visually-scrollable">
      <table class="data-table episodes-table">
        <thead>
          <tr>
            <th>Episode</th>
            <th>Podcast</th>
            <th>Published</th>
            <th>Duration</th>
            <th>Status</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="item in items"
            :key="item.ID"
          >
            <td>
              <div class="episodes-table__episode">
                <p class="episodes-table__title">{{ item.Title }}</p>
                <p class="episodes-table__summary">{{ item.Summary || "No summary available." }}</p>
                <div class="episodes-table__badges">
                  <button
                    v-if="transcriptPill(item.TranscriptStatus).visible"
                    type="button"
                    class="episodes-table__badge-button"
                    @click="emit('open-details', item, 'transcript')"
                  >
                    <UiBadge :tone="transcriptPill(item.TranscriptStatus).tone">
                      {{ transcriptPill(item.TranscriptStatus).label }}
                    </UiBadge>
                  </button>
                  <button
                    v-if="item.HasChapters"
                    type="button"
                    class="episodes-table__badge-button"
                    @click="emit('open-details', item, 'chapters')"
                  >
                    <UiBadge tone="info">Chapters</UiBadge>
                  </button>
                </div>
              </div>
            </td>
            <td class="meta-text">{{ item.Podcast?.Title || "Unknown podcast" }}</td>
            <td class="meta-text">{{ formatDateTime(item.PubDate) }}</td>
            <td class="meta-text">{{ formatDuration(item.Duration) }}</td>
            <td>
              <div class="episodes-table__status">
                <UiBadge :tone="item.IsPlayed ? 'success' : 'neutral'">
                  {{ item.IsPlayed ? "Played" : "Unplayed" }}
                </UiBadge>
                <UiBadge :tone="isBookmarked(item.BookmarkDate) ? 'info' : 'neutral'">
                  {{ isBookmarked(item.BookmarkDate) ? "Bookmarked" : "No bookmark" }}
                </UiBadge>
                <UiBadge :tone="downloadStatusTone(item.DownloadStatus)">
                  {{ downloadStatusLabel(item.DownloadStatus) }}
                </UiBadge>
              </div>
              <div v-if="item.DownloadStatus === 1" class="episodes-table__progress">
                <div class="episodes-table__progress-track">
                  <div
                    class="episodes-table__progress-fill"
                    :class="!hasKnownTotal(item) && 'episodes-table__progress-fill--unknown'"
                    :style="hasKnownTotal(item) ? { width: `${progressPercent(item)}%` } : undefined"
                  />
                </div>
                <p class="meta-text">{{ progressLabel(item) }}</p>
              </div>
            </td>
            <td>
              <div class="episodes-table__actions">
                <UiButton size="sm" variant="outline" @click="emit('play', item)">
                  Play
                </UiButton>
                <UiButton size="sm" variant="outline" @click="emit('toggle-played', item)">
                  {{ item.IsPlayed ? "Unplay" : "Played" }}
                </UiButton>
                <UiButton size="sm" variant="outline" @click="emit('toggle-bookmark', item)">
                  {{ isBookmarked(item.BookmarkDate) ? "Unbookmark" : "Bookmark" }}
                </UiButton>
                <UiButton
                  size="sm"
                  variant="secondary"
                  :disabled="isDownloaded(item.DownloadStatus)"
                  @click="emit('queue-download', item)"
                >
                  {{
                    isDownloaded(item.DownloadStatus)
                      ? "Downloaded"
                      : isPaused(item.DownloadStatus)
                        ? "Resume"
                        : "Download"
                  }}
                </UiButton>
                <UiButton
                  size="sm"
                  variant="ghost"
                  @click="emit('open-details', item, 'overview')"
                >
                  Details
                </UiButton>
                <UiButton
                  v-if="canCancel(item.DownloadStatus)"
                  size="sm"
                  variant="danger"
                  @click="emit('cancel-download', item)"
                >
                  Stop
                </UiButton>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </UiCard>
</template>

<style scoped>
.episodes-table {
  min-width: 1120px;
}

.episodes-table__episode {
  min-width: 320px;
}

.episodes-table__title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
}

.episodes-table__summary {
  margin: var(--space-1) 0 0;
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  max-width: 42ch;
}

.episodes-table__badges {
  margin-top: var(--space-2);
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.episodes-table__badge-button {
  border: 0;
  padding: 0;
  background: transparent;
  cursor: pointer;
}

.episodes-table__status {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.episodes-table__progress {
  margin-top: var(--space-2);
}

.episodes-table__progress-track {
  width: 160px;
  height: 6px;
  border-radius: 999px;
  background: var(--color-hover);
  overflow: hidden;
}

.episodes-table__progress-fill {
  height: 100%;
  border-radius: inherit;
  background: var(--color-accent);
}

.episodes-table__progress-fill--unknown {
  width: 50%;
  animation: pulse-track 1.2s infinite ease-in-out;
}

.episodes-table__actions {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-2);
  min-width: 220px;
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
