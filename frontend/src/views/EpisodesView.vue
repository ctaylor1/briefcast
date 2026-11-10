<script setup lang="ts">
import { episodePath } from "../lib/routes";
import { usePlayer } from "../composables/usePlayer";
import { toast } from "../lib/toast";
import { createLatest } from "../lib/latest";
import { computed, onMounted, reactive, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useDebouncedWatch } from "../composables/useDebouncedWatch";
import EpisodesFilters from "../components/episodes/EpisodesFilters.vue";
import EpisodesListItem from "../components/episodes/EpisodesListItem.vue";
import EpisodesPagination from "../components/episodes/EpisodesPagination.vue";
import EpisodesTable from "../components/episodes/EpisodesTable.vue";
import UiAlert from "../components/ui/UiAlert.vue";
import UiButton from "../components/ui/UiButton.vue";
import UiCard from "../components/ui/UiCard.vue";
import { downloadsApi, episodesApi, getErrorMessage, podcastsApi } from "../lib/api";
import { isEpisodeFavorited, UNBOOKMARKED_DATE } from "../lib/bookmarks";
import type { EpisodeSorting, EpisodeTriState, Podcast, PodcastItem } from "../types/api";

const player = usePlayer();
const route = useRoute();
const router = useRouter();

const isLoading = ref(true);
const errorMessage = ref("");
const items = ref<PodcastItem[]>([]);
const podcastOptions = ref<Podcast[]>([]);


