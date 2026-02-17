<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { useDebouncedWatch } from "../composables/useDebouncedWatch";
import { useEpisodeDrawer } from "../composables/useEpisodeDrawer";
import EpisodesFilters from "../components/episodes/EpisodesFilters.vue";
import EpisodesListItem from "../components/episodes/EpisodesListItem.vue";
import EpisodesPagination from "../components/episodes/EpisodesPagination.vue";
import EpisodesTable from "../components/episodes/EpisodesTable.vue";
import UiAlert from "../components/ui/UiAlert.vue";
import UiBadge from "../components/ui/UiBadge.vue";
import UiButton from "../components/ui/UiButton.vue";
import UiCard from "../components/ui/UiCard.vue";
import UiDrawer from "../components/ui/UiDrawer.vue";
import UiInput from "../components/ui/UiInput.vue";
import { downloadsApi, episodesApi, getErrorMessage, podcastsApi } from "../lib/api";
import { formatDuration } from "../lib/format";
import { isSponsorChapter } from "../lib/sponsor";
import type { EpisodeSorting, EpisodeTriState, Podcast, PodcastItem } from "../types/api";

const route = useRoute();

const isLoading = ref(true);
const errorMessage = ref("");
const infoMessage = ref("");
const items = ref<PodcastItem[]>([]);
const podcastOptions = ref<Podcast[]>([]);

const {
  drawerOpen,
  drawerItem,
  drawerTab,
  drawerChapters,
  drawerTranscriptStatus,
  drawerTranscriptSegments,
  drawerTranscriptText,
  drawerTranscriptAssets,
  drawerLoadingChapters,
  drawerLoadingTranscript,
  drawerLoadError,
  chaptersSearch,
  transcriptSearch,
  filteredChapters,
  filteredTranscriptSegments,
  transcriptLines,
  filteredTranscriptLines,
  transcriptDisplayText,
  openDrawer,
  setDrawerTab,
  closeDrawer,
  drawerTranscriptSummary,
  drawerChaptersSummary,
  drawerTabs,
} = useEpisodeDrawer();

const filter = reactive<{
  q: string;
  podcastIds: string[];
  page: number;
  count: number;
  sorting: EpisodeSorting;
  isDownloaded: EpisodeTriState;
  isPlayed: EpisodeTriState;
  nextPage: number;
  previousPage: number;
  totalPages: number;
  totalCount: number;
}>({
  q: "",
  podcastIds: [],
  page: 1,
  count: 20,
  sorting: "release_desc",
  isDownloaded: "nil",
  isPlayed: "nil",
  nextPage: 0,
  previousPage: 0,
  totalPages: 0,
  totalCount: 0,
});

const hasActiveFilters = computed(
  () =>
    filter.q.trim().length > 0 ||
    filter.podcastIds.length > 0 ||
    filter.sorting !== "release_desc" ||
    filter.count !== 20 ||
    filter.isDownloaded !== "nil" ||
    filter.isPlayed !== "nil",
);

const sortedPodcastOptions = computed(() =>
  [...podcastOptions.value].sort((a, b) => a.Title.localeCompare(b.Title)),
);

function parseQueryIdList(raw: unknown): string[] {
  if (typeof raw === "string") {
    return raw.split(",").map((value) => value.trim()).filter(Boolean);
  }
  if (Array.isArray(raw)) {
    return raw
      .flatMap((value) => (typeof value === "string" ? value.split(",") : []))
      .map((value) => value.trim())
      .filter(Boolean);
  }
  return [];
}

function parseRoutePodcastIds(): string[] {
  const query = route.query as Record<string, unknown>;
  const ids = [
    ...parseQueryIdList(query.podcastIds),
    ...parseQueryIdList(query["podcastIds[]"]),
  ];
  return Array.from(new Set(ids));
}

async function loadPodcastOptions(): Promise<void> {
  try {
    podcastOptions.value = await podcastsApi.list();
  } catch {
    podcastOptions.value = [];
  }
}

