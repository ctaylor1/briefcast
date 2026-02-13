<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
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

function openPlayer(podcastId: string): void {
  window.open(`/player?podcastId=${podcastId}`, "briefcast_player");
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

onMounted(loadPodcasts);
</script>

<template>
  <section class="stack-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 class="fluid-title-xl font-semibold tracking-tight text-slate-900">Podcasts</h1>
        <p class="fluid-subtle text-slate-600">
          Responsive cards on mobile, dense table on desktop.
        </p>
      </div>
      <RouterLink
        to="/add"
        class="rounded-md bg-slate-900 px-4 py-2 text-sm font-semibold text-white transition hover:bg-slate-800"
      >
        Add Podcast
      </RouterLink>
    </div>

    <UiAlert v-if="infoMessage" tone="success">
      {{ infoMessage }}
    </UiAlert>
    <UiAlert v-if="errorMessage" tone="danger">
      {{ errorMessage }}
    </UiAlert>

    <UiCard v-if="isLoading" padding="lg" class="text-sm text-slate-600">
      Loading podcasts...
    </UiCard>

    <UiCard v-else-if="sortedPodcasts.length === 0" padding="lg" class="text-sm text-slate-600">
      No podcasts found. Add your first feed to get started.
    </UiCard>

    <div v-else class="stack-3">
      <div class="grid gap-[var(--space-3)] md:grid-cols-2 xl:hidden">
        <PodcastCard
          v-for="podcast in sortedPodcasts"
          :key="podcast.ID"
          :podcast="podcast"
          :busy="activeId === podcast.ID"
          @play="openPlayer"
          @download-all="downloadAll"
          @toggle-pause="togglePause"
          @delete="requestDelete"
        />
      </div>

      <div class="hidden xl:block">
        <PodcastsTable
          :podcasts="sortedPodcasts"
          :active-id="activeId"
          @play="openPlayer"
          @download-all="downloadAll"
          @toggle-pause="togglePause"
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
