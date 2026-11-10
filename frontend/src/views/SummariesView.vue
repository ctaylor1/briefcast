<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { RouterLink } from "vue-router";
import EpisodesPagination from "../components/episodes/EpisodesPagination.vue";
import PodcastFilter from "../components/ui/PodcastFilter.vue";
import UiAlert from "../components/ui/UiAlert.vue";
import UiBadge from "../components/ui/UiBadge.vue";
import UiButton from "../components/ui/UiButton.vue";
import UiCard from "../components/ui/UiCard.vue";
import UiIcon from "../components/ui/UiIcon.vue";
import UiInput from "../components/ui/UiInput.vue";
import UiSelect from "../components/ui/UiSelect.vue";
import { useDebouncedWatch } from "../composables/useDebouncedWatch";
import { getErrorMessage, podcastsApi } from "../lib/api";
import { summariesApi } from "../lib/api/summaries";
import { formatDate } from "../lib/format";
import { createLatest } from "../lib/latest";
import { setReadingList } from "../lib/readingList";
import { episodePath } from "../lib/routes";
import { toast } from "../lib/toast";
import type { Podcast, SummaryListItem, SummarySorting } from "../types/api";

// The list of AI summaries. A summary opens on its episode page (Summary tab); the page
// gets this list as a reading list, so J/K and Previous/Next keep working there.
const isLoading = ref(true);
const errorMessage = ref("");
const items = ref<SummaryListItem[]>([]);
const podcastOptions = ref<Podcast[]>([]);
const filter = reactive<{
  q: string;
  podcastIds: string[];
  page: number;
  count: number;
  sorting: SummarySorting;
  favoritesOnly: boolean;
  nextPage: number;
  previousPage: number;
  totalPages: number;
  totalCount: number;
}>({
  q: "",
  podcastIds: [],
  page: 1,
  count: 20,
  sorting: "newest",
  favoritesOnly: false,
  nextPage: 0,
  previousPage: 0,
  totalPages: 0,
  totalCount: 0,
});

const podcastFilterOptions = computed(() =>
  podcastOptions.value.map((podcast) => ({ id: podcast.ID, title: podcast.Title })),
);

const hasActiveFilters = computed(
  () =>
    filter.q.trim().length > 0 ||
    filter.podcastIds.length > 0 ||
    filter.sorting !== "newest" ||
    filter.count !== 20 ||
    filter.favoritesOnly,
);

async function loadPodcastOptions(): Promise<void> {
  try {
    podcastOptions.value = await podcastsApi.list();
  } catch {
    podcastOptions.value = [];
  }
}

const summariesRequest = createLatest();

async function fetchSummaries(): Promise<void> {
  isLoading.value = true;
  errorMessage.value = "";
  let stale = false;
  try {
    const result = await summariesRequest.run(() =>
      summariesApi.list({
        page: filter.page,
        count: filter.count,
        sorting: filter.sorting,
        q: filter.q.trim() || undefined,
        podcastIds: filter.podcastIds.length > 0 ? filter.podcastIds : undefined,
        favoritesOnly: filter.favoritesOnly || undefined,
      }),
    );
    stale = result.stale;
    if (stale) {
      return;
    }
    const response = result.value;
    items.value = response.summaries;
    const next = response.filter;
    filter.page = next.page;
    filter.count = next.count;
    filter.nextPage = next.nextPage;
    filter.previousPage = next.previousPage;
    filter.totalPages = next.totalPages;
    filter.totalCount = next.totalCount;
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Failed to load summaries.");
  } finally {
    if (!stale) {
      isLoading.value = false;
    }
  }
}

function resetFilters(): void {
  filter.q = "";
  filter.podcastIds = [];
  filter.page = 1;
  filter.count = 20;
  filter.sorting = "newest";
  filter.favoritesOnly = false;
  void fetchSummaries();
}

async function toggleFavorite(item: SummaryListItem, event: Event): Promise<void> {
  event.stopPropagation();
  try {
    if (item.isFavorited) {
      await summariesApi.unfavorite(item.id);
      item.isFavorited = false;
    } else {
      await summariesApi.favorite(item.id);
      item.isFavorited = true;
    }
  } catch (error) {
    toast.error(getErrorMessage(error, "Failed to update favorite."));
  }
}

function summaryLink(item: SummaryListItem) {
  return { path: episodePath(item.id), query: { tab: "summary" } };
}

function rememberReadingList(): void {
  setReadingList(
    items.value.map((item) => item.id),
    "Summary",
  );
}

