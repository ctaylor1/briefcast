<script setup lang="ts">
import { formatDate } from "../../lib/format";
import type { Podcast } from "../../types/api";
import UiButton from "../ui/UiButton.vue";
import UiCard from "../ui/UiCard.vue";

defineProps<{
  podcasts: Podcast[];
  activeId: string | null;
}>();

const emit = defineEmits<{
  (event: "play", podcastId: string): void;
  (event: "download-all", podcast: Podcast): void;
  (event: "toggle-pause", podcast: Podcast): void;
  (event: "toggle-retention", podcast: Podcast): void;
  (event: "toggle-sponsor-skip", podcast: Podcast): void;
  (event: "delete", podcast: Podcast): void;
}>();

function getPodcastImage(id: string): string {
  return `/podcasts/${id}/image`;
}
</script>

<template>
  <UiCard padding="none" class="overflow-hidden">
    <div class="overflow-x-auto">
      <table class="min-w-full border-collapse">
        <thead class="bg-slate-50 text-left">
          <tr class="text-xs uppercase tracking-wide text-slate-500">
            <th class="px-4 py-3 font-semibold">Podcast</th>
            <th class="px-4 py-3 font-semibold">Last Episode</th>
            <th class="px-4 py-3 font-semibold">Stats</th>
            <th class="px-4 py-3 font-semibold">Status</th>
            <th class="px-4 py-3 font-semibold">Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="podcast in podcasts"
            :key="podcast.ID"
            class="border-t border-slate-200 align-top"
          >
            <td class="px-4 py-3">
              <div class="flex items-start gap-3">
                <img
                  :src="getPodcastImage(podcast.ID)"
                  :alt="podcast.Title"
                  class="h-14 w-14 rounded-md bg-slate-100 object-cover"
                  loading="lazy"
                />
                <div class="space-y-1">
                  <p class="text-sm font-semibold text-slate-900">{{ podcast.Title }}</p>
                  <p class="line-clamp-2 text-xs text-slate-600">
                    {{ podcast.Summary || "No summary available." }}
                  </p>
                </div>
              </div>
            </td>
            <td class="px-4 py-3 text-sm text-slate-700">
              {{ formatDate(podcast.LastEpisode) }}
            </td>
            <td class="px-4 py-3 text-xs text-slate-600">
              <div>Downloaded: {{ podcast.DownloadedEpisodesCount }}</div>
              <div>Queue: {{ podcast.DownloadingEpisodesCount }}</div>
              <div>Total: {{ podcast.AllEpisodesCount }}</div>
            </td>
            <td class="px-4 py-3 text-sm">
              <span
                class="inline-flex rounded-full px-2 py-1 text-xs font-semibold"
                :class="podcast.IsPaused ? 'bg-amber-100 text-amber-800' : 'bg-emerald-100 text-emerald-800'"
              >
                {{ podcast.IsPaused ? "Paused" : "Active" }}
              </span>
              <div class="mt-2 flex items-center gap-2 text-xs text-slate-500">
                <span>Retention: {{ podcast.RetentionKeepAll ? "Keep all" : "Global" }}</span>
                <UiButton
                  size="sm"
                  variant="ghost"
                  :disabled="activeId === podcast.ID"
                  @click="emit('toggle-retention', podcast)"
                >
                  {{ podcast.RetentionKeepAll ? "Use global" : "Keep all" }}
                </UiButton>
              </div>
              <div class="mt-2 flex items-center gap-2 text-xs text-slate-500">
                <span>Sponsor skip: {{ podcast.AutoSkipSponsorChapters ? "On" : "Off" }}</span>
                <UiButton
                  size="sm"
                  variant="ghost"
                  :disabled="activeId === podcast.ID"
                  @click="emit('toggle-sponsor-skip', podcast)"
                >
                  {{ podcast.AutoSkipSponsorChapters ? "Disable" : "Enable" }}
                </UiButton>
              </div>
            </td>
            <td class="px-4 py-3">
              <div class="flex flex-wrap gap-2">
                <UiButton size="sm" :disabled="activeId === podcast.ID" @click="emit('play', podcast.ID)">
                  Play
                </UiButton>
                <UiButton size="sm" variant="outline" :disabled="activeId === podcast.ID" @click="emit('download-all', podcast)">
                  Download
                </UiButton>
                <UiButton size="sm" variant="outline" :disabled="activeId === podcast.ID" @click="emit('toggle-pause', podcast)">
                  {{ podcast.IsPaused ? "Resume" : "Pause" }}
                </UiButton>
                <UiButton size="sm" variant="danger" :disabled="activeId === podcast.ID" @click="emit('delete', podcast)">
                  Delete
                </UiButton>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </UiCard>
</template>
