<script setup lang="ts">
import { onMounted, onUnmounted, reactive, ref, watch } from "vue";
import EpisodesFilters from "../components/episodes/EpisodesFilters.vue";
import EpisodesListItem from "../components/episodes/EpisodesListItem.vue";
import EpisodesPagination from "../components/episodes/EpisodesPagination.vue";
import EpisodesTable from "../components/episodes/EpisodesTable.vue";
import UiAlert from "../components/ui/UiAlert.vue";
import UiCard from "../components/ui/UiCard.vue";
import { episodesApi, getErrorMessage } from "../lib/api";
import type {
  EpisodeSorting,
  EpisodeTriState,
  PodcastItem,
} from "../types/api";

const isLoading = ref(true);
const errorMessage = ref("");
const infoMessage = ref("");
const items = ref<PodcastItem[]>([]);

const filter = reactive<{
  q: string;
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

let searchDebounce: number | undefined;

function isBookmarked(item: PodcastItem): boolean {
  return item.BookmarkDate !== "0001-01-01T00:00:00Z";
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
  const bookmarked = isBookmarked(item);
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
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Could not queue download.");
  }
}

function openPlayer(item: PodcastItem): void {
  window.open(`/player?itemIds=${item.ID}`, "briefcast_player");
}

watch(
  () => [filter.count, filter.sorting, filter.isDownloaded, filter.isPlayed],
  () => {
    filter.page = 1;
    void fetchEpisodes();
  },
);

watch(
  () => filter.q,
  () => {
    if (searchDebounce) {
      window.clearTimeout(searchDebounce);
    }
    searchDebounce = window.setTimeout(() => {
      filter.page = 1;
      void fetchEpisodes();
    }, 400);
  },
);

onMounted(fetchEpisodes);
onUnmounted(() => {
  if (searchDebounce) {
    window.clearTimeout(searchDebounce);
  }
});
</script>

<template>
  <section class="stack-4">
    <div class="stack-2">
      <h1 class="fluid-title-xl font-semibold tracking-tight text-slate-900">Episodes</h1>
      <p class="fluid-subtle text-slate-600">Card list on mobile, table density on desktop.</p>
    </div>

    <EpisodesFilters
      :query="filter.q"
      :sorting="filter.sorting"
      :count="filter.count"
      :is-downloaded="filter.isDownloaded"
      :is-played="filter.isPlayed"
      @update:query="filter.q = $event"
      @update:sorting="filter.sorting = $event"
      @update:count="filter.count = $event"
      @update:is-downloaded="filter.isDownloaded = $event"
      @update:is-played="filter.isPlayed = $event"
    />

    <UiAlert v-if="infoMessage" tone="success">
      {{ infoMessage }}
    </UiAlert>
    <UiAlert v-if="errorMessage" tone="danger">
      {{ errorMessage }}
    </UiAlert>

    <UiCard v-if="isLoading" padding="lg" class="text-sm text-slate-600">
      Loading episodes...
    </UiCard>

    <UiCard v-else-if="items.length === 0" padding="lg" class="text-sm text-slate-600">
      No episodes found for this filter.
    </UiCard>

    <div v-else class="stack-3">
      <div class="stack-3 xl:hidden">
        <EpisodesListItem
          v-for="item in items"
          :key="item.ID"
          :item="item"
          @play="openPlayer"
          @toggle-played="togglePlayed"
          @toggle-bookmark="toggleBookmark"
          @queue-download="queueDownload"
        />
      </div>

      <div class="hidden xl:block">
        <EpisodesTable
          :items="items"
          @play="openPlayer"
          @toggle-played="togglePlayed"
          @toggle-bookmark="toggleBookmark"
          @queue-download="queueDownload"
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
