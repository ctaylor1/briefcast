<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import { marked } from "marked";
import { sanitizeHtml } from "../lib/sanitize";
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
import { formatDate } from "../lib/format";
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

// Reader state
const readerOpen = ref(false);
const readerItem = ref<SummaryListItem | null>(null);
const readerSummaryHtml = ref("");
const readerSummaryRaw = ref("");
const readerLoadingSummary = ref(false);
const readerTocItems = ref<Array<{ id: string; text: string; level: number }>>([]);

// Reader tab state
const readerTab = ref<"summary" | "transcript">("summary");
const readerTranscriptText = ref("");
const readerLoadingTranscript = ref(false);
const readerTranscriptStatus = ref("");

// Scroll progress
const scrollProgress = ref(0);

const readerItemIndex = computed(() => {
  if (!readerItem.value) return -1;
  return items.value.findIndex((item) => item.id === readerItem.value?.id);
});

const readerHasPrevious = computed(() => readerItemIndex.value > 0);
const readerHasNext = computed(
  () => readerItemIndex.value >= 0 && readerItemIndex.value < items.value.length - 1,
);

const readerPosition = computed(() => {
  if (readerItemIndex.value < 0) return "";
  return `${readerItemIndex.value + 1} of ${items.value.length}`;
});

const sortedPodcastOptions = computed(() =>
  [...podcastOptions.value].sort((a, b) => a.Title.localeCompare(b.Title)),
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
      favoritesOnly: filter.favoritesOnly || undefined,
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
    if (readerItem.value && readerItem.value.id === item.id) {
      readerItem.value.isFavorited = item.isFavorited;
    }
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Failed to update favorite.");
  }
}

async function openReader(item: SummaryListItem): Promise<void> {
  readerItem.value = item;
  readerOpen.value = true;
  readerTab.value = "summary";
  readerLoadingSummary.value = true;
  readerSummaryHtml.value = "";
  readerSummaryRaw.value = "";
  readerTocItems.value = [];
  readerTranscriptText.value = "";
  readerTranscriptStatus.value = "";
  scrollProgress.value = 0;

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
  window.scrollTo({ top: 0, behavior: "smooth" });
}

async function fetchTranscript(): Promise<void> {
  if (!readerItem.value) return;
  if (readerTranscriptText.value || readerLoadingTranscript.value) return;

  readerLoadingTranscript.value = true;
  try {
    const response = await episodesApi.getTranscript(readerItem.value.id);
    readerTranscriptStatus.value = response.status || "missing";
    const transcript = response.transcript;

    if (transcript && typeof transcript === "object" && !Array.isArray(transcript)) {
      const maybeSegments = (transcript as { segments?: Array<Record<string, unknown>> }).segments;
      if (Array.isArray(maybeSegments)) {
        readerTranscriptText.value = maybeSegments
          .map((seg) => String(seg.text ?? seg.transcript ?? "").trim())
          .filter((t) => t.length > 0)
          .join("\n\n");
        return;
      }
    }
    if (Array.isArray(transcript)) {
      const contentAsset = transcript
        .filter((a) => a && typeof a === "object")
        .find((a) => typeof (a as Record<string, unknown>).content === "string");
      if (contentAsset && typeof (contentAsset as Record<string, unknown>).content === "string") {
        readerTranscriptText.value = (contentAsset as Record<string, unknown>).content as string;
        return;
      }
    }
    if (typeof transcript === "string") {
      readerTranscriptText.value = transcript;
    }
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Failed to load transcript.");
  } finally {
    readerLoadingTranscript.value = false;
  }
}

function switchTab(tab: "summary" | "transcript"): void {
  readerTab.value = tab;
  if (tab === "transcript") {
    void fetchTranscript();
  }
}

function closeReader(): void {
  readerOpen.value = false;
  readerItem.value = null;
}

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (target.isContentEditable) return true;
  const tagName = target.tagName;
  return tagName === "INPUT" || tagName === "TEXTAREA" || tagName === "SELECT";
}

