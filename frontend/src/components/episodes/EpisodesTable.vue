<script setup lang="ts">
import { formatDateTime, formatDuration } from "../../lib/format";
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
}>();

function isBookmarked(value: string): boolean {
  return value !== "0001-01-01T00:00:00Z";
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
                <UiButton size="sm" variant="outline" @click="emit('queue-download', item)">
                  Queue
                </UiButton>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </UiCard>
</template>