watch(
  () => [filter.count, filter.sorting, filter.podcastIds.join(","), filter.favoritesOnly],
  () => {
    filter.page = 1;
    void fetchSummaries();
  },
);

useDebouncedWatch(
  () => filter.q,
  () => {
    filter.page = 1;
    void fetchSummaries();
  },
  300,
);

onMounted(() => {
  void fetchSummaries();
  void loadPodcastOptions();
});
</script>

<template>
  <section class="summaries-page stack-4">
    <header class="page-header">
      <h2 class="section-title">Summaries</h2>
      <p class="section-subtitle">
        Browse and read AI-generated summaries of your podcast episodes.
      </p>
    </header>

    <UiAlert v-if="errorMessage" tone="danger">
      {{ errorMessage }}
    </UiAlert>

    <!-- ═══ Reader View ═══════════════════════════════════════ -->
    <UiCard padding="md" tone="subtle" class="summaries-filters">
      <div class="summaries-filters__row">
        <UiInput
          :model-value="filter.q"
          type="search"
          label="Search summaries"
          placeholder="Search by title or content"
          @update:model-value="filter.q = $event"
        />

        <PodcastFilter
          label="Show"
          :options="podcastFilterOptions"
          :model-value="filter.podcastIds"
          @update:model-value="filter.podcastIds = $event"
        />

        <UiSelect
          :model-value="filter.sorting"
          label="Sort"
          @update:model-value="filter.sorting = $event as SummarySorting"
        >
          <option value="newest">Newest first</option>
          <option value="oldest">Oldest first</option>
          <option value="title_asc">Title (A–Z)</option>
          <option value="title_desc">Title (Z–A)</option>
          <option value="shortest">Shortest read</option>
          <option value="longest">Longest read</option>
        </UiSelect>

        <UiSelect
          :model-value="filter.count"
          label="Rows"
          @update:model-value="filter.count = Number($event)"
        >
          <option :value="10">10 per page</option>
          <option :value="20">20 per page</option>
          <option :value="50">50 per page</option>
          <option :value="100">100 per page</option>
        </UiSelect>
      </div>

      <div class="summaries-filters__footer">
        <span v-if="!isLoading" class="meta-text">
          {{ filter.totalCount }} {{ filter.totalCount === 1 ? "summary" : "summaries" }}
        </span>
        <div class="summaries-filters__footer-actions">
          <label class="summaries-filters__favorites-toggle">
            <input
              type="checkbox"
              :checked="filter.favoritesOnly"
              @change="filter.favoritesOnly = !filter.favoritesOnly"
            />
            <UiIcon :name="filter.favoritesOnly ? 'star-filled' : 'star'" :size="14" />
            <span>Favorites only</span>
          </label>
          <UiButton v-if="hasActiveFilters" size="sm" variant="ghost" @click="resetFilters">
            Reset filters
          </UiButton>
        </div>
      </div>
    </UiCard>

    <UiCard v-if="isLoading" padding="md" class="summaries-skeleton">
      <div v-for="index in 5" :key="index" class="summaries-skeleton__row">
        <span class="skeleton summaries-skeleton__line summaries-skeleton__line--title"></span>
        <span class="skeleton summaries-skeleton__line"></span>
        <span class="skeleton summaries-skeleton__line summaries-skeleton__line--short"></span>
      </div>
    </UiCard>

    <UiCard v-else-if="items.length === 0" padding="lg" class="empty-state">
      <p class="empty-state__title">
        {{ filter.favoritesOnly ? "No favorite summaries" : "No summaries available" }}
      </p>
      <p class="empty-state__copy">
        <template v-if="filter.favoritesOnly">
          Star summaries you want to revisit and they will appear here.
        </template>
        <template v-else>
          Summaries are generated automatically after episodes are transcribed. Check your
          <RouterLink to="/settings">summarization settings</RouterLink> to enable AI summaries.
        </template>
      </p>
      <UiButton v-if="hasActiveFilters" variant="secondary" size="sm" @click="resetFilters">
        Reset filters
      </UiButton>
    </UiCard>

    <div v-else class="summaries-list">
      <div
        v-for="item in items"
        :key="item.id"
        class="summaries-list__row"
      >
        <button
          type="button"
          class="summaries-list__fav-btn"
          :class="{ 'summaries-list__fav-btn--active': item.isFavorited }"
          :title="item.isFavorited ? 'Remove from favorites' : 'Add to favorites'"
          :aria-label="`Favorite: ${item.episodeTitle}`"
          :aria-pressed="item.isFavorited ? 'true' : 'false'"
          @click="toggleFavorite(item, $event)"
        >
          <UiIcon :name="item.isFavorited ? 'star-filled' : 'star'" :size="16" />
        </button>
        <RouterLink :to="summaryLink(item)" class="summaries-list__content" @click="rememberReadingList">
          <div class="summaries-list__header">
            <h3 class="summaries-list__title">{{ item.episodeTitle }}</h3>
            <div class="summaries-list__badges">
              <UiBadge tone="info">{{ item.readTime }} min</UiBadge>
              <UiBadge v-if="item.isPlayed" tone="success">Played</UiBadge>
            </div>
          </div>
          <div class="summaries-list__meta">
            <span>{{ item.podcastTitle }}</span>
            <span v-if="item.pubDate"> &middot; {{ formatDate(item.pubDate) }}</span>
            <span v-if="item.generatedAt"> &middot; Summary: {{ formatDate(item.generatedAt) }}</span>
            <span v-if="item.model"> &middot; Summary: {{ item.model }}</span>
          </div>
          </RouterLink>
      </div>
    </div>

    <EpisodesPagination
      noun="summaries"
      :page="filter.page"
      :total-pages="filter.totalPages"
      :total-count="filter.totalCount"
      :has-previous="filter.page > 1 && filter.previousPage > 0"
      :has-next="filter.nextPage > 0"
      @first="filter.page = 1; void fetchSummaries()"
      @previous="filter.page = filter.previousPage; void fetchSummaries()"
      @next="filter.page = filter.nextPage; void fetchSummaries()"
    />
  </section>
