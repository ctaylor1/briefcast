import type {
  AppLogsResponse,
  AppSettings,
  AppSettingsUpdate,
  MonitoringSnapshot,
  OpenRouterModelsResponse,
  PromptVersion,
  RepairWorkResponse,
  ResummarizeFilter,
  ResummarizeResult,
  SummaryJobStartResponse,
  SummaryJobStatus,
} from "../../types/api";
import { httpClient } from "./http";
import { apiPath } from "./paths";

export const settingsApi = {
  get(): Promise<AppSettings> {
    return httpClient.get<AppSettings>("/settings");
  },
  update(payload: AppSettingsUpdate): Promise<AppSettings> {
    return httpClient.patch<AppSettings, AppSettingsUpdate>("/settings", payload);
  },
  backfillSummaries(limit: number): Promise<SummaryJobStartResponse> {
    return httpClient.post<SummaryJobStartResponse, { limit: number }>("/settings/backfill-summaries", { limit });
  },
  getBackfillStatus(): Promise<SummaryJobStatus> {
    return httpClient.get<SummaryJobStatus>("/settings/backfill-summaries");
  },
  stopSummaryJob(): Promise<SummaryJobStartResponse> {
    return httpClient.post<SummaryJobStartResponse>("/settings/backfill-summaries/stop");
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
    return httpClient.get<OpenRouterModelsResponse>("/settings/openrouter-models", {
      params: refresh ? { refresh: "true" } : undefined,
    });
  },
  getPromptVersions(type: "system" | "user"): Promise<PromptVersion[]> {
    return httpClient.get<PromptVersion[]>("/settings/prompt-versions", { params: { type } });
  },
  restorePromptVersion(id: string): Promise<AppSettings> {
    return httpClient.post<AppSettings>(apiPath`/settings/prompt-versions/${id}/restore`);
  },
  getLogs(limit = 200): Promise<AppLogsResponse> {
    return httpClient.get<AppLogsResponse>("/settings/logs", { params: { limit } });
  },
  getRepairWork(limit = 50): Promise<RepairWorkResponse> {
    return httpClient.get<RepairWorkResponse>("/settings/repair-work", { params: { limit } });
  },
  startRepairWork(limit = 50): Promise<RepairWorkResponse> {
    return httpClient.post<RepairWorkResponse>("/settings/repair-work", undefined, { params: { limit } });
  },
  getMonitoring(): Promise<MonitoringSnapshot> {
    return httpClient.get<MonitoringSnapshot>("/settings/monitoring");
  },
};
