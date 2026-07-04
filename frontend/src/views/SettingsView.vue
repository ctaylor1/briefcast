<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import UiAlert from "../components/ui/UiAlert.vue";
import UiBadge from "../components/ui/UiBadge.vue";
import UiButton from "../components/ui/UiButton.vue";
import UiCard from "../components/ui/UiCard.vue";
import UiDrawer from "../components/ui/UiDrawer.vue";
import UiInput from "../components/ui/UiInput.vue";
import { useStatusMessage } from "../composables/useStatusMessage";
import { useTheme } from "../composables/useTheme";
import { getErrorMessage, settingsApi } from "../lib/api";
import { DEFAULT_OBSIDIAN_FOLDER, DEFAULT_OBSIDIAN_VAULT } from "../lib/obsidian";
import type { AppLogEntry, AppLogsResponse, AppSettings, PromptVersion, RepairWorkResponse, WorkQueueItem } from "../types/api";

type SettingsTab = "configuration" | "work" | "logs";

type RetentionForm = {
  keepAllEpisodes: boolean;
  keepLatestEpisodes: string;
  deleteAfterDays: string;
  deleteOnlyPlayed: boolean;
};

type InitialDownloadMode = "count" | "months" | "all";

type BackCatalogForm = {
  autoDownload: boolean;
  downloadOnAdd: boolean;
  initialDownloadMode: InitialDownloadMode;
  initialDownloadCount: string;
  initialDownloadMonths: string;
};

type SummarizationForm = {
  summarizationEnabled: boolean;
  summarizationModel: string;
  summarizationPrompt: string;
  summarizationUserPrompt: string;
};

type ModelSettingsForm = {
  llmConcurrency: number;
};

type ObsidianForm = {
  obsidianVault: string;
  obsidianFolder: string;
};

const {
  themeMode,
  timezone,
  lightStartHour,
  darkStartHour,
  setThemeMode,
  setTimezone,
  setLightStartHour,
  setDarkStartHour,
} = useTheme();

const isLoading = ref(true);
const isSavingBackCatalog = ref(false);
const isSavingRetention = ref(false);
const isSavingSummarization = ref(false);
const isSavingAppearance = ref(false);
const isSavingModelSettings = ref(false);
const isSavingObsidian = ref(false);
const isSavingBriefpoint = ref(false);
const activeTab = ref<SettingsTab>("configuration");
const isLoadingLogs = ref(false);
const logsPayload = ref<AppLogsResponse | null>(null);
const logsErrorMessage = ref("");
const isLoadingWorkQueue = ref(false);
const isStartingRepairWork = ref(false);
const workQueuePayload = ref<RepairWorkResponse | null>(null);
const workQueueErrorMessage = ref("");
const briefpointAPIKeyConfigured = ref(false);
const defaultModel = ref("");
const defaultSystemPrompt = ref("");
const defaultUserPrompt = ref("");
const usedModels = ref<string[]>([]);
const isResummarizing = ref(false);
const resummarizeModelFilter = ref("");
const resummarizeBeforeDate = ref("");
const resummarizeDryRunCount = ref<number | null>(null);
const promptHistoryOpen = ref(false);
const promptHistoryType = ref<"system" | "user">("system");
const promptHistoryVersions = ref<PromptVersion[]>([]);
const promptHistoryLoading = ref(false);
const {
  errorMessage,
  successMessage,
  clearAll,
  clearError,
  setError,
  setSuccess,
} = useStatusMessage(5000);
let workQueuePollTimer: number | undefined;

const retentionForm = ref<RetentionForm>({
  keepAllEpisodes: true,
  keepLatestEpisodes: "0",
  deleteAfterDays: "0",
  deleteOnlyPlayed: true,
});

const backCatalogForm = ref<BackCatalogForm>({
  autoDownload: true,
  downloadOnAdd: true,
  initialDownloadMode: "count",
  initialDownloadCount: "5",
  initialDownloadMonths: "6",
});

const summarizationForm = ref<SummarizationForm>({
  summarizationEnabled: false,
  summarizationModel: "",
  summarizationPrompt: "",
  summarizationUserPrompt: "",
});

const modelSettingsForm = ref<ModelSettingsForm>({
  llmConcurrency: 1,
});

const obsidianForm = ref<ObsidianForm>({
  obsidianVault: DEFAULT_OBSIDIAN_VAULT,
  obsidianFolder: DEFAULT_OBSIDIAN_FOLDER,
});

type BriefpointForm = {
  briefpointEnabled: boolean;
  briefpointServerURL: string;
  briefpointAPIKey: string;
};

const briefpointForm = ref<BriefpointForm>({
  briefpointEnabled: false,
  briefpointServerURL: "",
  briefpointAPIKey: "",
});

const modelOptions = computed(() => {
  const set = new Set<string>();
  if (defaultModel.value) set.add(defaultModel.value);
  for (const m of usedModels.value) set.add(m);
  return Array.from(set).sort();
});

const retentionEnabled = computed(() => !retentionForm.value.keepAllEpisodes);
const usesInitialDownloadCount = computed(() => backCatalogForm.value.initialDownloadMode === "count");
const usesInitialDownloadMonths = computed(() => backCatalogForm.value.initialDownloadMode === "months");
const logEntries = computed(() => logsPayload.value?.entries ?? []);
const impactLogEntries = computed(() => logsPayload.value?.impactEntries ?? []);
const logSources = computed(() => logsPayload.value?.sources ?? []);
const logReadErrors = computed(() => logsPayload.value?.readErrors ?? []);
const workQueue = computed(() => workQueuePayload.value?.queue ?? null);
const workQueueItems = computed(() => workQueue.value?.items ?? []);
const workQueueRunning = computed(() => workQueuePayload.value?.running ?? false);
const lastRepairRun = computed(() => workQueuePayload.value?.lastRun ?? null);

function mapToRetentionForm(settings: AppSettings): RetentionForm {
  return {
    keepAllEpisodes: settings.keepAllEpisodes,
    keepLatestEpisodes: String(settings.keepLatestEpisodes ?? 0),
    deleteAfterDays: String(settings.deleteAfterDays ?? 0),
    deleteOnlyPlayed: settings.deleteOnlyPlayed,
  };
}

function mapToBackCatalogForm(settings: AppSettings): BackCatalogForm {
  const mode = settings.initialDownloadMode || "count";
  return {
    autoDownload: settings.autoDownload,
    downloadOnAdd: settings.downloadOnAdd,
    initialDownloadMode: mode,
    initialDownloadCount: String(settings.initialDownloadCount ?? 5),
    initialDownloadMonths: String(settings.initialDownloadMonths ?? 6),
  };
}

function mapToSummarizationForm(settings: AppSettings): SummarizationForm {
  return {
    summarizationEnabled: settings.summarizationEnabled,
    summarizationModel: settings.summarizationModel ?? "",
    summarizationPrompt:
      settings.effectiveSystemPrompt ||
      settings.summarizationPrompt ||
      settings.defaultSystemPrompt ||
      "",
    summarizationUserPrompt:
      settings.effectiveUserPrompt ||
      settings.summarizationUserPrompt ||
      settings.defaultUserPrompt ||
      "",
  };
}

function mapToObsidianForm(settings: AppSettings): ObsidianForm {
  return {
    obsidianVault: settings.obsidianVault || DEFAULT_OBSIDIAN_VAULT,
    obsidianFolder: settings.obsidianFolder || DEFAULT_OBSIDIAN_FOLDER,
  };
}

