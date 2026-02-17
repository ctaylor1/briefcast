<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import PodcastCard from "../components/dashboard/PodcastCard.vue";
import PodcastsTable from "../components/dashboard/PodcastsTable.vue";
import UiAlert from "../components/ui/UiAlert.vue";
import UiCard from "../components/ui/UiCard.vue";
import UiDialog from "../components/ui/UiDialog.vue";
import { getErrorMessage, podcastsApi } from "../lib/api";
import type { Podcast } from "../types/api";

const isLoading = ref(true);
const errorMessage = ref("");
const infoMessage = ref("");
const podcasts = ref<Podcast[]>([]);
const activeId = ref<string | null>(null);
const podcastToDelete = ref<Podcast | null>(null);
const router = useRouter();

const isDeleteBusy = computed(() => {
  if (!podcastToDelete.value) {
    return false;
  }
  return activeId.value === podcastToDelete.value.ID;
});

const sortedPodcasts = computed(() =>
  [...podcasts.value].sort((left, right) => {
    const a = left.LastEpisode ? Date.parse(left.LastEpisode) : 0;
    const b = right.LastEpisode ? Date.parse(right.LastEpisode) : 0;
    return b - a;
  }),
);

const totalEpisodes = computed(() =>
  sortedPodcasts.value.reduce((sum, podcast) => sum + podcast.AllEpisodesCount, 0),
);

const totalDownloaded = computed(() =>
  sortedPodcasts.value.reduce((sum, podcast) => sum + podcast.DownloadedEpisodesCount, 0),
);

const activeDownloads = computed(() =>
  sortedPodcasts.value.reduce((sum, podcast) => sum + podcast.DownloadingEpisodesCount, 0),
);

function openPlayer(podcastId: string): void {
  const target = `/app/#/player?podcastId=${encodeURIComponent(podcastId)}`;
  window.open(target, "briefcast_player");
}

function openPlayerScreen(podcastId: string): void {
  void router.push({
    path: "/player",
    query: {
      podcastId,
    },
  });
}

function requestDelete(podcast: Podcast): void {
  podcastToDelete.value = podcast;
}

async function loadPodcasts(): Promise<void> {
  isLoading.value = true;
  errorMessage.value = "";
  try {
    podcasts.value = await podcastsApi.list();
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Failed to load podcasts.");
  } finally {
    isLoading.value = false;
  }
}

async function runAction(id: string, action: () => Promise<void>, success: string): Promise<void> {
  activeId.value = id;
  infoMessage.value = "";
  errorMessage.value = "";
  try {
    await action();
    infoMessage.value = success;
    await loadPodcasts();
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "The action failed.");
  } finally {
    activeId.value = null;
  }
}

async function confirmDeletePodcast(): Promise<void> {
  if (!podcastToDelete.value) {
    return;
  }
  const target = podcastToDelete.value;
  await runAction(target.ID, () => podcastsApi.deleteById(target.ID), "Podcast deleted.");
  podcastToDelete.value = null;
}

async function downloadAll(podcast: Podcast): Promise<void> {
  await runAction(
    podcast.ID,
    () => podcastsApi.queueDownloadAll(podcast.ID),
    "Episodes were queued for download.",
  );
}

async function togglePause(podcast: Podcast): Promise<void> {
  await runAction(
    podcast.ID,
    () => podcastsApi.setPaused(podcast.ID, !podcast.IsPaused),
    podcast.IsPaused ? "Auto-download resumed." : "Auto-download paused.",
  );
}

async function toggleRetention(podcast: Podcast): Promise<void> {
  await runAction(
    podcast.ID,
    () => podcastsApi.setRetentionKeepAll(podcast.ID, !podcast.RetentionKeepAll),
    podcast.RetentionKeepAll ? "Retention override cleared." : "Retention override enabled.",
  );
}

async function toggleSponsorSkip(podcast: Podcast): Promise<void> {
  await runAction(
    podcast.ID,
    () => podcastsApi.setAutoSkipSponsorChapters(podcast.ID, !podcast.AutoSkipSponsorChapters),
    podcast.AutoSkipSponsorChapters ? "Sponsor auto-skip disabled." : "Sponsor auto-skip enabled.",
  );
}

onMounted(loadPodcasts);
</script>