async function fetchEpisodes(): Promise<void> {
  isLoading.value = true;
  errorMessage.value = "";
  try {
    const response = await episodesApi.list({
      page: filter.page,
      count: filter.count,
      sorting: filter.sorting,
      q: filter.q.trim() || undefined,
      isDownloaded: filter.isDownloaded !== "nil" ? filter.isDownloaded : undefined,
      isPlayed: filter.isPlayed !== "nil" ? filter.isPlayed : undefined,
      podcastIds: filter.podcastIds.length > 0 ? filter.podcastIds : undefined,
    });

    items.value = response.podcastItems;
    const next = response.filter;
    filter.page = next.page;
    filter.count = next.count;
    filter.nextPage = next.nextPage;
    filter.previousPage = next.previousPage;
    filter.totalPages = next.totalPages;
    filter.totalCount = next.totalCount;
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Failed to load episodes.");
  } finally {
    isLoading.value = false;
  }
}

function resetFilters(): void {
  filter.q = "";
  filter.podcastIds = [];
  filter.page = 1;
  filter.count = 20;
  filter.sorting = "release_desc";
  filter.isDownloaded = "nil";
  filter.isPlayed = "nil";
  void fetchEpisodes();
}

async function togglePlayed(item: PodcastItem): Promise<void> {
  infoMessage.value = "";
  errorMessage.value = "";
  try {
    await episodesApi.setPlayed(item.ID, !item.IsPlayed);
    item.IsPlayed = !item.IsPlayed;
    infoMessage.value = item.IsPlayed ? "Episode marked played." : "Episode marked unplayed.";
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Could not update played status.");
  }
}

async function toggleBookmark(item: PodcastItem): Promise<void> {
  const bookmarked = item.BookmarkDate !== "0001-01-01T00:00:00Z";
  infoMessage.value = "";
  errorMessage.value = "";
  try {
    await episodesApi.setBookmarked(item.ID, !bookmarked);
    item.BookmarkDate = bookmarked ? "0001-01-01T00:00:00Z" : new Date().toISOString();
    infoMessage.value = bookmarked ? "Bookmark removed." : "Bookmark saved.";
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Could not update bookmark.");
  }
}

async function queueDownload(item: PodcastItem): Promise<void> {
  infoMessage.value = "";
  errorMessage.value = "";
  try {
    await episodesApi.queueDownload(item.ID);
    infoMessage.value = "Episode download queued.";
    void fetchEpisodes();
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Could not queue download.");
  }
}

async function cancelDownload(item: PodcastItem): Promise<void> {
  infoMessage.value = "";
  errorMessage.value = "";
  try {
    await downloadsApi.cancelEpisode(item.ID);
    infoMessage.value = "Download cancelled.";
    void fetchEpisodes();
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Could not cancel download.");
  }
}

function openPlayer(item: PodcastItem): void {
  const target = `/app/#/player?itemIds=${encodeURIComponent(item.ID)}`;
  window.open(target, "briefcast_player");
}

function openPlayerAt(item: PodcastItem, startSeconds: number): void {
  const normalizedStart = Number.isFinite(startSeconds) ? Math.max(0, startSeconds) : 0;
  const target = `/app/#/player?itemIds=${encodeURIComponent(item.ID)}&start=${encodeURIComponent(normalizedStart.toString())}`;
  window.open(target, "briefcast_player");
}

watch(
  () => [filter.count, filter.sorting, filter.isDownloaded, filter.isPlayed, filter.podcastIds.join(",")],
  () => {
    filter.page = 1;
    void fetchEpisodes();
  },
);

useDebouncedWatch(
  () => filter.q,
  () => {
    filter.page = 1;
    void fetchEpisodes();
  },
  300,
);

onMounted(() => {
  const routePodcastIds = parseRoutePodcastIds();
  if (routePodcastIds.length > 0) {
    filter.podcastIds = routePodcastIds;
  } else {
    void fetchEpisodes();
  }
  void loadPodcastOptions();
});
</script>