function sanitizeNumber(value: string): number {
  const parsed = Number.parseInt(value, 10);
  if (Number.isNaN(parsed) || parsed < 0) {
    return 0;
  }
  return parsed;
}

async function loadSettings(): Promise<void> {
  isLoading.value = true;
  clearError();
  try {
    const settings = await settingsApi.get();
    retentionForm.value = mapToRetentionForm(settings);
    backCatalogForm.value = mapToBackCatalogForm(settings);
    summarizationForm.value = mapToSummarizationForm(settings);
    modelSettingsForm.value = {
      llmConcurrency: settings.llmConcurrency ?? 1,
    };
    obsidianForm.value = mapToObsidianForm(settings);
    briefpointForm.value = {
      briefpointEnabled: settings.briefpointEnabled ?? false,
      briefpointServerURL: settings.briefpointServerURL ?? "",
      briefpointAPIKey: "",
    };
    briefpointAPIKeyConfigured.value = settings.briefpointAPIKeyConfigured ?? false;
    defaultModel.value = settings.defaultModel ?? "";
    defaultSystemPrompt.value = settings.defaultSystemPrompt ?? "";
    defaultUserPrompt.value = settings.defaultUserPrompt ?? "";
    try {
      const { models } = await settingsApi.getSummaryModels();
      usedModels.value = models ?? [];
    } catch {
      usedModels.value = [];
    }
  } catch (error) {
    setError(getErrorMessage(error, "Failed to load settings."));
  } finally {
    isLoading.value = false;
  }
}

async function selectSettingsTab(tab: SettingsTab): Promise<void> {
  activeTab.value = tab;
  if (tab === "logs" && !logsPayload.value && !isLoadingLogs.value) {
    await loadLogs();
  }
  if (tab === "work" && !workQueuePayload.value && !isLoadingWorkQueue.value) {
    await loadWorkQueue();
  }
}

async function loadLogs(): Promise<void> {
  isLoadingLogs.value = true;
  logsErrorMessage.value = "";
  try {
    logsPayload.value = await settingsApi.getLogs(200);
  } catch (error) {
    logsErrorMessage.value = getErrorMessage(error, "Failed to load application logs.");
  } finally {
    isLoadingLogs.value = false;
  }
}

function clearWorkQueuePoll(): void {
  if (workQueuePollTimer !== undefined) {
    window.clearTimeout(workQueuePollTimer);
    workQueuePollTimer = undefined;
  }
}

function scheduleWorkQueuePoll(): void {
  clearWorkQueuePoll();
  if (!workQueuePayload.value?.running) {
    return;
  }
  workQueuePollTimer = window.setTimeout(() => {
    void loadWorkQueue({ silent: true });
  }, 5000);
}

async function loadWorkQueue(options: { silent?: boolean } = {}): Promise<void> {
  if (!options.silent) {
    isLoadingWorkQueue.value = true;
  }
  workQueueErrorMessage.value = "";
  try {
    workQueuePayload.value = await settingsApi.getRepairWork(50);
    scheduleWorkQueuePoll();
  } catch (error) {
    workQueueErrorMessage.value = getErrorMessage(error, "Failed to load work queue.");
    clearWorkQueuePoll();
  } finally {
    if (!options.silent) {
      isLoadingWorkQueue.value = false;
    }
  }
}

async function startRepairWork(): Promise<void> {
  isStartingRepairWork.value = true;
  workQueueErrorMessage.value = "";
  try {
    workQueuePayload.value = await settingsApi.startRepairWork(50);
    scheduleWorkQueuePoll();
  } catch (error) {
    workQueueErrorMessage.value = getErrorMessage(error, "Failed to start repair work.");
  } finally {
    isStartingRepairWork.value = false;
  }
}

function logLevelTone(level: AppLogEntry["level"]): "neutral" | "info" | "success" | "danger" | "warning" {
  switch (level) {
    case "fatal":
    case "error":
      return "danger";
    case "warn":
      return "warning";
    case "debug":
      return "neutral";
    default:
      return "info";
  }
}

function formatLogTimestamp(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return iso;
  }
  return date.toLocaleString();
}

function formatBytes(sizeBytes: number): string {
  if (!Number.isFinite(sizeBytes) || sizeBytes <= 0) {
    return "0 B";
  }
  const units = ["B", "KB", "MB", "GB"];
  let value = sizeBytes;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }
  return `${value.toFixed(unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`;
}

function formatLogFields(fields: AppLogEntry["fields"]): string {
  if (!fields || Object.keys(fields).length === 0) {
    return "";
  }
  return JSON.stringify(fields, null, 2);
}

function workQueueTone(item: WorkQueueItem): "neutral" | "info" | "success" | "danger" | "warning" {
  switch (item.category) {
    case "active":
      return "info";
    case "failed":
    case "blocked":
      return "danger";
    case "retry":
    case "queued":
      return "warning";
    default:
      return "neutral";
  }
}

function formatOptionalTimestamp(iso?: string | null): string {
  if (!iso) {
    return "";
  }
  return formatLogTimestamp(iso);
}

async function saveBackCatalogSettings(): Promise<void> {
  isSavingBackCatalog.value = true;
  clearAll();
  try {
    const updated = await settingsApi.update({
      autoDownload: backCatalogForm.value.autoDownload,
      downloadOnAdd: backCatalogForm.value.downloadOnAdd,
      initialDownloadMode: backCatalogForm.value.initialDownloadMode,
      initialDownloadCount: sanitizeNumber(backCatalogForm.value.initialDownloadCount),
      initialDownloadMonths: sanitizeNumber(backCatalogForm.value.initialDownloadMonths),
    });
    backCatalogForm.value = mapToBackCatalogForm(updated);
    setSuccess("Back catalog settings updated.");
  } catch (error) {
    setError(getErrorMessage(error, "Failed to update back catalog settings."));
  } finally {
    isSavingBackCatalog.value = false;
  }
}

async function saveRetentionSettings(): Promise<void> {
  isSavingRetention.value = true;
  clearAll();
  try {
    const updated = await settingsApi.update({
      keepAllEpisodes: retentionForm.value.keepAllEpisodes,
      keepLatestEpisodes: sanitizeNumber(retentionForm.value.keepLatestEpisodes),
      deleteAfterDays: sanitizeNumber(retentionForm.value.deleteAfterDays),
      deleteOnlyPlayed: retentionForm.value.deleteOnlyPlayed,
    });
    retentionForm.value = mapToRetentionForm(updated);
    setSuccess("Retention settings updated.");
  } catch (error) {
    setError(getErrorMessage(error, "Failed to update retention settings."));
  } finally {
    isSavingRetention.value = false;
  }
}

async function saveSummarizationSettings(): Promise<void> {
  isSavingSummarization.value = true;
  clearAll();
  try {
    const sysPrompt = summarizationForm.value.summarizationPrompt.trim();
    const usrPrompt = summarizationForm.value.summarizationUserPrompt.trim();
    const updated = await settingsApi.update({
      summarizationEnabled: summarizationForm.value.summarizationEnabled,
      summarizationModel: summarizationForm.value.summarizationModel,
      summarizationPrompt: sysPrompt === defaultSystemPrompt.value ? "" : sysPrompt,
      summarizationUserPrompt: usrPrompt === defaultUserPrompt.value ? "" : usrPrompt,
    });
    defaultSystemPrompt.value = updated.defaultSystemPrompt ?? "";
    defaultUserPrompt.value = updated.defaultUserPrompt ?? "";
    summarizationForm.value = mapToSummarizationForm(updated);
    setSuccess("Summarization settings updated.");
  } catch (error) {
    setError(getErrorMessage(error, "Failed to update summarization settings."));
  } finally {
    isSavingSummarization.value = false;
  }
}

