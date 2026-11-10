<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import AlternateFeedsDialog from "../components/dashboard/AlternateFeedsDialog.vue";
import PodcastActionsMenu from "../components/dashboard/PodcastActionsMenu.vue";
import UiAlert from "../components/ui/UiAlert.vue";
import UiButton from "../components/ui/UiButton.vue";
import UiCard from "../components/ui/UiCard.vue";
import UiDialog from "../components/ui/UiDialog.vue";
import UiIcon from "../components/ui/UiIcon.vue";
import UiIconButton from "../components/ui/UiIconButton.vue";
import { usePlayer } from "../composables/usePlayer";
import { usePodcastActions } from "../composables/usePodcastActions";
import { getErrorMessage, podcastsApi } from "../lib/api";
import { podcastImageUrl } from "../lib/api/paths";
import { formatDate, formatDuration } from "../lib/format";
import { createLatest } from "../lib/latest";
import type { Podcast, PodcastItem } from "../types/api";

const RECENT_EPISODES = 25;

const route = useRoute();
const router = useRouter();
const player = usePlayer();

const podcastId = computed(() => String(route.params.id ?? ""));
const podcast = ref<Podcast | null>(null);
const episodes = ref<PodcastItem[]>([]);
const isLoading = ref(true);
const errorMessage = ref("");
const notFound = ref(false);
const pageRequest = createLatest();

async function load(): Promise<void> {
  const id = podcastId.value;
  errorMessage.value = "";
  notFound.value = false;
  try {
    // The list endpoint carries the episode counts the single-podcast endpoint lacks.
    const result = await pageRequest.run(() => Promise.all([podcastsApi.list(), podcastsApi.items(id)]));
    if (result.stale) {
      return;
    }
    const [all, items] = result.value;
    podcast.value = all.find((entry) => entry.ID === id) ?? null;
    notFound.value = podcast.value === null;
    episodes.value = [...items].sort((a, b) => Date.parse(b.PubDate) - Date.parse(a.PubDate));
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Failed to load this podcast.");
  } finally {
    isLoading.value = false;
  }
}

const actions = usePodcastActions(load);
const recentEpisodes = computed(() => episodes.value.slice(0, RECENT_EPISODES));
const allEpisodesLink = computed(() => ({ path: "/episodes", query: { podcastIds: podcastId.value } }));

async function deletePodcast(): Promise<void> {
  if (await actions.confirmDelete()) {
    void router.push("/");
  }
}

watch(
  podcastId,
  () => {
    isLoading.value = true;
    podcast.value = null;
    episodes.value = [];
    void load();
  },
  { immediate: true },
);
</script>

