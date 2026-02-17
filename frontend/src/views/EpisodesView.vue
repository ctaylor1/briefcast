<script setup lang="ts">
import { onMounted, onUnmounted, reactive, ref, watch } from "vue";
import { useDebouncedWatch } from "../composables/useDebouncedWatch";
import { useDownloadQueue } from "../composables/useDownloadQueue";
import { useEpisodeDrawer } from "../composables/useEpisodeDrawer";
import { useGlobalSearch } from "../composables/useGlobalSearch";
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
import { episodesApi, getErrorMessage } from "../lib/api";
import { formatDuration } from "../lib/format";
import { isSponsorChapter } from "../lib/sponsor";
import type { EpisodeSorting, EpisodeTriState, PodcastItem, LocalSearchResult } from "../types/api";

const isLoading = ref(true);
const errorMessage = ref("");
const infoMessage = ref("");
const items = ref<PodcastItem[]>([]);

const {
  queueItems,
  queueLoading,
  queueError,
  queueCounts,
  downloadsPaused,
  fetchQueue,
  pauseDownloads,
  resumeDownloads,
  cancelAllDownloads: cancelAllQueuedDownloads,
  cancelEpisodeDownload,
  queueProgressPercent,
  queueProgressLabel,
  queueHasKnownTotal,
} = useDownloadQueue();

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

const fetchDownloadQueue = fetchQueue;
const {
  query: globalSearchQuery,
  results: globalSearchResults,
  loading: globalSearchLoading,
  error: globalSearchError,
  run: runGlobalSearch,
  typeLabel: searchTypeLabel,
  summary: searchSummary,
} = useGlobalSearch();


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
    await cancelEpisodeDownload(item.ID);
    infoMessage.value = "Download cancelled.";
    void fetchDownloadQueue();
    void fetchEpisodes();
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Could not cancel download.");
  }
}