async function openPromptHistory(type: "system" | "user"): Promise<void> {
  promptHistoryType.value = type;
  promptHistoryOpen.value = true;
  promptHistoryLoading.value = true;
  try {
    promptHistoryVersions.value = await settingsApi.getPromptVersions(type);
  } catch {
    promptHistoryVersions.value = [];
  } finally {
    promptHistoryLoading.value = false;
  }
}

async function restorePromptVersion(id: string): Promise<void> {
  try {
    const updated = await settingsApi.restorePromptVersion(id);
    defaultSystemPrompt.value = updated.defaultSystemPrompt ?? "";
    defaultUserPrompt.value = updated.defaultUserPrompt ?? "";
    summarizationForm.value = mapToSummarizationForm(updated);
    promptHistoryOpen.value = false;
    setSuccess("Prompt restored successfully.");
  } catch (error) {
    setError(getErrorMessage(error, "Failed to restore prompt version."));
  }
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString();
}

async function saveModelSettings(): Promise<void> {
  isSavingModelSettings.value = true;
  clearAll();
  try {
    const updated = await settingsApi.update({
      llmConcurrency: modelSettingsForm.value.llmConcurrency,
    });
    modelSettingsForm.value = {
      llmConcurrency: updated.llmConcurrency ?? 1,
    };
    setSuccess("Model settings updated.");
  } catch (error) {
    setError(getErrorMessage(error, "Failed to update model settings."));
  } finally {
    isSavingModelSettings.value = false;
  }
}

async function saveObsidianSettings(): Promise<void> {
  isSavingObsidian.value = true;
  clearAll();
  try {
    const updated = await settingsApi.update({
      obsidianVault: obsidianForm.value.obsidianVault.trim() || DEFAULT_OBSIDIAN_VAULT,
      obsidianFolder: obsidianForm.value.obsidianFolder.trim() || DEFAULT_OBSIDIAN_FOLDER,
    });
    obsidianForm.value = mapToObsidianForm(updated);
    setSuccess("Obsidian settings updated.");
  } catch (error) {
    setError(getErrorMessage(error, "Failed to update Obsidian settings."));
  } finally {
    isSavingObsidian.value = false;
  }
}

async function saveBriefpointSettings(): Promise<void> {
  isSavingBriefpoint.value = true;
  clearAll();
  try {
    const payload = {
      briefpointEnabled: briefpointForm.value.briefpointEnabled,
      briefpointServerURL: briefpointForm.value.briefpointServerURL,
    };
    const nextAPIKey = briefpointForm.value.briefpointAPIKey.trim();
    const updated = await settingsApi.update(
      nextAPIKey
        ? {
            ...payload,
            briefpointAPIKey: nextAPIKey,
          }
        : payload,
    );
    briefpointForm.value = {
      briefpointEnabled: updated.briefpointEnabled ?? false,
      briefpointServerURL: updated.briefpointServerURL ?? "",
      briefpointAPIKey: "",
    };
    briefpointAPIKeyConfigured.value = updated.briefpointAPIKeyConfigured ?? false;
    setSuccess("Briefpoint settings updated.");
  } catch (error) {
    setError(getErrorMessage(error, "Failed to update Briefpoint settings."));
  } finally {
    isSavingBriefpoint.value = false;
  }
}

const commonTimezones = [
  "America/New_York",
  "America/Chicago",
  "America/Denver",
  "America/Los_Angeles",
  "America/Anchorage",
  "Pacific/Honolulu",
  "America/Toronto",
  "America/Vancouver",
  "Europe/London",
  "Europe/Paris",
  "Europe/Berlin",
  "Europe/Amsterdam",
  "Europe/Rome",
  "Europe/Madrid",
  "Europe/Moscow",
  "Asia/Dubai",
  "Asia/Kolkata",
  "Asia/Singapore",
  "Asia/Shanghai",
  "Asia/Tokyo",
  "Asia/Seoul",
  "Australia/Sydney",
  "Australia/Melbourne",
  "Pacific/Auckland",
];

function formatHour(hour: number): string {
  if (hour === 0) return "12:00 AM";
  if (hour === 12) return "12:00 PM";
  if (hour < 12) return `${hour}:00 AM`;
  return `${hour - 12}:00 PM`;
}

async function saveAppearanceSettings(): Promise<void> {
  isSavingAppearance.value = true;
  clearAll();
  try {
    await settingsApi.update({
      themeMode: themeMode.value,
      timezone: timezone.value,
      lightStartHour: lightStartHour.value,
      darkStartHour: darkStartHour.value,
    });
    setSuccess("Appearance settings updated.");
  } catch (error) {
    setError(getErrorMessage(error, "Failed to update appearance settings."));
  } finally {
    isSavingAppearance.value = false;
  }
}

async function resummarizeDryRun(): Promise<void> {
  clearAll();
  try {
    const filter: Record<string, unknown> = { dryRun: true };
    if (resummarizeModelFilter.value) filter.model = resummarizeModelFilter.value;
    if (resummarizeBeforeDate.value) filter.before = new Date(resummarizeBeforeDate.value).toISOString();
    const result = await settingsApi.resummarize(filter as import("../types/api").ResummarizeFilter);
    resummarizeDryRunCount.value = result.total;
  } catch (error) {
    setError(getErrorMessage(error, "Failed to preview re-summarize."));
  }
}

async function startResummarize(): Promise<void> {
  isResummarizing.value = true;
  clearAll();
  resummarizeDryRunCount.value = null;
  try {
    const filter: Record<string, unknown> = {};
    if (resummarizeModelFilter.value) filter.model = resummarizeModelFilter.value;
    if (resummarizeBeforeDate.value) filter.before = new Date(resummarizeBeforeDate.value).toISOString();
    await settingsApi.resummarize(filter as import("../types/api").ResummarizeFilter);
    setSuccess("Re-summarize job started. Summaries will be regenerated in the background.");
  } catch (error) {
    setError(getErrorMessage(error, "Failed to start re-summarize."));
  } finally {
    isResummarizing.value = false;
  }
}

onMounted(loadSettings);
onUnmounted(clearWorkQueuePoll);
</script>

