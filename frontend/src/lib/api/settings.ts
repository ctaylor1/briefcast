import type { RetentionSettings } from "../../types/api";
import { httpClient } from "./http";

export const settingsApi = {
  get(): Promise<RetentionSettings> {
    return httpClient.get<RetentionSettings>("/settings");
  },
  update(payload: Partial<RetentionSettings>): Promise<RetentionSettings> {
    return httpClient.patch<RetentionSettings, Partial<RetentionSettings>>("/settings", payload);
  },
};
