<script setup lang="ts">
import { ref } from "vue";
import AddByUrlCard from "../components/add/AddByUrlCard.vue";
import OpmlImportCard from "../components/add/OpmlImportCard.vue";
import SearchPodcastsCard from "../components/add/SearchPodcastsCard.vue";
import UiAlert from "../components/ui/UiAlert.vue";
import { discoveryApi, getErrorMessage } from "../lib/api";
import type { SearchResult, SearchSource } from "../types/api";

const feedUrl = ref("");
const searchQuery = ref("");
const searchSource = ref<SearchSource>("itunes");
const isWorking = ref(false);
const errorMessage = ref("");
const infoMessage = ref("");
const results = ref<SearchResult[]>([]);
const selectedFile = ref<File | null>(null);

function onFileChange(event: Event): void {
  const target = event.target as HTMLInputElement;
  if (!target.files || target.files.length === 0) {
    selectedFile.value = null;
    return;
  }
  const [file] = Array.from(target.files);
  selectedFile.value = file ?? null;
}

async function addPodcast(url: string): Promise<void> {
  const trimmedUrl = url.trim();
  if (!trimmedUrl) {
    return;
  }
  isWorking.value = true;
  errorMessage.value = "";
  infoMessage.value = "";
  try {
    await discoveryApi.addPodcast({ url: trimmedUrl });
    infoMessage.value = "Podcast added successfully.";
    feedUrl.value = "";
    for (const result of results.value) {
      if (result.url === trimmedUrl) {
        result.already_saved = true;
      }
    }
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Could not add podcast.");
  } finally {
    isWorking.value = false;
  }
}

async function searchPodcasts(): Promise<void> {
  const trimmedQuery = searchQuery.value.trim();
  if (!trimmedQuery) {
    return;
  }
  isWorking.value = true;
  errorMessage.value = "";
  infoMessage.value = "";
  try {
    results.value = await discoveryApi.searchPodcasts({
      q: trimmedQuery,
      searchSource: searchSource.value,
    });
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Search failed.");
  } finally {
    isWorking.value = false;
  }
}

async function uploadOpml(): Promise<void> {
  if (!selectedFile.value) {
    return;
  }
  isWorking.value = true;
  errorMessage.value = "";
  infoMessage.value = "";
  try {
    await discoveryApi.uploadOpml(selectedFile.value);
    infoMessage.value = "OPML file uploaded successfully.";
    selectedFile.value = null;
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "OPML upload failed.");
  } finally {
    isWorking.value = false;
  }
}
</script>

<template>
  <section class="add-page stack-4">
    <header class="page-header">
      <h2 class="section-title">Add podcasts</h2>
      <p class="section-subtitle">
        Add a feed URL, import OPML subscriptions, or search the directory.
      </p>
    </header>

    <UiAlert v-if="infoMessage" tone="success">
      {{ infoMessage }}
    </UiAlert>
    <UiAlert v-if="errorMessage" tone="danger">
      {{ errorMessage }}
    </UiAlert>

    <div class="add-page__top-grid">
      <AddByUrlCard
        :feed-url="feedUrl"
        :working="isWorking"
        @update:feed-url="feedUrl = $event"
        @submit="addPodcast(feedUrl)"
      />

      <OpmlImportCard
        :working="isWorking"
        :has-file="selectedFile !== null"
        @file-change="onFileChange"
        @upload="uploadOpml"
      />
    </div>

    <SearchPodcastsCard
      :query="searchQuery"
      :source="searchSource"
      :working="isWorking"
      :results="results"
      @update:query="searchQuery = $event"
      @update:source="searchSource = $event"
      @search="searchPodcasts"
      @add="addPodcast"
    />
  </section>
</template>

<style scoped>
.add-page__top-grid {
  display: grid;
  gap: var(--space-4);
}

@media (min-width: 768px) {
  .add-page__top-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
