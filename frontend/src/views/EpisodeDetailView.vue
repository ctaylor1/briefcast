<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import EpisodeLinksPanel from "../components/episodes/EpisodeLinksPanel.vue";
import UiAlert from "../components/ui/UiAlert.vue";
import UiBadge from "../components/ui/UiBadge.vue";
import UiButton from "../components/ui/UiButton.vue";
import UiCard from "../components/ui/UiCard.vue";
import UiIcon from "../components/ui/UiIcon.vue";
import UiInput from "../components/ui/UiInput.vue";
import UiTabs from "../components/ui/UiTabs.vue";
import { useEpisodeDrawer } from "../composables/useEpisodeDrawer";
import { usePlayer } from "../composables/usePlayer";
import { downloadsApi, episodesApi, getErrorMessage } from "../lib/api";
import { episodeImageUrl, episodeMarkdownPackageUrl } from "../lib/api/paths";
import { isEpisodeFavorited, UNBOOKMARKED_DATE } from "../lib/bookmarks";
import { downloadStatusLabel, DownloadStatus } from "../lib/episodeStatus";
import { formatClock, formatDate, formatDuration } from "../lib/format";
import { readerKeyAction } from "../lib/keyboard";
import { createLatest } from "../lib/latest";
import { renderSummary } from "../lib/markdown";
import { readingPosition } from "../lib/readingList";
import { episodePath, podcastPath } from "../lib/routes";
import { sanitizeHtml } from "../lib/sanitize";
import { isSponsorChapter } from "../lib/sponsor";
import { toast } from "../lib/toast";
import type { PodcastItem } from "../types/api";

// One routed page for an episode, replacing the Episodes drawer and the Summaries reader.
// The tab is in the URL (?tab=summary), so Back works and every view can be linked.
type Tab = "overview" | "summary" | "transcript" | "chapters" | "links";
const TABS: Array<{ id: Tab; label: string }> = [
  { id: "overview", label: "Overview" },
  { id: "summary", label: "Summary" },
  { id: "transcript", label: "Transcript" },
  { id: "chapters", label: "Chapters" },
  { id: "links", label: "Links" },
];

const route = useRoute();
const router = useRouter();
const player = usePlayer();
const detail = useEpisodeDrawer();

const episodeId = computed(() => String(route.params.id ?? ""));
const tab = computed<Tab>(() => {
  const value = route.query.tab;
  return TABS.some((t) => t.id === value) ? (value as Tab) : "overview";
});
const episode = ref<PodcastItem | null>(null);
const isLoading = ref(true);
const loadError = ref("");
const busy = ref(false);
const request = createLatest();

const favorited = computed(() => (episode.value ? isEpisodeFavorited(episode.value) : false));
const showNotes = computed(() => (episode.value?.Summary ? sanitizeHtml(episode.value.Summary) : ""));
const summary = computed(() => renderSummary(detail.drawerSummaryText.value || ""));
const position = computed(() => readingPosition(episodeId.value));
const canCancel = computed(
  () => episode.value?.DownloadStatus === DownloadStatus.Queued || episode.value?.DownloadStatus === DownloadStatus.Downloading,
);

async function load(): Promise<void> {
  const id = episodeId.value;
  isLoading.value = true;
  loadError.value = "";
  try {
    const result = await request.run(() => episodesApi.getById(id));
    if (result.stale) {
      return;
    }
    episode.value = result.value;
    detail.openDrawer(result.value, tab.value);
  } catch (error) {
    episode.value = null;
    loadError.value = getErrorMessage(error, "Failed to load this episode.");
  } finally {
    isLoading.value = false;
  }
}

function selectTab(next: string): void {
  void router.replace({ query: { ...route.query, tab: next === "overview" ? undefined : next } });
}

async function run(action: () => Promise<void>, success: string, failure: string): Promise<void> {
  busy.value = true;
  try {
    await action();
    toast.success(success);
  } catch (error) {
    toast.error(getErrorMessage(error, failure));
  } finally {
    busy.value = false;
  }
}

