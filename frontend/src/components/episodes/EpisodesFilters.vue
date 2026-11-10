<script setup lang="ts">
import type { EpisodeSorting, EpisodeTriState } from "../../types/api";
import PodcastFilter, { type PodcastFilterOption } from "../ui/PodcastFilter.vue";
import UiButton from "../ui/UiButton.vue";
import UiIcon from "../ui/UiIcon.vue";
import UiCard from "../ui/UiCard.vue";
import UiInput from "../ui/UiInput.vue";
import UiSelect from "../ui/UiSelect.vue";

const props = defineProps<{
  query: string;
  sorting: EpisodeSorting;
  count: number;
  isDownloaded: EpisodeTriState;
  isPlayed: EpisodeTriState;
  favoritesOnly: boolean;
  podcastOptions: PodcastFilterOption[];
  selectedPodcastIds: string[];
}>();

const emit = defineEmits<{
  (event: "update:query", value: string): void;
  (event: "update:sorting", value: EpisodeSorting): void;
  (event: "update:count", value: number): void;
  (event: "update:isDownloaded", value: EpisodeTriState): void;
  (event: "update:isPlayed", value: EpisodeTriState): void;
  (event: "update:favoritesOnly", value: boolean): void;
  (event: "update:selectedPodcastIds", value: string[]): void;
}>();

function clearSelectedPodcasts(): void {
  emit("update:selectedPodcastIds", []);
}

function resetFilters(): void {
  emit("update:query", "");
  clearSelectedPodcasts();
  emit("update:sorting", "release_desc");
  emit("update:count", 20);
  emit("update:isDownloaded", "nil");
  emit("update:isPlayed", "nil");
  emit("update:favoritesOnly", false);
}
</script>

<template>
  <UiCard padding="md" tone="subtle" class="episodes-filters">
    <div class="episodes-filters__row">
      <UiInput
        :model-value="props.query"
        type="search"
        label="Search"
        placeholder="Try a person name, topic, or phrase"
        @update:model-value="emit('update:query', $event)"
      />

      <PodcastFilter
        :options="props.podcastOptions"
        :model-value="props.selectedPodcastIds"
        @update:model-value="emit('update:selectedPodcastIds', $event)"
      />

      <UiSelect
        :model-value="props.sorting"
        label="Sort"
        @update:model-value="emit('update:sorting', $event as EpisodeSorting)"
      >
        <option value="release_desc">Newest release</option>
        <option value="release_asc">Oldest release</option>
        <option value="duration_desc">Longest duration</option>
        <option value="duration_asc">Shortest duration</option>
      </UiSelect>

      <UiSelect
        :model-value="props.count"
        label="Rows"
        @update:model-value="emit('update:count', Number($event))"
      >
        <option :value="10">10 per page</option>
        <option :value="20">20 per page</option>
        <option :value="50">50 per page</option>
        <option :value="100">100 per page</option>
      </UiSelect>

      <UiSelect
        :model-value="props.isDownloaded"
        label="Download status"
        @update:model-value="emit('update:isDownloaded', $event as EpisodeTriState)"
      >
        <option value="nil">All download states</option>
        <option value="true">Downloaded only</option>
        <option value="false">Not downloaded only</option>
      </UiSelect>

      <UiSelect
        :model-value="props.isPlayed"
        label="Play status"
        @update:model-value="emit('update:isPlayed', $event as EpisodeTriState)"
      >
        <option value="nil">All play states</option>
        <option value="true">Played only</option>
        <option value="false">Unplayed only</option>
      </UiSelect>
    </div>

    <div class="episodes-filters__actions">
      <label class="episodes-filters__favorites-toggle">
        <input
          type="checkbox"
          :checked="props.favoritesOnly"
          @change="emit('update:favoritesOnly', !props.favoritesOnly)"
        />
        <UiIcon :name="props.favoritesOnly ? 'star-filled' : 'star'" :size="14" />
        <span>Favorites only</span>
      </label>
      <UiButton size="sm" variant="ghost" @click="resetFilters">
        Reset filters
      </UiButton>
    </div>
  </UiCard>
</template>

<style scoped>
.episodes-filters {
  display: grid;
  gap: var(--space-3);
}

.episodes-filters__row {
  display: grid;
  gap: var(--space-3);
  grid-template-columns: 1fr;
}

.episodes-filters__actions {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  justify-content: flex-end;
}

.episodes-filters__favorites-toggle {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  cursor: pointer;
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
  white-space: nowrap;
}

.episodes-filters__favorites-toggle input[type="checkbox"] {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
}

.episodes-filters__favorites-toggle svg {
  color: var(--color-text-tertiary);
  transition: color var(--duration-fast) var(--ease-enter);
}

.episodes-filters__favorites-toggle:has(input:checked) svg {
  color: var(--color-favorite);
}

@media (min-width: 768px) {
  .episodes-filters__row {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (min-width: 1200px) {
  .episodes-filters__row {
    grid-template-columns: 1.6fr 1.4fr repeat(4, minmax(0, 1fr));
  }
}
</style>
