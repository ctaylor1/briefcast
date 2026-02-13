<script setup lang="ts">
import type { EpisodeSorting, EpisodeTriState } from "../../types/api";
import UiCard from "../ui/UiCard.vue";

const props = defineProps<{
  query: string;
  sorting: EpisodeSorting;
  count: number;
  isDownloaded: EpisodeTriState;
  isPlayed: EpisodeTriState;
}>();

const emit = defineEmits<{
  (event: "update:query", value: string): void;
  (event: "update:sorting", value: EpisodeSorting): void;
  (event: "update:count", value: number): void;
  (event: "update:isDownloaded", value: EpisodeTriState): void;
  (event: "update:isPlayed", value: EpisodeTriState): void;
}>();
</script>

<template>
  <UiCard padding="sm" class="grid gap-[var(--space-2)] sm:grid-cols-2 xl:grid-cols-5">
    <input
      :value="props.query"
      type="search"
      class="min-h-10 rounded-md border border-slate-300 px-3 py-2 text-sm focus:border-cyan-500 focus:outline-none"
      placeholder="Search episodes"
      @input="emit('update:query', ($event.target as HTMLInputElement).value)"
    />
    <select
      :value="props.sorting"
      class="min-h-10 rounded-md border border-slate-300 px-3 py-2 text-sm focus:border-cyan-500 focus:outline-none"
      @change="emit('update:sorting', ($event.target as HTMLSelectElement).value as EpisodeSorting)"
    >
      <option value="release_desc">Release (newest)</option>
      <option value="release_asc">Release (oldest)</option>
      <option value="duration_desc">Duration (longest)</option>
      <option value="duration_asc">Duration (shortest)</option>
    </select>
    <select
      :value="props.count"
      class="min-h-10 rounded-md border border-slate-300 px-3 py-2 text-sm focus:border-cyan-500 focus:outline-none"
      @change="emit('update:count', Number(($event.target as HTMLSelectElement).value))"
    >
      <option :value="10">10 / page</option>
      <option :value="20">20 / page</option>
      <option :value="50">50 / page</option>
      <option :value="100">100 / page</option>
    </select>
    <select
      :value="props.isDownloaded"
      class="min-h-10 rounded-md border border-slate-300 px-3 py-2 text-sm focus:border-cyan-500 focus:outline-none"
      @change="emit('update:isDownloaded', ($event.target as HTMLSelectElement).value as EpisodeTriState)"
    >
      <option value="nil">All download states</option>
      <option value="true">Downloaded only</option>
      <option value="false">Not downloaded only</option>
    </select>
    <select
      :value="props.isPlayed"
      class="min-h-10 rounded-md border border-slate-300 px-3 py-2 text-sm focus:border-cyan-500 focus:outline-none"
      @change="emit('update:isPlayed', ($event.target as HTMLSelectElement).value as EpisodeTriState)"
    >
      <option value="nil">All play states</option>
      <option value="true">Played only</option>
      <option value="false">Unplayed only</option>
    </select>
  </UiCard>
</template>
