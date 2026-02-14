<script setup lang="ts">
import type { EpisodeSorting, EpisodeTriState } from "../../types/api";
import UiCard from "../ui/UiCard.vue";
import UiInput from "../ui/UiInput.vue";
import UiSelect from "../ui/UiSelect.vue";

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
    <UiInput
      :model-value="props.query"
      type="search"
      placeholder="Search episodes"
      @update:model-value="emit('update:query', $event)"
    />
    <UiSelect
      :model-value="props.sorting"
      @update:model-value="emit('update:sorting', $event as EpisodeSorting)"
    >
      <option value="release_desc">Release (newest)</option>
      <option value="release_asc">Release (oldest)</option>
      <option value="duration_desc">Duration (longest)</option>
      <option value="duration_asc">Duration (shortest)</option>
    </UiSelect>
    <UiSelect
      :model-value="props.count"
      @update:model-value="emit('update:count', Number($event))"
    >
      <option :value="10">10 / page</option>
      <option :value="20">20 / page</option>
      <option :value="50">50 / page</option>
      <option :value="100">100 / page</option>
    </UiSelect>
    <UiSelect
      :model-value="props.isDownloaded"
      @update:model-value="emit('update:isDownloaded', $event as EpisodeTriState)"
    >
      <option value="nil">All download states</option>
      <option value="true">Downloaded only</option>
      <option value="false">Not downloaded only</option>
    </UiSelect>
    <UiSelect
      :model-value="props.isPlayed"
      @update:model-value="emit('update:isPlayed', $event as EpisodeTriState)"
    >
      <option value="nil">All play states</option>
      <option value="true">Played only</option>
      <option value="false">Unplayed only</option>
    </UiSelect>
  </UiCard>
</template>
