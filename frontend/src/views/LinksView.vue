<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { RouterLink } from "vue-router";
import UiAlert from "../components/ui/UiAlert.vue";
import UiBadge from "../components/ui/UiBadge.vue";
import UiButton from "../components/ui/UiButton.vue";
import UiCard from "../components/ui/UiCard.vue";
import UiDrawer from "../components/ui/UiDrawer.vue";
import UiInput from "../components/ui/UiInput.vue";
import UiSelect from "../components/ui/UiSelect.vue";
import { getErrorMessage, linksApi, podcastsApi } from "../lib/api";
import { formatDate } from "../lib/format";
import {
  buildTopLinksQuery,
  DEFAULT_LINK_FILTERS,
  hasActiveLinkFilters,
  LINK_KIND_OPTIONS,
  LINK_PLATFORM_OPTIONS,
  linkKindLabel,
  linkPlatformLabel,
  linkTargetLabel,
  pluralize,
  safeExternalUrl,
  type LinkFilters,
} from "../lib/links";
import type { BoilerplateRule, LinkTargetDetail, LinkTargetRank, LinkTargetSummary, Podcast } from "../types/api";

const OCCURRENCE_LIMIT = 100;

const filters = reactive<LinkFilters>({ ...DEFAULT_LINK_FILTERS });
const targets = ref<LinkTargetRank[]>([]);
const boilerplateRule = ref<BoilerplateRule | null>(null);
const podcastOptions = ref<Podcast[]>([]);
const isLoading = ref(true);
const errorMessage = ref("");
let latestListRequest = 0;

const detailOpen = ref(false);
const detailTitle = ref("");
const detail = ref<LinkTargetDetail | null>(null);
const detailLoading = ref(false);
const detailError = ref("");
let latestDetailRequest = 0;

const sortedPodcastOptions = computed(() =>
  [...podcastOptions.value].sort((a, b) => a.Title.localeCompare(b.Title)),
);

const hasActiveFilters = computed(() => hasActiveLinkFilters(filters));

const boilerplateHint = computed(() => {
  const rule = boilerplateRule.value;
  if (!rule) {
    return "";
  }
  return `Recurring links appear in at least ${rule.percent}% of a show's episodes, such as host profiles and sponsors. Shows need ${rule.minEpisodes} episodes with links to qualify.`;
});

const detailDescription = computed(() => {
  const target = detail.value?.target;
  return target ? `${linkPlatformLabel(target.platform)} · ${linkKindLabel(target.kind)}` : "";
});

const detailHref = computed(() => safeExternalUrl(detail.value?.target.canonicalUrl));
const detailPodcasts = computed(() => detail.value?.podcasts ?? []);
const detailOccurrences = computed(() => detail.value?.occurrences ?? []);

function targetMeta(target: LinkTargetRank): string {
  const parts = [linkPlatformLabel(target.platform), linkKindLabel(target.kind)];
  if (target.lastSeen) {
    parts.push(`Last seen ${formatDate(target.lastSeen)}`);
  }
  return parts.join(" · ");
}

async function loadPodcastOptions(): Promise<void> {
  try {
    podcastOptions.value = await podcastsApi.list();
  } catch {
    podcastOptions.value = [];
  }
}

async function fetchTargets(): Promise<void> {
  const request = ++latestListRequest;
  isLoading.value = true;
  errorMessage.value = "";
  try {
    const response = await linksApi.top(buildTopLinksQuery(filters));
    if (request !== latestListRequest) {
      return;
    }
    targets.value = response.targets ?? [];
    boilerplateRule.value = response.boilerplateRule;
  } catch (error) {
    if (request !== latestListRequest) {
      return;
    }
    targets.value = [];
    errorMessage.value = getErrorMessage(error, "Failed to load references.");
  } finally {
    if (request === latestListRequest) {
      isLoading.value = false;
    }
  }
}

async function openTarget(target: LinkTargetSummary): Promise<void> {
  const request = ++latestDetailRequest;
  detailTitle.value = linkTargetLabel(target);
  detail.value = null;
  detailError.value = "";
  detailLoading.value = true;
  detailOpen.value = true;
  try {
    const response = await linksApi.getTarget(target.targetId, OCCURRENCE_LIMIT);
    if (request === latestDetailRequest) {
      detail.value = response;
    }
  } catch (error) {
    if (request === latestDetailRequest) {
      detailError.value = getErrorMessage(error, "Failed to load this reference.");
    }
  } finally {
    if (request === latestDetailRequest) {
      detailLoading.value = false;
    }
  }
}

function openParent(): void {
  const parent = detail.value?.parent;
  if (parent) {
    void openTarget(parent);
  }
}

function resetFilters(): void {
  Object.assign(filters, DEFAULT_LINK_FILTERS);
}

