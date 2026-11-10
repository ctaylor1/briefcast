import type { Podcast, PodcastItem } from "../../types/api";
import { httpClient } from "./http";
import { apiPath } from "./paths";

export const podcastsApi = {
  list(): Promise<Podcast[]> {
    return httpClient.get<Podcast[]>("/podcasts");
  },
  items(id: string): Promise<PodcastItem[]> {
    return httpClient.get<PodcastItem[]>(apiPath`/podcasts/${id}/items`);
  },
  deleteById(id: string): Promise<void> {
    return httpClient.del<void>(apiPath`/podcasts/${id}`);
  },
  queueDownloadAll(id: string): Promise<void> {
    return httpClient.post<void>(apiPath`/podcasts/${id}/download`);
  },
  setPaused(id: string, paused: boolean): Promise<void> {
    return httpClient.patch<void>(apiPath`/podcasts/${id}/${paused ? "pause" : "unpause"}`);
  },
  setRetentionKeepAll(id: string, keepAll: boolean): Promise<void> {
    return httpClient.patch<void, { keepAll: boolean }>(apiPath`/podcasts/${id}/retention`, { keepAll });
  },
  setAutoSkipSponsorChapters(id: string, enabled: boolean): Promise<void> {
    return httpClient.patch<void, { autoSkipSponsorChapters: boolean }>(
      apiPath`/podcasts/${id}/sponsor-skip`,
      { autoSkipSponsorChapters: enabled },
    );
  },
  setBriefpointEnabled(id: string, enabled: boolean): Promise<void> {
    return httpClient.patch<void, { briefpointEnabled: boolean }>(
      apiPath`/podcasts/${id}/briefpoint`,
      { briefpointEnabled: enabled },
    );
  },
  syncToBriefpoint(id: string): Promise<void> {
    return httpClient.post<void>(apiPath`/podcasts/${id}/briefpoint/sync`);
  },
  getAlternateFeeds(id: string): Promise<{ urls: string[] }> {
    return httpClient.get<{ urls: string[] }>(apiPath`/podcasts/${id}/alternate-feeds`);
  },
  setAlternateFeeds(id: string, urls: string[]): Promise<void> {
    return httpClient.patch<void, { alternateFeedURLs: string[] }>(
      apiPath`/podcasts/${id}/alternate-feeds`,
      { alternateFeedURLs: urls },
    );
  },
};
