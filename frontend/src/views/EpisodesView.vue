<script setup lang="ts">
import { onMounted, onUnmounted, reactive, ref, watch } from "vue";
import EpisodesFilters from "../components/episodes/EpisodesFilters.vue";
import EpisodesListItem from "../components/episodes/EpisodesListItem.vue";
import EpisodesPagination from "../components/episodes/EpisodesPagination.vue";
import EpisodesTable from "../components/episodes/EpisodesTable.vue";
import UiAlert from "../components/ui/UiAlert.vue";
import UiButton from "../components/ui/UiButton.vue";
import UiCard from "../components/ui/UiCard.vue";
import UiDrawer from "../components/ui/UiDrawer.vue";
import { downloadsApi, episodesApi, getErrorMessage } from "../lib/api";
import { formatBytes, formatDuration } from "../lib/format";
import type {
  Chapter,
  ChaptersResponse,
  EpisodeSorting,
  EpisodeTriState,
  PodcastItem,
  TranscriptResponse,
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
  paused: 0,
});
const downloadsPaused = ref(false);

const drawerOpen = ref(false);
const drawerItem = ref<PodcastItem | null>(null);
const drawerTab = ref<"overview" | "chapters" | "transcript">("overview");
const drawerChapters = ref<Chapter[]>([]);
const drawerChaptersSource = ref("");
const drawerTranscriptStatus = ref("missing");
const drawerTranscriptSegments = ref<
  Array<{ start: number; end: number; text: string; speaker?: string }>
>([]);
const drawerTranscriptText = ref("");
const drawerTranscriptAssets = ref<Array<{ url?: string; type?: string; language?: string }>>([]);
const drawerLoadingChapters = ref(false);
const drawerLoadingTranscript = ref(false);
const drawerLoadError = ref("");

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
    queueCounts.paused = response.counts.paused ?? 0;
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

function openPlayerAt(item: PodcastItem, startSeconds: number): void {
  const target = `/app/#/player?itemIds=${encodeURIComponent(item.ID)}&start=${Math.max(0, Math.floor(startSeconds))}`;
  window.open(target, "briefcast_player");
}

function openDrawer(item: PodcastItem, tab: "overview" | "chapters" | "transcript" = "overview"): void {
  drawerItem.value = item;
  drawerTab.value = tab;
  drawerOpen.value = true;
  void fetchDrawerData(item.ID);
}

function closeDrawer(): void {
  drawerOpen.value = false;
}

async function fetchDrawerData(id: string): Promise<void> {
  drawerLoadError.value = "";
  drawerLoadingChapters.value = true;
  drawerLoadingTranscript.value = true;
  drawerChapters.value = [];
  drawerChaptersSource.value = "";
  drawerTranscriptSegments.value = [];
  drawerTranscriptText.value = "";
  drawerTranscriptAssets.value = [];

  await Promise.all([fetchChapters(id), fetchTranscript(id)]);
}

async function fetchChapters(id: string): Promise<void> {
  drawerLoadingChapters.value = true;
  try {
    const response = await episodesApi.getChapters(id);
    applyChaptersResponse(response);
  } catch (error) {
    drawerLoadError.value = getErrorMessage(error, "Failed to load chapters.");
  } finally {
    drawerLoadingChapters.value = false;
  }
}

function applyChaptersResponse(response: ChaptersResponse): void {
  drawerChaptersSource.value = response.source || "unknown";
  drawerChapters.value = response.chapters ?? [];
}

async function fetchTranscript(id: string): Promise<void> {
  drawerLoadingTranscript.value = true;
  try {
    const response = await episodesApi.getTranscript(id);
    applyTranscriptResponse(response);
  } catch (error) {
    drawerLoadError.value = getErrorMessage(error, "Failed to load transcript.");
  } finally {
    drawerLoadingTranscript.value = false;
  }
}