<template>
  <section class="settings-page stack-4">
    <header class="page-header">
      <h2 class="section-title">Settings</h2>
      <p class="section-subtitle">
        Configure retention, summarization, and other application behavior.
      </p>
    </header>

    <UiAlert v-if="successMessage" tone="success">
      {{ successMessage }}
    </UiAlert>
    <UiAlert v-if="errorMessage" tone="danger">
      {{ errorMessage }}
    </UiAlert>

    <div class="settings-tabs" role="tablist" aria-label="Settings sections">
      <button
        type="button"
        class="settings-tab"
        :class="{ 'settings-tab--active': activeTab === 'configuration' }"
        role="tab"
        :aria-selected="activeTab === 'configuration'"
        @click="selectSettingsTab('configuration')"
      >
        Configuration
      </button>
      <button
        type="button"
        class="settings-tab"
        :class="{ 'settings-tab--active': activeTab === 'work' }"
        role="tab"
        :aria-selected="activeTab === 'work'"
        @click="selectSettingsTab('work')"
      >
        Work Queue
      </button>
      <button
        type="button"
        class="settings-tab"
        :class="{ 'settings-tab--active': activeTab === 'logs' }"
        role="tab"
        :aria-selected="activeTab === 'logs'"
        @click="selectSettingsTab('logs')"
      >
        Logs
      </button>
    </div>

    <template v-if="activeTab === 'configuration'">
      <UiCard v-if="isLoading" padding="lg" class="stack-2">
        <span class="skeleton settings-skeleton-line settings-skeleton-line--title"></span>
        <span class="skeleton settings-skeleton-line"></span>
        <span class="skeleton settings-skeleton-line"></span>
        <span class="skeleton settings-skeleton-line settings-skeleton-line--short"></span>
      </UiCard>

      <!-- Appearance settings -->
      <UiCard v-if="!isLoading" padding="lg" class="stack-4">
      <div class="stack-2">
        <h3 class="settings-section-title">Appearance</h3>
        <p class="section-subtitle">
          Control the theme and schedule automatic light/dark switching.
        </p>
      </div>

      <div class="surface-grid surface-grid--3">
        <div class="stack-1">
          <label class="settings-label" for="settings-theme-mode">Theme</label>
          <select
            id="settings-theme-mode"
            class="ui-select"
            :value="themeMode"
            @change="setThemeMode(($event.target as HTMLSelectElement).value as 'light' | 'dark' | 'auto')"
          >
            <option value="auto">Auto (schedule)</option>
            <option value="light">Light</option>
            <option value="dark">Dark</option>
          </select>
        </div>

        <div class="stack-1">
          <label class="settings-label" for="settings-timezone">Timezone</label>
          <select
            id="settings-timezone"
            class="ui-select"
            :value="timezone"
            @change="setTimezone(($event.target as HTMLSelectElement).value)"
          >
            <option v-for="tz in commonTimezones" :key="tz" :value="tz">{{ tz.replace(/_/g, ' ') }}</option>
          </select>
        </div>
      </div>

      <div class="surface-grid surface-grid--3">
        <div class="stack-1">
          <label class="settings-label" for="settings-light-start">Light mode starts at</label>
          <select
            id="settings-light-start"
            class="ui-select"
            :disabled="themeMode !== 'auto'"
            :value="lightStartHour"
            @change="setLightStartHour(Number(($event.target as HTMLSelectElement).value))"
          >
            <option v-for="h in 24" :key="h - 1" :value="h - 1">{{ formatHour(h - 1) }}</option>
          </select>
        </div>

        <div class="stack-1">
          <label class="settings-label" for="settings-dark-start">Dark mode starts at</label>
          <select
            id="settings-dark-start"
            class="ui-select"
            :disabled="themeMode !== 'auto'"
            :value="darkStartHour"
            @change="setDarkStartHour(Number(($event.target as HTMLSelectElement).value))"
          >
            <option v-for="h in 24" :key="h - 1" :value="h - 1">{{ formatHour(h - 1) }}</option>
          </select>
        </div>
      </div>

      <div class="surface-row">
        <UiButton :disabled="isSavingAppearance" @click="saveAppearanceSettings">
          {{ isSavingAppearance ? "Saving..." : "Save appearance settings" }}
        </UiButton>
        <p class="meta-text">
          In Auto mode, the app switches between light and dark at the configured times.
        </p>
      </div>
    </UiCard>

    <!-- Back catalog download settings -->
    <UiCard v-if="!isLoading" padding="lg" class="stack-4">
      <div class="stack-2">
        <h3 class="settings-section-title">Back Catalog Downloads</h3>
        <p class="section-subtitle">
          Choose which episodes are queued when a podcast is first added. Feed entries are still saved when the provider exposes them.
        </p>
      </div>

      <label class="settings-checkbox-row">
        <input
          v-model="backCatalogForm.autoDownload"
          type="checkbox"
          class="settings-checkbox"
        />
        <div>
          <p class="settings-checkbox-title">Automatically download new episodes</p>
          <p class="meta-text">When enabled, refreshes queue newly published episodes after the podcast is subscribed.</p>
        </div>
      </label>

      <label class="settings-checkbox-row">
        <input
          v-model="backCatalogForm.downloadOnAdd"
          type="checkbox"
          class="settings-checkbox"
          :disabled="!backCatalogForm.autoDownload"
        />
        <div>
          <p class="settings-checkbox-title">Queue episodes when adding a podcast</p>
          <p class="meta-text">If disabled, imported episodes are saved but not queued for download.</p>
        </div>
      </label>

      <div class="surface-grid surface-grid--3">
        <label class="stack-1">
          <span class="ui-label">Initial back catalog</span>
          <select
            v-model="backCatalogForm.initialDownloadMode"
            class="ui-input"
            :disabled="!backCatalogForm.autoDownload || !backCatalogForm.downloadOnAdd"
          >
            <option value="count">Latest episode count</option>
            <option value="months">Past N months</option>
            <option value="all">All feed episodes</option>
          </select>
        </label>

        <UiInput
          v-model="backCatalogForm.initialDownloadCount"
          type="number"
          min="0"
          :disabled="!backCatalogForm.autoDownload || !backCatalogForm.downloadOnAdd || !usesInitialDownloadCount"
          label="Episode count"
          placeholder="5"
        />

        <UiInput
          v-model="backCatalogForm.initialDownloadMonths"
          type="number"
          min="0"
          :disabled="!backCatalogForm.autoDownload || !backCatalogForm.downloadOnAdd || !usesInitialDownloadMonths"
          label="Months"
          placeholder="6"
        />
      </div>

      <div class="surface-row">
        <UiButton :disabled="isSavingBackCatalog" @click="saveBackCatalogSettings">
          {{ isSavingBackCatalog ? "Saving..." : "Save back catalog settings" }}
        </UiButton>
        <p class="meta-text">
          Set count or months to 0 to queue all episodes returned by the feed.
        </p>
      </div>
    </UiCard>

    <!-- Retention settings -->
    <UiCard v-if="!isLoading" padding="lg" class="stack-4">
      <div class="stack-2">
        <h3 class="settings-section-title">Retention</h3>
        <p class="section-subtitle">
          Keep all episodes by default. Switch off to enable automatic cleanup rules.
        </p>
      </div>

      <label class="settings-checkbox-row">
        <input
          v-model="retentionForm.keepAllEpisodes"
          type="checkbox"
          class="settings-checkbox"
        />
        <div>
          <p class="settings-checkbox-title">Keep all episodes (default)</p>
          <p class="meta-text">
            No files are deleted automatically, even if episodes have been played.
          </p>
        </div>
      </label>

      <div class="surface-grid surface-grid--2">
        <div class="stack-1">
          <UiInput
            v-model="retentionForm.keepLatestEpisodes"
            type="number"
            min="0"
            :disabled="!retentionEnabled"
            label="Keep latest episodes"
            placeholder="0"
          />
          <p class="meta-text">
            Set to 0 to disable. When enabled, older downloaded episodes are removed regardless of played status.
          </p>
        </div>

        <div class="stack-1">
          <UiInput
            v-model="retentionForm.deleteAfterDays"
            type="number"
            min="0"
            :disabled="!retentionEnabled"
            label="Delete after (days)"
            placeholder="0"
          />
          <p class="meta-text">
            Set to 0 to disable. This applies only to episodes older than the number of days you set.
          </p>
        </div>
      </div>

      <label class="settings-checkbox-row">
        <input
          v-model="retentionForm.deleteOnlyPlayed"
          type="checkbox"
          class="settings-checkbox"
          :disabled="!retentionEnabled"
        />
        <div>
          <p class="settings-checkbox-title">Only delete played episodes</p>
          <p class="meta-text">
            If unchecked, episodes older than the threshold are removed whether or not they were played.
          </p>
        </div>
      </label>

      <div class="surface-row">
        <UiButton :disabled="isSavingRetention" @click="saveRetentionSettings">
          {{ isSavingRetention ? "Saving..." : "Save retention settings" }}
        </UiButton>
        <p class="meta-text">
          Retention cleanup runs daily. Use podcast-level overrides to keep everything for specific feeds.
        </p>
      </div>
    </UiCard>

    <!-- AI Summarization settings -->
    <UiCard v-if="!isLoading" padding="lg" class="stack-4">
      <div class="stack-2">
        <h3 class="settings-section-title">AI Summarization</h3>
        <p class="section-subtitle">
          Automatically generate a summary for each episode after transcription completes.
          Requires an LLM API key configured via environment variables.
        </p>
      </div>

      <label class="settings-checkbox-row">
        <input
          v-model="summarizationForm.summarizationEnabled"
          type="checkbox"
          class="settings-checkbox"
        />
        <div>
          <p class="settings-checkbox-title">Enable summarization</p>
          <p class="meta-text">
            When enabled, new transcriptions will be sent to your configured LLM for summarization.
          </p>
        </div>
      </label>

      <div class="stack-1">
        <label class="settings-label" for="summarization-model">Model</label>
        <select
          id="summarization-model"
          v-model="summarizationForm.summarizationModel"
          class="ui-select"
          :disabled="!summarizationForm.summarizationEnabled"
        >
          <option value="">Default{{ defaultModel ? ` (${defaultModel})` : '' }}</option>
          <option v-for="m in modelOptions" :key="m" :value="m">{{ m }}</option>
        </select>
        <p class="meta-text">
          The LLM model to use for summarization. "Default" uses the model from your .env configuration.
        </p>
      </div>

      <div class="stack-1">
        <div class="settings-label-row">
          <label class="settings-label" for="summarization-system-prompt">System prompt</label>
          <UiButton variant="ghost" size="sm" @click="openPromptHistory('system')">History</UiButton>
        </div>
        <textarea
          id="summarization-system-prompt"
          v-model="summarizationForm.summarizationPrompt"
          rows="6"
          class="settings-textarea"
          placeholder="Enter a system prompt…"
        />
        <p class="meta-text">
          Sent as the system message to the LLM. Edit directly or clear to reset to the default.
        </p>
      </div>

      <div class="stack-1">
        <div class="settings-label-row">
          <label class="settings-label" for="summarization-user-prompt">User prompt prefix</label>
          <UiButton variant="ghost" size="sm" @click="openPromptHistory('user')">History</UiButton>
        </div>
        <textarea
          id="summarization-user-prompt"
          v-model="summarizationForm.summarizationUserPrompt"
          rows="4"
          class="settings-textarea"
          placeholder="Enter a user prompt prefix…"
        />
        <p class="meta-text">
          Prepended to the transcript in the user message. Edit directly or clear to reset to the default.
        </p>
      </div>

      <UiAlert tone="info">
        Changes to prompts only apply to future summarizations. Existing summaries are not affected.
      </UiAlert>

      <div class="surface-row">
        <UiButton :disabled="isSavingSummarization" @click="saveSummarizationSettings">
          {{ isSavingSummarization ? "Saving..." : "Save summarization settings" }}
        </UiButton>
      </div>

      <hr class="settings-divider" />

      <div class="stack-2">
        <h4 class="settings-section-subtitle">Re-summarize existing summaries</h4>
        <p class="meta-text">
          Regenerate summaries using the current model and prompts. Use filters to target
          summaries generated by a specific model or before a certain date.
        </p>
      </div>

      <div class="surface-grid surface-grid--2">
        <div class="stack-1">
          <label class="settings-label" for="resummarize-model">Generated by model</label>
          <select
            id="resummarize-model"
            v-model="resummarizeModelFilter"
            class="ui-select"
            :disabled="!summarizationForm.summarizationEnabled"
          >
            <option value="">All models</option>
            <option v-for="m in usedModels" :key="m" :value="m">{{ m }}</option>
          </select>
        </div>

        <div class="stack-1">
          <label class="settings-label" for="resummarize-before">Generated before</label>
          <input
            id="resummarize-before"
            v-model="resummarizeBeforeDate"
            type="date"
            class="ui-input"
            :disabled="!summarizationForm.summarizationEnabled"
          />
        </div>
      </div>

      <div class="surface-row resummarize-actions">
        <UiButton
          :disabled="!summarizationForm.summarizationEnabled"
          variant="secondary"
          @click="resummarizeDryRun"
        >
          Preview
        </UiButton>
        <UiButton
          :disabled="!summarizationForm.summarizationEnabled || isResummarizing"
          @click="startResummarize"
        >
          {{ isResummarizing ? "Starting..." : "Re-summarize" }}
        </UiButton>
        <p v-if="resummarizeDryRunCount !== null" class="meta-text">
          {{ resummarizeDryRunCount }} {{ resummarizeDryRunCount === 1 ? 'summary' : 'summaries' }} will be regenerated.
        </p>
      </div>
    </UiCard>

    <!-- Model Settings -->
    <UiCard v-if="!isLoading" padding="lg" class="stack-4">
      <div class="stack-2">
        <h3 class="settings-section-title">Model Settings</h3>
        <p class="section-subtitle">
          Configure how the LLM is used during batch summarization jobs.
        </p>
      </div>

      <div class="surface-grid surface-grid--3">
        <div class="stack-1">
          <label class="settings-label" for="settings-llm-concurrency">Concurrency</label>
          <select
            id="settings-llm-concurrency"
            v-model.number="modelSettingsForm.llmConcurrency"
            class="ui-select"
          >
            <option v-for="n in 10" :key="n" :value="n">{{ n }}</option>
          </select>
          <p class="meta-text">
            Number of parallel requests sent to the LLM provider during backfill and re-summarize jobs. Higher values speed up batch processing but may hit rate limits.
          </p>
        </div>
      </div>

      <div class="surface-row">
        <UiButton :disabled="isSavingModelSettings" @click="saveModelSettings">
          {{ isSavingModelSettings ? "Saving..." : "Save model settings" }}
        </UiButton>
      </div>
    </UiCard>

    <!-- Obsidian Integration -->
    <UiCard v-if="!isLoading" padding="lg" class="stack-4">
      <div class="stack-2">
        <h3 class="settings-section-title">Obsidian</h3>
        <p class="section-subtitle">
          Choose where episode summaries and transcripts are sent in Obsidian.
        </p>
      </div>

      <div class="surface-grid surface-grid--2">
        <UiInput
          v-model="obsidianForm.obsidianVault"
          type="text"
          label="Vault"
          :placeholder="DEFAULT_OBSIDIAN_VAULT"
          hint="Use the exact Obsidian vault name."
        />
        <UiInput
          v-model="obsidianForm.obsidianFolder"
          type="text"
          label="Folder"
          :placeholder="DEFAULT_OBSIDIAN_FOLDER"
          hint="Use a vault-relative folder such as Clippings or Podcasts/Summaries."
        />
      </div>

      <div class="surface-row">
        <UiButton :disabled="isSavingObsidian" @click="saveObsidianSettings">
          {{ isSavingObsidian ? "Saving..." : "Save Obsidian settings" }}
        </UiButton>
      </div>
    </UiCard>

    <!-- Briefpoint Integration -->
    <UiCard v-if="!isLoading" padding="lg" class="stack-4">
      <div class="stack-2">
        <h3 class="settings-section-title">Briefpoint Integration</h3>
        <p class="section-subtitle">
          Send completed episodes (transcripts and summaries) to Briefpoint for scoring and surfacing.
          Enable per-podcast sync from the dashboard.
        </p>
      </div>

      <label class="settings-checkbox-row">
        <input
          v-model="briefpointForm.briefpointEnabled"
          type="checkbox"
          class="settings-checkbox"
        />
        <div>
          <p class="settings-checkbox-title">Enable Briefpoint integration</p>
          <p class="meta-text">
            When enabled, podcasts with Briefpoint toggled on will send their episodes to the Briefpoint ingest API.
          </p>
        </div>
      </label>

      <div class="surface-grid surface-grid--2">
        <UiInput
          v-model="briefpointForm.briefpointServerURL"
          type="text"
          :disabled="!briefpointForm.briefpointEnabled"
          label="Server URL"
          placeholder="http://localhost:12314"
        />

        <UiInput
          v-model="briefpointForm.briefpointAPIKey"
          type="password"
          :disabled="!briefpointForm.briefpointEnabled"
          label="API Key"
          :placeholder="briefpointAPIKeyConfigured ? 'Saved key' : 'sk_...'"
        />
      </div>

      <UiAlert tone="info">
        The API key is shown once when you register Briefcast as an ingest client on Briefpoint.
        Store it securely. If lost, rotate the key from the Briefpoint admin UI.
      </UiAlert>

      <div class="surface-row">
        <UiButton :disabled="isSavingBriefpoint" @click="saveBriefpointSettings">
          {{ isSavingBriefpoint ? "Saving..." : "Save Briefpoint settings" }}
        </UiButton>
      </div>
      </UiCard>
    </template>

    <template v-else-if="activeTab === 'work'">
      <UiCard padding="lg" class="stack-4">
        <div class="surface-row surface-row--between">
          <div class="stack-2">
            <h3 class="settings-section-title">Work Queue</h3>
            <p class="section-subtitle">
              Transcript and summary status across completed, queued, failed, and retrying work.
            </p>
          </div>
          <div class="work-queue-actions">
            <UiButton variant="secondary" :disabled="isLoadingWorkQueue" @click="loadWorkQueue()">
              {{ isLoadingWorkQueue ? "Refreshing..." : "Refresh" }}
            </UiButton>
            <UiButton :disabled="isStartingRepairWork || workQueueRunning" @click="startRepairWork">
              {{ isStartingRepairWork || workQueueRunning ? "Repairing..." : "Repair failed/missing work" }}
            </UiButton>
          </div>
        </div>

        <UiAlert v-if="workQueueErrorMessage" tone="danger">
          {{ workQueueErrorMessage }}
        </UiAlert>

        <UiAlert v-if="workQueueRunning" tone="info">
          Repair is running. This page will refresh while the job is active.
        </UiAlert>

        <div v-if="isLoadingWorkQueue" class="log-loading stack-2">
          <span class="skeleton settings-skeleton-line settings-skeleton-line--title"></span>
          <span class="skeleton settings-skeleton-line"></span>
          <span class="skeleton settings-skeleton-line settings-skeleton-line--short"></span>
        </div>

        <template v-else-if="workQueue">
          <div class="work-queue-grid">
            <section class="work-queue-panel stack-3" aria-labelledby="summary-work-heading">
              <div>
                <h4 id="summary-work-heading" class="settings-section-subtitle">Summaries</h4>
                <p class="meta-text">Generated from canonical transcripts.</p>
              </div>
              <div class="work-count-grid">
                <div class="work-count-item">
                  <span class="meta-text">Complete</span>
                  <strong>{{ workQueue.summary.complete }}</strong>
                </div>
                <div class="work-count-item">
                  <span class="meta-text">Processing</span>
                  <strong>{{ workQueue.summary.processing }}</strong>
                </div>
                <div class="work-count-item">
                  <span class="meta-text">Missing</span>
                  <strong>{{ workQueue.summary.missing }}</strong>
                </div>
                <div class="work-count-item">
                  <span class="meta-text">Failed</span>
                  <strong>{{ workQueue.summary.failed }}</strong>
                </div>
                <div class="work-count-item">
                  <span class="meta-text">Repair eligible</span>
                  <strong>{{ workQueue.summary.eligibleForBackfill }}</strong>
                </div>
                <div class="work-count-item">
                  <span class="meta-text">No transcript</span>
                  <strong>{{ workQueue.summary.blockedNoTranscript }}</strong>
                </div>
              </div>
            </section>

            <section class="work-queue-panel stack-3" aria-labelledby="transcript-work-heading">
              <div>
                <h4 id="transcript-work-heading" class="settings-section-subtitle">Transcripts</h4>
                <p class="meta-text">Created by the WhisperX worker after downloads finish.</p>
              </div>
              <div class="work-count-grid">
                <div class="work-count-item">
                  <span class="meta-text">Complete</span>
                  <strong>{{ workQueue.transcripts.complete }}</strong>
                </div>
                <div class="work-count-item">
                  <span class="meta-text">Queued</span>
                  <strong>{{ workQueue.transcripts.queued }}</strong>
                </div>
                <div class="work-count-item">
                  <span class="meta-text">Processing</span>
                  <strong>{{ workQueue.transcripts.processing }}</strong>
                </div>
                <div class="work-count-item">
                  <span class="meta-text">Failed</span>
                  <strong>{{ workQueue.transcripts.failed }}</strong>
                </div>
                <div class="work-count-item">
                  <span class="meta-text">Retry due</span>
                  <strong>{{ workQueue.transcripts.retryDue }}</strong>
                </div>
                <div class="work-count-item">
                  <span class="meta-text">Retry scheduled</span>
                  <strong>{{ workQueue.transcripts.retryScheduled }}</strong>
                </div>
                <div class="work-count-item">
                  <span class="meta-text">Blocked</span>
                  <strong>{{ workQueue.transcripts.blocked }}</strong>
                </div>
              </div>
            </section>
          </div>

          <div class="work-config-alerts">
            <UiAlert v-if="!workQueue.config.whisperxEnabled" tone="warning">
              WhisperX is disabled. Transcript repair can queue work, but it cannot run transcriptions.
            </UiAlert>
            <UiAlert v-if="!workQueue.config.llmEnabled || !workQueue.config.llmApiKeyConfigured || !workQueue.config.summarizationEnabled" tone="warning">
              Summary repair requires summarization enabled, LLM enabled, and an LLM API key.
            </UiAlert>
          </div>

          <section v-if="lastRepairRun" class="work-queue-panel stack-2" aria-labelledby="last-repair-heading">
            <h4 id="last-repair-heading" class="settings-section-subtitle">Last Repair</h4>
            <p class="meta-text">
              Started {{ formatOptionalTimestamp(lastRepairRun.startedAt) }}<template v-if="lastRepairRun.finishedAt">, finished {{ formatOptionalTimestamp(lastRepairRun.finishedAt) }}</template>.
            </p>
            <div class="work-repair-summary">
              <span>Summaries: {{ lastRepairRun.summary.succeeded }} repaired, {{ lastRepairRun.summary.failed }} failed, {{ lastRepairRun.summary.eligible }} eligible.</span>
              <span>Transcripts: {{ lastRepairRun.transcripts.forcedDue }} retries queued, {{ lastRepairRun.transcripts.queued }} ready now.</span>
            </div>
            <UiAlert v-if="lastRepairRun.error" tone="warning">
              {{ lastRepairRun.error }}
            </UiAlert>
          </section>

          <section class="stack-2" aria-labelledby="work-items-heading">
            <div class="surface-row surface-row--between">
              <h4 id="work-items-heading" class="settings-section-subtitle">Active And Attention Items</h4>
              <p class="meta-text">Showing up to {{ workQueue.limit }} items.</p>
            </div>

            <div v-if="workQueueItems.length" class="work-item-list">
              <article v-for="item in workQueueItems" :key="`${item.kind}-${item.id}`" class="work-item">
                <div class="work-item-header">
                  <div class="log-entry-badges">
                    <UiBadge :tone="item.kind === 'summary' ? 'info' : 'neutral'">{{ item.kind }}</UiBadge>
                    <UiBadge :tone="workQueueTone(item)">{{ item.statusLabel }}</UiBadge>
                  </div>
                  <time class="log-entry-time">{{ formatOptionalTimestamp(item.updatedAt) }}</time>
                </div>
                <p class="log-entry-message">{{ item.title }}</p>
                <p class="log-entry-meta">
                  {{ item.podcastTitle || "Unknown podcast" }}
                  <template v-if="item.model"> | {{ item.model }}</template>
                </p>
                <div class="work-item-meta">
                  <span v-if="item.progressPct">Progress {{ item.progressPct }}%</span>
                  <span v-if="item.progressStage">{{ item.progressStage.replace(/_/g, " ") }}</span>
                  <span v-if="item.retryCount">Retried {{ item.retryCount }} {{ item.retryCount === 1 ? "time" : "times" }}</span>
                  <span v-if="item.nextAttempt">Next attempt {{ formatOptionalTimestamp(item.nextAttempt) }}</span>
                </div>
                <p v-if="item.lastError" class="work-item-error">{{ item.lastError }}</p>
              </article>
            </div>
            <div v-else class="empty-state">
              No active, failed, retrying, or blocked transcript/summary work was found.
            </div>
          </section>
        </template>
      </UiCard>
    </template>

    <template v-else>
      <UiCard padding="lg" class="stack-4">
        <div class="surface-row surface-row--between">
          <div class="stack-2">
            <h3 class="settings-section-title">Latest Logs</h3>
            <p class="section-subtitle">
              Recent application, job, download, summary, and transcript logs.
            </p>
          </div>
          <UiButton variant="secondary" :disabled="isLoadingLogs" @click="loadLogs">
            {{ isLoadingLogs ? "Refreshing..." : "Refresh" }}
          </UiButton>
        </div>

        <UiAlert v-if="logsErrorMessage" tone="danger">
          {{ logsErrorMessage }}
        </UiAlert>

        <div v-if="isLoadingLogs" class="log-loading stack-2">
          <span class="skeleton settings-skeleton-line settings-skeleton-line--title"></span>
          <span class="skeleton settings-skeleton-line"></span>
          <span class="skeleton settings-skeleton-line settings-skeleton-line--short"></span>
        </div>

        <template v-else>
          <div v-if="logsPayload" class="log-summary-grid">
            <div class="log-summary-item">
              <span class="meta-text">User-impacting</span>
              <strong>{{ impactLogEntries.length }}</strong>
            </div>
            <div class="log-summary-item">
              <span class="meta-text">Entries shown</span>
              <strong>{{ logEntries.length }}</strong>
            </div>
            <div class="log-summary-item">
              <span class="meta-text">Sources</span>
              <strong>{{ logSources.length }}</strong>
            </div>
          </div>

          <section v-if="impactLogEntries.length" class="stack-2" aria-labelledby="impact-log-heading">
            <h4 id="impact-log-heading" class="settings-section-subtitle">Needs Attention</h4>
            <div class="log-list log-list--compact">
              <article
                v-for="entry in impactLogEntries"
                :key="`impact-${entry.id}`"
                class="log-entry log-entry--impact"
              >
                <div class="log-entry-header">
                  <div class="log-entry-badges">
                    <UiBadge :tone="logLevelTone(entry.level)">{{ entry.level.toUpperCase() }}</UiBadge>
                    <UiBadge tone="warning">{{ entry.category }}</UiBadge>
                  </div>
                  <time class="log-entry-time">{{ formatLogTimestamp(entry.timestamp) }}</time>
                </div>
                <p class="log-entry-message">{{ entry.humanMessage }}</p>
                <p class="log-entry-meta">
                  {{ entry.source }}<template v-if="entry.caller"> | {{ entry.caller }}</template>
                </p>
              </article>
            </div>
          </section>

          <UiAlert v-else-if="logsPayload" tone="success">
            No user-impacting errors were found in the latest log entries.
          </UiAlert>

          <UiAlert v-if="logReadErrors.length" tone="warning">
            {{ logReadErrors.length }} log source{{ logReadErrors.length === 1 ? "" : "s" }} could not be read.
          </UiAlert>

          <div v-if="logSources.length" class="log-source-list">
            <span v-for="source in logSources" :key="source.name" class="log-source-chip">
              {{ source.name }} ({{ formatBytes(source.sizeBytes) }})
            </span>
          </div>

          <section class="stack-2" aria-labelledby="all-log-heading">
            <div class="surface-row surface-row--between">
              <h4 id="all-log-heading" class="settings-section-subtitle">All Recent Entries</h4>
              <p v-if="logsPayload && logsPayload.totalDiscovered > logEntries.length" class="meta-text">
                Showing {{ logEntries.length }} of {{ logsPayload.totalDiscovered }} discovered entries.
              </p>
            </div>

            <div v-if="logEntries.length" class="log-list">
              <article
                v-for="entry in logEntries"
                :key="entry.id"
                class="log-entry"
                :class="{ 'log-entry--impact': entry.userImpact }"
              >
                <div class="log-entry-header">
                  <div class="log-entry-badges">
                    <UiBadge :tone="logLevelTone(entry.level)">{{ entry.level.toUpperCase() }}</UiBadge>
                    <UiBadge :tone="entry.userImpact ? 'warning' : 'neutral'">{{ entry.category }}</UiBadge>
                  </div>
                  <time class="log-entry-time">{{ formatLogTimestamp(entry.timestamp) }}</time>
                </div>
                <p class="log-entry-message">{{ entry.humanMessage }}</p>
                <p class="log-entry-meta">
                  {{ entry.source }}<template v-if="entry.service"> | {{ entry.service }}</template><template v-if="entry.caller"> | {{ entry.caller }}</template>
                </p>
                <details v-if="formatLogFields(entry.fields) || entry.raw" class="log-entry-details">
                  <summary>Details</summary>
                  <pre v-if="formatLogFields(entry.fields)" class="log-entry-pre">{{ formatLogFields(entry.fields) }}</pre>
                  <pre v-if="entry.raw" class="log-entry-pre">{{ entry.raw }}</pre>
                </details>
              </article>
            </div>
            <div v-else class="empty-state">
              No file-backed logs were found.
            </div>
          </section>
        </template>
      </UiCard>
    </template>
  </section>

  <UiDrawer
    :open="promptHistoryOpen"
    :title="`${promptHistoryType === 'system' ? 'System' : 'User'} prompt history`"
    description="Previous versions saved when you changed the prompt."
    @close="promptHistoryOpen = false"
  >
    <p v-if="promptHistoryLoading" class="meta-text">Loading…</p>
    <p v-else-if="promptHistoryVersions.length === 0" class="meta-text">No previous versions yet.</p>
    <div v-else class="prompt-version-list">
      <div v-for="v in promptHistoryVersions" :key="v.ID" class="prompt-version-item">
        <div class="prompt-version-header">
          <span class="prompt-version-date">{{ formatDate(v.CreatedAt) }}</span>
          <UiButton variant="ghost" size="sm" @click="restorePromptVersion(v.ID)">Restore</UiButton>
        </div>
        <pre class="prompt-version-content">{{ v.Content }}</pre>
      </div>
    </div>
  </UiDrawer>
