<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch, nextTick } from "vue";
import { marked } from "marked";
import { useDebouncedWatch } from "../composables/useDebouncedWatch";
import EpisodesPagination from "../components/episodes/EpisodesPagination.vue";
import UiAlert from "../components/ui/UiAlert.vue";
import UiBadge from "../components/ui/UiBadge.vue";
import UiButton from "../components/ui/UiButton.vue";
import UiCard from "../components/ui/UiCard.vue";
import UiInput from "../components/ui/UiInput.vue";
import UiSelect from "../components/ui/UiSelect.vue";
import { episodesApi, getErrorMessage, podcastsApi } from "../lib/api";
import { summariesApi } from "../lib/api/summaries";
import { formatDate, formatDuration } from "../lib/format";
import type { Podcast, SummaryListItem, SummarySorting } from "../types/api";

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
  nextPage: 0,
  previousPage: 0,
  totalPages: 0,
  totalCount: 0,
});

// Reader state
const readerOpen = ref(false);
const readerItem = ref<SummaryListItem | null>(null);
const readerSummaryHtml = ref("");
const readerSummaryRaw = ref("");
const readerLoadingSummary = ref(false);
const readerTocItems = ref<Array<{ id: string; text: string; level: number }>>([]);

const sortedPodcastOptions = computed(() =>
  [...podcastOptions.value].sort((a, b) => a.Title.localeCompare(b.Title)),
);

const hasActiveFilters = computed(
  () =>
    filter.q.trim().length > 0 ||
    filter.podcastIds.length > 0 ||
    filter.sorting !== "newest" ||
    filter.count !== 20,
);

async function loadPodcastOptions(): Promise<void> {
  try {
    podcastOptions.value = await podcastsApi.list();
  } catch {
    podcastOptions.value = [];
  }
}

async function fetchSummaries(): Promise<void> {
  isLoading.value = true;
  errorMessage.value = "";
  try {
    const response = await summariesApi.list({
      page: filter.page,
      count: filter.count,
      sorting: filter.sorting,
      q: filter.q.trim() || undefined,
      podcastIds: filter.podcastIds.length > 0 ? filter.podcastIds : undefined,
    });

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
    isLoading.value = false;
  }
}

function resetFilters(): void {
  filter.q = "";
  filter.podcastIds = [];
  filter.page = 1;
  filter.count = 20;
  filter.sorting = "newest";
  void fetchSummaries();
}

async function openReader(item: SummaryListItem): Promise<void> {
  readerItem.value = item;
  readerOpen.value = true;
  readerLoadingSummary.value = true;
  readerSummaryHtml.value = "";
  readerSummaryRaw.value = "";
  readerTocItems.value = [];

  try {
    const response = await episodesApi.getSummary(item.id);
    if (response.summary) {
      readerSummaryRaw.value = response.summary;
      renderMarkdown(response.summary);
    }
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Failed to load summary.");
  } finally {
    readerLoadingSummary.value = false;
  }

  await nextTick();
  const el = document.getElementById("summary-reader-top");
  if (el) {
    el.scrollIntoView({ behavior: "smooth", block: "start" });
  }
}

function closeReader(): void {
  readerOpen.value = false;
  readerItem.value = null;
}

function renderMarkdown(text: string): void {
  const toc: Array<{ id: string; text: string; level: number }> = [];
  let headingIndex = 0;

  const renderer = new marked.Renderer();
  renderer.heading = function ({ text, depth }: { text: string; depth: number }) {
    headingIndex++;
    const id = `summary-heading-${headingIndex}`;
    toc.push({ id, text, level: depth });
    return `<h${depth} id="${id}">${text}</h${depth}>`;
  };

  const html = marked.parse(text, { renderer, async: false }) as string;
  readerSummaryHtml.value = html;
  readerTocItems.value = toc;
}

function scrollToHeading(id: string): void {
  const el = document.getElementById(id);
  if (el) {
    el.scrollIntoView({ behavior: "smooth", block: "start" });
  }
}

function podcastFilterLabel(): string {
  if (filter.podcastIds.length === 0) return "All podcasts";
  if (filter.podcastIds.length === 1) {
    const found = podcastOptions.value.find((p) => p.ID === filter.podcastIds[0]);
    return found?.Title ?? "1 podcast";
  }
  return `${filter.podcastIds.length} podcasts`;
}