function togglePlayed(): void {
  const item = episode.value;
  if (!item) return;
  const next = !item.IsPlayed;
  void run(
    async () => {
      await episodesApi.setPlayed(item.ID, next);
      item.IsPlayed = next;
    },
    next ? "Marked as played." : "Marked as unplayed.",
    "Could not update played status.",
  );
}

function toggleFavorite(): void {
  const item = episode.value;
  if (!item) return;
  const next = !isEpisodeFavorited(item);
  void run(
    async () => {
      await episodesApi.setBookmarked(item.ID, next);
      item.BookmarkDate = next ? new Date().toISOString() : UNBOOKMARKED_DATE;
      item.IsSummaryFavorited = next;
    },
    next ? "Added to favorites." : "Removed from favorites.",
    "Could not update favorite.",
  );
}

function queueDownload(): void {
  const item = episode.value;
  if (!item) return;
  void run(
    async () => {
      await episodesApi.queueDownload(item.ID);
      item.DownloadStatus = DownloadStatus.Queued;
    },
    "Download queued.",
    "Could not queue the download.",
  );
}

function cancelDownload(): void {
  const item = episode.value;
  if (!item) return;
  void run(
    async () => {
      await downloadsApi.cancelEpisode(item.ID);
      item.DownloadStatus = DownloadStatus.NotQueued;
    },
    "Download cancelled.",
    "Could not cancel the download.",
  );
}

function playFrom(seconds: number): void {
  if (episode.value) {
    void player.playEpisodeAt(episode.value, seconds);
  }
}

function goTo(id: string | null): void {
  if (id) {
    void router.push({ path: episodePath(id), query: { tab: tab.value === "overview" ? undefined : tab.value } });
  }
}

function scrollToHeading(id: string): void {
  document.getElementById(id)?.scrollIntoView({ behavior: "smooth", block: "start" });
}

// J/K move through the list the reader came from; Escape goes back to it.
function onKeydown(event: KeyboardEvent): void {
  if (!position.value) return;
  const action = readerKeyAction(event);
  if (!action) return;
  event.preventDefault();
  if (action === "next") goTo(position.value.next);
  else if (action === "previous") goTo(position.value.previous);
  else router.back();
}

watch(episodeId, () => void load(), { immediate: true });
watch(tab, (next) => detail.setDrawerTab(next));
onMounted(() => window.addEventListener("keydown", onKeydown));
onBeforeUnmount(() => window.removeEventListener("keydown", onKeydown));
</script>

