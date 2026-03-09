import type { SummariesResponse, SummarySorting } from "../../types/api";
import { httpClient } from "./http";

export interface SummaryListQuery {
  page: number;
  count: number;
  sorting: SummarySorting;
  q?: string;
  podcastIds?: string[];
  favoritesOnly?: boolean;
}

export const summariesApi = {
  list(query: SummaryListQuery): Promise<SummariesResponse> {
    const params: Record<string, string | number | string[]> = {
      page: query.page,
      count: query.count,
      sorting: query.sorting,
    };
    if (query.q) {
      params.q = query.q;
    }
    if (query.podcastIds && query.podcastIds.length > 0) {
      params["podcastIds[]"] = query.podcastIds;
    }
    if (query.favoritesOnly) {
      params.favoritesOnly = "true";
    }
    return httpClient.get<SummariesResponse>("/summaries", { params });
  },

  favorite(id: string): Promise<void> {
    return httpClient.post(`/summaries/${id}/favorite`);
  },

  unfavorite(id: string): Promise<void> {
    return httpClient.post(`/summaries/${id}/unfavorite`);
  },
};
