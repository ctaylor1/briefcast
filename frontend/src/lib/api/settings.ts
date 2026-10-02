import type {
  AppLogsResponse,
  AppSettings,
  AppSettingsUpdate,
  OpenRouterModelsResponse,
  PromptVersion,
  RepairWorkResponse,
  ResummarizeFilter,
  ResummarizeResult,
  SummaryJobStartResponse,
  SummaryJobStatus,
} from "../../types/api";
import { httpClient } from "./http";

export const settingsApi = {
  get(): Promise<AppSettings> {
    return httpClient.get<AppSettings>("/settings");
  },
  update(payload: AppSettingsUpdate): Promise<AppSettings> {
    return httpClient.patch<AppSettings, AppSettingsUpdate>("/settings", payload);
  },
  backfillSummaries(): Promise<SummaryJobStartResponse> {
    return httpClient.post<SummaryJobStartResponse>("/settings/backfill-summaries");
  },
  getBackfillStatus(): Promise<SummaryJobStatus> {
    return httpClient.get<SummaryJobStatus>("/settings/backfill-summaries");
  },
  previewResummarize(filter: ResummarizeFilter): Promise<ResummarizeResult> {
    return httpClient.post<ResummarizeResult, ResummarizeFilter>("/settings/resummarize", filter);
  },
  startResummarize(filter: ResummarizeFilter): Promise<SummaryJobStartResponse> {
    return httpClient.post<SummaryJobStartResponse, ResummarizeFilter>("/settings/resummarize", filter);
  },
  getSummaryModels(): Promise<{ models: string[] }> {
    return httpClient.get("/settings/summary-models");
  },
  getOpenRouterModels(refresh = false): Promise<OpenRouterModelsResponse> {
    return httpClient.get<OpenRouterModelsResponse>(`/settings/openrouter-models${refresh ? "?refresh=true" : ""}`);
  },
  getPromptVersions(type: "system" | "user"): Promise<PromptVersion[]> {
    return httpClient.get<PromptVersion[]>(`/settings/prompt-versions?type=${type}`);
  },
  restorePromptVersion(id: string): Promise<AppSettings> {
    return httpClient.post<AppSettings>(`/settings/prompt-versions/${id}/restore`);
  },
  getLogs(limit = 200): Promise<AppLogsResponse> {
    return httpClient.get<AppLogsResponse>(`/settings/logs?limit=${encodeURIComponent(String(limit))}`);
  },
  getRepairWork(limit = 50): Promise<RepairWorkResponse> {
    return httpClient.get<RepairWorkResponse>(`/settings/repair-work?limit=${encodeURIComponent(String(limit))}`);
  },
  startRepairWork(limit = 50): Promise<RepairWorkResponse> {
    return httpClient.post<RepairWorkResponse>(`/settings/repair-work?limit=${encodeURIComponent(String(limit))}`);
  },
};