<template>
  <section class="episode-page stack-4">
    <div class="episode-page__topline">
      <RouterLink v-if="episode" :to="podcastPath(episode.PodcastID)" class="episode-page__back">
        <UiIcon name="arrow-left" :size="16" />
        {{ episode.Podcast?.Title || "Podcast" }}
      </RouterLink>
      <div v-if="position" class="episode-page__reading" aria-label="Reading list">
        <span class="meta-text">{{ position.label }} {{ position.index + 1 }} of {{ position.total }} · J/K</span>
        <UiButton size="sm" variant="ghost" :disabled="!position.previous" @click="goTo(position.previous)">
          <UiIcon name="chevron-left" :size="16" />
          Previous
        </UiButton>
        <UiButton size="sm" variant="ghost" :disabled="!position.next" @click="goTo(position.next)">
          Next
          <UiIcon name="chevron-right" :size="16" />
        </UiButton>
      </div>
    </div>

    <UiAlert v-if="loadError" tone="danger">{{ loadError }}</UiAlert>

    <UiCard v-if="isLoading && !episode" padding="lg" class="stack-2">
      <span class="skeleton episode-page__skeleton episode-page__skeleton--title"></span>
      <span class="skeleton episode-page__skeleton"></span>
    </UiCard>

    <template v-else-if="episode">
      <header class="episode-page__header">
        <img :src="episodeImageUrl(episode.ID)" alt="" class="episode-page__cover" />
        <div class="stack-2">
          <h2 class="episode-page__title">{{ episode.Title }}</h2>
          <p class="meta-text">
            {{ formatDate(episode.PubDate) }} · {{ formatDuration(episode.Duration) }}
            <template v-if="episode.PlaybackPositionSeconds && !episode.IsPlayed">
              · stopped at {{ formatClock(episode.PlaybackPositionSeconds) }}
            </template>
          </p>
          <p class="episode-page__badges">
            <UiBadge v-if="episode.IsPlayed" tone="success">Played</UiBadge>
            <UiBadge v-if="episode.DownloadStatus !== DownloadStatus.NotQueued" tone="neutral">
              {{ downloadStatusLabel(episode.DownloadStatus) }}
            </UiBadge>
          </p>
          <div class="episode-page__actions">
            <UiButton @click="player.playEpisode(episode)">
              <UiIcon name="play" :size="16" />
              {{ episode.PlaybackPositionSeconds && !episode.IsPlayed ? "Resume" : "Play" }}
            </UiButton>
            <UiButton
              variant="secondary"
              :disabled="busy"
              :aria-pressed="episode.IsPlayed ? 'true' : 'false'"
              @click="togglePlayed"
            >
              <UiIcon :name="episode.IsPlayed ? 'check-circle' : 'circle'" :size="16" />
              Played
            </UiButton>
            <UiButton
              variant="secondary"
              :disabled="busy"
              :aria-pressed="favorited ? 'true' : 'false'"
              @click="toggleFavorite"
            >
              <UiIcon :name="favorited ? 'star-filled' : 'star'" :size="16" />
              Favorite
            </UiButton>
            <UiButton
              v-if="canCancel"
              variant="ghost"
              :disabled="busy"
              @click="cancelDownload"
            >
              <UiIcon name="close" :size="16" />
              Cancel download
            </UiButton>
            <UiButton
              v-else-if="episode.DownloadStatus !== DownloadStatus.Downloaded"
              variant="ghost"
              :disabled="busy"
              @click="queueDownload"
            >
              <UiIcon name="download" :size="16" />
              Download
            </UiButton>
          </div>
        </div>
      </header>

      <UiCard padding="md">
        <UiTabs :model-value="tab" :tabs="TABS" label="Episode" @update:model-value="selectTab">
          <UiAlert v-if="detail.drawerLoadError.value" tone="danger">{{ detail.drawerLoadError.value }}</UiAlert>

          <div v-if="tab === 'overview'" class="stack-3">
            <!-- Feed HTML, sanitized with DOMPurify before rendering. -->
            <div v-if="showNotes" class="episode-page__prose" v-html="showNotes"></div>
            <p v-else class="meta-text">No show notes.</p>
            <div class="episode-page__facts">
              <p><span class="meta-text">Transcript</span><br />{{ detail.drawerTranscriptSummary() }}</p>
              <p><span class="meta-text">Chapters</span><br />{{ detail.drawerChaptersSummary() }}</p>
              <p><span class="meta-text">Summary</span><br />{{ detail.drawerSummarySummary() }}</p>
            </div>
          </div>

          <div v-else-if="tab === 'summary'" class="stack-3">
            <p v-if="detail.drawerLoadingSummary.value" class="meta-text">Loading the summary...</p>
            <p v-else-if="detail.drawerSummaryStatus.value !== 'available'" class="meta-text">
              {{ detail.drawerSummarySummary() }}
            </p>
            <template v-else>
              <div class="episode-page__tools">
                <a class="ui-button ui-button--ghost ui-button--sm" :href="episodeMarkdownPackageUrl(episode.ID, 'summary')">
                  Download Markdown package
                </a>
                <UiButton size="sm" variant="ghost" @click="detail.sendSummaryToObsidian">Send to Obsidian</UiButton>
              </div>
              <div class="episode-page__summary">
                <nav v-if="summary.toc.length > 2" class="episode-page__toc" aria-label="Summary contents">
                  <p class="meta-text">Contents</p>
                  <ol>
                    <li v-for="heading in summary.toc" :key="heading.id" :class="`episode-page__toc--l${heading.level}`">
                      <button type="button" @click="scrollToHeading(heading.id)">{{ heading.text }}</button>
                    </li>
                  </ol>
                </nav>
                <!-- Rendered Markdown, sanitized with DOMPurify in renderSummary. -->
                <article class="episode-page__prose" v-html="summary.html"></article>
              </div>
              <p v-if="detail.drawerSummaryModel.value || detail.drawerSummaryDate.value" class="meta-text">
                <span v-if="detail.drawerSummaryModel.value">Model: {{ detail.drawerSummaryModel.value }}</span>
                <span v-if="detail.drawerSummaryDate.value"> · Generated {{ formatDate(detail.drawerSummaryDate.value) }}</span>
              </p>
            </template>
          </div>

          <div v-else-if="tab === 'transcript'" class="stack-3">
            <div class="episode-page__tools">
              <UiInput v-model="detail.transcriptSearch.value" type="search" placeholder="Search the transcript" aria-label="Search the transcript" />
              <template v-if="detail.drawerCanonicalTranscript.value && detail.drawerTranscriptStatus.value === 'available'">
                <UiButton size="sm" variant="ghost" @click="detail.downloadCanonicalTranscript">Download .txt</UiButton>
                <UiButton size="sm" variant="ghost" @click="detail.sendToChatGPT">Open in ChatGPT</UiButton>
                <UiButton size="sm" variant="ghost" @click="detail.sendToClaude">Open in Claude</UiButton>
                <a class="ui-button ui-button--ghost ui-button--sm" :href="episodeMarkdownPackageUrl(episode.ID, 'transcript')">
                  Download Markdown package
                </a>
                <UiButton size="sm" variant="ghost" @click="detail.sendTranscriptToObsidian">Send to Obsidian</UiButton>
              </template>
            </div>
            <p v-if="detail.drawerLoadingTranscript.value" class="meta-text">Loading the transcript...</p>
            <p v-else-if="detail.drawerTranscriptStatus.value !== 'available'" class="meta-text">
              {{ detail.drawerTranscriptSummary() }}
            </p>
            <template v-else-if="detail.drawerTranscriptSegments.value.length > 0">
              <p class="meta-text">
                Showing {{ detail.filteredTranscriptSegments.value.length }} of {{ detail.drawerTranscriptSegments.value.length }} lines.
                Select a line to play from there.
              </p>
              <ol class="episode-page__lines">
                <li v-for="segment in detail.filteredTranscriptSegments.value" :key="`${segment.start}-${segment.text}`">
                  <button type="button" class="episode-page__line" @click="playFrom(segment.start)">
                    <span class="episode-page__stamp">{{ formatClock(segment.start) }}</span>
                    <span><strong v-if="segment.speaker">{{ segment.speaker }}: </strong>{{ segment.text }}</span>
                  </button>
                </li>
              </ol>
            </template>
            <pre v-else-if="detail.drawerTranscriptText.value" class="episode-page__raw">{{ detail.transcriptDisplayText.value }}</pre>
            <p v-else class="meta-text">The transcript is available but could not be shown.</p>
          </div>

          <div v-else-if="tab === 'chapters'" class="stack-3">
            <UiInput v-model="detail.chaptersSearch.value" type="search" placeholder="Search chapters" aria-label="Search chapters" />
            <p v-if="detail.drawerLoadingChapters.value" class="meta-text">Loading chapters...</p>
            <p v-else-if="detail.drawerChapters.value.length === 0" class="meta-text">This episode has no chapters.</p>
            <ol v-else class="episode-page__lines">
              <li v-for="chapter in detail.filteredChapters.value" :key="`${chapter.title}-${chapter.startSeconds}`">
                <button type="button" class="episode-page__line" @click="playFrom(chapter.startSeconds)">
                  <span class="episode-page__stamp">{{ formatClock(chapter.startSeconds) }}</span>
                  <span>
                    {{ chapter.title }}
                    <UiBadge v-if="isSponsorChapter(chapter.title)" tone="info">Sponsor</UiBadge>
                  </span>
                </button>
              </li>
            </ol>
          </div>

          <EpisodeLinksPanel v-else :links="detail.drawerLinks.value" :loading="detail.drawerLoadingLinks.value" />
        </UiTabs>
      </UiCard>
    </template>
  </section>
