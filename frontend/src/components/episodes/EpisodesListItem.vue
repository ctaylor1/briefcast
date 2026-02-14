<script setup lang="ts">
import { formatDateTime, formatDuration } from "../../lib/format";
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
</script>

<template>
  <UiCard class="grid gap-[var(--space-2)] sm:grid-cols-[104px_1fr]">
    <img :src="getImage(props.item.ID)" :alt="props.item.Title" class="h-28 w-full rounded-md bg-slate-100 object-cover sm:h-full" />
    <div class="stack-3">
      <div class="flex flex-wrap items-start justify-between gap-[var(--space-1)]">
        <div>
          <h2 class="text-sm font-semibold text-slate-900 sm:text-base">{{ props.item.Title }}</h2>
          <p class="text-xs text-slate-500">
            {{ props.item.Podcast?.Title || "Unknown Podcast" }} • {{ formatDateTime(props.item.PubDate) }} •
            {{ formatDuration(props.item.Duration) }}
          </p>
          <div class="mt-2 flex flex-wrap gap-2 text-xs">
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
            {{ isDownloaded(props.item.DownloadStatus) ? "Downloaded" : "Download" }}
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
    </div>
  </UiCard>
</template>