</template>

<style scoped>
.settings-section-title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-section-size);
  line-height: var(--font-section-line-height);
  font-weight: var(--font-section-weight);
}

.settings-tabs {
  display: inline-flex;
  align-items: center;
  width: fit-content;
  max-width: 100%;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-3);
  background: var(--color-bg-primary);
  padding: var(--space-1);
  overflow-x: auto;
}

.settings-tab {
  min-height: 40px;
  border: 0;
  border-radius: var(--radius-2);
  background: transparent;
  color: var(--color-text-secondary);
  font-size: var(--font-button-size);
  font-weight: var(--font-button-weight);
  padding: var(--space-2) var(--space-4);
  white-space: nowrap;
  cursor: pointer;
}

.settings-tab:hover {
  background: var(--color-hover);
  color: var(--color-text-primary);
}

.settings-tab--active {
  background: var(--color-accent-subtle);
  color: var(--color-accent-hover);
}

.log-loading {
  border: 1px solid var(--color-border);
  border-radius: var(--radius-3);
  background: var(--color-bg-secondary);
  padding: var(--space-4);
}

.log-summary-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: var(--space-3);
}

.log-summary-item {
  display: grid;
  gap: var(--space-1);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
  padding: var(--space-3);
}

.log-summary-item strong {
  color: var(--color-text-primary);
  font-size: var(--font-section-size);
  line-height: var(--font-section-line-height);
}