</template>

<style scoped>
.episode-page__topline {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}

.episode-page__back {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
  text-decoration: none;
}

.episode-page__back:hover {
  color: var(--color-text-primary);
}

.episode-page__reading {
  display: flex;
  align-items: center;
  gap: var(--space-1);
}

.episode-page__header {
  display: grid;
  gap: var(--space-4);
}

.episode-page__cover {
  width: min(100%, 160px);
  aspect-ratio: 1 / 1;
  border-radius: var(--radius-4);
  object-fit: cover;
  background: var(--color-hover);
}

.episode-page__title {
  margin: 0;
  font-size: var(--font-page-title-size);
  line-height: var(--font-page-title-line-height);
  font-weight: var(--font-page-title-weight);
  overflow-wrap: anywhere;
}

.episode-page__badges,
.episode-page__actions,
.episode-page__tools {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
  margin: 0;
}

.episode-page__tools > :first-child {
  flex: 1 1 220px;
}

.episode-page__facts {
  display: grid;
  gap: var(--space-3);
}

.episode-page__facts p {
  margin: 0;
}

.episode-page__summary {
  display: grid;
  gap: var(--space-4);
}

.episode-page__toc ol {
  margin: var(--space-1) 0 0;
  padding: 0;
  list-style: none;
}

.episode-page__toc button {
  padding: var(--space-1) 0;
  border: 0;
  background: transparent;
  color: var(--color-accent);
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.episode-page__toc--l3 {
  padding-left: var(--space-3);
}

.episode-page__prose {
  max-width: 72ch;
  overflow-wrap: anywhere;
  line-height: 1.65;
}

.episode-page__prose :deep(h1),
.episode-page__prose :deep(h2),
.episode-page__prose :deep(h3) {
  margin: 1.4em 0 0.4em;
  line-height: 1.3;
  /* Headings scrolled to from the contents clear the sticky top bar. */
  scroll-margin-top: calc(var(--topbar-height) + var(--space-4));
}

.episode-page__prose :deep(a) {
  color: var(--color-accent);
}

.episode-page__prose :deep(blockquote) {
  margin: 0;
  padding-left: var(--space-3);
  border-left: 3px solid var(--color-border);
  color: var(--color-text-secondary);
}

.episode-page__lines {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: 2px;
}

.episode-page__line {
  width: 100%;
  display: grid;
  grid-template-columns: 64px minmax(0, 1fr);
  gap: var(--space-3);
  align-items: baseline;
  padding: var(--space-2);
  border: 0;
  border-radius: var(--radius-2);
  background: transparent;
  color: var(--color-text-primary);
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.episode-page__line:hover {
  background: var(--color-hover);
}

.episode-page__stamp {
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
  font-variant-numeric: tabular-nums;
}

.episode-page__raw {
  white-space: pre-wrap;
  font: inherit;
  margin: 0;
}

.episode-page__skeleton {
  display: block;
  height: 14px;
  width: 60%;
}

.episode-page__skeleton--title {
  height: 26px;
  width: 50%;
}

@media (min-width: 768px) {
  .episode-page__header {
    grid-template-columns: 160px minmax(0, 1fr);
    align-items: start;
  }

  .episode-page__facts {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (min-width: 1200px) {
  .episode-page__summary {
    grid-template-columns: minmax(0, 1fr) 220px;
  }

  .episode-page__toc {
    order: 2;
    position: sticky;
    top: calc(var(--topbar-height) + var(--space-4));
    align-self: start;
  }
}
</style>
