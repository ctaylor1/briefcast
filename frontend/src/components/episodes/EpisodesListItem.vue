<script setup lang="ts">
import { formatBytes, formatDateTime, formatDuration } from "../../lib/format";
import type { PodcastItem } from "../../types/api";
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

function downloadStatusClass(status: number): string {
  switch (status) {
    case 0:
      return "bg-amber-100 text-amber-800";
    case 1:
      return "bg-blue-100 text-blue-800";
    case 2:
      return "bg-emerald-100 text-emerald-800";
    case 3:
      return "bg-slate-100 text-slate-700";
    case 4:
      return "bg-slate-200 text-slate-700";
    default:
      return "bg-slate-100 text-slate-700";
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

function transcriptPill(status: string): { label: string; className: string; visible: boolean } {
  switch (status) {
    case "available":
      return { label: "Transcript Ready", className: "bg-emerald-100 text-emerald-800", visible: true };
    case "processing":
      return { label: "Transcribing", className: "bg-blue-100 text-blue-800", visible: true };
    case "pending_whisperx":
      return { label: "Transcript Pending", className: "bg-amber-100 text-amber-800", visible: true };
    case "failed":
      return { label: "Transcript Failed", className: "bg-rose-100 text-rose-800", visible: true };
    default:
      return { label: "Transcript Missing", className: "bg-slate-100 text-slate-700", visible: false };
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
  <UiCard class="grid gap-[var(--space-2)] sm:grid-cols-[104px_1fr]">
    <img :src="getImage(props.item.ID)" :alt="props.item.Title" class="h-28 w-full rounded-md bg-slate-100 object-cover sm:h-full" />
    <div class="stack-3">
      <div class="flex flex-wrap items-start justify-between gap-[var(--space-1)]">
        <div>
          <h2 class="text-sm font-semibold text-slate-900 sm:text-base">
            {{ props.item.Title }}
          </h2>
          <p class="text-xs text-slate-500">
            {{ props.item.Podcast?.Title || "Unknown Podcast" }} • {{ formatDateTime(props.item.PubDate) }} •
            {{ formatDuration(props.item.Duration) }}
          </p>
          <div class="mt-2 flex flex-wrap gap-2 text-xs">
            <button
              v-if="transcriptPill(props.item.TranscriptStatus).visible"
              type="button"
              class="rounded-full px-2 py-1 font-medium"
              :class="transcriptPill(props.item.TranscriptStatus).className"
              @click="emit('open-details', props.item, 'transcript')"
            >
              {{ transcriptPill(props.item.TranscriptStatus).label }}
            </button>
            <button
              v-if="props.item.HasChapters"
              type="button"
              class="rounded-full bg-violet-100 px-2 py-1 font-medium text-violet-800"
              @click="emit('open-details', props.item, 'chapters')"
            >
              Chapters
            </button>
            <span class="rounded-full px-2 py-1 font-medium" :class="downloadStatusClass(props.item.DownloadStatus)">
              {{ downloadStatusLabel(props.item.DownloadStatus) }}
            </span>
          </div>
        </div>
        <div class="flex flex-wrap gap-[var(--space-1)]">
          <UiButton size="sm" variant="outline" @click="emit('play', props.item)">
            Play
          </UiButton>
          <UiButton size="sm" variant="outline" @click="emit('toggle-played', props.item)">
            {{ props.item.IsPlayed ? "Mark Unplayed" : "Mark Played" }}
          </UiButton>
          <UiButton size="sm" variant="outline" @click="emit('toggle-bookmark', props.item)">
            {{ isBookmarked(props.item.BookmarkDate) ? "Unbookmark" : "Bookmark" }}
          </UiButton>
          <UiButton
            size="sm"
            variant="outline"
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
      <p class="line-clamp-3 text-sm text-slate-600">{{ props.item.Summary || "No summary available." }}</p>
      <div v-if="props.item.DownloadStatus === 1" class="stack-1">
        <div class="h-1.5 w-full overflow-hidden rounded-full bg-slate-100">
          <div
            class="h-full rounded-full bg-blue-500"
            :class="!hasKnownTotal(props.item) && 'animate-pulse w-1/2'"
            :style="hasKnownTotal(props.item) ? { width: `${progressPercent(props.item)}%` } : undefined"
          />
        </div>
        <p class="text-xs text-slate-500">{{ progressLabel(props.item) }}</p>
      </div>
    </div>
  </UiCard>
</template>
