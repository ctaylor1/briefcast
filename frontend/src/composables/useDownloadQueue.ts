import { reactive, ref } from "vue";
import { downloadsApi, getErrorMessage } from "../lib/api";
import { formatBytes } from "../lib/format";
import type { PodcastItem } from "../types/api";

export function useDownloadQueue() {
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

  async function fetchQueue(limit = 15): Promise<void> {
    queueLoading.value = true;
    queueError.value = "";
    try {
      const response = await downloadsApi.getQueue(limit);
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

  async function pauseDownloads(): Promise<void> {
    await downloadsApi.pause();
    downloadsPaused.value = true;
  }

  async function resumeDownloads(): Promise<void> {
    await downloadsApi.resume();
    downloadsPaused.value = false;
  }

  async function cancelAllDownloads(): Promise<void> {
    await downloadsApi.cancelAll();
  }

  async function cancelEpisodeDownload(id: string): Promise<void> {
    await downloadsApi.cancelEpisode(id);
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

  return {
    queueItems,
    queueLoading,
    queueError,
    queueCounts,
    downloadsPaused,
    fetchQueue,
    pauseDownloads,
    resumeDownloads,
    cancelAllDownloads,
    cancelEpisodeDownload,
    queueProgressPercent,
    queueProgressLabel,
    queueHasKnownTotal,
  };
}
