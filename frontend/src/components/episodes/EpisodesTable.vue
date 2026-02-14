<script setup lang="ts">
import { formatBytes, formatDateTime, formatDuration } from "../../lib/format";
import type { PodcastItem } from "../../types/api";
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
  <UiCard padding="none" class="overflow-hidden">
    <div class="overflow-x-auto">
      <table class="min-w-full border-collapse">
        <thead class="bg-slate-50 text-left">
          <tr class="text-xs uppercase tracking-wide text-slate-500">
            <th class="px-4 py-3 font-semibold">Episode</th>
            <th class="px-4 py-3 font-semibold">Podcast</th>
            <th class="px-4 py-3 font-semibold">Published</th>
            <th class="px-4 py-3 font-semibold">Duration</th>
            <th class="px-4 py-3 font-semibold">Flags</th>
            <th class="px-4 py-3 font-semibold">Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="item in items"
            :key="item.ID"
            class="border-t border-slate-200 align-top"
          >
            <td class="px-4 py-3">
              <div class="max-w-sm space-y-1">
                <p class="text-sm font-semibold text-slate-900">{{ item.Title }}</p>
                <p class="line-clamp-2 text-xs text-slate-600">{{ item.Summary || "No summary available." }}</p>
                <div class="flex flex-wrap gap-2 text-[11px]">
                  <button
                    v-if="transcriptPill(item.TranscriptStatus).visible"
                    type="button"
                    class="rounded-full px-2 py-1 font-medium"
                    :class="transcriptPill(item.TranscriptStatus).className"
                    @click="emit('open-details', item, 'transcript')"
                  >
                    {{ transcriptPill(item.TranscriptStatus).label }}
                  </button>
                  <button
                    v-if="item.HasChapters"
                    type="button"
                    class="rounded-full bg-violet-100 px-2 py-1 font-medium text-violet-800"
                    @click="emit('open-details', item, 'chapters')"
                  >
                    Chapters
                  </button>
                </div>
              </div>
            </td>
            <td class="px-4 py-3 text-sm text-slate-700">{{ item.Podcast?.Title || "Unknown Podcast" }}</td>
            <td class="px-4 py-3 text-sm text-slate-700">{{ formatDateTime(item.PubDate) }}</td>
            <td class="px-4 py-3 text-sm text-slate-700">{{ formatDuration(item.Duration) }}</td>
            <td class="px-4 py-3">
              <div class="flex flex-wrap gap-1.5 text-xs">
                <span
                  class="rounded-full px-2 py-1 font-medium"
                  :class="item.IsPlayed ? 'bg-emerald-100 text-emerald-800' : 'bg-slate-100 text-slate-700'"
                >
                  {{ item.IsPlayed ? "Played" : "Unplayed" }}
                </span>
                <span
                  class="rounded-full px-2 py-1 font-medium"
                  :class="isBookmarked(item.BookmarkDate) ? 'bg-cyan-100 text-cyan-800' : 'bg-slate-100 text-slate-700'"
                >
                  {{ isBookmarked(item.BookmarkDate) ? "Bookmarked" : "No Bookmark" }}
                </span>
                <span
                  class="rounded-full px-2 py-1 font-medium"
                  :class="downloadStatusClass(item.DownloadStatus)"
                >
                  {{ downloadStatusLabel(item.DownloadStatus) }}
                </span>
              </div>
              <div v-if="item.DownloadStatus === 1" class="mt-2">
                <div class="h-1.5 w-28 overflow-hidden rounded-full bg-slate-100">
                  <div
                    class="h-full rounded-full bg-blue-500"
                    :class="!hasKnownTotal(item) && 'animate-pulse w-1/2'"
                    :style="hasKnownTotal(item) ? { width: `${progressPercent(item)}%` } : undefined"
                  />
                </div>
                <p class="mt-1 text-[10px] text-slate-500">{{ progressLabel(item) }}</p>
              </div>
            </td>
            <td class="px-4 py-3">
              <div class="flex flex-wrap gap-2">
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
                  variant="outline"
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