watch(filters, () => {
  void fetchTargets();
});

onMounted(() => {
  void loadPodcastOptions();
  void fetchTargets();
});
</script>

<template>
  <section class="links-page stack-4">
    <header class="page-header">
      <h2 class="section-title">Most referenced</h2>
      <p class="section-subtitle">
        The people, posts, and articles your podcasts link to most in their show notes.
      </p>
    </header>

    <UiAlert v-if="errorMessage" tone="danger">
      {{ errorMessage }}
    </UiAlert>

    <UiCard padding="md" tone="subtle" class="links-filters">
      <div class="links-filters__row">
        <UiSelect v-model="filters.kind" label="Type">
          <option value="">All types</option>
          <option v-for="option in LINK_KIND_OPTIONS" :key="option.value" :value="option.value">
            {{ option.label }}
          </option>
        </UiSelect>
        <UiSelect v-model="filters.platform" label="Platform">
          <option value="">All platforms</option>
          <option v-for="option in LINK_PLATFORM_OPTIONS" :key="option.value" :value="option.value">
            {{ option.label }}
          </option>
        </UiSelect>
        <UiSelect v-model="filters.podcastId" label="Podcast">
          <option value="">All podcasts</option>
          <option v-for="podcast in sortedPodcastOptions" :key="podcast.ID" :value="podcast.ID">
            {{ podcast.Title }}
          </option>
        </UiSelect>
        <UiInput v-model="filters.since" type="date" label="From" />
        <UiInput v-model="filters.until" type="date" label="To" />
      </div>
      <label class="links-filters__toggle">
        <input v-model="filters.includeBoilerplate" type="checkbox" />
        <span>Include recurring links</span>
      </label>
      <p v-if="boilerplateHint" class="meta-text links-filters__hint">{{ boilerplateHint }}</p>
    </UiCard>

    <UiCard v-if="isLoading && targets.length === 0" padding="md" class="links-skeleton">
      <span v-for="index in 5" :key="index" class="skeleton links-skeleton__line"></span>
    </UiCard>

    <UiCard v-else-if="targets.length === 0 && !errorMessage" padding="lg" class="empty-state">
      <p class="links-empty__title">No references found</p>
      <p class="links-empty__copy">
        Links are captured as new episodes arrive. Older episodes appear after the link backfill runs.
      </p>
      <UiButton v-if="hasActiveFilters" variant="secondary" size="sm" @click="resetFilters">
        Reset filters
      </UiButton>
    </UiCard>

    <ol v-else-if="targets.length > 0" class="links-list" :aria-busy="isLoading">
      <li v-for="(target, index) in targets" :key="target.targetId" class="links-row">
        <span class="links-row__rank">{{ index + 1 }}</span>
        <div class="links-row__main">
          <button type="button" class="links-row__title" @click="openTarget(target)">
            {{ linkTargetLabel(target) }}
          </button>
          <p class="meta-text links-row__meta">{{ targetMeta(target) }}</p>
        </div>
        <div class="links-row__counts">
          <span class="links-row__count">{{ pluralize(target.episodeCount, "episode") }}</span>
          <span class="meta-text links-row__meta">{{ pluralize(target.podcastCount, "podcast") }}</span>
          <UiBadge v-if="target.boilerplatePodcastCount > 0">Recurring</UiBadge>
        </div>
      </li>
    </ol>

    <UiDrawer
      :open="detailOpen"
      :title="detailTitle"
      :description="detailDescription"
      @close="detailOpen = false"
    >
      <div class="stack-4">
        <UiAlert v-if="detailError" tone="danger">
          {{ detailError }}
        </UiAlert>
        <p v-if="detailLoading" class="meta-text">Loading references...</p>

        <template v-else-if="detail">
          <div class="links-detail__summary">
            <p class="links-detail__copy">
              Linked from {{ pluralize(detail.episodeCount, "episode") }}
              across {{ pluralize(detail.podcastCount, "podcast") }}.
            </p>
            <a
              v-if="detailHref"
              class="links-detail__external"
              :href="detailHref"
              target="_blank"
              rel="noopener noreferrer"
            >
              Open link
            </a>
          </div>

          <p v-if="detail.parent" class="meta-text links-detail__parent">
            Posted by
            <button type="button" class="links-detail__link" @click="openParent">
              {{ linkTargetLabel(detail.parent) }}
            </button>
          </p>

          <div v-if="detailPodcasts.length > 0" class="stack-2">
            <p class="meta-text links-detail__heading">Podcasts</p>
            <ul class="links-detail__list">
              <li v-for="podcast in detailPodcasts" :key="podcast.podcastId" class="links-detail__row">
                <div class="links-detail__main">
                  <p class="links-detail__title">{{ podcast.podcastTitle }}</p>
                  <p class="meta-text links-detail__meta">
                    {{ podcast.episodeCount }} of {{ pluralize(podcast.podcastEpisodeCount, "episode") }} with links
                  </p>
                </div>
                <UiBadge v-if="podcast.boilerplate">Recurring</UiBadge>
              </li>
            </ul>
          </div>

          <div v-if="detailOccurrences.length > 0" class="stack-2">
            <p class="meta-text links-detail__heading">
              {{ detailOccurrences.length >= OCCURRENCE_LIMIT ? `Newest ${OCCURRENCE_LIMIT} references` : "References" }}
            </p>
            <ul class="links-detail__list">
              <li
                v-for="occurrence in detailOccurrences"
                :key="`${occurrence.episodeId}:${occurrence.url}`"
                class="links-detail__row"
              >
                <div class="links-detail__main">
                  <RouterLink
                    class="links-detail__link links-detail__title"
                    :to="{ path: '/episodes', query: { q: occurrence.episodeTitle, podcastIds: occurrence.podcastId } }"
                  >
                    {{ occurrence.episodeTitle }}
                  </RouterLink>
                  <p class="meta-text links-detail__meta">
                    {{ occurrence.podcastTitle }} · {{ formatDate(occurrence.pubDate) }}
                  </p>
                  <p v-if="occurrence.contextLabel" class="meta-text links-detail__meta">
                    {{ occurrence.contextLabel }}
                  </p>
                </div>
              </li>
            </ul>
          </div>

          <p v-if="detailOccurrences.length === 0" class="meta-text">
            No episodes link to this directly.
          </p>
        </template>
      </div>
    </UiDrawer>
  </section>
