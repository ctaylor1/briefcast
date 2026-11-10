import type { LinkTargetDetail, TopLinksResponse } from "../../types/api";
import { httpClient } from "./http";
import { apiPath } from "./paths";

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
    return httpClient.get<LinkTargetDetail>(apiPath`/links/targets/${id}`, {
      params: { limit },
    });
  },
};
