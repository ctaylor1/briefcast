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
}>();

function getImage(id: string): string {
  return `/podcastitems/${id}/image`;
}

function isBookmarked(value: string): boolean {
  return value !== "0001-01-01T00:00:00Z";
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
          <UiButton size="sm" variant="outline" @click="emit('queue-download', props.item)">
            Queue Download
          </UiButton>
        </div>
      </div>
      <p class="line-clamp-3 text-sm text-slate-600">{{ props.item.Summary || "No summary available." }}</p>
    </div>
  </UiCard>
</template>
