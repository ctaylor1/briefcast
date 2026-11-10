import { formatBytes } from "./format";
import type { PodcastItem } from "../types/api";

// PodcastItem.DownloadStatus values (db.DownloadStatus on the server).
export const DownloadStatus = {
  Queued: 0,
  Downloading: 1,
  Downloaded: 2,
  NotQueued: 3,
  Paused: 4,
} as const;

export type BadgeTone = "neutral" | "info" | "success" | "warning" | "danger";

export const isQueued = (item: PodcastItem): boolean => item.DownloadStatus === DownloadStatus.Queued;
export const isDownloading = (item: PodcastItem): boolean => item.DownloadStatus === DownloadStatus.Downloading;
export const isDownloaded = (item: PodcastItem): boolean => item.DownloadStatus === DownloadStatus.Downloaded;
export const isPaused = (item: PodcastItem): boolean => item.DownloadStatus === DownloadStatus.Paused;

export function downloadStatusLabel(status: number): string {
  switch (status) {
    case DownloadStatus.Queued:
      return "Queued";
    case DownloadStatus.Downloading:
      return "Downloading";
    case DownloadStatus.Downloaded:
      return "Downloaded";
    case DownloadStatus.NotQueued:
      return "Not queued";
    case DownloadStatus.Paused:
      return "Paused";
    default:
      return "Unknown";
  }
}

export function downloadStatusTone(status: number): BadgeTone {
  switch (status) {
    case DownloadStatus.Downloading:
      return "info";
    case DownloadStatus.Downloaded:
      return "success";
    case DownloadStatus.Queued:
    case DownloadStatus.Paused:
      return "warning";
    default:
      return "neutral";
  }
}

export function transcriptPill(status: string): { label: string; tone: BadgeTone; visible: boolean } {
  switch (status) {
    case "available":
      return { label: "Transcript ready", tone: "success", visible: true };
    case "processing":
      return { label: "Transcript in progress", tone: "info", visible: true };
    case "failed":
      return { label: "Transcript failed", tone: "danger", visible: true };
    default:
      if (status.startsWith("pending_")) {
        return { label: "Transcript queued", tone: "warning", visible: true };
      }
      return { label: "Transcript missing", tone: "neutral", visible: false };
  }
}

export function hasKnownTotal(item: PodcastItem): boolean {
  return item.DownloadTotalBytes > 0;
}

export function progressPercent(item: PodcastItem): number {
  if (!hasKnownTotal(item)) {
    return 0;
  }
  return Math.min(100, Math.round((item.DownloadedBytes / item.DownloadTotalBytes) * 100));
}

export function progressLabel(item: PodcastItem): string {
  if (hasKnownTotal(item)) {
    return `${progressPercent(item)}% (${formatBytes(item.DownloadedBytes)} / ${formatBytes(item.DownloadTotalBytes)})`;
  }
  if (item.DownloadedBytes > 0) {
    return `${formatBytes(item.DownloadedBytes)} downloaded`;
  }
  return "Downloading...";
}

export function progressAriaValue(item: PodcastItem): number | undefined {
  return hasKnownTotal(item) ? progressPercent(item) : undefined;
}

export function progressAriaText(item: PodcastItem): string {
  return `Download progress for ${item.Title}: ${progressLabel(item)}`;
}
