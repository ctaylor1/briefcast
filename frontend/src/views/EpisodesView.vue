<script setup lang="ts">
import { onMounted, onUnmounted, reactive, ref, watch } from "vue";
import EpisodesFilters from "../components/episodes/EpisodesFilters.vue";
import EpisodesListItem from "../components/episodes/EpisodesListItem.vue";
import EpisodesPagination from "../components/episodes/EpisodesPagination.vue";
import EpisodesTable from "../components/episodes/EpisodesTable.vue";
import UiAlert from "../components/ui/UiAlert.vue";
import UiButton from "../components/ui/UiButton.vue";
import UiCard from "../components/ui/UiCard.vue";
import { downloadsApi, episodesApi, getErrorMessage } from "../lib/api";
import type {
  EpisodeSorting,
  EpisodeTriState,
  PodcastItem,
} from "../types/api";

const isLoading = ref(true);
const errorMessage = ref("");
const infoMessage = ref("");
const items = ref<PodcastItem[]>([]);
const queueItems = ref<PodcastItem[]>([]);
const queueLoading = ref(false);
const queueError = ref("");
const queueCounts = reactive({
  queued: 0,
  downloading: 0,
  downloaded: 0,
});
const downloadsPaused = ref(false);

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
let queueInterval: number | undefined;

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
    void fetchDownloadQueue();
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
    void fetchDownloadQueue();
    void fetchEpisodes();
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Could not cancel download.");
  }
}

async function fetchDownloadQueue(): Promise<void> {
  queueLoading.value = true;
  queueError.value = "";
  try {
    const response = await downloadsApi.getQueue(15);
    queueItems.value = response.items;
    queueCounts.queued = response.counts.queued ?? 0;
    queueCounts.downloading = response.counts.downloading ?? 0;
    queueCounts.downloaded = response.counts.downloaded ?? 0;
    downloadsPaused.value = response.paused;
  } catch (error) {
    queueError.value = getErrorMessage(error, "Failed to load download queue.");
  } finally {
    queueLoading.value = false;
  }
}

async function toggleDownloadsPause(): Promise<void> {
  infoMessage.value = "";
  errorMessage.value = "";
  try {
    if (downloadsPaused.value) {
      await downloadsApi.resume();
      downloadsPaused.value = false;
      infoMessage.value = "Downloads resumed.";
    } else {
      await downloadsApi.pause();
      downloadsPaused.value = true;
      infoMessage.value = "Downloads paused.";
    }
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Could not update download pause.");
  }
}

async function cancelAllDownloads(): Promise<void> {
  infoMessage.value = "";
  errorMessage.value = "";
  try {
    await downloadsApi.cancelAll();
    infoMessage.value = "All queued downloads cancelled.";
    void fetchDownloadQueue();
    void fetchEpisodes();
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Could not cancel downloads.");
  }
}

function openPlayer(item: PodcastItem): void {
  const target = `/app/#/player?itemIds=${encodeURIComponent(item.ID)}`;
  window.open(target, "briefcast_player");
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

onMounted(() => {
  void fetchEpisodes();
  void fetchDownloadQueue();
  queueInterval = window.setInterval(() => {
    void fetchDownloadQueue();
  }, 5000);
});
onUnmounted(() => {
  if (searchDebounce) {
    window.clearTimeout(searchDebounce);
  }
  if (queueInterval) {
    window.clearInterval(queueInterval);
  }
});
</script>

<template>
  <section class="stack-4">
    <div class="stack-2">
      <h1 class="fluid-title-xl font-semibold tracking-tight text-slate-900">Episodes</h1>
      <p class="fluid-subtle text-slate-600">Card list on mobile, table density on desktop.</p>
    </div>

    <UiCard padding="lg" class="stack-3">
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p class="text-xs font-semibold uppercase tracking-[0.24em] text-slate-400">Download Queue</p>
          <h2 class="text-lg font-semibold text-slate-900">Downloads</h2>
          <p class="text-sm text-slate-600">
            Queued: {{ queueCounts.queued }} • Downloading: {{ queueCounts.downloading }} • Downloaded:
            {{ queueCounts.downloaded }}
          </p>
        </div>
        <div class="flex flex-wrap gap-2">
          <UiButton size="sm" variant="outline" @click="toggleDownloadsPause">
            {{ downloadsPaused ? "Resume downloads" : "Pause downloads" }}
          </UiButton>
          <UiButton
            size="sm"
            variant="danger"
            :disabled="queueCounts.queued === 0 && queueCounts.downloading === 0"
            @click="cancelAllDownloads"
          >
            Stop all
          </UiButton>
          <UiButton size="sm" variant="ghost" @click="fetchDownloadQueue">
            Refresh
          </UiButton>
        </div>
      </div>

      <UiAlert v-if="queueError" tone="danger">
        {{ queueError }}
      </UiAlert>

      <div v-if="queueLoading" class="text-sm text-slate-600">
        Loading queue...
      </div>
      <div v-else-if="queueItems.length === 0" class="text-sm text-slate-600">
        No queued downloads.
      </div>
      <ul v-else class="divide-y divide-slate-100 text-sm">
        <li v-for="item in queueItems" :key="item.ID" class="flex flex-wrap items-center justify-between gap-2 py-2">
          <div>
            <p class="font-semibold text-slate-900">{{ item.Title }}</p>
            <p class="text-xs text-slate-500">{{ item.Podcast?.Title || "Unknown Podcast" }}</p>
          </div>
          <div class="flex items-center gap-2">
            <span
              class="rounded-full px-2 py-1 text-xs font-medium"
              :class="item.DownloadStatus === 1 ? 'bg-blue-100 text-blue-800' : 'bg-amber-100 text-amber-800'"
            >
              {{ item.DownloadStatus === 1 ? "Downloading" : "Queued" }}
            </span>
            <UiButton size="sm" variant="danger" @click="cancelDownload(item)">
              Stop
            </UiButton>
          </div>
        </li>
      </ul>
    </UiCard>

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
          @cancel-download="cancelDownload"
        />
      </div>

      <div class="hidden xl:block">
        <EpisodesTable
          :items="items"
          @play="openPlayer"
          @toggle-played="togglePlayed"
          @toggle-bookmark="toggleBookmark"
          @queue-download="queueDownload"
          @cancel-download="cancelDownload"
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