async function moveReader(delta: number): Promise<void> {
  if (!readerOpen.value || readerLoadingSummary.value) return;
  const currentIndex = readerItemIndex.value;
  if (currentIndex < 0) return;
  const nextIndex = currentIndex + delta;
  if (nextIndex < 0 || nextIndex >= items.value.length) return;
  const nextItem = items.value[nextIndex];
  if (!nextItem) return;
  await openReader(nextItem);
}

function onReaderKeydown(event: KeyboardEvent): void {
  if (!readerOpen.value) return;
  if (event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey) return;
  if (isEditableTarget(event.target)) return;

  if (event.key === "ArrowDown" || event.key === "j" || event.key === "J") {
    event.preventDefault();
    void moveReader(1);
    return;
  }
  if (event.key === "ArrowUp" || event.key === "k" || event.key === "K") {
    event.preventDefault();
    void moveReader(-1);
    return;
  }
  if (event.key === "Escape") {
    event.preventDefault();
    closeReader();
  }
}

function onScroll(): void {
  if (!readerOpen.value) return;
  const scrollTop = window.scrollY || document.documentElement.scrollTop;
  const docHeight = document.documentElement.scrollHeight - document.documentElement.clientHeight;
  scrollProgress.value = docHeight > 0 ? Math.min(1, scrollTop / docHeight) : 0;
}

/**
 * Convert the bullet-list summary format produced by the LLM into proper
 * markdown with headings so that `marked` can render it with visual hierarchy.
 */
function normalizeSummaryMarkdown(raw: string): string {
  if (/^#{1,3}\s+/m.test(raw)) return raw;

  const sectionRe =
    /^-\s+(Title|Core Thesis|Key Points|Notable Details|Actionable Takeaways|Open Questions(?:\s+or\s+Uncertainties)?)\s*:\s*(.*)/i;

  const lines = raw.split("\n");
  const out: string[] = [];

  for (const line of lines) {
    const m = line.match(sectionRe);
    if (m && m[1] && m[2] !== undefined) {
      const label = m[1].trim();
      const rest = m[2].trim();
      if (/^title$/i.test(label) && rest) {
        out.push(`## ${rest}`);
      } else {
        out.push(`## ${label}`);
        if (rest) {
          out.push("");
          out.push(rest);
        }
      }
    } else {
      out.push(line);
    }
  }
  return out.join("\n");
}

/** Strip markdown formatting from plain text (for TOC entries, etc.) */
function stripMarkdown(text: string): string {
  return text
    .replace(/\*\*(.+?)\*\*/g, "$1")  // **bold**
    .replace(/__(.+?)__/g, "$1")       // __bold__
    .replace(/\*(.+?)\*/g, "$1")       // *italic*
    .replace(/_(.+?)_/g, "$1")         // _italic_
    .replace(/~~(.+?)~~/g, "$1")       // ~~strikethrough~~
    .replace(/`(.+?)`/g, "$1")         // `code`
    .replace(/\[([^\]]+)\]\([^)]+\)/g, "$1") // [link](url)
    .replace(/^#{1,6}\s+/gm, "")       // leading # headings
    .replace(/---+/g, "")              // horizontal rules
    .trim();
}

