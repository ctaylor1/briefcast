<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { RouterLink } from "vue-router";
import AlternateFeedsDialog from "../components/dashboard/AlternateFeedsDialog.vue";
import PodcastCard from "../components/dashboard/PodcastCard.vue";
import UiAlert from "../components/ui/UiAlert.vue";
import UiCard from "../components/ui/UiCard.vue";
import UiDialog from "../components/ui/UiDialog.vue";
import { usePlayer } from "../composables/usePlayer";
import { usePodcastActions } from "../composables/usePodcastActions";
import { getErrorMessage, podcastsApi } from "../lib/api";
import type { Podcast } from "../types/api";

const player = usePlayer();
const isLoading = ref(true);
const errorMessage = ref("");
const podcasts = ref<Podcast[]>([]);

async function loadPodcasts(): Promise<void> {
  try {
    podcasts.value = await podcastsApi.list();
    errorMessage.value = "";
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Failed to load podcasts.");
  } finally {
    isLoading.value = false;
  }
}

const actions = usePodcastActions(loadPodcasts);

const sortedPodcasts = computed(() =>
  [...podcasts.value].sort((left, right) => {
    const a = left.LastEpisode ? Date.parse(left.LastEpisode) : 0;
    const b = right.LastEpisode ? Date.parse(right.LastEpisode) : 0;
    return b - a;
  }),
);

const totals = computed(() => ({
  episodes: podcasts.value.reduce((sum, podcast) => sum + podcast.AllEpisodesCount, 0),
  downloaded: podcasts.value.reduce((sum, podcast) => sum + podcast.DownloadedEpisodesCount, 0),
  downloading: podcasts.value.reduce((sum, podcast) => sum + podcast.DownloadingEpisodesCount, 0),
}));

// Queues the podcast's episodes, newest first, in the player bar.
function playLatest(podcastId: string): void {
  void player.playPodcast(podcastId);
}

onMounted(loadPodcasts);
</script>

<template>
  <section class="dashboard stack-4">
    <header class="page-header surface-row surface-row--between">
      <div>
        <h2 class="section-title">Library</h2>
        <p v-if="!isLoading && podcasts.length > 0" class="section-subtitle">
          {{ podcasts.length }} podcasts · {{ totals.episodes }} episodes · {{ totals.downloaded }} downloaded<template
            v-if="totals.downloading > 0"
          >
            · {{ totals.downloading }} downloading</template>
        </p>
      </div>
      <RouterLink to="/add" class="ui-button ui-button--primary">Add podcast</RouterLink>
    </header>

    <UiAlert v-if="errorMessage" tone="danger">
      {{ errorMessage }}
    </UiAlert>

    <div v-if="isLoading" class="library-grid">
      <div v-for="index in 6" :key="index" class="stack-2">
        <span class="skeleton library-skeleton-cover"></span>
        <span class="skeleton library-skeleton-line"></span>
      </div>
    </div>

    <UiCard v-else-if="sortedPodcasts.length === 0 && !errorMessage" padding="lg" class="empty-state">
      <p class="empty-state__title">No podcasts in your library yet</p>
      <p class="empty-state__copy">Add a feed URL or import an OPML file to start building your queue.</p>
      <RouterLink to="/add" class="ui-button ui-button--primary empty-state__action">Add your first podcast</RouterLink>
    </UiCard>

    <div v-else class="library-grid">
      <PodcastCard
        v-for="podcast in sortedPodcasts"
        :key="podcast.ID"
        :podcast="podcast"
        :busy="actions.activeId.value === podcast.ID"
        @play="playLatest"
        @download-all="actions.downloadAll"
        @toggle-pause="actions.togglePause"
        @toggle-retention="actions.toggleRetention"
        @toggle-sponsor-skip="actions.toggleSponsorSkip"
        @toggle-briefpoint="actions.toggleBriefpoint"
        @edit-feeds="actions.openAlternateFeeds"
        @delete="actions.requestDelete"
      />
    </div>

    <UiDialog
      :open="actions.podcastToDelete.value !== null"
      tone="danger"
      title="Delete podcast"
      :description="actions.podcastToDelete.value ? `Delete '${actions.podcastToDelete.value.Title}' and all downloaded files?` : ''"
      confirm-label="Delete"
      :busy="actions.isDeleteBusy.value"
      @close="actions.podcastToDelete.value = null"
      @confirm="actions.confirmDelete"
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
.library-grid {
  display: grid;
  gap: var(--space-4);
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
}

.library-skeleton-cover {
  display: block;
  aspect-ratio: 1 / 1;
  border-radius: var(--radius-3);
}

.library-skeleton-line {
  display: block;
  height: 14px;
  width: 70%;
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
  max-width: 44ch;
}

.empty-state__action {
  margin-top: var(--space-4);
}

@media (min-width: 768px) {
  .library-grid {
    grid-template-columns: repeat(auto-fill, minmax(190px, 1fr));
  }
}
</style>