.log-list {
  display: grid;
  gap: var(--space-3);
}

.log-list--compact {
  gap: var(--space-2);
}

.log-entry {
  min-width: 0;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-3);
  background: var(--color-bg-primary);
  padding: var(--space-4);
}

.log-entry--impact {
  border-color: rgba(203, 145, 47, 0.55);
  background: rgba(203, 145, 47, 0.08);
}

.log-entry-header {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}

.log-entry-badges {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
}

.log-entry-time {
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
}

.log-entry-message {
  margin: var(--space-3) 0 0;
  color: var(--color-text-primary);
  font-size: var(--font-body-size);
  line-height: var(--font-body-line-height);
  overflow-wrap: anywhere;
}

.log-entry-meta {
  margin: var(--space-2) 0 0;
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
  overflow-wrap: anywhere;
}

.log-entry-details {
  margin-top: var(--space-3);
}

.log-entry-details summary {
  color: var(--color-accent);
  cursor: pointer;
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
}

.log-entry-pre {
  max-height: 260px;
  overflow: auto;
  margin: var(--space-2) 0 0;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
  color: var(--color-text-primary);
  padding: var(--space-3);
  font-family: ui-monospace, SFMono-Regular, Consolas, "Liberation Mono", monospace;
  font-size: 12px;
  line-height: 18px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.log-source-list {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.log-source-chip {
  max-width: 100%;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
  color: var(--color-text-secondary);
  padding: var(--space-1) var(--space-2);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
  overflow-wrap: anywhere;
}

.work-queue-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: var(--space-2);
}

.work-queue-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: var(--space-4);
}

