import type { AppSettings, PromptVersion, ResummarizeFilter, ResummarizeResult } from "../../types/api";
import { httpClient } from "./http";

export const settingsApi = {
  get(): Promise<AppSettings> {
    return httpClient.get<AppSettings>("/settings");
  },
  update(payload: Partial<AppSettings>): Promise<AppSettings> {
    return httpClient.patch<AppSettings, Partial<AppSettings>>("/settings", payload);
  },
  backfillSummaries(): Promise<{ message: string }> {
    return httpClient.post("/settings/backfill-summaries");
  },
  getBackfillStatus(): Promise<{ running: boolean }> {
    return httpClient.get("/settings/backfill-summaries");
  },
  resummarize(filter: ResummarizeFilter): Promise<ResummarizeResult> {
    return httpClient.post("/settings/resummarize", filter);
  },
  getSummaryModels(): Promise<{ models: string[] }> {
    return httpClient.get("/settings/summary-models");
  },
  getPromptVersions(type: "system" | "user"): Promise<PromptVersion[]> {
    return httpClient.get<PromptVersion[]>(`/settings/prompt-versions?type=${type}`);
  },
  restorePromptVersion(id: string): Promise<AppSettings> {
    return httpClient.post<AppSettings>(`/settings/prompt-versions/${id}/restore`);
  },
};
