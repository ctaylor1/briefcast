<script setup lang="ts">
import type { Podcast } from "../../types/api";
import { formatDate } from "../../lib/format";
import UiBadge from "../ui/UiBadge.vue";
import UiButton from "../ui/UiButton.vue";
import UiCard from "../ui/UiCard.vue";

defineProps<{
  podcast: Podcast;
  busy: boolean;
}>();

const emit = defineEmits<{
  (event: "open-player", podcastId: string): void;
  (event: "play", podcastId: string): void;
  (event: "download-all", podcast: Podcast): void;
  (event: "toggle-pause", podcast: Podcast): void;
  (event: "toggle-retention", podcast: Podcast): void;
  (event: "toggle-sponsor-skip", podcast: Podcast): void;
  (event: "delete", podcast: Podcast): void;
}>();

function getPodcastImage(id: string): string {
  return `/podcasts/${id}/image`;
}
</script>

<template>
  <UiCard padding="none" class="podcast-card">
    <button
      type="button"
      class="podcast-card__cover-trigger"
      :aria-label="`Open player for ${podcast.Title}`"
      @click="emit('open-player', podcast.ID)"
    >
      <img
        :src="getPodcastImage(podcast.ID)"
        :alt="podcast.Title"
        class="podcast-card__image"
        loading="lazy"
      />
    </button>
    <div class="podcast-card__content stack-3">
      <div class="stack-1">
        <h3 class="podcast-card__title-wrap">
          <button
            type="button"
            class="podcast-card__title podcast-card__title-trigger"
            @click="emit('open-player', podcast.ID)"
          >
            {{ podcast.Title }}
          </button>
        </h3>
        <p class="meta-text">Last episode: {{ formatDate(podcast.LastEpisode) }}</p>
      </div>
      <p class="podcast-card__summary">{{ podcast.Summary || "No summary available." }}</p>
      <div class="podcast-card__stats">
        <UiBadge tone="neutral">
          Downloaded: {{ podcast.DownloadedEpisodesCount }}
        </UiBadge>
        <UiBadge tone="neutral">
          Queue: {{ podcast.DownloadingEpisodesCount }}
        </UiBadge>
        <UiBadge tone="neutral">
          Total: {{ podcast.AllEpisodesCount }}
        </UiBadge>
      </div>
      <div class="podcast-card__toggle">
        <span class="meta-text">
          Retention: {{ podcast.RetentionKeepAll ? "Keep all" : "Global" }}
        </span>
        <UiButton size="sm" variant="ghost" :disabled="busy" @click="emit('toggle-retention', podcast)">
          {{ podcast.RetentionKeepAll ? "Use global" : "Keep all" }}
        </UiButton>
      </div>
      <div class="podcast-card__toggle">
        <span class="meta-text">
          Sponsor skip: {{ podcast.AutoSkipSponsorChapters ? "On" : "Off" }}
        </span>
        <UiButton size="sm" variant="ghost" :disabled="busy" @click="emit('toggle-sponsor-skip', podcast)">
          {{ podcast.AutoSkipSponsorChapters ? "Disable" : "Enable" }}
        </UiButton>
      </div>
      <div class="podcast-card__actions">
        <UiButton size="sm" :disabled="busy" @click="emit('play', podcast.ID)">
          Play
        </UiButton>
        <UiButton size="sm" variant="secondary" :disabled="busy" @click="emit('download-all', podcast)">
          Download All
        </UiButton>
        <UiButton size="sm" variant="outline" :disabled="busy" @click="emit('toggle-pause', podcast)">
          {{ podcast.IsPaused ? "Resume" : "Pause" }}
        </UiButton>
        <UiButton size="sm" variant="danger" :disabled="busy" @click="emit('delete', podcast)">
          Delete
        </UiButton>
      </div>
    </div>
  </UiCard>
</template>

<style scoped>
.podcast-card {
  overflow: hidden;
}

.podcast-card__image {
  width: 100%;
  aspect-ratio: 1 / 1;
  height: auto;
  background: var(--color-hover);
  object-fit: cover;
}

.podcast-card__cover-trigger {
  border: 0;
  padding: 0;
  background: transparent;
  cursor: pointer;
  display: block;
}

.podcast-card__content {
  padding: var(--space-4);
}

.podcast-card__title-wrap {
  margin: 0;
}

.podcast-card__title {
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
}

.podcast-card__title-trigger {
  border: 0;
  padding: 0;
  margin: 0;
  background: transparent;
  text-align: left;
  cursor: pointer;
}

.podcast-card__title-trigger:hover {
  color: var(--color-accent-hover);
}

.podcast-card__summary {
  margin: 0;
  color: var(--color-text-secondary);
  font-size: var(--font-body-size);
  line-height: var(--font-body-line-height);
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.podcast-card__stats {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.podcast-card__toggle {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  min-height: 44px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
  padding: var(--space-2) var(--space-3);
}

.podcast-card__actions {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--space-2);
}

</style>