</template>

<style scoped>
.links-filters {
  display: grid;
  gap: var(--space-3);
}

.links-filters__row {
  display: grid;
  gap: var(--space-3);
  grid-template-columns: minmax(0, 1fr);
}

.links-filters__toggle {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  cursor: pointer;
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
}

.links-filters__toggle input[type="checkbox"] {
  width: 20px;
  height: 20px;
  margin: 0;
  flex: 0 0 20px;
  accent-color: var(--color-accent);
}

.links-filters__hint {
  margin: 0;
}

.links-skeleton {
  display: grid;
  gap: var(--space-3);
}

.links-skeleton__line {
  height: 20px;
}

.links-empty__title {
  margin: 0 0 var(--space-2);
  color: var(--color-text-primary);
  font-weight: 600;
}

.links-empty__copy {
  margin: 0 0 var(--space-3);
}

.links-list,
.links-detail__list {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: var(--space-2);
}

.links-row {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-items: center;
  gap: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
  padding: var(--space-3);
}

.links-row__rank {
  min-width: 2ch;
  text-align: right;
  color: var(--color-text-tertiary);
  font-variant-numeric: tabular-nums;
}

.links-row__main,
.links-detail__main {
  min-width: 0;
}

.links-row__title {
  appearance: none;
  border: 0;
  background: none;
  padding: 0;
  cursor: pointer;
  text-align: left;
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
  overflow-wrap: anywhere;
}

.links-row__title:hover {
  color: var(--color-accent);
  text-decoration: underline;
}

.links-row__title:focus-visible,
.links-detail__link:focus-visible,
.links-detail__external:focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
}

.links-row__meta,
.links-detail__meta {
  margin: 0;
  overflow-wrap: anywhere;
}

.links-row__counts {
  grid-column: 2;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
}

.links-row__count {
  color: var(--color-text-primary);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.links-detail__summary {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2) var(--space-3);
}

.links-detail__copy,
.links-detail__heading,
.links-detail__parent {
  margin: 0;
}

.links-detail__copy {
  color: var(--color-text-primary);
}

.links-detail__row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
  padding: var(--space-3);
}

.links-detail__title {
  margin: 0;
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
  overflow-wrap: anywhere;
}

.links-detail__link {
  appearance: none;
  border: 0;
  background: none;
  padding: 0;
  cursor: pointer;
  color: var(--color-accent);
  font: inherit;
  text-align: left;
  text-decoration: none;
}

a.links-detail__title {
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
}

.links-detail__link:hover,
.links-detail__external:hover {
  text-decoration: underline;
}

.links-detail__external {
  color: var(--color-accent);
}

@media (min-width: 640px) {
  .links-filters__row {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .links-row {
    grid-template-columns: auto minmax(0, 1fr) auto;
  }

  .links-row__counts {
    grid-column: 3;
    justify-content: flex-end;
  }
}

@media (min-width: 1024px) {
  .links-filters__row {
    grid-template-columns: repeat(5, minmax(0, 1fr));
  }
}
</style>
