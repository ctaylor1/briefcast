<script setup lang="ts">
import { formatBytes, formatDateTime, formatDuration } from "../../lib/format";
import type { PodcastItem } from "../../types/api";
import UiBadge from "../ui/UiBadge.vue";
import UiButton from "../ui/UiButton.vue";
import UiCard from "../ui/UiCard.vue";

const props = defineProps<{
  item: PodcastItem;
}>();

const emit = defineEmits<{
  (event: "play", item: PodcastItem): void;
  (event: "toggle-played", item: PodcastItem): void;
  (event: "toggle-bookmark", item: PodcastItem): void;
  (event: "queue-download", item: PodcastItem): void;
  (event: "cancel-download", item: PodcastItem): void;
  (event: "open-details", item: PodcastItem, tab?: "overview" | "chapters" | "transcript"): void;
}>();

function getImage(id: string): string {
  return `/podcastitems/${id}/image`;
}

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
  <UiCard class="episode-list-item">
    <img :src="getImage(props.item.ID)" :alt="props.item.Title" class="episode-list-item__image" />
    <div class="episode-list-item__body stack-2">
      <div class="episode-list-item__header">
        <div>
          <h3 class="episode-list-item__title">
            {{ props.item.Title }}
          </h3>
          <p class="meta-text">
            {{ props.item.Podcast?.Title || "Unknown Podcast" }} • {{ formatDateTime(props.item.PubDate) }} •
            {{ formatDuration(props.item.Duration) }}
          </p>
          <div class="episode-list-item__badges">
            <button
              v-if="transcriptPill(props.item.TranscriptStatus).visible"
              type="button"
              class="episode-list-item__badge-button"
              @click="emit('open-details', props.item, 'transcript')"
            >
              <UiBadge :tone="transcriptPill(props.item.TranscriptStatus).tone">
                {{ transcriptPill(props.item.TranscriptStatus).label }}
              </UiBadge>
            </button>
            <button
              v-if="props.item.HasChapters"
              type="button"
              class="episode-list-item__badge-button"
              @click="emit('open-details', props.item, 'chapters')"
            >
              <UiBadge tone="info">Chapters</UiBadge>
            </button>
            <UiBadge :tone="downloadStatusTone(props.item.DownloadStatus)">
              {{ downloadStatusLabel(props.item.DownloadStatus) }}
            </UiBadge>
          </div>
        </div>
        <div class="episode-list-item__actions">
          <UiButton size="sm" variant="outline" @click="emit('play', props.item)">
            Play
          </UiButton>
          <UiButton size="sm" variant="outline" @click="emit('toggle-played', props.item)">
            {{ props.item.IsPlayed ? "Mark unplayed" : "Mark played" }}
          </UiButton>
          <UiButton size="sm" variant="outline" @click="emit('toggle-bookmark', props.item)">
            {{ isBookmarked(props.item.BookmarkDate) ? "Unbookmark" : "Bookmark" }}
          </UiButton>
          <UiButton
            size="sm"
            variant="secondary"
            :disabled="isDownloaded(props.item.DownloadStatus)"
            @click="emit('queue-download', props.item)"
          >
            {{
              isDownloaded(props.item.DownloadStatus)
                ? "Downloaded"
                : isPaused(props.item.DownloadStatus)
                  ? "Resume"
                  : "Download"
            }}
          </UiButton>
          <UiButton
            size="sm"
            variant="ghost"
            @click="emit('open-details', props.item, 'overview')"
          >
            Details
          </UiButton>
          <UiButton
            v-if="canCancel(props.item.DownloadStatus)"
            size="sm"
            variant="danger"
            @click="emit('cancel-download', props.item)"
          >
            Stop
          </UiButton>
        </div>
      </div>
      <p class="episode-list-item__summary">{{ props.item.Summary || "No summary available." }}</p>
      <div v-if="props.item.DownloadStatus === 1" class="stack-1">
        <div class="episode-list-item__progress-track">
          <div
            class="episode-list-item__progress-fill"
            :class="!hasKnownTotal(props.item) && 'episode-list-item__progress-fill--unknown'"
            :style="hasKnownTotal(props.item) ? { width: `${progressPercent(props.item)}%` } : undefined"
          />
        </div>
        <p class="meta-text">{{ progressLabel(props.item) }}</p>
      </div>
    </div>
  </UiCard>
</template>

<style scoped>
.episode-list-item {
  display: grid;
  gap: var(--space-3);
  grid-template-columns: 1fr;
}

.episode-list-item__image {
  width: 100%;
  height: 170px;
  border-radius: var(--radius-2);
  background: var(--color-hover);
  object-fit: cover;
}

.episode-list-item__body {
  min-width: 0;
}

.episode-list-item__header {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-3);
}

.episode-list-item__title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
}

.episode-list-item__badges {
  margin-top: var(--space-2);
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.episode-list-item__badge-button {
  border: 0;
  padding: 0;
  background: transparent;
  cursor: pointer;
}

.episode-list-item__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.episode-list-item__summary {
  margin: 0;
  color: var(--color-text-secondary);
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.episode-list-item__progress-track {
  width: 100%;
  height: 6px;
  border-radius: 999px;
  background: var(--color-hover);
  overflow: hidden;
}

.episode-list-item__progress-fill {
  height: 100%;
  border-radius: inherit;
  background: var(--color-accent);
}

.episode-list-item__progress-fill--unknown {
  width: 50%;
  animation: pulse-track 1.2s infinite ease-in-out;
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
  .episode-list-item {
    grid-template-columns: 124px 1fr;
  }

  .episode-list-item__image {
    height: 124px;
  }
}
</style>