watch(
  () => [filter.count, filter.sorting, filter.podcastIds.join(",")],
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

    <!-- Reader View -->
    <div v-if="readerOpen && readerItem" id="summary-reader-top" class="summary-reader stack-4">
      <div class="summary-reader__nav">
        <UiButton size="sm" variant="ghost" @click="closeReader">
          &larr; Back to list
        </UiButton>
      </div>

      <div class="summary-reader__layout">
        <!-- TOC Sidebar -->
        <aside v-if="readerTocItems.length > 2" class="summary-reader__toc visually-scrollable">
          <p class="summary-reader__toc-title">Contents</p>
          <nav aria-label="Table of contents">
            <ul class="summary-reader__toc-list">
              <li
                v-for="heading in readerTocItems"
                :key="heading.id"
                :class="`summary-reader__toc-item summary-reader__toc-item--l${heading.level}`"
              >
                <button
                  type="button"
                  class="summary-reader__toc-link"
                  @click="scrollToHeading(heading.id)"
                >
                  {{ heading.text }}
                </button>
              </li>
            </ul>
          </nav>
        </aside>

        <!-- Main content -->
        <article class="summary-reader__content">
          <header class="summary-reader__header stack-2">
            <h2 class="summary-reader__title">{{ readerItem.episodeTitle }}</h2>
            <div class="summary-reader__meta surface-row">
              <span class="meta-text">{{ readerItem.podcastTitle }}</span>
              <UiBadge tone="neutral">{{ readerItem.readTime }} min read</UiBadge>
              <span v-if="readerItem.duration > 0" class="meta-text">
                Episode: {{ formatDuration(readerItem.duration) }}
              </span>
              <span v-if="readerItem.generatedAt" class="meta-text">
                Generated {{ formatDate(readerItem.generatedAt) }}
              </span>
              <span v-if="readerItem.model" class="meta-text">
                Model: {{ readerItem.model }}
              </span>
            </div>
          </header>

          <p v-if="readerLoadingSummary" class="meta-text">Loading summary...</p>
          <div
            v-else-if="readerSummaryHtml"
            class="summary-reader__body summary-prose"
            v-html="readerSummaryHtml"
          ></div>
          <p v-else class="meta-text">No summary content available.</p>
        </article>
      </div>
    </div>

    <!-- Library List View -->
    <template v-else>
      <UiCard padding="md" tone="subtle" class="summaries-filters">
        <div class="summaries-filters__row">
          <UiInput
            :model-value="filter.q"
            type="search"
            label="Search summaries"
            placeholder="Search by title or content"
            @update:model-value="filter.q = $event"
          />

          <div class="summaries-filters__podcast-filter">
            <label class="ui-label" for="summaries-podcast-filter-trigger">Show</label>
            <details class="summaries-filters__podcast-dropdown">
              <summary id="summaries-podcast-filter-trigger" class="summaries-filters__podcast-trigger">
                <span class="summaries-filters__podcast-label">{{ podcastFilterLabel() }}</span>
                <span class="meta-text">
                  {{ filter.podcastIds.length === 0 ? "No filter" : `${filter.podcastIds.length} selected` }}
                </span>
              </summary>
              <div class="summaries-filters__podcast-panel">
                <ul class="summaries-filters__podcast-list visually-scrollable">
                  <li v-for="podcast in sortedPodcastOptions" :key="podcast.ID">
                    <label class="summaries-filters__podcast-option">
                      <input
                        type="checkbox"
                        :checked="filter.podcastIds.includes(podcast.ID)"
                        @change="
                          filter.podcastIds.includes(podcast.ID)
                            ? (filter.podcastIds = filter.podcastIds.filter((id) => id !== podcast.ID))
                            : (filter.podcastIds = [...filter.podcastIds, podcast.ID])
                        "
                      />
                      <span>{{ podcast.Title }}</span>
                    </label>
                  </li>
                </ul>
                <div class="summaries-filters__podcast-actions">
                  <UiButton size="sm" variant="ghost" :disabled="filter.podcastIds.length === 0" @click="filter.podcastIds = []">
                    Clear
                  </UiButton>
                </div>
              </div>
            </details>
          </div>

          <UiSelect
            :model-value="filter.sorting"
            label="Sort"
            @update:model-value="filter.sorting = $event as SummarySorting"
          >
            <option value="newest">Newest first</option>
            <option value="oldest">Oldest first</option>
            <option value="title_asc">Title A-Z</option>
            <option value="title_desc">Title Z-A</option>
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
          <UiButton v-if="hasActiveFilters" size="sm" variant="ghost" @click="resetFilters">
            Reset filters
          </UiButton>
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
        <p class="empty-state__title">No summaries available</p>
        <p class="empty-state__copy">
          Summaries are generated automatically after episodes are transcribed. Check your
          <RouterLink to="/settings">summarization settings</RouterLink> to enable AI summaries.
        </p>
        <UiButton v-if="hasActiveFilters" variant="secondary" size="sm" @click="resetFilters">
          Reset filters
        </UiButton>
      </UiCard>

      <div v-else class="summaries-list">
        <button
          v-for="item in items"
          :key="item.id"
          type="button"
          class="summaries-list__row"
          @click="openReader(item)"
        >
          <div class="summaries-list__content">
            <div class="summaries-list__header">
              <h3 class="summaries-list__title">{{ item.episodeTitle }}</h3>
              <div class="summaries-list__badges">
                <UiBadge tone="info">{{ item.readTime }} min</UiBadge>
                <UiBadge v-if="item.isPlayed" tone="success">Played</UiBadge>
              </div>
            </div>
            <div class="summaries-list__meta">
              <span>{{ item.podcastTitle }}</span>
              <span v-if="item.duration > 0"> &middot; {{ formatDuration(item.duration) }}</span>
              <span v-if="item.generatedAt"> &middot; {{ formatDate(item.generatedAt) }}</span>
              <span v-if="item.model"> &middot; {{ item.model }}</span>
            </div>
            <p v-if="item.excerpt" class="summaries-list__excerpt">{{ item.excerpt }}</p>
          </div>
        </button>
      </div>

      <EpisodesPagination
        :page="filter.page"
        :total-pages="filter.totalPages"
        :total-count="filter.totalCount"
        :has-previous="filter.page > 1 && filter.previousPage > 0"
        :has-next="filter.nextPage > 0"
        @first="filter.page = 1; void fetchSummaries()"
        @previous="filter.page = filter.previousPage; void fetchSummaries()"
        @next="filter.page = filter.nextPage; void fetchSummaries()"
      />
    </template>
  </section>
</template>

<style scoped>
/* ── Filters ─────────────────────────────────────────────── */
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

.summaries-filters__podcast-filter {
  display: grid;
  gap: var(--space-2);
}

.summaries-filters__podcast-dropdown {
  position: relative;
}

.summaries-filters__podcast-dropdown > summary {
  list-style: none;
}

.summaries-filters__podcast-dropdown > summary::-webkit-details-marker {
  display: none;
}

.summaries-filters__podcast-trigger {
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

.summaries-filters__podcast-label {
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.summaries-filters__podcast-panel {
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

.summaries-filters__podcast-dropdown:not([open]) .summaries-filters__podcast-panel {
  display: none;
}

.summaries-filters__podcast-list {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: var(--space-1);
  max-height: 220px;
  overflow: auto;
}

.summaries-filters__podcast-option {
  min-height: 44px;
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2);
  border-radius: var(--radius-2);
  color: var(--color-text-primary);
  cursor: pointer;
}

.summaries-filters__podcast-option:hover {
  background: var(--color-hover);
}

.summaries-filters__podcast-option input[type="checkbox"] {
  width: 20px;
  height: 20px;
  margin: 0;
  flex: 0 0 20px;
  accent-color: var(--color-accent);
}

.summaries-filters__podcast-actions {
  display: flex;
  justify-content: flex-end;
}

/* ── Skeleton ────────────────────────────────────────────── */
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

/* ── List View ───────────────────────────────────────────── */
.summaries-list {
  display: grid;
  gap: var(--space-2);
}

.summaries-list__row {
  width: 100%;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-3);
  background: var(--color-bg-primary);
  color: inherit;
  text-align: left;
  padding: var(--space-4);
  cursor: pointer;
  transition: background-color var(--duration-fast) var(--ease-enter);
}

.summaries-list__row:hover {
  background: var(--color-hover);
}

.summaries-list__content {
  display: grid;
  gap: var(--space-2);
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

/* ── Reader View ─────────────────────────────────────────── */
.summary-reader__nav {
  margin-bottom: var(--space-2);
}

.summary-reader__layout {
  display: grid;
  gap: var(--space-6);
  grid-template-columns: 1fr;
}

.summary-reader__toc {
  display: none;
}

.summary-reader__header {
  padding-bottom: var(--space-4);
  border-bottom: 1px solid var(--color-border);
}

.summary-reader__title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-section-size);
  font-weight: var(--font-section-weight);
  line-height: var(--font-section-line-height);
}

.summary-reader__meta {
  flex-wrap: wrap;
}

/* ── Prose Typography ────────────────────────────────────── */
.summary-prose {
  max-width: 68ch;
  color: var(--color-text-primary);
  font-size: var(--font-body-size);
  line-height: 1.7;
}

.summary-prose :deep(h1),
.summary-prose :deep(h2),
.summary-prose :deep(h3),
.summary-prose :deep(h4) {
  margin: 1.5em 0 0.5em;
  color: var(--color-text-primary);
  font-weight: 600;
  line-height: 1.3;
}

.summary-prose :deep(h1) {
  font-size: 1.5em;
}

.summary-prose :deep(h2) {
  font-size: 1.25em;
}

.summary-prose :deep(h3) {
  font-size: 1.1em;
}

.summary-prose :deep(p) {
  margin: 0.75em 0;
}

.summary-prose :deep(ul),
.summary-prose :deep(ol) {
  margin: 0.75em 0;
  padding-left: 1.5em;
}

.summary-prose :deep(li) {
  margin: 0.25em 0;
}

.summary-prose :deep(blockquote) {
  margin: 1em 0;
  padding: var(--space-3) var(--space-4);
  border-left: 3px solid var(--color-accent);
  background: var(--color-bg-secondary);
  border-radius: 0 var(--radius-2) var(--radius-2) 0;
  color: var(--color-text-secondary);
}

.summary-prose :deep(code) {
  font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
  font-size: 0.9em;
  background: var(--color-bg-secondary);
  padding: 0.15em 0.3em;
  border-radius: var(--radius-1);
}

.summary-prose :deep(pre) {
  margin: 1em 0;
  padding: var(--space-3);
  background: var(--color-bg-secondary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  overflow-x: auto;
}

.summary-prose :deep(pre code) {
  padding: 0;
  background: none;
}

.summary-prose :deep(strong) {
  font-weight: 600;
}

.summary-prose :deep(hr) {
  margin: 1.5em 0;
  border: 0;
  border-top: 1px solid var(--color-border);
}

.summary-prose :deep(a) {
  color: var(--color-accent);
}

.summary-prose :deep(a:hover) {
  color: var(--color-accent-hover);
}

/* ── TOC Sidebar ─────────────────────────────────────────── */
.summary-reader__toc-title {
  margin: 0 0 var(--space-3);
  color: var(--color-text-tertiary);
  font-size: var(--font-caption-size);
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.summary-reader__toc-list {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: var(--space-1);
}

.summary-reader__toc-item--l3 {
  padding-left: var(--space-3);
}

.summary-reader__toc-item--l4 {
  padding-left: var(--space-6);
}

.summary-reader__toc-link {
  width: 100%;
  border: 0;
  background: transparent;
  color: var(--color-text-secondary);
  text-align: left;
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-2);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
  cursor: pointer;
}

.summary-reader__toc-link:hover {
  background: var(--color-hover);
  color: var(--color-text-primary);
}

/* ── Empty State ─────────────────────────────────────────── */
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

/* ── Responsive ──────────────────────────────────────────── */
@media (min-width: 768px) {
  .summaries-filters__row {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (min-width: 1024px) {
  .summaries-filters__row {
    grid-template-columns: 1.6fr 1fr 1fr 0.8fr;
  }

  .summary-reader__layout {
    grid-template-columns: 200px 1fr;
  }

  .summary-reader__toc {
    display: block;
    position: sticky;
    top: calc(var(--topbar-height) + var(--space-4));
    max-height: calc(100vh - var(--topbar-height) - var(--space-8));
    overflow-y: auto;
    padding-right: var(--space-3);
    border-right: 1px solid var(--color-border);
  }
}
</style>