<template>
  <section class="episodes-page stack-4">
    <header class="page-header">
      <h2 class="section-title">Episode Workspace</h2>
      <p class="section-subtitle">
        Search transcripts, filter your library, and manage episode actions in one place.
      </p>
    </header>

    <UiAlert v-if="infoMessage" tone="success">
      {{ infoMessage }}
    </UiAlert>
    <UiAlert v-if="errorMessage" tone="danger">
      {{ errorMessage }}
    </UiAlert>

    <EpisodesFilters
      :query="filter.q"
      :selected-podcast-ids="filter.podcastIds"
      :podcast-options="sortedPodcastOptions.map((podcast) => ({ id: podcast.ID, title: podcast.Title }))"
      :sorting="filter.sorting"
      :count="filter.count"
      :is-downloaded="filter.isDownloaded"
      :is-played="filter.isPlayed"
      @update:query="filter.q = $event"
      @update:selected-podcast-ids="filter.podcastIds = $event"
      @update:sorting="filter.sorting = $event"
      @update:count="filter.count = $event"
      @update:is-downloaded="filter.isDownloaded = $event"
      @update:is-played="filter.isPlayed = $event"
    />

    <UiCard v-if="isLoading" padding="md" class="episodes-skeleton">
      <div v-for="index in 4" :key="index" class="episodes-skeleton__row">
        <span class="skeleton episodes-skeleton__line episodes-skeleton__line--title"></span>
        <span class="skeleton episodes-skeleton__line"></span>
      </div>
    </UiCard>

    <UiCard v-else-if="items.length === 0" padding="lg" class="empty-state">
      <p class="empty-state__title">No episodes match the current filter</p>
      <p class="empty-state__copy">
        Clear filters to see your full feed list or update downloads to pull fresh episodes.
      </p>
      <UiButton v-if="hasActiveFilters" variant="secondary" size="sm" @click="resetFilters">
        Reset filters
      </UiButton>
    </UiCard>

    <div v-else class="stack-3">
      <div class="episodes-list-mobile stack-3">
        <EpisodesListItem
          v-for="item in items"
          :key="item.ID"
          :item="item"
          @play="openPlayer"
          @toggle-played="togglePlayed"
          @toggle-bookmark="toggleBookmark"
          @queue-download="queueDownload"
          @cancel-download="cancelDownload"
          @open-details="openDrawer"
        />
      </div>

      <div class="episodes-list-desktop">
        <EpisodesTable
          :items="items"
          @play="openPlayer"
          @toggle-played="togglePlayed"
          @toggle-bookmark="toggleBookmark"
          @queue-download="queueDownload"
          @cancel-download="cancelDownload"
          @open-details="openDrawer"
        />
      </div>
    </div>

    <EpisodesPagination
      :page="filter.page"
      :total-pages="filter.totalPages"
      :total-count="filter.totalCount"
      :has-previous="filter.page > 1 && filter.previousPage > 0"
      :has-next="filter.nextPage > 0"
      @first="filter.page = 1; void fetchEpisodes()"
      @previous="filter.page = filter.previousPage; void fetchEpisodes()"
      @next="filter.page = filter.nextPage; void fetchEpisodes()"
    />

    <UiDrawer
      :open="drawerOpen"
      :title="drawerItem?.Title || 'Episode details'"
      :description="drawerItem?.Podcast?.Title || ''"
      @close="closeDrawer"
    >
      <div class="stack-4">
        <div class="drawer-tabs">
          <UiButton
            v-for="tab in drawerTabs"
            :key="tab.id"
            size="sm"
            :variant="drawerTab === tab.id ? 'primary' : 'ghost'"
            @click="setDrawerTab(tab.id)"
          >
            {{ tab.label }}
          </UiButton>
        </div>

        <UiAlert v-if="drawerLoadError" tone="danger">
          {{ drawerLoadError }}
        </UiAlert>

        <div v-if="drawerTab === 'overview'" class="stack-3">
          <div>
            <p class="meta-text">Summary</p>
            <p class="drawer-body-copy">{{ drawerItem?.Summary || "No summary available." }}</p>
          </div>
          <div class="drawer-summary-grid">
            <UiCard padding="sm" tone="subtle">
              <p class="meta-text">Transcript</p>
              <p class="drawer-body-copy">{{ drawerTranscriptSummary() }}</p>
            </UiCard>
            <UiCard padding="sm" tone="subtle">
              <p class="meta-text">Chapters</p>
              <p class="drawer-body-copy">{{ drawerChaptersSummary() }}</p>
            </UiCard>
          </div>
        </div>

        <div v-else-if="drawerTab === 'chapters'" class="stack-3">
          <div class="surface-row">
            <UiInput v-model="chaptersSearch" placeholder="Search chapters" />
            <span v-if="drawerChapters.length > 0" class="meta-text">
              Showing {{ filteredChapters.length }} of {{ drawerChapters.length }}
            </span>
          </div>
          <p v-if="drawerLoadingChapters" class="meta-text">Loading chapters...</p>
          <p v-else-if="drawerChapters.length === 0" class="meta-text">No chapters available for this episode.</p>
          <p v-else-if="filteredChapters.length === 0" class="meta-text">No chapters match “{{ chaptersSearch }}”.</p>
          <ul v-else class="drawer-list">
            <li
              v-for="chapter in filteredChapters"
              :key="`${chapter.title}-${chapter.startSeconds}`"
              class="drawer-list__row"
            >
              <div>
                <div class="surface-row">
                  <p class="drawer-list__title">{{ chapter.title }}</p>
                  <UiBadge v-if="isSponsorChapter(chapter.title)" tone="info">Sponsor</UiBadge>
                </div>
                <p class="meta-text">Starts at {{ formatDuration(Math.floor(chapter.startSeconds)) }}</p>
              </div>
              <UiButton size="sm" variant="outline" @click="drawerItem && openPlayerAt(drawerItem, chapter.startSeconds)">
                Play from here
              </UiButton>
            </li>
          </ul>
        </div>

        <div v-else-if="drawerTab === 'transcript'" class="stack-3">
          <div class="surface-row">
            <UiInput v-model="transcriptSearch" placeholder="Search transcript" />
            <span v-if="drawerTranscriptSegments.length > 0" class="meta-text">
              Showing {{ filteredTranscriptSegments.length }} of {{ drawerTranscriptSegments.length }}
            </span>
            <span v-else-if="drawerTranscriptText" class="meta-text">
              Showing {{ filteredTranscriptLines.length }} of {{ transcriptLines.length }} lines
            </span>
          </div>
          <p v-if="drawerLoadingTranscript" class="meta-text">Loading transcript...</p>
          <p v-else-if="drawerTranscriptStatus !== 'available'" class="meta-text">{{ drawerTranscriptSummary() }}</p>
          <div v-else-if="drawerTranscriptSegments.length > 0" class="drawer-list">
            <p v-if="filteredTranscriptSegments.length === 0" class="meta-text">
              No transcript segments match “{{ transcriptSearch }}”.
            </p>
            <button
              v-for="segment in filteredTranscriptSegments"
              :key="`${segment.start}-${segment.text}`"
              type="button"
              class="drawer-transcript-row"
              @click="drawerItem && openPlayerAt(drawerItem, segment.start)"
            >
              <div>
                <p class="meta-text">
                  {{ formatDuration(Math.floor(segment.start)) }}
                  <span v-if="segment.speaker"> • {{ segment.speaker }}</span>
                </p>
                <p class="drawer-body-copy">{{ segment.text }}</p>
              </div>
              <span class="meta-text">Play</span>
            </button>
          </div>
          <div v-else-if="drawerTranscriptText" class="drawer-transcript-raw">
            <pre>{{ transcriptDisplayText }}</pre>
          </div>
          <div v-else class="stack-2">
            <p class="meta-text">Transcript is available but could not be rendered.</p>
            <ul v-if="drawerTranscriptAssets.length > 0" class="meta-text">
              <li v-for="asset in drawerTranscriptAssets" :key="asset.url || asset.type">
                {{ asset.language || "unknown" }} {{ asset.type || "text" }}
                <span v-if="asset.url">(downloadable)</span>
              </li>
            </ul>
          </div>
        </div>
      </div>
    </UiDrawer>
  </section>
