<script setup lang="ts">
import { formatDate } from "../../lib/format";
import type { Podcast } from "../../types/api";
import UiBadge from "../ui/UiBadge.vue";
import UiCard from "../ui/UiCard.vue";
import UiTooltip from "../ui/UiTooltip.vue";

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
  (event: "toggle-briefpoint", podcast: Podcast): void;
  (event: "edit-feeds", podcast: Podcast): void;
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
              </div>
            </td>
            <td>
              <div class="podcasts-table__actions">
                <UiTooltip text="Open player">
                  <button
                    type="button"
                    class="icon-btn"
                    :disabled="activeId === podcast.ID"
                    @click="emit('play', podcast.ID)"
                  >
                    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                      <path d="M8 5v14l11-7z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
                    </svg>
                  </button>
                </UiTooltip>

                <UiTooltip text="Download all">
                  <button
                    type="button"
                    class="icon-btn"
                    :disabled="activeId === podcast.ID"
                    @click="emit('download-all', podcast)"
                  >
                    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                      <path d="M12 4v10M8 10l4 4 4-4M5 20h14" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
                    </svg>
                  </button>
                </UiTooltip>

                <UiTooltip :text="podcast.IsPaused ? 'Resume downloads' : 'Pause downloads'">
                  <button
                    type="button"
                    class="icon-btn"
                    :class="{ 'icon-btn--active': podcast.IsPaused }"
                    :disabled="activeId === podcast.ID"
                    @click="emit('toggle-pause', podcast)"
                  >
                    <svg v-if="!podcast.IsPaused" viewBox="0 0 24 24" fill="none" aria-hidden="true">
                      <path d="M10 4H6v16h4V4zM18 4h-4v16h4V4z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
                    </svg>
                    <svg v-else viewBox="0 0 24 24" fill="none" aria-hidden="true">
                      <path d="M8 5v14l11-7z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
                    </svg>
                  </button>
                </UiTooltip>

                <UiTooltip :text="podcast.RetentionKeepAll ? 'Retention: Keep all' : 'Retention: Global'">
                  <button
                    type="button"
                    class="icon-btn"
                    :class="{ 'icon-btn--active': podcast.RetentionKeepAll }"
                    :disabled="activeId === podcast.ID"
                    @click="emit('toggle-retention', podcast)"
                  >
                    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                      <path d="M20 7H4a1 1 0 0 0-1 1v10a1 1 0 0 0 1 1h16a1 1 0 0 0 1-1V8a1 1 0 0 0-1-1zM12 5v2M8 5v2M16 5v2" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
                    </svg>
                  </button>
                </UiTooltip>

                <UiTooltip :text="podcast.AutoSkipSponsorChapters ? 'Sponsor skip: On' : 'Sponsor skip: Off'">
                  <button
                    type="button"
                    class="icon-btn"
                    :class="{ 'icon-btn--active': podcast.AutoSkipSponsorChapters }"
                    :disabled="activeId === podcast.ID"
                    @click="emit('toggle-sponsor-skip', podcast)"
                  >
                    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                      <path d="M17 8l4-4M17 16l4 4M3 12h14M7 8l-4 4 4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
                    </svg>
                  </button>
                </UiTooltip>

                <UiTooltip :text="podcast.BriefpointEnabled ? 'Briefpoint: On' : 'Briefpoint: Off'">
                  <button
                    type="button"
                    class="icon-btn"
                    :class="{ 'icon-btn--active': podcast.BriefpointEnabled }"
                    :disabled="activeId === podcast.ID"
                    @click="emit('toggle-briefpoint', podcast)"
                  >
                    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                      <path d="M12 5l7 4v6l-7 4-7-4V9l7-4zM12 12v7M12 12L5 9M12 12l7-3" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
                    </svg>
                  </button>
                </UiTooltip>

                <UiTooltip text="Alternate feeds">
                  <button
                    type="button"
                    class="icon-btn"
                    :disabled="activeId === podcast.ID"
                    @click="emit('edit-feeds', podcast)"
                  >
                    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                      <path d="M4 11a9 9 0 0 1 9-9M4 4a16 16 0 0 1 16 16M5 20a1 1 0 1 0 0-2 1 1 0 0 0 0 2z" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
                    </svg>
                  </button>
                </UiTooltip>

                <UiTooltip text="Delete podcast">
                  <button
                    type="button"
                    class="icon-btn icon-btn--danger"
                    :disabled="activeId === podcast.ID"
                    @click="emit('delete', podcast)"
                  >
                    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                      <path d="M3 6h18M8 6V4h8v2M19 6v14a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V6" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
                    </svg>
                  </button>
                </UiTooltip>
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
  min-width: 840px;
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

.podcasts-table__actions {
  display: grid;
  grid-template-columns: repeat(4, auto);
  gap: var(--space-1);
  justify-content: start;
}

.icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: color 0.15s, background 0.15s, border-color 0.15s;
}

.icon-btn svg {
  width: 16px;
  height: 16px;
}

.icon-btn:hover:not(:disabled) {
  color: var(--color-text-primary);
  background: var(--color-hover);
  border-color: var(--color-text-secondary);
}

.icon-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.icon-btn--active {
  color: var(--color-accent);
  border-color: var(--color-accent);
  background: var(--color-accent-subtle);
}

.icon-btn--active:hover:not(:disabled) {
  color: var(--color-accent-hover);
  border-color: var(--color-accent-hover);
}

.icon-btn--danger:hover:not(:disabled) {
  color: var(--color-danger);
  border-color: var(--color-danger);
  background: var(--color-danger-subtle, var(--color-hover));
}
</style>
