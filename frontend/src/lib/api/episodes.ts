import type { ChaptersResponse, EpisodeSorting, EpisodesResponse, PodcastItem, TranscriptResponse } from "../../types/api";
import { httpClient } from "./http";

export interface EpisodeListQuery {
  page: number;
  count: number;
  sorting: EpisodeSorting;
  q?: string;
  isDownloaded?: "true" | "false";
  isPlayed?: "true" | "false";
}

export const episodesApi = {
  list(query: EpisodeListQuery): Promise<EpisodesResponse> {
    const params: Record<string, string | number> = {
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
    return httpClient.get<EpisodesResponse>("/podcastitems", { params });
  },
  getById(id: string): Promise<PodcastItem> {
    return httpClient.get<PodcastItem>(`/podcastitems/${id}`);
  },
  setPlayed(id: string, played: boolean): Promise<void> {
    return httpClient.get<void>(`/podcastitems/${id}/${played ? "markPlayed" : "markUnplayed"}`);
  },
  setBookmarked(id: string, bookmarked: boolean): Promise<void> {
    return httpClient.get<void>(`/podcastitems/${id}/${bookmarked ? "bookmark" : "unbookmark"}`);
  },
  queueDownload(id: string): Promise<void> {
    return httpClient.get<void>(`/podcastitems/${id}/download`);
  },
  getChapters(id: string): Promise<ChaptersResponse> {
    return httpClient.get<ChaptersResponse>(`/podcastitems/${id}/chapters`);
  },
  getTranscript(id: string): Promise<TranscriptResponse> {
    return httpClient.get<TranscriptResponse>(`/podcastitems/${id}/transcript`);
  },
};
