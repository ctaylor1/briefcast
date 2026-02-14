<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import UiAlert from "../components/ui/UiAlert.vue";
import UiButton from "../components/ui/UiButton.vue";
import UiCard from "../components/ui/UiCard.vue";
import UiInput from "../components/ui/UiInput.vue";
import { getErrorMessage, settingsApi } from "../lib/api";
import type { RetentionSettings } from "../types/api";

type RetentionForm = {
  keepAllEpisodes: boolean;
  keepLatestEpisodes: string;
  deleteAfterDays: string;
  deleteOnlyPlayed: boolean;
};

const isLoading = ref(true);
const isSaving = ref(false);
const errorMessage = ref("");
const successMessage = ref("");

const form = ref<RetentionForm>({
  keepAllEpisodes: true,
  keepLatestEpisodes: "0",
  deleteAfterDays: "0",
  deleteOnlyPlayed: true,
});

const retentionEnabled = computed(() => !form.value.keepAllEpisodes);

function mapToForm(settings: RetentionSettings): RetentionForm {
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
  errorMessage.value = "";
  try {
    const settings = await settingsApi.get();
    form.value = mapToForm(settings);
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Failed to load settings.");
  } finally {
    isLoading.value = false;
  }
}

async function saveSettings(): Promise<void> {
  isSaving.value = true;
  errorMessage.value = "";
  successMessage.value = "";
  const payload: RetentionSettings = {
    keepAllEpisodes: form.value.keepAllEpisodes,
    keepLatestEpisodes: sanitizeNumber(form.value.keepLatestEpisodes),
    deleteAfterDays: sanitizeNumber(form.value.deleteAfterDays),
    deleteOnlyPlayed: form.value.deleteOnlyPlayed,
  };
  try {
    const updated = await settingsApi.update(payload);
    form.value = mapToForm(updated);
    successMessage.value = "Retention settings updated.";
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Failed to update settings.");
  } finally {
    isSaving.value = false;
  }
}

onMounted(loadSettings);
</script>

<template>
  <section class="stack-4">
    <div>
      <h1 class="fluid-title-xl font-semibold tracking-tight text-slate-900">Settings</h1>
      <p class="fluid-subtle text-slate-600">
        Control retention so your library stays tidy without losing what matters.
      </p>
    </div>

    <UiAlert v-if="successMessage" tone="success">
      {{ successMessage }}
    </UiAlert>
    <UiAlert v-if="errorMessage" tone="danger">
      {{ errorMessage }}
    </UiAlert>

    <UiCard v-if="isLoading" padding="lg" class="text-sm text-slate-600">
      Loading settings...
    </UiCard>

    <UiCard v-else padding="lg" class="stack-4">
      <div class="stack-2">
        <h2 class="text-base font-semibold text-slate-900">Retention</h2>
        <p class="text-sm text-slate-600">
          Keep all episodes by default. Switch off to enable automatic cleanup rules.
        </p>
      </div>

      <label class="flex items-start gap-3 rounded-lg border border-slate-200 bg-slate-50 p-3 text-sm text-slate-700">
        <input
          v-model="form.keepAllEpisodes"
          type="checkbox"
          class="mt-0.5 h-4 w-4 rounded border-slate-300 text-cyan-600 focus:ring-cyan-500"
        />
        <div>
          <p class="font-medium text-slate-900">Keep all episodes (default)</p>
          <p class="text-xs text-slate-500">
            No files are deleted automatically, even if episodes have been played.
          </p>
        </div>
      </label>

      <div class="grid gap-4 md:grid-cols-2">
        <div class="space-y-2">
          <label class="text-sm font-medium text-slate-700">Keep latest episodes</label>
          <UiInput
            v-model="form.keepLatestEpisodes"
            type="number"
            min="0"
            :disabled="!retentionEnabled"
            placeholder="0"
          />
          <p class="text-xs text-slate-500">
            Set to 0 to disable. When enabled, older downloaded episodes are removed regardless of played status.
          </p>
        </div>

        <div class="space-y-2">
          <label class="text-sm font-medium text-slate-700">Delete after (days)</label>
          <UiInput
            v-model="form.deleteAfterDays"
            type="number"
            min="0"
            :disabled="!retentionEnabled"
            placeholder="0"
          />
          <p class="text-xs text-slate-500">
            Set to 0 to disable. This applies only to episodes older than the number of days you set.
          </p>
        </div>
      </div>

      <label class="flex items-start gap-3 text-sm text-slate-700">
        <input
          v-model="form.deleteOnlyPlayed"
          type="checkbox"
          class="mt-0.5 h-4 w-4 rounded border-slate-300 text-cyan-600 focus:ring-cyan-500"
          :disabled="!retentionEnabled"
        />
        <div>
          <p class="font-medium text-slate-900">Only delete played episodes</p>
          <p class="text-xs text-slate-500">
            If unchecked, episodes older than the threshold are removed whether or not they were played.
          </p>
        </div>
      </label>

      <div class="flex flex-wrap items-center gap-3">
        <UiButton :disabled="isSaving" @click="saveSettings">
          {{ isSaving ? "Saving..." : "Save retention settings" }}
        </UiButton>
        <p class="text-xs text-slate-500">
          Retention cleanup runs daily. Use podcast-level overrides to keep everything for specific feeds.
        </p>
      </div>
    </UiCard>
  </section>
</template>