<template>
  <section class="dashboard stack-4">
    <header class="page-header">
      <div class="surface-row surface-row--between">
        <div>
          <h2 class="section-title">Podcast Library</h2>
          <p class="section-subtitle">
            Browse your shows, queue fresh episodes, and open the player with one tap.
          </p>
        </div>
        <RouterLink to="/add" class="ui-button ui-button--primary">
          Add podcast
        </RouterLink>
      </div>
    </header>

    <UiAlert v-if="infoMessage" tone="success">
      {{ infoMessage }}
    </UiAlert>
    <UiAlert v-if="errorMessage" tone="danger">
      {{ errorMessage }}
    </UiAlert>

    <div v-if="isLoading" class="dashboard-skeleton-grid">
      <UiCard v-for="index in 4" :key="index" padding="lg" class="stack-2">
        <span class="skeleton dashboard-skeleton-line dashboard-skeleton-line--title"></span>
        <span class="skeleton dashboard-skeleton-line"></span>
        <span class="skeleton dashboard-skeleton-line dashboard-skeleton-line--short"></span>
        <span class="skeleton dashboard-skeleton-block"></span>
      </UiCard>
    </div>

    <UiCard v-else-if="sortedPodcasts.length === 0" padding="lg" class="empty-state">
      <p class="empty-state__icon" aria-hidden="true">+</p>
      <p class="empty-state__title">No podcasts in your library yet</p>
      <p class="empty-state__copy">
        Add a feed URL or import an OPML file to start building your queue.
      </p>
      <RouterLink to="/add" class="ui-button ui-button--primary empty-state__action">
        Add your first podcast
      </RouterLink>
    </UiCard>

    <div v-else class="stack-3">
      <UiCard padding="sm" tone="subtle">
        <div class="dashboard-metrics">
          <div>
            <p class="meta-text">Podcasts</p>
            <p class="dashboard-metric-value">{{ sortedPodcasts.length }}</p>
          </div>
          <div>
            <p class="meta-text">Downloaded episodes</p>
            <p class="dashboard-metric-value">{{ totalDownloaded }}</p>
          </div>
          <div>
            <p class="meta-text">Active downloads</p>
            <p class="dashboard-metric-value">{{ activeDownloads }}</p>
          </div>
          <div>
            <p class="meta-text">Episodes indexed</p>
            <p class="dashboard-metric-value">{{ totalEpisodes }}</p>
          </div>
        </div>
      </UiCard>

      <div class="dashboard-cards">
        <PodcastCard
          v-for="podcast in sortedPodcasts"
          :key="podcast.ID"
          :podcast="podcast"
          :busy="activeId === podcast.ID"
          @open-player="openPlayerScreen"
          @play="openPlayer"
          @download-all="downloadAll"
          @toggle-pause="togglePause"
          @toggle-retention="toggleRetention"
          @toggle-sponsor-skip="toggleSponsorSkip"
          @delete="requestDelete"
        />
      </div>

      <div class="dashboard-table">
        <PodcastsTable
          :podcasts="sortedPodcasts"
          :active-id="activeId"
          @open-player="openPlayerScreen"
          @play="openPlayer"
          @download-all="downloadAll"
          @toggle-pause="togglePause"
          @toggle-retention="toggleRetention"
          @toggle-sponsor-skip="toggleSponsorSkip"
          @delete="requestDelete"
        />
      </div>
    </div>

    <UiDialog
      :open="podcastToDelete !== null"
      tone="danger"
      title="Delete podcast"
      :description="podcastToDelete ? `Delete '${podcastToDelete.Title}' and all downloaded files?` : ''"
      confirm-label="Delete"
      :busy="isDeleteBusy"
      @close="podcastToDelete = null"
      @confirm="confirmDeletePodcast"
    />
  </section>
</template>

<style scoped>
.dashboard-metrics {
  display: grid;
  gap: var(--space-4);
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.dashboard-metric-value {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-section-size);
  font-weight: var(--font-section-weight);
  line-height: var(--font-section-line-height);
}

.dashboard-cards {
  display: grid;
  gap: var(--space-4);
}

.dashboard-table {
  display: none;
}

.dashboard-skeleton-grid {
  display: grid;
  gap: var(--space-4);
  grid-template-columns: 1fr;
}

.dashboard-skeleton-line {
  height: 14px;
}

.dashboard-skeleton-line--title {
  width: 68%;
  height: 20px;
}

.dashboard-skeleton-line--short {
  width: 46%;
}

.dashboard-skeleton-block {
  height: 120px;
}

.empty-state__icon {
  margin: 0 auto var(--space-3);
  width: 40px;
  height: 40px;
  border-radius: 999px;
  background: var(--color-accent-subtle);
  color: var(--color-accent-hover);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
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
  .dashboard-metrics {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }

  .dashboard-cards {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .dashboard-skeleton-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (min-width: 1200px) {
  .dashboard-cards {
    display: none;
  }

  .dashboard-table {
    display: block;
  }
}
</style>
