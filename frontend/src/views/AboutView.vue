<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import UiAlert from "../components/ui/UiAlert.vue";
import UiCard from "../components/ui/UiCard.vue";
import { getErrorMessage, httpClient } from "../lib/api/http";
import type { RuntimeVersionInfo } from "../types/api";

const fallbackRepoURL = "https://github.com/ctaylor1/briefcast";

const isLoading = ref(true);
const errorMessage = ref("");
const runtime = ref<RuntimeVersionInfo | null>(null);

const runningVersion = computed(() => {
  const value = runtime.value?.version?.trim();
  return value && value.length > 0 ? value : "unknown";
});

const repositoryURL = computed(() => {
  const value = runtime.value?.repoUrl?.trim();
  return value && value.length > 0 ? value : fallbackRepoURL;
});

async function loadRuntimeVersion(): Promise<void> {
  isLoading.value = true;
  errorMessage.value = "";
  try {
    runtime.value = await httpClient.get<RuntimeVersionInfo>("/version");
  } catch (error) {
    runtime.value = null;
    errorMessage.value = getErrorMessage(error, "Failed to load running version.");
  } finally {
    isLoading.value = false;
  }
}

onMounted(loadRuntimeVersion);
</script>

<template>
  <section class="about-page stack-4">
    <header class="page-header">
      <h2 class="section-title">About</h2>
      <p class="section-subtitle">
        Briefcast keeps podcast downloading, transcription, and playback in one focused workspace.
      </p>
    </header>

    <UiAlert v-if="errorMessage" tone="warning">
      {{ errorMessage }}
    </UiAlert>

    <UiCard padding="lg" class="stack-3">
      <div class="about-version-block">
        <p class="meta-text">Running version</p>
        <p class="about-version">{{ isLoading ? "Loading..." : runningVersion }}</p>
      </div>

      <p class="section-subtitle">
        Source code and release history are available on GitHub.
      </p>
      <a
        class="about-link"
        :href="repositoryURL"
        target="_blank"
        rel="noopener noreferrer"
      >
        {{ repositoryURL }}
      </a>
    </UiCard>
  </section>
</template>

<style scoped>
.about-version-block {
  padding: var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
}

.about-version {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-section-size);
  line-height: var(--font-section-line-height);
  font-weight: 700;
}

.about-link {
  color: var(--color-accent-hover);
  word-break: break-word;
}

.about-link:hover {
  color: var(--color-accent);
}
</style>
