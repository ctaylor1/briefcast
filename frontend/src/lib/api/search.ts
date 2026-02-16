import type { LocalSearchResult } from "../../types/api";
import { httpClient } from "./http";

export const searchApi = {
  local(query: string, limit = 50): Promise<LocalSearchResult[]> {
    return httpClient.get<LocalSearchResult[]>("/search/local", {
      params: { q: query, limit },
    });
  },
};
