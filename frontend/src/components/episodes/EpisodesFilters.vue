<script setup lang="ts">
import { computed, ref } from "vue";
import type { EpisodeSorting, EpisodeTriState } from "../../types/api";
import UiButton from "../ui/UiButton.vue";
import UiCard from "../ui/UiCard.vue";
import UiInput from "../ui/UiInput.vue";
import UiSelect from "../ui/UiSelect.vue";

interface PodcastFilterOption {
  id: string;
  title: string;
}

const props = defineProps<{
  query: string;
  sorting: EpisodeSorting;
  count: number;
  isDownloaded: EpisodeTriState;
  isPlayed: EpisodeTriState;
  podcastOptions: PodcastFilterOption[];
  selectedPodcastIds: string[];
}>();

const emit = defineEmits<{
  (event: "update:query", value: string): void;
  (event: "update:sorting", value: EpisodeSorting): void;
  (event: "update:count", value: number): void;
  (event: "update:isDownloaded", value: EpisodeTriState): void;
  (event: "update:isPlayed", value: EpisodeTriState): void;
  (event: "update:selectedPodcastIds", value: string[]): void;
}>();

const podcastSearch = ref("");

const filteredPodcastOptions = computed(() => {
  const term = podcastSearch.value.trim().toLowerCase();
  if (!term) {
    return props.podcastOptions;
  }
  return props.podcastOptions.filter((podcast) => podcast.title.toLowerCase().includes(term));
});

const selectedPodcastLabel = computed(() => {
  if (props.selectedPodcastIds.length === 0) {
    return "All podcasts";
  }
  if (props.selectedPodcastIds.length === 1) {
    const selected = props.podcastOptions.find((podcast) => podcast.id === props.selectedPodcastIds[0]);
    return selected?.title ?? "1 podcast";
  }
  return `${props.selectedPodcastIds.length} podcasts`;
});

function isSelectedPodcast(id: string): boolean {
  return props.selectedPodcastIds.includes(id);
}

function togglePodcast(id: string): void {
  const selected = new Set(props.selectedPodcastIds);
  if (selected.has(id)) {
    selected.delete(id);
  } else {
    selected.add(id);
  }
  emit("update:selectedPodcastIds", Array.from(selected));
}

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

      <div class="episodes-filters__podcast-filter">
        <label class="ui-label" for="episodes-podcast-filter-trigger">Podcasts</label>
        <details class="episodes-filters__podcast-dropdown">
          <summary id="episodes-podcast-filter-trigger" class="episodes-filters__podcast-trigger">
            <span class="episodes-filters__podcast-label">{{ selectedPodcastLabel }}</span>
            <span class="meta-text">
              {{
                props.selectedPodcastIds.length === 0
                  ? "No podcast filter"
                  : `${props.selectedPodcastIds.length} selected`
              }}
            </span>
          </summary>
          <div class="episodes-filters__podcast-panel">
            <input
              v-model="podcastSearch"
              type="search"
              class="ui-input episodes-filters__podcast-search"
              placeholder="Search podcasts"
            />

            <p v-if="filteredPodcastOptions.length === 0" class="meta-text">
              No podcasts match.
            </p>
            <ul v-else class="episodes-filters__podcast-list visually-scrollable">
              <li v-for="podcast in filteredPodcastOptions" :key="podcast.id">
                <label class="episodes-filters__podcast-option">
                  <input
                    type="checkbox"
                    :checked="isSelectedPodcast(podcast.id)"
                    @change="togglePodcast(podcast.id)"
                  />
                  <span>{{ podcast.title }}</span>
                </label>
              </li>
            </ul>

            <div class="episodes-filters__podcast-actions">
              <UiButton
                size="sm"
                variant="ghost"
                :disabled="props.selectedPodcastIds.length === 0"
                @click="clearSelectedPodcasts"
              >
                Clear podcasts
              </UiButton>
            </div>
          </div>
        </details>
      </div>

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

.episodes-filters__podcast-filter {
  display: grid;
  gap: var(--space-2);
}

.episodes-filters__podcast-dropdown {
  position: relative;
}

.episodes-filters__podcast-dropdown > summary {
  list-style: none;
}

.episodes-filters__podcast-dropdown > summary::-webkit-details-marker {
  display: none;
}

.episodes-filters__podcast-trigger {
  min-height: 48px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-primary);
  color: var(--color-text-primary);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  cursor: pointer;
}

.episodes-filters__podcast-label {
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.episodes-filters__podcast-panel {
  position: absolute;
  top: calc(100% + var(--space-2));
  left: 0;
  right: 0;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-3);
  background: var(--color-bg-primary);
  padding: var(--space-3);
  z-index: 30;
  display: grid;
  gap: var(--space-2);
}

.episodes-filters__podcast-dropdown:not([open]) .episodes-filters__podcast-panel {
  display: none;
}

.episodes-filters__podcast-search {
  min-height: 40px;
}

.episodes-filters__podcast-list {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: var(--space-1);
  max-height: 220px;
  overflow: auto;
}

.episodes-filters__podcast-option {
  min-height: 36px;
  display: flex;
  align-items: center;
  gap: var(--space-2);
  color: var(--color-text-primary);
  cursor: pointer;
}

.episodes-filters__podcast-option input[type="checkbox"] {
  width: 16px;
  height: 16px;
  accent-color: var(--color-accent);
}

.episodes-filters__podcast-actions {
  display: flex;
  justify-content: flex-end;
}

.episodes-filters__actions {
  display: flex;
  justify-content: flex-end;
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
