import type { Podcast, PodcastItem } from "../../types/api";
import { httpClient } from "./http";

export const podcastsApi = {
  list(): Promise<Podcast[]> {
    return httpClient.get<Podcast[]>("/podcasts");
  },
  items(id: string): Promise<PodcastItem[]> {
    return httpClient.get<PodcastItem[]>(`/podcasts/${id}/items`);
  },
  deleteById(id: string): Promise<void> {
    return httpClient.del<void>(`/podcasts/${id}`);
  },
  queueDownloadAll(id: string): Promise<void> {
    return httpClient.get<void>(`/podcasts/${id}/download`);
  },
  setPaused(id: string, paused: boolean): Promise<void> {
    return httpClient.get<void>(`/podcasts/${id}/${paused ? "pause" : "unpause"}`);
  },
  setRetentionKeepAll(id: string, keepAll: boolean): Promise<void> {
    return httpClient.patch<void, { keepAll: boolean }>(`/podcasts/${id}/retention`, { keepAll });
  },
  setAutoSkipSponsorChapters(id: string, enabled: boolean): Promise<void> {
    return httpClient.patch<void, { autoSkipSponsorChapters: boolean }>(
      `/podcasts/${id}/sponsor-skip`,
      { autoSkipSponsorChapters: enabled },
    );
  },
};
