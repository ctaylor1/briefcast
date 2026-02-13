import type { Podcast } from "../../types/api";
import { httpClient } from "./http";

export const podcastsApi = {
  list(): Promise<Podcast[]> {
    return httpClient.get<Podcast[]>("/podcasts");
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
};
