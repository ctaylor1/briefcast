<script setup lang="ts">
import AboutView from "./AboutView.vue";
import { toast } from "../lib/toast";
import { computed, nextTick, onMounted, onUnmounted, ref } from "vue";
import { useRoute } from "vue-router";
import UiAlert from "../components/ui/UiAlert.vue";
import UiBadge from "../components/ui/UiBadge.vue";
import UiButton from "../components/ui/UiButton.vue";
import UiCard from "../components/ui/UiCard.vue";
import UiDrawer from "../components/ui/UiDrawer.vue";
import UiInput from "../components/ui/UiInput.vue";
import { useStatusMessage } from "../composables/useStatusMessage";
import { type ThemeMode, useTheme } from "../composables/useTheme";
import { getErrorMessage, settingsApi } from "../lib/api";
import { DEFAULT_OBSIDIAN_FOLDER, DEFAULT_OBSIDIAN_VAULT } from "../lib/obsidian";
import { availableModelOptions, formatPricePerMillion, formatTokenCapacity } from "../lib/openrouterModels";
import { userAgentSourceLabel } from "../lib/userAgent";
import type { AppSettings, OpenRouterModelsResponse, PromptVersion, SummaryJobStatus } from "../types/api";


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

const route = useRoute();
const isLoading = ref(true);
const loadFailed = ref(false);
const settingsReady = computed(() => !isLoading.value && !loadFailed.value);
const isSavingBackCatalog = ref(false);
const isSavingRetention = ref(false);
const isSavingSummarization = ref(false);
const isSavingModelSettings = ref(false);
const isSavingObsidian = ref(false);
const isSavingBriefpoint = ref(false);
const briefpointAPIKeyConfigured = ref(false);
const outboundUserAgent = ref("");
const outboundUserAgentSource = ref("");
const defaultModel = ref("");
const defaultProvider = ref("");
const openRouterCatalog = ref<OpenRouterModelsResponse | null>(null);
const isLoadingOpenRouterModels = ref(false);
const openRouterModelsErrorMessage = ref("");
const defaultSystemPrompt = ref("");
const defaultUserPrompt = ref("");
const usedModels = ref<string[]>([]);
const isResummarizing = ref(false);
const resummarizeModelFilter = ref("");
const resummarizeBeforeDate = ref("");
const resummarizeDryRunCount = ref<number | null>(null);
const summaryJobStatus = ref<SummaryJobStatus | null>(null);
const summaryJobErrorMessage = ref("");
const promptHistoryOpen = ref(false);
const promptHistoryType = ref<"system" | "user">("system");
const promptHistoryVersions = ref<PromptVersion[]>([]);
const promptHistoryLoading = ref(false);
const {
  errorMessage,
  clearAll,
  clearError,
  setError,
} = useStatusMessage(5000);
let summaryJobPollTimer: number | undefined;

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
  return availableModelOptions(
    defaultModel.value,
    summarizationForm.value.summarizationModel,
    usedModels.value,
    openRouterCatalog.value?.models.map((model) => model.id) ?? [],
  );
});
const openRouterModels = computed(() => openRouterCatalog.value?.models ?? []);
const isOpenRouterProvider = computed(() => defaultProvider.value.toLowerCase() === "openrouter");
const providerLabel = computed(() => isOpenRouterProvider.value ? "OpenRouter" : defaultProvider.value || "Unknown");

const retentionEnabled = computed(() => !retentionForm.value.keepAllEpisodes);
const usesInitialDownloadCount = computed(() => backCatalogForm.value.initialDownloadMode === "count");
const usesInitialDownloadMonths = computed(() => backCatalogForm.value.initialDownloadMode === "months");

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
  loadFailed.value = false;
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
    outboundUserAgent.value = settings.userAgent ?? "";
    outboundUserAgentSource.value = settings.userAgentSource ?? "";
    defaultModel.value = settings.defaultModel ?? "";
    defaultProvider.value = settings.defaultProvider ?? "";
    defaultSystemPrompt.value = settings.defaultSystemPrompt ?? "";
    defaultUserPrompt.value = settings.defaultUserPrompt ?? "";
    if (isOpenRouterProvider.value) void loadOpenRouterModels();
    try {
      const { models } = await settingsApi.getSummaryModels();
      usedModels.value = models ?? [];
    } catch {
      usedModels.value = [];
    }
  } catch (error) {
    // Keep the forms hidden: saving them now would overwrite real settings with blanks.
    loadFailed.value = true;
    setError(getErrorMessage(error, "Failed to load settings."));
  } finally {
    isLoading.value = false;
  }
}