<template>
  <section class="podcast-page stack-4">
    <RouterLink to="/" class="podcast-page__back">
      <UiIcon name="arrow-left" :size="16" />
      Library
    </RouterLink>

    <UiAlert v-if="errorMessage" tone="danger">{{ errorMessage }}</UiAlert>

    <UiCard v-if="isLoading" padding="lg" class="stack-2">
      <span class="skeleton podcast-page__skeleton podcast-page__skeleton--title"></span>
      <span class="skeleton podcast-page__skeleton"></span>
    </UiCard>

    <UiCard v-else-if="notFound" padding="lg" class="empty-state">
      <p class="empty-state__title">This podcast is not in your library</p>
      <p class="empty-state__copy">It may have been deleted. <RouterLink to="/">Back to the library</RouterLink></p>
    </UiCard>

    <template v-else-if="podcast">
      <header class="podcast-page__header">
        <img :src="podcastImageUrl(podcast.ID)" alt="" class="podcast-page__cover" />
        <div class="podcast-page__intro stack-2">
          <h2 class="podcast-page__title">{{ podcast.Title }}</h2>
          <p v-if="podcast.Author" class="meta-text">{{ podcast.Author }}</p>
          <p class="meta-text">
            {{ podcast.AllEpisodesCount }} episodes · {{ podcast.DownloadedEpisodesCount }} downloaded · last episode
            {{ formatDate(podcast.LastEpisode) }}
          </p>
          <p v-if="podcast.Summary" class="podcast-page__summary">{{ podcast.Summary }}</p>
          <div class="podcast-page__actions">
            <UiButton :disabled="episodes.length === 0" @click="player.playPodcast(podcast.ID)">
              <UiIcon name="play" :size="16" />
              Play latest
            </UiButton>
            <UiButton variant="secondary" :disabled="actions.activeId.value === podcast.ID" @click="actions.downloadAll(podcast)">
              <UiIcon name="download" :size="16" />
              Download all
            </UiButton>
            <PodcastActionsMenu
              :podcast="podcast"
              :busy="actions.activeId.value === podcast.ID"
              @download-all="actions.downloadAll"
              @toggle-pause="actions.togglePause"
              @toggle-retention="actions.toggleRetention"
              @toggle-sponsor-skip="actions.toggleSponsorSkip"
              @toggle-briefpoint="actions.toggleBriefpoint"
              @edit-feeds="actions.openAlternateFeeds"
              @delete="actions.requestDelete"
            />
          </div>
        </div>
      </header>

      <UiCard padding="md" class="stack-3">
        <h3 class="settings-section-title">Settings for this podcast</h3>
        <div class="podcast-page__settings">
          <label class="podcast-setting">
            <input
              type="checkbox"
              :checked="podcast.IsPaused"
              :disabled="actions.activeId.value === podcast.ID"
              @change="actions.togglePause(podcast)"
            />
            <span>
              <span class="podcast-setting__title">Pause automatic downloads</span>
              <span class="meta-text">New episodes are listed but not downloaded.</span>
            </span>
          </label>
          <label class="podcast-setting">
            <input
              type="checkbox"
              :checked="podcast.RetentionKeepAll"
              :disabled="actions.activeId.value === podcast.ID"
              @change="actions.toggleRetention(podcast)"
            />
            <span>
              <span class="podcast-setting__title">Keep all episodes</span>
              <span class="meta-text">Ignore the global retention limit for this podcast.</span>
            </span>
          </label>
          <label class="podcast-setting">
            <input
              type="checkbox"
              :checked="podcast.AutoSkipSponsorChapters"
              :disabled="actions.activeId.value === podcast.ID"
              @change="actions.toggleSponsorSkip(podcast)"
            />
            <span>
              <span class="podcast-setting__title">Skip sponsor chapters</span>
              <span class="meta-text">The player jumps over chapters that look like ads.</span>
            </span>
          </label>
          <label class="podcast-setting">
            <input
              type="checkbox"
              :checked="podcast.BriefpointEnabled"
              :disabled="actions.activeId.value === podcast.ID"
              @change="actions.toggleBriefpoint(podcast)"
            />
            <span>
              <span class="podcast-setting__title">Sync to Briefpoint</span>
              <span class="meta-text">Send this podcast's episodes to Briefpoint.</span>
            </span>
          </label>
        </div>
      </UiCard>

      <UiCard padding="md" class="stack-2">
        <div class="surface-row surface-row--between">
          <h3 class="settings-section-title">Latest episodes</h3>
          <RouterLink :to="allEpisodesLink">All episodes</RouterLink>
        </div>
        <p v-if="recentEpisodes.length === 0" class="meta-text">No episodes yet.</p>
        <ol v-else class="podcast-page__episodes">
          <li v-for="episode in recentEpisodes" :key="episode.ID" class="podcast-page__episode">
            <UiIconButton size="sm" :label="`Play ${episode.Title}`" tooltip="Play" @click="player.playEpisode(episode)">
              <UiIcon name="play" :size="16" />
            </UiIconButton>
            <span class="podcast-page__episode-title">{{ episode.Title }}</span>
            <span class="podcast-page__episode-meta">
              {{ formatDate(episode.PubDate) }} · {{ formatDuration(episode.Duration) }}<template v-if="episode.IsPlayed"> · played</template>
            </span>
          </li>
        </ol>
      </UiCard>
    </template>

    <UiDialog
      :open="actions.podcastToDelete.value !== null"
      tone="danger"
      title="Delete podcast"
      :description="actions.podcastToDelete.value ? `Delete '${actions.podcastToDelete.value.Title}' and all downloaded files?` : ''"
      confirm-label="Delete"
      :busy="actions.isDeleteBusy.value"
      @close="actions.podcastToDelete.value = null"
      @confirm="deletePodcast"
    />

    <AlternateFeedsDialog
      :open="actions.feedsPodcast.value !== null"
      :urls="actions.feedsUrls.value"
      :saving="actions.feedsSaving.value"
      :podcast-title="actions.feedsPodcast.value?.Title ?? ''"
      :error="actions.feedsError.value"
      @close="actions.closeAlternateFeeds"
      @save="actions.saveAlternateFeeds"
    />
  </section>
</template>

<style scoped>
.podcast-page__back {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  color: var(--color-text-secondary);
  text-decoration: none;
  font-size: var(--font-caption-size);
}

.podcast-page__back:hover {
  color: var(--color-text-primary);
}

.podcast-page__header {
  display: grid;
  gap: var(--space-4);
}

.podcast-page__cover {
  width: min(100%, 200px);
  aspect-ratio: 1 / 1;
  border-radius: var(--radius-4);
  object-fit: cover;
  background: var(--color-hover);
}

.podcast-page__title {
  margin: 0;
  font-size: var(--font-page-title-size);
  line-height: var(--font-page-title-line-height);
  font-weight: var(--font-page-title-weight);
}

.podcast-page__summary {
  margin: 0;
  max-width: 70ch;
  color: var(--color-text-secondary);
  display: -webkit-box;
  -webkit-line-clamp: 4;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.podcast-page__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
}

.podcast-page__settings {
  display: grid;
  gap: var(--space-2);
}

.podcast-setting {
  display: flex;
  align-items: flex-start;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  cursor: pointer;
}

.podcast-setting input {
  margin-top: 3px;
  accent-color: var(--color-accent);
}

.podcast-setting > span {
  display: grid;
}

.podcast-setting__title {
  font-weight: 600;
}

.podcast-page__episodes {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
}

.podcast-page__episode {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-items: center;
  column-gap: var(--space-3);
  padding: var(--space-2) 0;
  border-top: 1px solid var(--color-border);
}

.podcast-page__episode:first-child {
  border-top: 0;
}

.podcast-page__episode-title {
  color: var(--color-text-primary);
  font-weight: 600;
  text-decoration: none;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.podcast-page__episode-title:hover {
  text-decoration: underline;
}

.podcast-page__episode-meta {
  grid-column: 2;
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
}

.podcast-page__skeleton {
  display: block;
  height: 14px;
  width: 60%;
}

.podcast-page__skeleton--title {
  height: 26px;
  width: 40%;
}

@media (min-width: 768px) {
  .podcast-page__header {
    grid-template-columns: 200px minmax(0, 1fr);
    align-items: start;
  }

  .podcast-page__settings {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .podcast-page__episode {
    grid-template-columns: auto minmax(0, 1fr) auto;
  }

  .podcast-page__episode-meta {
    grid-column: auto;
  }
}
</style>
