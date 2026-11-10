import type { ChaptersResponse, EpisodeLinksResponse, EpisodeSorting, EpisodesResponse, PodcastItem, SummaryResponse, TranscriptResponse } from "../../types/api";
import { httpClient } from "./http";
import { apiPath } from "./paths";

export interface EpisodeListQuery {
  page: number;
  count: number;
  sorting: EpisodeSorting;
  q?: string;
  isDownloaded?: "true" | "false";
  isPlayed?: "true" | "false";
  isBookmarked?: "true" | "false";
  podcastIds?: string[];
}

export const episodesApi = {
  list(query: EpisodeListQuery): Promise<EpisodesResponse> {
    const params: Record<string, string | number | string[]> = {
      page: query.page,
      count: query.count,
      sorting: query.sorting,
    };
    if (query.q) {
      params.q = query.q;
    }
    if (query.isDownloaded) {
      params.isDownloaded = query.isDownloaded;
    }
    if (query.isPlayed) {
      params.isPlayed = query.isPlayed;
    }
    if (query.isBookmarked) {
      params.isBookmarked = query.isBookmarked;
    }
    if (query.podcastIds && query.podcastIds.length > 0) {
      params["podcastIds[]"] = query.podcastIds;
    }
    return httpClient.get<EpisodesResponse>("/podcastitems", { params });
  },
  getById(id: string): Promise<PodcastItem> {
    return httpClient.get<PodcastItem>(apiPath`/podcastitems/${id}`);
  },
  setPlayed(id: string, played: boolean): Promise<void> {
    return httpClient.patch<void>(apiPath`/podcastitems/${id}/${played ? "markPlayed" : "markUnplayed"}`);
  },
  setBookmarked(id: string, bookmarked: boolean): Promise<void> {
    return httpClient.patch<void>(apiPath`/podcastitems/${id}/${bookmarked ? "bookmark" : "unbookmark"}`);
  },
  queueDownload(id: string): Promise<void> {
    return httpClient.post<void>(apiPath`/podcastitems/${id}/download`);
  },
  getChapters(id: string): Promise<ChaptersResponse> {
    return httpClient.get<ChaptersResponse>(apiPath`/podcastitems/${id}/chapters`);
  },
  getTranscript(id: string): Promise<TranscriptResponse> {
    return httpClient.get<TranscriptResponse>(apiPath`/podcastitems/${id}/transcript`);
  },
  getLinks(id: string): Promise<EpisodeLinksResponse> {
    return httpClient.get<EpisodeLinksResponse>(apiPath`/podcastitems/${id}/links`);
  },
  getSummary(id: string): Promise<SummaryResponse> {
    return httpClient.get<SummaryResponse>(apiPath`/podcastitems/${id}/summary`);
  },
};