async function toggleDownloadsPause(): Promise<void> {
  infoMessage.value = "";
  errorMessage.value = "";
  try {
    if (downloadsPaused.value) {
      await resumeDownloads();
      infoMessage.value = "Downloads resumed.";
    } else {
      await pauseDownloads();
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
    await cancelAllQueuedDownloads();
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

function openPlayerAt(item: PodcastItem, startSeconds: number): void {
  const target = `/app/#/player?itemIds=${encodeURIComponent(item.ID)}&start=${Math.max(0, Math.floor(startSeconds))}`;
  window.open(target, "briefcast_player");
}

async function openSearchResult(result: LocalSearchResult): Promise<void> {
  globalSearchError.value = "";
  if (!result.episodeId) {
    return;
  }
  let item = items.value.find((entry) => entry.ID === result.episodeId) || null;
  if (!item) {
    try {
      item = await episodesApi.getById(result.episodeId);
    } catch (error) {
      globalSearchError.value = getErrorMessage(error, "Unable to load episode details.");
      return;
    }
  }

  const term = globalSearchQuery.value.trim();
  if (result.type === "chapter") {
    openDrawer(item, "chapters", term);
    return;
  }
  if (result.type === "transcript") {
    openDrawer(item, "transcript", term);
    return;
  }
  openDrawer(item, "overview");
}

watch(
  () => [filter.count, filter.sorting, filter.isDownloaded, filter.isPlayed],
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
  400,
);

useDebouncedWatch(
  () => globalSearchQuery.value,
  () => {
    void runGlobalSearch();
  },
  300,
);

onMounted(() => {
  void fetchEpisodes();
  void fetchDownloadQueue();
  queueInterval = window.setInterval(() => {
    void fetchDownloadQueue();
  }, 5000);
});
onUnmounted(() => {
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
            Queued: {{ queueCounts.queued }} • Downloading: {{ queueCounts.downloading }} • Paused: {{
              queueCounts.paused
            }} • Downloaded: {{ queueCounts.downloaded }}
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
        <li v-for="item in queueItems" :key="item.ID" class="flex flex-wrap items-center justify-between gap-4 py-2">
          <div class="min-w-[220px] flex-1">
            <p class="font-semibold text-slate-900">{{ item.Title }}</p>
            <p class="text-xs text-slate-500">{{ item.Podcast?.Title || "Unknown Podcast" }}</p>
            <div class="mt-2">
              <div class="h-1.5 w-full overflow-hidden rounded-full bg-slate-100">
                <div
                  class="h-full rounded-full bg-blue-500"
                  :class="!queueHasKnownTotal(item) && 'animate-pulse w-1/2'"
                  :style="queueHasKnownTotal(item) ? { width: `${queueProgressPercent(item)}%` } : undefined"
                />
              </div>
              <p class="mt-1 text-[11px] text-slate-500">{{ queueProgressLabel(item) }}</p>
            </div>
          </div>
          <div class="flex items-center gap-2">
            <span
              class="rounded-full px-2 py-1 text-xs font-medium"
              :class="item.DownloadStatus === 1
                ? 'bg-blue-100 text-blue-800'
                : item.DownloadStatus === 4
                  ? 'bg-slate-200 text-slate-700'
                  : 'bg-amber-100 text-amber-800'"
            >
              {{
                item.DownloadStatus === 1
                  ? "Downloading"
                  : item.DownloadStatus === 4
                    ? "Paused"
                    : "Queued"
              }}
            </span>
            <UiButton size="sm" variant="danger" @click="cancelDownload(item)">
              Stop
            </UiButton>
          </div>
        </li>
      </ul>
    </UiCard>

    <UiCard padding="lg" class="stack-3">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p class="text-xs font-semibold uppercase tracking-[0.24em] text-slate-400">Global Search</p>
          <h2 class="text-lg font-semibold text-slate-900">Find episodes, chapters, and transcripts</h2>
          <p class="text-sm text-slate-600">
            Searches episode descriptions, chapter titles, and transcript text.
          </p>
        </div>
        <UiInput v-model="globalSearchQuery" type="search" placeholder="Search across podcasts and episodes" />
      </div>

      <UiAlert v-if="globalSearchError" tone="danger">
        {{ globalSearchError }}
      </UiAlert>

      <div v-if="globalSearchLoading" class="text-sm text-slate-600">
        Searching...
      </div>
      <div v-else-if="globalSearchQuery && globalSearchResults.length === 0" class="text-sm text-slate-600">
        No results for "{{ globalSearchQuery }}".
      </div>
      <ul v-else-if="globalSearchResults.length > 0" class="divide-y divide-slate-100 text-sm">
        <li v-for="result in globalSearchResults" :key="`${result.type}-${result.episodeId || result.podcastId}-${result.startSeconds || 0}`" class="py-3">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div class="min-w-[220px] flex-1 space-y-1">
              <UiBadge>{{ searchTypeLabel(result) }}</UiBadge>
              <p class="text-sm font-semibold text-slate-900">
                {{ result.episodeTitle || result.podcastTitle || "Untitled" }}
              </p>
              <p v-if="result.podcastTitle && result.episodeTitle" class="text-xs text-slate-500">
                {{ result.podcastTitle }}
              </p>
              <p v-if="searchSummary(result)" class="text-xs text-slate-600">
                {{ searchSummary(result) }}
              </p>
            </div>
            <UiButton
              v-if="result.episodeId"
              size="sm"
              variant="outline"
              @click="openSearchResult(result)"
            >
              Open episode
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
          @open-details="openDrawer"
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
        <div class="flex flex-wrap gap-2">
          <button
            v-for="tab in drawerTabs"
            :key="tab.id"
            type="button"
            class="rounded-full px-3 py-1 text-xs font-semibold"
            :class="drawerTab === tab.id ? 'bg-slate-900 text-white' : 'bg-slate-100 text-slate-700'"
            @click="setDrawerTab(tab.id)"
          >
            {{ tab.label }}
          </button>
        </div>

        <UiAlert v-if="drawerLoadError" tone="danger">
          {{ drawerLoadError }}
        </UiAlert>

        <div v-if="drawerTab === 'overview'" class="stack-3 text-sm text-slate-700">
          <div>
            <p class="text-xs font-semibold uppercase tracking-[0.2em] text-slate-400">Summary</p>
            <p class="mt-2 text-sm text-slate-700">
              {{ drawerItem?.Summary || "No summary available." }}
            </p>
          </div>
          <div class="grid gap-3 sm:grid-cols-2">
            <div class="rounded-lg border border-slate-200 p-3">
              <p class="text-xs font-semibold uppercase tracking-[0.2em] text-slate-400">Transcript</p>
              <p class="mt-2 text-sm text-slate-700">{{ drawerTranscriptSummary() }}</p>
            </div>
            <div class="rounded-lg border border-slate-200 p-3">
              <p class="text-xs font-semibold uppercase tracking-[0.2em] text-slate-400">Chapters</p>
              <p class="mt-2 text-sm text-slate-700">{{ drawerChaptersSummary() }}</p>
            </div>
          </div>
        </div>

        <div v-else-if="drawerTab === 'chapters'" class="stack-3">
          <div class="flex flex-wrap items-center gap-3">
            <UiInput v-model="chaptersSearch" placeholder="Search chapters" />
            <span v-if="drawerChapters.length > 0" class="text-xs text-slate-500">
              Showing {{ filteredChapters.length }} of {{ drawerChapters.length }}
            </span>
          </div>
          <div v-if="drawerLoadingChapters" class="text-sm text-slate-600">
            Loading chapters...
          </div>
          <div v-else-if="drawerChapters.length === 0" class="text-sm text-slate-600">
            No chapters available for this episode.
          </div>
          <div v-else-if="filteredChapters.length === 0" class="text-sm text-slate-600">
            No chapters match "{{ chaptersSearch }}".
          </div>
          <ul v-else class="divide-y divide-slate-100">
            <li
              v-for="chapter in filteredChapters"
              :key="`${chapter.title}-${chapter.startSeconds}`"
              class="flex items-center justify-between gap-3 py-2"
            >
              <div>
                <div class="flex flex-wrap items-center gap-2">
                  <p class="text-sm font-semibold text-slate-900">{{ chapter.title }}</p>
                  <UiBadge v-if="isSponsorChapter(chapter.title)" tone="info">Sponsor</UiBadge>
                </div>
                <p class="text-xs text-slate-500">
                  Starts at {{ formatDuration(Math.floor(chapter.startSeconds)) }}
                </p>
              </div>
              <UiButton size="sm" variant="outline" @click="drawerItem && openPlayerAt(drawerItem, chapter.startSeconds)">
                Play from here
              </UiButton>
            </li>
          </ul>
        </div>

        <div v-else-if="drawerTab === 'transcript'" class="stack-3">
          <div class="flex flex-wrap items-center gap-3">
            <UiInput v-model="transcriptSearch" placeholder="Search transcript" />
            <span v-if="drawerTranscriptSegments.length > 0" class="text-xs text-slate-500">
              Showing {{ filteredTranscriptSegments.length }} of {{ drawerTranscriptSegments.length }}
            </span>
            <span v-else-if="drawerTranscriptText" class="text-xs text-slate-500">
              Showing {{ filteredTranscriptLines.length }} of {{ transcriptLines.length }} lines
            </span>
          </div>
          <div v-if="drawerLoadingTranscript" class="text-sm text-slate-600">
            Loading transcript...
          </div>
          <div v-else-if="drawerTranscriptStatus !== 'available'" class="text-sm text-slate-600">
            {{ drawerTranscriptSummary() }}
          </div>
          <div v-else-if="drawerTranscriptSegments.length > 0" class="divide-y divide-slate-100">
            <div v-if="filteredTranscriptSegments.length === 0" class="py-2 text-sm text-slate-600">
              No transcript segments match "{{ transcriptSearch }}".
            </div>
            <button
              v-for="segment in filteredTranscriptSegments"
              :key="`${segment.start}-${segment.text}`"
              type="button"
              class="flex w-full items-start justify-between gap-3 py-2 text-left hover:bg-slate-50"
              @click="drawerItem && openPlayerAt(drawerItem, segment.start)"
            >
              <div>
                <p class="text-xs font-semibold uppercase tracking-[0.2em] text-slate-400">
                  {{ formatDuration(Math.floor(segment.start)) }}
                  <span v-if="segment.speaker"> • {{ segment.speaker }}</span>
                </p>
                <p class="text-sm text-slate-700">{{ segment.text }}</p>
              </div>
              <span class="text-xs text-slate-400">Play</span>
            </button>
          </div>
          <div v-else-if="drawerTranscriptText" class="rounded-lg border border-slate-200 bg-slate-50 p-3 text-xs text-slate-700">
            <pre class="whitespace-pre-wrap">{{ transcriptDisplayText }}</pre>
          </div>
          <div v-else class="text-sm text-slate-600">
            Transcript is available but could not be rendered.
            <div v-if="drawerTranscriptAssets.length > 0" class="mt-2 text-xs text-slate-500">
              Assets:
              <ul class="list-disc pl-4">
                <li v-for="asset in drawerTranscriptAssets" :key="asset.url || asset.type">
                  {{ asset.language || "unknown" }} {{ asset.type || "text" }}
                  <span v-if="asset.url"> (downloadable)</span>
                </li>
              </ul>
            </div>
          </div>
        </div>
      </div>
    </UiDrawer>
  </section>
</template>