const filter = reactive<{
  q: string;
  podcastIds: string[];
  page: number;
  count: number;
  sorting: EpisodeSorting;
  isDownloaded: EpisodeTriState;
  isPlayed: EpisodeTriState;
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
  sorting: "release_desc",
  isDownloaded: "nil",
  isPlayed: "nil",
  favoritesOnly: false,
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
    filter.isPlayed !== "nil" ||
    filter.favoritesOnly,
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

function parseRouteQueryText(): string {
  const query = route.query as Record<string, unknown>;
  const raw = query.q;
  if (typeof raw === "string") {
    return raw.trim();
  }
  if (Array.isArray(raw)) {
    const first = raw.find((value): value is string => typeof value === "string");
    return first?.trim() || "";
  }
  return "";
}

function sameStringList(a: string[], b: string[]): boolean {
  if (a.length !== b.length) {
    return false;
  }
  const left = [...a].sort();
  const right = [...b].sort();
  return left.every((value, index) => value === right[index]);
}

async function loadPodcastOptions(): Promise<void> {
  try {
    podcastOptions.value = await podcastsApi.list();
  } catch {
    podcastOptions.value = [];
  }
}

const episodesRequest = createLatest();

async function fetchEpisodes(): Promise<void> {
  isLoading.value = true;
  errorMessage.value = "";
  let stale = false;
  try {
    const result = await episodesRequest.run(() => episodesApi.list({
      page: filter.page,
      count: filter.count,
      sorting: filter.sorting,
      q: filter.q.trim() || undefined,
      isDownloaded: filter.isDownloaded !== "nil" ? filter.isDownloaded : undefined,
      isPlayed: filter.isPlayed !== "nil" ? filter.isPlayed : undefined,
      isBookmarked: filter.favoritesOnly ? "true" : undefined,
      podcastIds: filter.podcastIds.length > 0 ? filter.podcastIds : undefined,
    }));
    stale = result.stale;
    if (stale) {
      return;
    }
    const response = result.value;

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
  filter.sorting = "release_desc";
  filter.isDownloaded = "nil";
  filter.isPlayed = "nil";
  filter.favoritesOnly = false;
  void fetchEpisodes();
}

async function togglePlayed(item: PodcastItem): Promise<void> {
  const previousValue = item.IsPlayed;
  const nextValue = !previousValue;
  item.IsPlayed = nextValue;

  try {
    await episodesApi.setPlayed(item.ID, nextValue);
    toast.success(nextValue ? "Episode marked played." : "Episode marked unplayed.");
  } catch (error) {
    item.IsPlayed = previousValue;
    toast.error(getErrorMessage(error, "Could not update played status."));
  }
}

async function toggleBookmark(item: PodcastItem): Promise<void> {
  const previousValue = item.BookmarkDate;
  const previousSummaryFavorite = item.IsSummaryFavorited;
  const currentlyBookmarked = isEpisodeFavorited(item);
  item.BookmarkDate = currentlyBookmarked ? UNBOOKMARKED_DATE : new Date().toISOString();
  item.IsSummaryFavorited = !currentlyBookmarked;

  try {
    await episodesApi.setBookmarked(item.ID, !currentlyBookmarked);
    toast.success(currentlyBookmarked ? "Favorite removed." : "Favorite saved.");
  } catch (error) {
    item.BookmarkDate = previousValue;
    item.IsSummaryFavorited = previousSummaryFavorite;
    toast.error(getErrorMessage(error, "Could not update favorite."));
  }
}

async function queueDownload(item: PodcastItem): Promise<void> {
  try {
    await episodesApi.queueDownload(item.ID);
    toast.success("Episode download queued.");
    void fetchEpisodes();
  } catch (error) {
    toast.error(getErrorMessage(error, "Could not queue download."));
  }
}

async function cancelDownload(item: PodcastItem): Promise<void> {
  try {
    await downloadsApi.cancelEpisode(item.ID);
    toast.success("Download cancelled.");
    void fetchEpisodes();
  } catch (error) {
    toast.error(getErrorMessage(error, "Could not cancel download."));
  }
}

// Opens the episode page; the row's Chapters/Transcript buttons open that tab.
function openDetails(item: PodcastItem, tab?: "overview" | "chapters" | "transcript"): void {
  void router.push({ path: episodePath(item.ID), query: tab && tab !== "overview" ? { tab } : {} });
}

// Plays in the persistent player bar; the user stays on this page.
function openPlayer(item: PodcastItem): void {
  void player.playEpisode(item);
}


watch(
  () => [filter.count, filter.sorting, filter.isDownloaded, filter.isPlayed, filter.favoritesOnly, filter.podcastIds.join(",")],
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

watch(
  () => route.query,
  () => {
    const nextQuery = parseRouteQueryText();
    const nextPodcastIds = parseRoutePodcastIds();
    const queryChanged = nextQuery !== filter.q;
    const podcastsChanged = !sameStringList(nextPodcastIds, filter.podcastIds);
    if (!queryChanged && !podcastsChanged) {
      return;
    }
    filter.page = 1;
    filter.q = nextQuery;
    filter.podcastIds = nextPodcastIds;
  },
);

onMounted(() => {
  filter.q = parseRouteQueryText();
  const routePodcastIds = parseRoutePodcastIds();
  if (routePodcastIds.length > 0) {
    filter.podcastIds = routePodcastIds;
  }
  if (routePodcastIds.length === 0 && filter.q.trim() === "") {
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
      :favorites-only="filter.favoritesOnly"
      @update:query="filter.q = $event"
      @update:selected-podcast-ids="filter.podcastIds = $event"
      @update:sorting="filter.sorting = $event"
      @update:count="filter.count = $event"
      @update:is-downloaded="filter.isDownloaded = $event"
      @update:is-played="filter.isPlayed = $event"
      @update:favorites-only="filter.favoritesOnly = $event"
    />

    <UiCard v-if="isLoading" padding="md" class="episodes-skeleton">
      <div v-for="index in 4" :key="index" class="episodes-skeleton__row">
        <span class="skeleton episodes-skeleton__line episodes-skeleton__line--title"></span>
        <span class="skeleton episodes-skeleton__line"></span>
      </div>
    </UiCard>

    <UiCard v-else-if="items.length === 0" padding="lg" class="empty-state">
      <p class="empty-state__title">
        {{ filter.favoritesOnly ? "No favorite episodes" : "No episodes match the current filter" }}
      </p>
      <p class="empty-state__copy">
        <template v-if="filter.favoritesOnly">
          Star episodes you want to revisit and they will appear here.
        </template>
        <template v-else>
          Clear filters to see your full feed list or update downloads to pull fresh episodes.
        </template>
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
          @open-details="openDetails"
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
          @open-details="openDetails"
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


@media (min-width: 1200px) {
  .episodes-list-mobile {
    display: none;
  }

  .episodes-list-desktop {
    display: block;
  }
}
</style>
