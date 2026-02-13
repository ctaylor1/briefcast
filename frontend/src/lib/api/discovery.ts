import type { SearchResult, SearchSource } from "../../types/api";
import { httpClient } from "./http";

export interface AddPodcastPayload {
  url: string;
}

export interface SearchPodcastsQuery {
  q: string;
  searchSource: SearchSource;
}

export const discoveryApi = {
  addPodcast(payload: AddPodcastPayload): Promise<void> {
    return httpClient.post<void, AddPodcastPayload>("/podcasts", payload);
  },
  searchPodcasts(query: SearchPodcastsQuery): Promise<SearchResult[]> {
    return httpClient.get<SearchResult[]>("/search", { params: query });
  },
  uploadOpml(file: File): Promise<void> {
    const formData = new FormData();
    formData.append("file", file);
    return httpClient.post<void, FormData>("/opml", formData, {
      headers: { "Content-Type": "multipart/form-data" },
    });
  },
};