function renderMarkdown(text: string): void {
  const toc: Array<{ id: string; text: string; level: number }> = [];
  let headingIndex = 0;

  const renderer = new marked.Renderer();
  renderer.heading = function ({ text, depth }: { text: string; depth: number }) {
    headingIndex++;
    const id = `summary-heading-${headingIndex}`;
    const cleanText = stripMarkdown(text);
    toc.push({ id, text: cleanText, level: depth });
    return `<h${depth} id="${id}">${cleanText}</h${depth}>`;
  };

  const html = marked.parse(normalizeSummaryMarkdown(text), { renderer, async: false }) as string;
  readerSummaryHtml.value = sanitizeHtml(html);
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

function sendToObsidian(): void {
  if (!readerItem.value || !readerSummaryRaw.value) return;

  const title = readerItem.value.episodeTitle;
  const podcast = readerItem.value.podcastTitle;
  const pubDate = readerItem.value.pubDate ? formatDate(readerItem.value.pubDate) : "";
  const model = readerItem.value.model || "";

  const frontmatter = [
    "---",
    `title: "${title.replace(/"/g, '\\"')}"`,
    `podcast: "${podcast.replace(/"/g, '\\"')}"`,
    pubDate ? `date: ${readerItem.value.pubDate.split("T")[0]}` : "",
    model ? `model: ${model}` : "",
    "tags: [briefcast, podcast-summary]",
    "---",
  ]
    .filter(Boolean)
    .join("\n");

  const content = `${frontmatter}\n\n# ${title}\n\n${readerSummaryRaw.value}`;

  const sanitizeName = (s: string) =>
    s
      .replace(/[\\/:*?"<>|#^[\]]/g, "")
      .replace(/\s+/g, " ")
      .trim();
  const name = encodeURIComponent(sanitizeName(`${podcast} - ${title}`));
  const encodedContent = encodeURIComponent(content);
  window.location.href = `obsidian://new?name=${name}&content=${encodedContent}`;
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
  window.addEventListener("keydown", onReaderKeydown);
  window.addEventListener("scroll", onScroll, { passive: true });
});

onBeforeUnmount(() => {
  window.removeEventListener("keydown", onReaderKeydown);
  window.removeEventListener("scroll", onScroll);
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
    <div v-if="readerOpen && readerItem" class="reader">
      <!-- Progress bar -->
      <div class="reader__progress" :style="{ width: `${scrollProgress * 100}%` }"></div>

      <!-- Sticky toolbar -->
      <div class="reader__toolbar">
        <button type="button" class="reader__toolbar-btn" @click="closeReader" title="Back to list">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <line x1="19" y1="12" x2="5" y2="12" />
            <polyline points="12 19 5 12 12 5" />
          </svg>
          <span class="reader__toolbar-btn-label">Back</span>
        </button>

        <div class="reader__toolbar-center">
          <span class="reader__toolbar-position">{{ readerPosition }}</span>
        </div>

        <div class="reader__toolbar-nav">
          <span class="reader__toolbar-hint">
            <kbd>K</kbd> prev
            <span class="reader__toolbar-hint-sep">/</span>
            <kbd>J</kbd> next
          </span>
          <button
            type="button"
            class="reader__toolbar-btn"
            :class="{ 'reader__toolbar-btn--disabled': !readerHasPrevious }"
            :disabled="!readerHasPrevious || readerLoadingSummary"
            title="Previous summary"
            @click="moveReader(-1)"
          >
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="15 18 9 12 15 6" />
            </svg>
          </button>
          <button
            type="button"
            class="reader__toolbar-btn"
            :class="{ 'reader__toolbar-btn--disabled': !readerHasNext }"
            :disabled="!readerHasNext || readerLoadingSummary"
            title="Next summary"
            @click="moveReader(1)"
          >
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="9 18 15 12 9 6" />
            </svg>
          </button>
        </div>
      </div>

      <!-- Reader content area -->
      <div class="reader__container">
        <div class="reader__sidebar" v-if="readerTocItems.length > 2 && readerTab === 'summary'">
          <div class="reader__toc visually-scrollable">
            <p class="reader__toc-title">Contents</p>
            <nav aria-label="Table of contents">
              <ul class="reader__toc-list">
                <li
                  v-for="heading in readerTocItems"
                  :key="heading.id"
                  :class="`reader__toc-item reader__toc-item--l${heading.level}`"
                >
                  <button
                    type="button"
                    class="reader__toc-link"
                    @click="scrollToHeading(heading.id)"
                  >
                    {{ heading.text }}
                  </button>
                </li>
              </ul>
            </nav>
            <div class="reader__toc-actions">
              <button
                type="button"
                class="reader__obsidian-btn"
                title="Send to Obsidian"
                @click="sendToObsidian"
              >
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
                  <polyline points="14 2 14 8 20 8" />
                  <line x1="12" y1="18" x2="12" y2="12" />
                  <line x1="9" y1="15" x2="15" y2="15" />
                </svg>
                <span>Send to Obsidian</span>
              </button>
            </div>
          </div>
        </div>

        <article class="reader__main">
          <!-- Header -->
          <header class="reader__header">
            <h1 class="reader__title">{{ readerItem.episodeTitle }}</h1>
            <p class="reader__podcast">{{ readerItem.podcastTitle }}</p>
            <div class="reader__meta">
              <UiBadge tone="neutral">{{ readerItem.readTime }} min read</UiBadge>
              <span v-if="readerItem.pubDate" class="meta-text">
                {{ formatDate(readerItem.pubDate) }}
              </span>
              <span v-if="readerItem.model" class="meta-text">
                {{ readerItem.model }}
              </span>
              <button
                type="button"
                class="reader__fav-btn"
                :class="{ 'reader__fav-btn--active': readerItem.isFavorited }"
                :title="readerItem.isFavorited ? 'Remove from favorites' : 'Add to favorites'"
                @click="toggleFavorite(readerItem, $event)"
              >
                <svg width="18" height="18" viewBox="0 0 24 24" :fill="readerItem.isFavorited ? 'currentColor' : 'none'" stroke="currentColor" stroke-width="2">
                  <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
                </svg>
              </button>
            </div>
          </header>

          <!-- Tabs -->
          <div class="reader__tabs" role="tablist">
            <button
              role="tab"
              type="button"
              class="reader__tab"
              :class="{ 'reader__tab--active': readerTab === 'summary' }"
              :aria-selected="readerTab === 'summary'"
              @click="switchTab('summary')"
            >
              Summary
            </button>
            <button
              role="tab"
              type="button"
              class="reader__tab"
              :class="{ 'reader__tab--active': readerTab === 'transcript' }"
              :aria-selected="readerTab === 'transcript'"
              @click="switchTab('transcript')"
            >
              Transcript
            </button>
          </div>

          <!-- Tab: Summary -->
          <div v-if="readerTab === 'summary'" class="reader__body">
            <p v-if="readerLoadingSummary" class="meta-text">Loading summary...</p>
            <div
              v-else-if="readerSummaryHtml"
              class="reader__prose"
              v-html="readerSummaryHtml"
            ></div>
            <p v-else class="meta-text">No summary content available.</p>
          </div>

          <!-- Tab: Transcript -->
          <div v-else-if="readerTab === 'transcript'" class="reader__body">
            <p v-if="readerLoadingTranscript" class="meta-text">Loading transcript...</p>
            <template v-else-if="readerTranscriptText">
              <div class="reader__transcript">
                <pre>{{ readerTranscriptText }}</pre>
              </div>
            </template>
            <p v-else-if="readerTranscriptStatus === 'missing'" class="meta-text">
              No transcript available for this episode.
            </p>
            <p v-else class="meta-text">
              Transcript not yet available (status: {{ readerTranscriptStatus || "unknown" }}).
            </p>
          </div>

          <!-- Bottom navigation -->
          <footer class="reader__footer">
            <button
              type="button"
              class="reader__footer-nav"
              :class="{ 'reader__footer-nav--disabled': !readerHasPrevious }"
              :disabled="!readerHasPrevious || readerLoadingSummary"
              @click="moveReader(-1)"
            >
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="15 18 9 12 15 6" />
              </svg>
              <span>Previous</span>
            </button>
            <button
              type="button"
              class="reader__footer-nav"
              :class="{ 'reader__footer-nav--disabled': !readerHasNext }"
              :disabled="!readerHasNext || readerLoadingSummary"
              @click="moveReader(1)"
            >
              <span>Next</span>
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="9 18 15 12 9 6" />
              </svg>
            </button>
          </footer>
        </article>
      </div>
    </div>

    <!-- ═══ Library List View ═════════════════════════════════ -->
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
          <div class="summaries-filters__footer-actions">
            <label class="summaries-filters__favorites-toggle">
              <input
                type="checkbox"
                :checked="filter.favoritesOnly"
                @change="filter.favoritesOnly = !filter.favoritesOnly"
              />
              <svg width="14" height="14" viewBox="0 0 24 24" :fill="filter.favoritesOnly ? 'currentColor' : 'none'" stroke="currentColor" stroke-width="2">
                <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
              </svg>
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
            @click="toggleFavorite(item, $event)"
          >
            <svg width="16" height="16" viewBox="0 0 24 24" :fill="item.isFavorited ? 'currentColor' : 'none'" stroke="currentColor" stroke-width="2">
              <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
            </svg>
          </button>
          <button
            type="button"
            class="summaries-list__content"
            @click="openReader(item)"
          >
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
            <p v-if="item.excerpt" class="summaries-list__excerpt">{{ item.excerpt }}</p>
          </button>
        </div>
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
/* ═══════════════════════════════════════════════════════════════
   Reader View
   Modelled after Pocket / Instapaper / Safari Reader:
   - scroll progress bar
   - sticky compact toolbar
   - centered prose column that scales from phone → ultrawide
   - TOC sidebar on wide screens
   - bottom prev/next navigation
   ═══════════════════════════════════════════════════════════════ */

/* ── Progress bar ───────────────────────────────────────── */
.reader__progress {
  position: fixed;
  top: 0;
  left: 0;
  height: 3px;
  background: var(--color-accent);
  z-index: 100;
  transition: width 80ms linear;
  pointer-events: none;
}

/* ── Sticky toolbar ─────────────────────────────────────── */
.reader__toolbar {
  position: sticky;
  top: 0;
  z-index: 50;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  background: var(--color-bg-primary);
  border-bottom: 1px solid var(--color-border);
  margin: calc(-1 * var(--space-4)) calc(-1 * var(--space-4)) 0;
  border-radius: var(--radius-3) var(--radius-3) 0 0;
}

.reader__toolbar-btn {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  border: 0;
  background: transparent;
  color: var(--color-text-secondary);
  padding: var(--space-2);
  border-radius: var(--radius-2);
  cursor: pointer;
  font-size: var(--font-caption-size);
  line-height: 1;
  transition: color var(--duration-fast) var(--ease-enter),
              background-color var(--duration-fast) var(--ease-enter);
}

.reader__toolbar-btn:hover:not(:disabled) {
  color: var(--color-text-primary);
  background: var(--color-hover);
}

.reader__toolbar-btn:disabled,
.reader__toolbar-btn--disabled {
  opacity: 0.3;
  cursor: default;
}

.reader__toolbar-center {
  flex: 1;
  text-align: center;
}

.reader__toolbar-position {
  color: var(--color-text-tertiary);
  font-size: var(--font-caption-size);
}

.reader__toolbar-nav {
  display: flex;
  align-items: center;
  gap: var(--space-1);
}

.reader__toolbar-hint {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--color-text-tertiary);
  font-size: 11px;
  margin-right: var(--space-2);
}

.reader__toolbar-hint kbd {
  padding: 1px 5px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-1);
  background: var(--color-bg-secondary);
  font-family: inherit;
  font-size: inherit;
}

.reader__toolbar-hint-sep {
  opacity: 0.5;
}

.reader__toolbar-btn-label {
  display: none;
}

/* ── Container: sidebar + main ──────────────────────────── */
.reader__container {
  display: flex;
  justify-content: center;
  gap: var(--space-6);
  padding-top: var(--space-6);
}

.reader__sidebar {
  display: none;
}

/* ── Main article ───────────────────────────────────────── */
.reader__main {
  min-width: 0;
  max-width: min(72ch, 100%);
  width: 100%;
  padding-inline: var(--space-2);
}

/* ── Header ─────────────────────────────────────────────── */
.reader__header {
  padding-bottom: var(--space-5);
  border-bottom: 1px solid var(--color-border);
  margin-bottom: var(--space-4);
}

.reader__title {
  margin: 0 0 var(--space-2);
  color: var(--color-text-primary);
  font-size: 1.75rem;
  font-weight: 700;
  line-height: 1.25;
  letter-spacing: -0.01em;
}

.reader__podcast {
  margin: 0 0 var(--space-3);
  color: var(--color-text-secondary);
  font-size: var(--font-body-size);
}

.reader__meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-3);
}

