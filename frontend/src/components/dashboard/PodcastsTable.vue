<script setup lang="ts">
import { formatDate } from "../../lib/format";
import type { Podcast } from "../../types/api";
import UiBadge from "../ui/UiBadge.vue";
import UiButton from "../ui/UiButton.vue";
import UiCard from "../ui/UiCard.vue";

defineProps<{
  podcasts: Podcast[];
  activeId: string | null;
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
  <UiCard padding="none">
    <div class="table-wrap visually-scrollable">
      <table class="data-table podcasts-table">
        <thead>
          <tr>
            <th>Podcast</th>
            <th>Recent</th>
            <th>Stats</th>
            <th>Status</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="podcast in podcasts"
            :key="podcast.ID"
          >
            <td>
              <button
                type="button"
                class="podcasts-table__identity podcasts-table__identity-button"
                @click="emit('open-player', podcast.ID)"
              >
                <img
                  :src="getPodcastImage(podcast.ID)"
                  :alt="podcast.Title"
                  class="podcasts-table__image"
                  loading="lazy"
                />
                <div class="stack-1">
                  <p class="podcasts-table__title">{{ podcast.Title }}</p>
                  <p class="podcasts-table__summary">
                    {{ podcast.Summary || "No summary available." }}
                  </p>
                </div>
              </button>
            </td>
            <td class="meta-text">
              {{ formatDate(podcast.LastEpisode) }}
            </td>
            <td>
              <div class="podcasts-table__stats">
                <UiBadge tone="neutral">Downloaded: {{ podcast.DownloadedEpisodesCount }}</UiBadge>
                <UiBadge tone="neutral">Queue: {{ podcast.DownloadingEpisodesCount }}</UiBadge>
                <UiBadge tone="neutral">Total: {{ podcast.AllEpisodesCount }}</UiBadge>
              </div>
            </td>
            <td>
              <div class="podcasts-table__status">
                <UiBadge :tone="podcast.IsPaused ? 'warning' : 'success'">
                  {{ podcast.IsPaused ? "Paused" : "Active" }}
                </UiBadge>
                <div class="podcasts-table__inline-action">
                  <span class="meta-text">Retention: {{ podcast.RetentionKeepAll ? "Keep all" : "Global" }}</span>
                  <UiButton
                    size="sm"
                    variant="ghost"
                    :disabled="activeId === podcast.ID"
                    @click="emit('toggle-retention', podcast)"
                  >
                    {{ podcast.RetentionKeepAll ? "Use global" : "Keep all" }}
                  </UiButton>
                </div>
                <div class="podcasts-table__inline-action">
                  <span class="meta-text">
                    Sponsor skip: {{ podcast.AutoSkipSponsorChapters ? "On" : "Off" }}
                  </span>
                  <UiButton
                    size="sm"
                    variant="ghost"
                    :disabled="activeId === podcast.ID"
                    @click="emit('toggle-sponsor-skip', podcast)"
                  >
                    {{ podcast.AutoSkipSponsorChapters ? "Disable" : "Enable" }}
                  </UiButton>
                </div>
              </div>
            </td>
            <td>
              <div class="podcasts-table__actions">
                <UiButton size="sm" :disabled="activeId === podcast.ID" @click="emit('play', podcast.ID)">
                  Play
                </UiButton>
                <UiButton
                  size="sm"
                  variant="secondary"
                  :disabled="activeId === podcast.ID"
                  @click="emit('download-all', podcast)"
                >
                  Download
                </UiButton>
                <UiButton
                  size="sm"
                  variant="outline"
                  :disabled="activeId === podcast.ID"
                  @click="emit('toggle-pause', podcast)"
                >
                  {{ podcast.IsPaused ? "Resume" : "Pause" }}
                </UiButton>
                <UiButton
                  size="sm"
                  variant="danger"
                  :disabled="activeId === podcast.ID"
                  @click="emit('delete', podcast)"
                >
                  Delete
                </UiButton>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </UiCard>
</template>

<style scoped>
.podcasts-table {
  min-width: 980px;
}

.podcasts-table__identity {
  display: flex;
  align-items: flex-start;
  gap: var(--space-3);
  min-width: 280px;
}

.podcasts-table__identity-button {
  width: 100%;
  border: 0;
  padding: 0;
  background: transparent;
  text-align: left;
  cursor: pointer;
}

.podcasts-table__identity-button:hover .podcasts-table__title {
  color: var(--color-accent-hover);
}

.podcasts-table__image {
  width: 56px;
  height: 56px;
  border-radius: var(--radius-2);
  background: var(--color-hover);
  object-fit: cover;
  flex: 0 0 auto;
}

.podcasts-table__title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
}

.podcasts-table__summary {
  margin: 0;
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  max-width: 34ch;
}

.podcasts-table__stats {
  display: grid;
  gap: var(--space-2);
}

.podcasts-table__status {
  display: grid;
  gap: var(--space-2);
}

.podcasts-table__inline-action {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}

.podcasts-table__actions {
  display: grid;
  gap: var(--space-2);
  grid-template-columns: repeat(2, minmax(0, 1fr));
  min-width: 210px;
}
</style>