</template>

<style scoped>
.episodes-skeleton {
  display: grid;
  gap: var(--space-3);
}

.episodes-skeleton__row {
  display: grid;
  gap: var(--space-2);
}

.episodes-skeleton__line {
  height: 12px;
}

.episodes-skeleton__line--title {
  width: 64%;
  height: 18px;
}

.episodes-list-desktop {
  display: none;
}

.drawer-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.drawer-body-copy {
  margin: var(--space-2) 0 0;
  color: var(--color-text-secondary);
}

.drawer-summary-grid {
  display: grid;
  gap: var(--space-3);
  grid-template-columns: 1fr;
}

.drawer-list {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: var(--space-2);
}

.drawer-list__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
  padding: var(--space-3);
}

.drawer-list__title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
}

.drawer-transcript-row {
  width: 100%;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
  color: inherit;
  text-align: left;
  display: flex;
  justify-content: space-between;
  gap: var(--space-3);
  padding: var(--space-3);
  cursor: pointer;
}

.drawer-transcript-row:hover {
  background: var(--color-hover);
}

.drawer-transcript-raw {
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
  padding: var(--space-3);
}

.drawer-transcript-raw pre {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--color-text-secondary);
  font-family: var(--font-family);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
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
  .drawer-summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (min-width: 1200px) {
  .episodes-list-mobile {
    display: none;
  }

  .episodes-list-desktop {
    display: block;
  }
}
</style>
