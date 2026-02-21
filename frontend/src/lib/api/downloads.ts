import type { DownloadQueueResponse } from "../../types/api";
import { httpClient } from "./http";

export const downloadsApi = {
  getQueue(limit?: number): Promise<DownloadQueueResponse> {
    const params = limit ? { limit } : undefined;
    return httpClient.get<DownloadQueueResponse>("/downloads/queue", { params });
  },
  pause(): Promise<void> {
    return httpClient.post<void>("/downloads/pause");
  },
  resume(): Promise<void> {
    return httpClient.post<void>("/downloads/resume");
  },
  cancelAll(): Promise<void> {
    return httpClient.post<void>("/downloads/cancel");
  },
  cancelEpisode(id: string): Promise<void> {
    return httpClient.post<void>(`/podcastitems/${id}/cancel`);
  },
  resumeEpisode(id: string): Promise<void> {
    return httpClient.post<void>(`/podcastitems/${id}/resume`);
  },
};