</template>

<style scoped>
.summaries-filters {
  display: grid;
  gap: var(--space-3);
}

.summaries-filters__row {
  display: grid;
  gap: var(--space-3);
  grid-template-columns: 1fr;
}

.summaries-filters__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
}

.summaries-filters__footer-actions {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.summaries-filters__favorites-toggle {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  cursor: pointer;
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
  white-space: nowrap;
}

.summaries-filters__favorites-toggle input[type="checkbox"] {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
}

.summaries-filters__favorites-toggle svg {
  color: var(--color-text-tertiary);
  transition: color var(--duration-fast) var(--ease-enter);
}

.summaries-filters__favorites-toggle:has(input:checked) svg {
  color: var(--color-favorite);
}

.summaries-skeleton {
  display: grid;
  gap: var(--space-4);
}

.summaries-skeleton__row {
  display: grid;
  gap: var(--space-2);
}

.summaries-skeleton__line {
  height: 12px;
}

.summaries-skeleton__line--title {
  width: 64%;
  height: 18px;
}

.summaries-skeleton__line--short {
  width: 40%;
}

.summaries-list {
  display: grid;
  gap: var(--space-2);
}

.summaries-list__row {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-3);
  background: var(--color-bg-primary);
  padding: var(--space-3) var(--space-4);
  transition: background-color var(--duration-fast) var(--ease-enter);
}

.summaries-list__row:hover {
  background: var(--color-hover);
}

.summaries-list__fav-btn {
  flex: 0 0 auto;
  border: 0;
  background: transparent;
  padding: var(--space-1);
  margin-top: 2px;
  cursor: pointer;
  color: var(--color-text-tertiary);
  transition: color var(--duration-fast) var(--ease-enter);
}

.summaries-list__fav-btn:hover {
  color: var(--color-favorite);
}

.summaries-list__fav-btn--active {
  color: var(--color-favorite);
}

.summaries-list__content {
  flex: 1;
  min-width: 0;
  border: 0;
  background: transparent;
  color: inherit;
  text-align: left;
  padding: 0;
  cursor: pointer;
  display: grid;
  gap: var(--space-2);
  text-decoration: none;
  border-radius: var(--radius-2);
}

.summaries-list__content:hover .summaries-list__title {
  color: var(--color-accent-hover);
}

.summaries-list__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-3);
}

.summaries-list__title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
  overflow-wrap: anywhere;
}

.summaries-list__badges {
  display: flex;
  flex-shrink: 0;
  gap: var(--space-2);
}

.summaries-list__meta {
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
}

.summaries-list__excerpt {
  margin: 0;
  color: var(--color-text-secondary);
  font-size: var(--font-body-size);
  line-height: var(--font-body-line-height);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.empty-state__title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
}

.empty-state__copy {
  margin: var(--space-2) auto 0;
  max-width: 46ch;
}

@media (min-width: 768px) {
  .summaries-filters__row {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (min-width: 1024px) {
  .summaries-filters__row {
    grid-template-columns: 1.6fr 1fr 1fr 0.8fr;
  }
}
</style>
