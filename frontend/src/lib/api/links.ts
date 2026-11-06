import type { LinkTargetDetail, TopLinksResponse } from "../../types/api";
import { httpClient } from "./http";

export interface TopLinksQuery {
  kind?: string;
  platform?: string;
  podcastId?: string;
  since?: string;
  until?: string;
  includeBoilerplate?: boolean;
  limit?: number;
}

export const linksApi = {
  top(query: TopLinksQuery): Promise<TopLinksResponse> {
    return httpClient.get<TopLinksResponse>("/links/top", { params: query });
  },
  getTarget(id: string, limit: number): Promise<LinkTargetDetail> {
    return httpClient.get<LinkTargetDetail>(`/links/targets/${encodeURIComponent(id)}`, {
      params: { limit },
    });
  },
};
