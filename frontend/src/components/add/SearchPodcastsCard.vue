<script setup lang="ts">
import type { SearchResult, SearchSource } from "../../types/api";
import UiButton from "../ui/UiButton.vue";
import UiCard from "../ui/UiCard.vue";
import UiInput from "../ui/UiInput.vue";
import UiSelect from "../ui/UiSelect.vue";

defineProps<{
  query: string;
  source: SearchSource;
  working: boolean;
  results: SearchResult[];
}>();

const emit = defineEmits<{
  (event: "update:query", value: string): void;
  (event: "update:source", value: SearchSource): void;
  (event: "search"): void;
  (event: "add", url: string): void;
}>();
</script>

<template>
  <UiCard padding="lg" class="search-card stack-3">
    <div class="stack-1">
      <h3 class="search-card__title">Discover podcasts</h3>
      <p class="meta-text">Search major podcast directories and add any result with one click.</p>
    </div>

    <form class="search-card__form" @submit.prevent="emit('search')">
      <UiInput
        :model-value="query"
        type="search"
        label="Search query"
        required
        placeholder="Search podcast title"
        @update:model-value="emit('update:query', $event)"
      />
      <UiSelect
        :model-value="source"
        label="Source"
        @update:model-value="emit('update:source', $event as SearchSource)"
      >
        <option value="itunes">iTunes</option>
        <option value="podcastindex">PodcastIndex</option>
      </UiSelect>
      <UiButton type="submit" :disabled="working" class="search-card__submit">
        {{ working ? "Searching..." : "Search" }}
      </UiButton>
    </form>

    <p v-if="working" class="meta-text" role="status">Searching directory...</p>

    <UiCard
      v-else-if="query && results.length === 0"
      padding="md"
      tone="subtle"
      class="empty-state"
    >
      <p class="search-card__empty-title">No matches for “{{ query }}”</p>
      <p class="meta-text">Try a broader term or switch sources.</p>
    </UiCard>

    <ul v-else-if="results.length > 0" class="search-card__results">
      <li v-for="item in results" :key="item.url">
        <article class="search-card__result">
          <img
            :src="item.image"
            :alt="item.title"
            class="search-card__result-image"
            loading="lazy"
          />
          <div class="stack-2">
            <h4 class="search-card__result-title">{{ item.title }}</h4>
            <p class="search-card__result-description">
              {{ item.description || "No description provided." }}
            </p>
            <p v-if="item.categories?.length" class="meta-text">
              {{ item.categories.join(", ") }}
            </p>
          </div>
          <div class="search-card__result-action">
            <p class="meta-text search-card__source">{{ source === "itunes" ? "iTunes" : "PodcastIndex" }}</p>
            <UiButton
              size="sm"
              variant="outline"
              :disabled="item.already_saved"
              @click="emit('add', item.url)"
            >
              {{ item.already_saved ? "Added" : "Add" }}
            </UiButton>
          </div>
        </article>
      </li>
    </ul>
  </UiCard>
</template>

<style scoped>
.search-card__title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
}

.search-card__form {
  display: grid;
  gap: var(--space-3);
}

.search-card__submit {
  width: 100%;
}

.search-card__results {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: var(--space-3);
}

.search-card__result {
  border: 1px dashed var(--color-border);
  border-radius: var(--radius-3);
  background: var(--color-bg-primary);
  padding: var(--space-3);
  display: grid;
  gap: var(--space-3);
}

.search-card__result-image {
  width: 100%;
  max-width: 96px;
  aspect-ratio: 1 / 1;
  border-radius: var(--radius-2);
  background: var(--color-hover);
  object-fit: cover;
}

.search-card__result-title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
}

.search-card__result-description {
  margin: 0;
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.search-card__result-action {
  display: grid;
  justify-items: start;
  gap: var(--space-2);
}

.search-card__source {
  margin: 0;
}

.search-card__empty-title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
}

@media (min-width: 768px) {
  .search-card__form {
    grid-template-columns: 1fr 180px auto;
    align-items: end;
  }

  .search-card__submit {
    width: auto;
  }

  .search-card__result {
    grid-template-columns: 96px 1fr auto;
    align-items: start;
  }

  .search-card__result-action {
    justify-items: end;
  }
}
</style>
