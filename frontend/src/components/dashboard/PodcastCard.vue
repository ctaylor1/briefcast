<script setup lang="ts">
import type { Podcast } from "../../types/api";
import { formatDate } from "../../lib/format";
import UiBadge from "../ui/UiBadge.vue";
import UiButton from "../ui/UiButton.vue";
import UiCard from "../ui/UiCard.vue";

defineProps<{
  podcast: Podcast;
  busy: boolean;
}>();

const emit = defineEmits<{
  (event: "play", podcastId: string): void;
  (event: "download-all", podcast: Podcast): void;
  (event: "toggle-pause", podcast: Podcast): void;
  (event: "delete", podcast: Podcast): void;
}>();

function getPodcastImage(id: string): string {
  return `/podcasts/${id}/image`;
}
</script>

<template>
  <UiCard padding="none" class="overflow-hidden">
    <img
      :src="getPodcastImage(podcast.ID)"
      :alt="podcast.Title"
      class="h-44 w-full bg-slate-100 object-cover"
      loading="lazy"
    />
    <div class="stack-3 p-[var(--space-3)]">
      <div class="stack-2">
        <h2 class="line-clamp-2 text-base font-semibold text-slate-900">{{ podcast.Title }}</h2>
        <p class="text-xs text-slate-500">Last episode: {{ formatDate(podcast.LastEpisode) }}</p>
      </div>
      <p class="line-clamp-3 text-sm text-slate-600">{{ podcast.Summary || "No summary available." }}</p>
      <div class="flex flex-wrap gap-[var(--space-1)]">
        <UiBadge>
          Downloaded: {{ podcast.DownloadedEpisodesCount }}
        </UiBadge>
        <UiBadge>
          Queue: {{ podcast.DownloadingEpisodesCount }}
        </UiBadge>
        <UiBadge>
          Total: {{ podcast.AllEpisodesCount }}
        </UiBadge>
      </div>
      <div class="grid grid-cols-2 gap-[var(--space-1)]">
        <UiButton size="sm" :disabled="busy" @click="emit('play', podcast.ID)">
          Play
        </UiButton>
        <UiButton size="sm" variant="outline" :disabled="busy" @click="emit('download-all', podcast)">
          Download All
        </UiButton>
        <UiButton size="sm" variant="outline" :disabled="busy" @click="emit('toggle-pause', podcast)">
          {{ podcast.IsPaused ? "Resume" : "Pause" }}
        </UiButton>
        <UiButton size="sm" variant="danger" :disabled="busy" @click="emit('delete', podcast)">
          Delete
        </UiButton>
      </div>
      <a
        class="inline-flex text-xs font-medium text-cyan-700 underline decoration-cyan-300 underline-offset-2 hover:text-cyan-800"
        :href="`/podcasts/${podcast.ID}/view`"
      >
        Open Legacy Episode List
      </a>
    </div>
  </UiCard>
</template>