function applyTranscriptResponse(response: TranscriptResponse): void {
  drawerTranscriptStatus.value = response.status || "missing";
  const transcript = response.transcript;
  if (transcript && typeof transcript === "object" && !Array.isArray(transcript)) {
    const maybeSegments = (transcript as { segments?: Array<Record<string, unknown>> }).segments;
    if (Array.isArray(maybeSegments)) {
      drawerTranscriptSegments.value = maybeSegments
        .map((segment) => ({
          start: Number(segment.start ?? segment.start_time ?? 0),
          end: Number(segment.end ?? segment.end_time ?? 0),
          text: String(segment.text ?? segment.transcript ?? "").trim(),
          speaker: typeof segment.speaker === "string" ? segment.speaker : undefined,
        }))
        .filter((segment) => segment.text.length > 0);
      return;
    }
  }

  if (Array.isArray(transcript)) {
    const assets = transcript
      .filter((asset) => asset && typeof asset === "object")
      .map((asset) => asset as Record<string, unknown>);
    const contentAsset = assets.find((asset) => typeof asset.content === "string" && asset.content.trim().length > 0);
    if (contentAsset && typeof contentAsset.content === "string") {
      drawerTranscriptText.value = contentAsset.content;
    }
    drawerTranscriptAssets.value = assets.map((asset) => ({
      url: typeof asset.url === "string" ? asset.url : undefined,
      type: typeof asset.type === "string" ? asset.type : undefined,
      language: typeof asset.language === "string" ? asset.language : undefined,
    }));
    return;
  }

  if (typeof transcript === "string") {
    drawerTranscriptText.value = transcript;
  }
}

function drawerTranscriptSummary(): string {
  switch (drawerTranscriptStatus.value) {
    case "available":
      return "Transcript is ready.";
    case "processing":
      return "WhisperX is transcribing this episode.";
    case "pending_whisperx":
      return "Waiting for WhisperX transcription.";
    case "failed":
      return "Transcript failed to generate.";
    default:
      return "No transcript available.";
  }
}

function drawerChaptersSummary(): string {
  if (drawerChapters.value.length === 0) {
    return "No chapters available.";
  }
  return `${drawerChapters.value.length} chapters available.`;
}

function queueProgressPercent(item: PodcastItem): number {
  if (item.DownloadTotalBytes <= 0) {
    return 0;
  }
  return Math.min(100, Math.round((item.DownloadedBytes / item.DownloadTotalBytes) * 100));
}

function queueProgressLabel(item: PodcastItem): string {
  if (item.DownloadTotalBytes > 0) {
    return `${queueProgressPercent(item)}% (${formatBytes(item.DownloadedBytes)} / ${formatBytes(item.DownloadTotalBytes)})`;
  }
  if (item.DownloadedBytes > 0) {
    return `${formatBytes(item.DownloadedBytes)} downloaded`;
  }
  if (item.DownloadStatus === 4) {
    return "Paused";
  }
  return item.DownloadStatus === 1 ? "Downloading..." : "Queued";
}

function queueHasKnownTotal(item: PodcastItem): boolean {
  return item.DownloadTotalBytes > 0;
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
            type="button"
            class="rounded-full px-3 py-1 text-xs font-semibold"
            :class="drawerTab === 'overview' ? 'bg-slate-900 text-white' : 'bg-slate-100 text-slate-700'"
            @click="drawerTab = 'overview'"
          >
            Overview
          </button>
          <button
            type="button"
            class="rounded-full px-3 py-1 text-xs font-semibold"
            :class="drawerTab === 'chapters' ? 'bg-slate-900 text-white' : 'bg-slate-100 text-slate-700'"
            @click="drawerTab = 'chapters'"
          >
            Chapters
          </button>
          <button
            type="button"
            class="rounded-full px-3 py-1 text-xs font-semibold"
            :class="drawerTab === 'transcript' ? 'bg-slate-900 text-white' : 'bg-slate-100 text-slate-700'"
            @click="drawerTab = 'transcript'"
          >
            Transcript
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
          <div v-if="drawerLoadingChapters" class="text-sm text-slate-600">
            Loading chapters...
          </div>
          <div v-else-if="drawerChapters.length === 0" class="text-sm text-slate-600">
            No chapters available for this episode.
          </div>
          <ul v-else class="divide-y divide-slate-100">
            <li
              v-for="chapter in drawerChapters"
              :key="`${chapter.title}-${chapter.startSeconds}`"
              class="flex items-center justify-between gap-3 py-2"
            >
              <div>
                <p class="text-sm font-semibold text-slate-900">{{ chapter.title }}</p>
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
          <div v-if="drawerLoadingTranscript" class="text-sm text-slate-600">
            Loading transcript...
          </div>
          <div v-else-if="drawerTranscriptStatus !== 'available'" class="text-sm text-slate-600">
            {{ drawerTranscriptSummary() }}
          </div>
          <div v-else-if="drawerTranscriptSegments.length > 0" class="divide-y divide-slate-100">
            <button
              v-for="segment in drawerTranscriptSegments"
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
            <pre class="whitespace-pre-wrap">{{ drawerTranscriptText }}</pre>
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
