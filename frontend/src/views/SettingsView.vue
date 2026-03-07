<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import UiAlert from "../components/ui/UiAlert.vue";
import UiButton from "../components/ui/UiButton.vue";
import UiCard from "../components/ui/UiCard.vue";
import UiInput from "../components/ui/UiInput.vue";
import { useStatusMessage } from "../composables/useStatusMessage";
import { getErrorMessage, settingsApi } from "../lib/api";
import type { AppSettings } from "../types/api";

type RetentionForm = {
  keepAllEpisodes: boolean;
  keepLatestEpisodes: string;
  deleteAfterDays: string;
  deleteOnlyPlayed: boolean;
};

type SummarizationForm = {
  summarizationEnabled: boolean;
  summarizationPrompt: string;
  summarizationUserPrompt: string;
};

const isLoading = ref(true);
const isSavingRetention = ref(false);
const isSavingSummarization = ref(false);
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

const summarizationForm = ref<SummarizationForm>({
  summarizationEnabled: false,
  summarizationPrompt: "",
  summarizationUserPrompt: "",
});

const retentionEnabled = computed(() => !retentionForm.value.keepAllEpisodes);

function mapToRetentionForm(settings: AppSettings): RetentionForm {
  return {
    keepAllEpisodes: settings.keepAllEpisodes,
    keepLatestEpisodes: String(settings.keepLatestEpisodes ?? 0),
    deleteAfterDays: String(settings.deleteAfterDays ?? 0),
    deleteOnlyPlayed: settings.deleteOnlyPlayed,
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