.reader__fav-btn {
  border: 0;
  background: transparent;
  padding: var(--space-1);
  cursor: pointer;
  color: var(--color-text-tertiary);
  transition: color var(--duration-fast) var(--ease-enter);
}

.reader__fav-btn:hover {
  color: var(--color-warning, #e5a00d);
}

.reader__fav-btn--active {
  color: var(--color-warning, #e5a00d);
}

/* ── Tabs ───────────────────────────────────────────────── */
.reader__tabs {
  display: flex;
  gap: 0;
  border-bottom: 2px solid var(--color-border);
  margin-bottom: var(--space-5);
}

.reader__tab {
  border: 0;
  background: transparent;
  color: var(--color-text-secondary);
  padding: var(--space-3) var(--space-4);
  font-size: var(--font-body-size);
  font-weight: 500;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  margin-bottom: -2px;
  transition: color var(--duration-fast) var(--ease-enter),
              border-color var(--duration-fast) var(--ease-enter);
}

.reader__tab:hover {
  color: var(--color-text-primary);
}

.reader__tab--active {
  color: var(--color-text-primary);
  border-bottom-color: var(--color-accent);
}

/* ── Body ───────────────────────────────────────────────── */
.reader__body {
  min-height: 40vh;
}

/* ── Prose typography ───────────────────────────────────── */
.reader__prose {
  color: var(--color-text-primary);
  font-size: 1.05rem;
  line-height: 1.8;
}

.reader__prose :deep(h1),
.reader__prose :deep(h2),
.reader__prose :deep(h3),
.reader__prose :deep(h4) {
  margin: 1.6em 0 0.6em;
  color: var(--color-text-primary);
  font-weight: 600;
  line-height: 1.3;
}

.reader__prose :deep(h1) { font-size: 1.5em; }
.reader__prose :deep(h2) { font-size: 1.3em; }
.reader__prose :deep(h3) { font-size: 1.15em; }

.reader__prose :deep(p) {
  margin: 0.8em 0;
}

.reader__prose :deep(ul),
.reader__prose :deep(ol) {
  margin: 0.8em 0;
  padding-left: 1.6em;
}

.reader__prose :deep(li) {
  margin: 0.35em 0;
}

.reader__prose :deep(blockquote) {
  margin: 1.2em 0;
  padding: var(--space-3) var(--space-4);
  border-left: 3px solid var(--color-accent);
  background: var(--color-bg-secondary);
  border-radius: 0 var(--radius-2) var(--radius-2) 0;
  color: var(--color-text-secondary);
}

.reader__prose :deep(code) {
  font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
  font-size: 0.9em;
  background: var(--color-bg-secondary);
  padding: 0.15em 0.3em;
  border-radius: var(--radius-1);
}

.reader__prose :deep(pre) {
  margin: 1em 0;
  padding: var(--space-3);
  background: var(--color-bg-secondary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  overflow-x: auto;
}

.reader__prose :deep(pre code) {
  padding: 0;
  background: none;
}

.reader__prose :deep(strong) {
  font-weight: 600;
}

.reader__prose :deep(hr) {
  margin: 1.8em 0;
  border: 0;
  border-top: 1px solid var(--color-border);
}

.reader__prose :deep(a) {
  color: var(--color-accent);
}

.reader__prose :deep(a:hover) {
  color: var(--color-accent-hover);
}

/* ── Transcript ─────────────────────────────────────────── */
.reader__transcript {
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
  padding: var(--space-4);
}

.reader__transcript pre {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--color-text-primary);
  font-family: var(--font-family);
  font-size: var(--font-body-size);
  line-height: 1.7;
}

/* ── Footer navigation ──────────────────────────────────── */
.reader__footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-top: var(--space-5);
  margin-top: var(--space-6);
  border-top: 1px solid var(--color-border);
}

.reader__footer-nav {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-primary);
  color: var(--color-text-secondary);
  padding: var(--space-2) var(--space-4);
  font-size: var(--font-body-size);
  cursor: pointer;
  transition: color var(--duration-fast) var(--ease-enter),
              background-color var(--duration-fast) var(--ease-enter),
              border-color var(--duration-fast) var(--ease-enter);
}

