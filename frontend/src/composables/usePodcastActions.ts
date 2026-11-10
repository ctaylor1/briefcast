import { computed, ref } from "vue";
import { getErrorMessage, podcastsApi } from "../lib/api";
import { toast } from "../lib/toast";
import type { Podcast } from "../types/api";

// Podcast actions shared by the library and the podcast page: the toggles, download all,
// alternate feeds and delete, with their dialogs and busy state. `reload` refreshes
// whatever list or page shows the podcast after a change.
export function usePodcastActions(reload: () => Promise<void>) {
  const activeId = ref<string | null>(null);
  const podcastToDelete = ref<Podcast | null>(null);
  const feedsPodcast = ref<Podcast | null>(null);
  const feedsUrls = ref<string[]>([]);
  const feedsSaving = ref(false);
  const feedsError = ref("");

  const isDeleteBusy = computed(() => !!podcastToDelete.value && activeId.value === podcastToDelete.value.ID);

  async function runAction(id: string, action: () => Promise<void>, success: string): Promise<boolean> {
    activeId.value = id;
    try {
      await action();
      toast.success(success);
      await reload();
      return true;
    } catch (error) {
      toast.error(getErrorMessage(error, "The action failed."));
      return false;
    } finally {
      activeId.value = null;
    }
  }

  const downloadAll = (podcast: Podcast) =>
    runAction(podcast.ID, () => podcastsApi.queueDownloadAll(podcast.ID), "Episodes were queued for download.");

  const togglePause = (podcast: Podcast) =>
    runAction(
      podcast.ID,
      () => podcastsApi.setPaused(podcast.ID, !podcast.IsPaused),
      podcast.IsPaused ? "Automatic downloads resumed." : "Automatic downloads paused.",
    );

  const toggleRetention = (podcast: Podcast) =>
    runAction(
      podcast.ID,
      () => podcastsApi.setRetentionKeepAll(podcast.ID, !podcast.RetentionKeepAll),
      podcast.RetentionKeepAll ? "This podcast now follows the global retention setting." : "All episodes of this podcast will be kept.",
    );

  const toggleSponsorSkip = (podcast: Podcast) =>
    runAction(
      podcast.ID,
      () => podcastsApi.setAutoSkipSponsorChapters(podcast.ID, !podcast.AutoSkipSponsorChapters),
      podcast.AutoSkipSponsorChapters ? "Sponsor auto-skip turned off." : "Sponsor auto-skip turned on.",
    );

  const toggleBriefpoint = (podcast: Podcast) =>
    runAction(
      podcast.ID,
      () => podcastsApi.setBriefpointEnabled(podcast.ID, !podcast.BriefpointEnabled),
      podcast.BriefpointEnabled ? "Briefpoint sync turned off." : "Briefpoint sync turned on. Sending episodes in the background.",
    );

  async function openAlternateFeeds(podcast: Podcast): Promise<void> {
    feedsError.value = "";
    feedsUrls.value = [];
    try {
      const response = await podcastsApi.getAlternateFeeds(podcast.ID);
      feedsUrls.value = response.urls ?? [];
    } catch (error) {
      // Do not open the editor with an empty list: saving it would delete the real feeds.
      toast.error(getErrorMessage(error, "Could not load alternate feeds."));
      return;
    }
    feedsPodcast.value = podcast;
  }

  function closeAlternateFeeds(): void {
    feedsPodcast.value = null;
    feedsError.value = "";
  }

  async function saveAlternateFeeds(urls: string[]): Promise<void> {
    if (!feedsPodcast.value) {
      return;
    }
    feedsSaving.value = true;
    feedsError.value = "";
    try {
      await podcastsApi.setAlternateFeeds(feedsPodcast.value.ID, urls);
      feedsPodcast.value = null;
      toast.success("Alternate feeds saved. Resolving alternate download URLs in the background.");
    } catch (error) {
      feedsError.value = getErrorMessage(error, "Failed to save alternate feeds.");
    } finally {
      feedsSaving.value = false;
    }
  }

  function requestDelete(podcast: Podcast): void {
    podcastToDelete.value = podcast;
  }

  async function confirmDelete(): Promise<boolean> {
    const target = podcastToDelete.value;
    if (!target) {
      return false;
    }
    const deleted = await runAction(target.ID, () => podcastsApi.deleteById(target.ID), "Podcast deleted.");
    podcastToDelete.value = null;
    return deleted;
  }

  return {
    activeId,
    podcastToDelete,
    isDeleteBusy,
    feedsPodcast,
    feedsUrls,
    feedsSaving,
    feedsError,
    downloadAll,
    togglePause,
    toggleRetention,
    toggleSponsorSkip,
    toggleBriefpoint,
    openAlternateFeeds,
    closeAlternateFeeds,
    saveAlternateFeeds,
    requestDelete,
    confirmDelete,
  };
}
