<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import UiAlert from "../components/ui/UiAlert.vue";
import UiButton from "../components/ui/UiButton.vue";
import UiCard from "../components/ui/UiCard.vue";
import UiInput from "../components/ui/UiInput.vue";
import { useStatusMessage } from "../composables/useStatusMessage";
import { useTheme } from "../composables/useTheme";
import { getErrorMessage, settingsApi } from "../lib/api";
import type { AppSettings } from "../types/api";

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
  summarizationPrompt: string;
  summarizationUserPrompt: string;
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
const defaultSystemPrompt = ref("");
const defaultUserPrompt = ref("");
const {
  errorMessage,
  successMessage,
  clearAll,
  clearError,
  setError,
  setSuccess,
} = useStatusMessage(5000);

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
  summarizationPrompt: "",
  summarizationUserPrompt: "",
});

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
    summarizationForm.value = {
      summarizationEnabled: settings.summarizationEnabled,
      summarizationPrompt: settings.summarizationPrompt ?? "",
      summarizationUserPrompt: settings.summarizationUserPrompt ?? "",
    };
    defaultSystemPrompt.value = settings.defaultSystemPrompt ?? "";
    defaultUserPrompt.value = settings.defaultUserPrompt ?? "";
  } catch (error) {
    setError(getErrorMessage(error, "Failed to load settings."));
  } finally {
    isLoading.value = false;
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
    const updated = await settingsApi.update({
      summarizationEnabled: summarizationForm.value.summarizationEnabled,
      summarizationPrompt: summarizationForm.value.summarizationPrompt,
      summarizationUserPrompt: summarizationForm.value.summarizationUserPrompt,
    });
    summarizationForm.value = {
      summarizationEnabled: updated.summarizationEnabled,
      summarizationPrompt: updated.summarizationPrompt ?? "",
      summarizationUserPrompt: updated.summarizationUserPrompt ?? "",
    };
    setSuccess("Summarization settings updated.");
  } catch (error) {
    setError(getErrorMessage(error, "Failed to update summarization settings."));
  } finally {
    isSavingSummarization.value = false;
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

onMounted(loadSettings);
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
        <label class="settings-label" for="summarization-system-prompt">System prompt</label>
        <textarea
          id="summarization-system-prompt"
          v-model="summarizationForm.summarizationPrompt"
          rows="6"
          class="settings-textarea"
          :disabled="!summarizationForm.summarizationEnabled"
          :placeholder="defaultSystemPrompt || 'Leave blank to use the default system prompt from your environment configuration.'"
        />
        <p class="meta-text">
          Sent as the system message to the LLM. Leave blank to use the default prompt from your .env configuration.
        </p>
      </div>

      <div class="stack-1">
        <label class="settings-label" for="summarization-user-prompt">User prompt prefix</label>
        <textarea
          id="summarization-user-prompt"
          v-model="summarizationForm.summarizationUserPrompt"
          rows="4"
          class="settings-textarea"
          :disabled="!summarizationForm.summarizationEnabled"
          :placeholder="defaultUserPrompt || 'Leave blank to use the default user prompt prefix.'"
        />
        <p class="meta-text">
          Prepended to the transcript in the user message. Leave blank to use the default prefix.
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
    </UiCard>
  </section>
</template>

<style scoped>
.settings-section-title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-section-size);
  line-height: var(--font-section-line-height);
  font-weight: var(--font-section-weight);
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
</style>
