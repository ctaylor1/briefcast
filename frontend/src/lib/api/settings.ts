import type { AppSettings } from "../../types/api";
import { httpClient } from "./http";

export const settingsApi = {
  get(): Promise<AppSettings> {
    return httpClient.get<AppSettings>("/settings");
  },
  update(payload: Partial<AppSettings>): Promise<AppSettings> {
    return httpClient.patch<AppSettings, Partial<AppSettings>>("/settings", payload);
  },
};