async function loadOpenRouterModels(refresh = false): Promise<void> {
  if (isLoadingOpenRouterModels.value) return;
  isLoadingOpenRouterModels.value = true;
  openRouterModelsErrorMessage.value = "";
  try {
    openRouterCatalog.value = await settingsApi.getOpenRouterModels(refresh);
  } catch (error) {
    openRouterModelsErrorMessage.value = getErrorMessage(error, "Could not load OpenRouter models. Try refreshing the list.");
  } finally {
    isLoadingOpenRouterModels.value = false;
  }
}

function clearSummaryJobPoll(): void {
  if (summaryJobPollTimer !== undefined) {
    window.clearTimeout(summaryJobPollTimer);
    summaryJobPollTimer = undefined;
  }
}

function scheduleSummaryJobPoll(): void {
  clearSummaryJobPoll();
  if (!summaryJobStatus.value?.running) return;
  summaryJobPollTimer = window.setTimeout(() => {
    void loadSummaryJobStatus();
  }, 5000);
}

async function loadSummaryJobStatus(): Promise<void> {
  try {
    summaryJobStatus.value = await settingsApi.getBackfillStatus();
    summaryJobErrorMessage.value = "";
    scheduleSummaryJobPoll();
  } catch (error) {
    summaryJobErrorMessage.value = getErrorMessage(error, "Failed to load summary job status.");
    clearSummaryJobPoll();
  }
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
    toast.success("Back catalog settings updated.");
  } catch (error) {
    toast.error(getErrorMessage(error, "Failed to update back catalog settings."));
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
    toast.success("Retention settings updated.");
  } catch (error) {
    toast.error(getErrorMessage(error, "Failed to update retention settings."));
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
    toast.success("Summarization settings updated.");
  } catch (error) {
    toast.error(getErrorMessage(error, "Failed to update summarization settings."));
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
    toast.success("Prompt restored successfully.");
  } catch (error) {
    toast.error(getErrorMessage(error, "Failed to restore prompt version."));
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
    toast.success("Model settings updated.");
  } catch (error) {
    toast.error(getErrorMessage(error, "Failed to update model settings."));
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
    toast.success("Obsidian settings updated.");
  } catch (error) {
    toast.error(getErrorMessage(error, "Failed to update Obsidian settings."));
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
    toast.success("Briefpoint settings updated.");
  } catch (error) {
    toast.error(getErrorMessage(error, "Failed to update Briefpoint settings."));
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

const themeOptions: Array<{ value: ThemeMode; label: string; description: string }> = [
  { value: "system", label: "System", description: "Follow this device's light or dark setting." },
  { value: "light", label: "Light", description: "Always light." },
  { value: "dark", label: "Dark", description: "Always dark." },
  { value: "schedule", label: "Schedule", description: "Light and dark at set hours." },
];

function formatHour(hour: number): string {
  if (hour === 0) return "12:00 AM";
  if (hour === 12) return "12:00 PM";
  if (hour < 12) return `${hour}:00 AM`;
  return `${hour - 12}:00 PM`;
}

async function resummarizeDryRun(): Promise<void> {
  clearAll();
  try {
    const filter: Record<string, unknown> = { dryRun: true };
    if (resummarizeModelFilter.value) filter.model = resummarizeModelFilter.value;
    if (resummarizeBeforeDate.value) filter.before = new Date(resummarizeBeforeDate.value).toISOString();
    const result = await settingsApi.previewResummarize(filter as import("../types/api").ResummarizeFilter);
    resummarizeDryRunCount.value = result.total;
  } catch (error) {
    toast.error(getErrorMessage(error, "Failed to preview re-summarize."));
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
    const result = await settingsApi.startResummarize(filter as import("../types/api").ResummarizeFilter);
    summaryJobStatus.value = result.status;
    summaryJobErrorMessage.value = "";
    scheduleSummaryJobPoll();
    toast.success("Re-summarize job started. Summaries will be regenerated in the background.");
  } catch (error) {
    toast.error(getErrorMessage(error, "Failed to start re-summarize."));
  } finally {
    isResummarizing.value = false;
  }
}

onMounted(async () => {
  void loadSummaryJobStatus();
  await loadSettings();
  // /about and the palette's About entry land here.
  if (route.query.section === "about") {
    await nextTick();
    document.getElementById("settings-about")?.scrollIntoView({ block: "start" });
  }
});
onUnmounted(() => {
  clearSummaryJobPoll();
});
</script>

<template>
  <section class="settings-page stack-4">
    <header class="page-header">
      <h2 class="section-title">Settings</h2>
      <p class="section-subtitle">
        Configure retention, summarization, and other application behavior.
      </p>
    </header>

    <UiAlert v-if="errorMessage" tone="danger">
      {{ errorMessage }}
    </UiAlert>

      <UiCard v-if="isLoading" padding="lg" class="stack-2">
        <span class="skeleton settings-skeleton-line settings-skeleton-line--title"></span>
        <span class="skeleton settings-skeleton-line"></span>
        <span class="skeleton settings-skeleton-line"></span>
        <span class="skeleton settings-skeleton-line settings-skeleton-line--short"></span>
      </UiCard>

      <UiCard v-if="!isLoading && loadFailed" padding="lg" class="stack-3 settings-load-failed">
        <p>Settings could not be loaded, so they are hidden to avoid saving blanks over them.</p>
        <div>
          <UiButton variant="secondary" size="sm" @click="loadSettings">Try again</UiButton>
        </div>
      </UiCard>

      <!-- Appearance settings -->
      <UiCard v-if="settingsReady" padding="lg" class="stack-4">
        <div class="stack-2">
          <h3 class="settings-section-title">Appearance</h3>
          <p class="section-subtitle">Changes apply and save immediately.</p>
        </div>

        <fieldset class="settings-theme-modes">
          <legend class="settings-label">Theme</legend>
          <label v-for="option in themeOptions" :key="option.value" class="settings-theme-mode">
            <input
              type="radio"
              name="settings-theme-mode"
              :value="option.value"
              :checked="themeMode === option.value"
              @change="setThemeMode(option.value)"
            />
            <span>
              <span class="settings-theme-mode__title">{{ option.label }}</span>
              <span class="meta-text">{{ option.description }}</span>
            </span>
          </label>
        </fieldset>

        <div v-if="themeMode === 'schedule'" class="surface-grid surface-grid--3">
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
          <div class="stack-1">
            <label class="settings-label" for="settings-light-start">Light from</label>
            <select
              id="settings-light-start"
              class="ui-select"
              :value="lightStartHour"
              @change="setLightStartHour(Number(($event.target as HTMLSelectElement).value))"
            >
              <option v-for="h in 24" :key="h - 1" :value="h - 1">{{ formatHour(h - 1) }}</option>
            </select>
          </div>
          <div class="stack-1">
            <label class="settings-label" for="settings-dark-start">Dark from</label>
            <select
              id="settings-dark-start"
              class="ui-select"
              :value="darkStartHour"
              @change="setDarkStartHour(Number(($event.target as HTMLSelectElement).value))"
            >
              <option v-for="h in 24" :key="h - 1" :value="h - 1">{{ formatHour(h - 1) }}</option>
            </select>
          </div>
        </div>
      </UiCard>

    <!-- Back catalog download settings -->
    <UiCard v-if="settingsReady" padding="lg" class="stack-4">
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
    <UiCard v-if="settingsReady" padding="lg" class="stack-4">
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
    <UiCard v-if="settingsReady" padding="lg" class="stack-4">
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
          <optgroup v-if="openRouterModels.length" label="Recommended on OpenRouter">
            <option v-for="model in openRouterModels" :key="model.id" :value="model.id">
              {{ model.name }} ({{ model.id }})
            </option>
          </optgroup>
          <optgroup v-if="modelOptions.length" label="Configured and previously used">
            <option v-for="m in modelOptions" :key="m" :value="m">{{ m }}</option>
          </optgroup>
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
          :disabled="!summarizationForm.summarizationEnabled || isResummarizing || summaryJobStatus?.running"
          @click="startResummarize"
        >
          {{ isResummarizing ? "Starting..." : "Re-summarize" }}
        </UiButton>
        <p v-if="resummarizeDryRunCount !== null" class="meta-text">
          {{ resummarizeDryRunCount }} {{ resummarizeDryRunCount === 1 ? 'summary' : 'summaries' }} will be regenerated.
        </p>
      </div>
      <UiAlert v-if="summaryJobErrorMessage" tone="warning">{{ summaryJobErrorMessage }}</UiAlert>
      <UiAlert v-else-if="summaryJobStatus?.error" tone="warning">{{ summaryJobStatus.error }}</UiAlert>
      <p v-else-if="summaryJobStatus?.running" class="meta-text">
        Summary job running: {{ summaryJobStatus.completed }} of {{ summaryJobStatus.total }} processed,
        {{ summaryJobStatus.failed }} failed.
      </p>
      <p v-else-if="summaryJobStatus && summaryJobStatus.total > 0" class="meta-text">
        Last summary job: {{ summaryJobStatus.completed }} of {{ summaryJobStatus.total }} processed,
        {{ summaryJobStatus.failed }} failed.
      </p>
    </UiCard>

    <!-- Model Settings -->
    <UiCard v-if="settingsReady" padding="lg" class="stack-4">
      <div class="stack-2">
        <h3 class="settings-section-title">Model Settings</h3>
        <p class="section-subtitle">
          Configure how the LLM is used during batch summarization jobs.
        </p>
      </div>

      <div class="stack-2">
        <div class="surface-row surface-row--between">
          <div class="stack-1">
            <h4 class="settings-section-subtitle">Model provider</h4>
            <p class="meta-text">Configured by LLM_PROVIDER in the server environment.</p>
          </div>
          <UiBadge :tone="isOpenRouterProvider ? 'info' : 'neutral'">{{ providerLabel }}</UiBadge>
        </div>
        <UiAlert v-if="isOpenRouterProvider && openRouterCatalog && !openRouterCatalog.configured" tone="warning">
          OpenRouter is selected, but no LLM API key is configured. Summaries need an API key.
        </UiAlert>
      </div>

      <div v-if="isOpenRouterProvider" class="stack-2">
        <div class="surface-row surface-row--between">
          <div class="stack-1">
            <h4 class="settings-section-subtitle">Recommended OpenRouter models</h4>
            <p class="meta-text">Models suited to long podcast transcripts. Availability and prices come from OpenRouter.</p>
          </div>
          <UiButton
            variant="secondary"
            size="sm"
            :disabled="isLoadingOpenRouterModels"
            @click="loadOpenRouterModels(true)"
          >
            {{ isLoadingOpenRouterModels ? "Refreshing..." : "Refresh list" }}
          </UiButton>
        </div>
        <p v-if="openRouterCatalog?.fetchedAt" class="meta-text">
          Updated {{ formatDate(openRouterCatalog.fetchedAt) }}. Prices are estimates and may change.
        </p>
        <UiAlert v-if="openRouterModelsErrorMessage" tone="warning">{{ openRouterModelsErrorMessage }}</UiAlert>
        <UiAlert v-if="openRouterCatalog?.stale" tone="warning">
          OpenRouter could not be reached. Showing the last available model list.
        </UiAlert>
        <p v-if="isLoadingOpenRouterModels && !openRouterCatalog" class="meta-text" role="status">Loading models from OpenRouter...</p>
        <div v-else-if="openRouterModels.length" class="recommended-model-list">
          <div v-for="model in openRouterModels" :key="model.id" class="recommended-model">
            <div class="recommended-model__heading">
              <div class="stack-1">
                <strong>{{ model.name }}</strong>
                <code class="recommended-model__id">{{ model.id }}</code>
              </div>
              <UiButton
                variant="outline"
                size="sm"
                :disabled="!summarizationForm.summarizationEnabled || summarizationForm.summarizationModel === model.id"
                @click="summarizationForm.summarizationModel = model.id"
              >
                {{ summarizationForm.summarizationModel === model.id ? "Selected" : "Use model" }}
              </UiButton>
            </div>
            <p v-if="model.recommendation" class="meta-text">{{ model.recommendation }}</p>
            <div class="recommended-model__facts meta-text">
              <span>Context: {{ formatTokenCapacity(model.contextLength) }}</span>
              <span v-if="model.maxCompletionTokens > 0">Max output: {{ formatTokenCapacity(model.maxCompletionTokens) }}</span>
              <span>Input: {{ formatPricePerMillion(model.promptPrice) }}</span>
              <span>Output: {{ formatPricePerMillion(model.completionPrice) }}</span>
            </div>
          </div>
        </div>
        <p v-else-if="!isLoadingOpenRouterModels && !openRouterModelsErrorMessage" class="meta-text">
          No recommended models are currently available from OpenRouter.
        </p>
        <p v-if="openRouterModels.length" class="meta-text">
          Choose a model here, then save summarization settings above to apply it to future summaries.
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
    <UiCard v-if="settingsReady" padding="lg" class="stack-4">
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

    <!-- Network identity (read-only; configured in .env) -->
    <UiCard v-if="!isLoading && outboundUserAgent" padding="lg" class="stack-4">
      <div class="stack-2">
        <h3 class="settings-section-title">Network Identity</h3>
        <p class="section-subtitle">
          The User-Agent Briefcast sends when it fetches feeds, audio, artwork and charts.
          Podcast hosts use it to count downloads by app.
        </p>
      </div>
      <div class="stack-1">
        <span class="settings-label">User-Agent</span>
        <code class="settings-user-agent">{{ outboundUserAgent }}</code>
        <p class="meta-text">{{ userAgentSourceLabel(outboundUserAgentSource) }}</p>
      </div>
    </UiCard>

    <!-- Briefpoint Integration -->
    <UiCard v-if="settingsReady" padding="lg" class="stack-4">
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

    <!-- About (moved here from its own page). -->
    <section id="settings-about" class="stack-3">
      <AboutView />
    </section>
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

.settings-user-agent {
  display: block;
  padding: 0.5rem 0.75rem;
  border-radius: 0.5rem;
  background: var(--color-bg-secondary);
  color: var(--color-text-primary);
  font-size: 0.875rem;
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

.recommended-model-list {
  display: grid;
  gap: var(--space-3);
}

.recommended-model {
  display: grid;
  gap: var(--space-2);
  min-width: 0;
  padding: var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
}

.recommended-model__heading,
.recommended-model__facts {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  gap: var(--space-2) var(--space-4);
}

.recommended-model__heading {
  justify-content: space-between;
}

.recommended-model__id {
  color: var(--color-text-secondary);
  overflow-wrap: anywhere;
}

.prompt-version-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.prompt-version-item {
  border: 1px solid var(--color-border);
  border-radius: var(--radius-3);
  padding: var(--space-3);
}

.prompt-version-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-2);
}

.prompt-version-date {
  font-size: var(--font-body-size);
  color: var(--color-text-secondary);
}

.prompt-version-content {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: var(--font-body-size);
  color: var(--color-text-primary);
  max-height: 12rem;
  overflow-y: auto;
}


.settings-theme-modes {
  display: grid;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  border: 0;
}

.settings-theme-modes legend {
  margin-bottom: var(--space-2);
}

.settings-theme-mode {
  display: flex;
  align-items: flex-start;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  cursor: pointer;
}

.settings-theme-mode:has(input:checked) {
  border-color: var(--color-accent);
  background: var(--color-accent-subtle);
}

.settings-theme-mode input {
  margin-top: 3px;
  accent-color: var(--color-accent);
}

.settings-theme-mode > span {
  display: grid;
}

.settings-theme-mode__title {
  font-weight: 600;
}

@media (min-width: 768px) {
  .settings-theme-modes {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}
</style>