.reader__footer-nav:hover:not(:disabled) {
  color: var(--color-text-primary);
  background: var(--color-hover);
  border-color: var(--color-text-tertiary);
}

.reader__footer-nav:disabled,
.reader__footer-nav--disabled {
  opacity: 0.3;
  cursor: default;
}

/* ── TOC Sidebar ────────────────────────────────────────── */
.reader__toc {
  position: sticky;
  top: calc(var(--topbar-height) + 60px);
  max-height: calc(100vh - var(--topbar-height) - 80px);
  overflow-y: auto;
  padding-right: var(--space-3);
}

.reader__toc-title {
  margin: 0 0 var(--space-3);
  color: var(--color-text-tertiary);
  font-size: var(--font-caption-size);
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.reader__toc-list {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: var(--space-1);
}

.reader__toc-item--l3 { padding-left: var(--space-3); }
.reader__toc-item--l4 { padding-left: var(--space-6); }

.reader__toc-link {
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

.reader__toc-link:hover {
  background: var(--color-hover);
  color: var(--color-text-primary);
}

.reader__toc-actions {
  margin-top: var(--space-4);
  padding-top: var(--space-3);
  border-top: 1px solid var(--color-border);
}

.reader__obsidian-btn {
  width: 100%;
  display: flex;
  align-items: center;
  gap: var(--space-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
  color: var(--color-text-secondary);
  padding: var(--space-2) var(--space-3);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
  cursor: pointer;
  transition: background-color var(--duration-fast) var(--ease-enter),
              color var(--duration-fast) var(--ease-enter);
}

.reader__obsidian-btn:hover {
  background: var(--color-hover);
  color: var(--color-text-primary);
}

/* ═══════════════════════════════════════════════════════════════
   Filters
   ═══════════════════════════════════════════════════════════════ */
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
  color: var(--color-warning, #e5a00d);
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

/* ═══════════════════════════════════════════════════════════════
   Skeleton
   ═══════════════════════════════════════════════════════════════ */
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

/* ═══════════════════════════════════════════════════════════════
   List View
   ═══════════════════════════════════════════════════════════════ */
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
  color: var(--color-warning, #e5a00d);
}

.summaries-list__fav-btn--active {
  color: var(--color-warning, #e5a00d);
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

/* ═══════════════════════════════════════════════════════════════
   Empty State
   ═══════════════════════════════════════════════════════════════ */
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

/* ═══════════════════════════════════════════════════════════════
   Responsive Breakpoints
   ═══════════════════════════════════════════════════════════════ */

/* Tablet (≥ 768px) */
@media (min-width: 768px) {
  .summaries-filters__row {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .reader__toolbar-btn-label {
    display: inline;
  }

  .reader__title {
    font-size: 2rem;
  }
}

/* Desktop (≥ 1024px) */
@media (min-width: 1024px) {
  .summaries-filters__row {
    grid-template-columns: 1.6fr 1fr 1fr 0.8fr;
  }

  .reader__sidebar {
    display: block;
    flex-shrink: 0;
    width: 200px;
  }

  .reader__main {
    max-width: min(72ch, 100%);
  }

  .reader__title {
    font-size: 2.25rem;
  }
}

/* Wide desktop (≥ 1440px) */
@media (min-width: 1440px) {
  .reader__sidebar {
    width: 220px;
  }

  .reader__main {
    max-width: min(78ch, 100%);
  }
}
</style>