.work-queue-panel {
  min-width: 0;
  border-top: 1px solid var(--color-border);
  padding-top: var(--space-4);
}

.work-count-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-2);
}

.work-count-item {
  display: grid;
  gap: var(--space-1);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
  padding: var(--space-3);
}

.work-count-item strong {
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
}

.work-config-alerts {
  display: grid;
  gap: var(--space-2);
}

.work-repair-summary {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-4);
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
}

.work-item-list {
  display: grid;
  gap: var(--space-3);
}

.work-item {
  min-width: 0;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-3);
  background: var(--color-bg-primary);
  padding: var(--space-4);
}

.work-item-header {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}

.work-item-meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-3);
  margin-top: var(--space-2);
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
}

.work-item-error {
  margin: var(--space-3) 0 0;
  color: var(--color-danger);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
  overflow-wrap: anywhere;
}

.settings-checkbox-row {
  display: flex;
  align-items: center;
  min-height: 44px;
  gap: var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
  padding: var(--space-3);
}

.settings-checkbox {
  margin: 0;
  width: 24px;
  height: 24px;
  flex: 0 0 24px;
  accent-color: var(--color-accent);
}

.settings-checkbox-title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
}

.settings-label {
  display: block;
  color: var(--color-text-primary);
  font-size: var(--font-body-size);
  font-weight: 500;
}

.settings-textarea {
  width: 100%;
  min-height: 100px;
  padding: var(--space-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
  color: var(--color-text-primary);
  font-family: inherit;
  font-size: var(--font-body-size);
  line-height: var(--font-body-line-height);
  resize: vertical;
}

.settings-textarea:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.settings-skeleton-line {
  height: 14px;
}

.settings-skeleton-line--title {
  width: 48%;
  height: 20px;
}

.settings-skeleton-line--short {
  width: 60%;
}

.settings-divider {
  border: none;
  border-top: 1px solid var(--color-border);
  margin: 0;
}

.settings-section-subtitle {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
}

.resummarize-actions {
  flex-wrap: wrap;
  align-items: center;
}

.settings-label-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.prompt-version-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-3, 0.75rem);
}

.prompt-version-item {
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md, 0.5rem);
  padding: var(--spacing-3, 0.75rem);
}

.prompt-version-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--spacing-2, 0.5rem);
}

.prompt-version-date {
  font-size: var(--font-sm-size, 0.875rem);
  color: var(--color-text-secondary);
}

.prompt-version-content {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: var(--font-sm-size, 0.875rem);
  color: var(--color-text-primary);
  max-height: 12rem;
  overflow-y: auto;
}

@media (min-width: 768px) {
  .log-summary-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .work-queue-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 520px) {
  .settings-tabs {
    width: 100%;
  }

  .settings-tab {
    flex: 1 1 0;
    padding-inline: var(--space-3);
  }
}
</style>
