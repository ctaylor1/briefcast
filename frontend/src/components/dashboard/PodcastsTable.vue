<script setup lang="ts">
import { formatDate } from "../../lib/format";
import { podcastImageUrl } from "../../lib/api/paths";
import type { Podcast } from "../../types/api";
import UiBadge from "../ui/UiBadge.vue";
import UiCard from "../ui/UiCard.vue";
import UiIcon from "../ui/UiIcon.vue";
import UiIconButton from "../ui/UiIconButton.vue";

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
  return podcastImageUrl(id);
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
              <div class="podcasts-table__actions" role="group" :aria-label="`Actions for ${podcast.Title}`">
                <UiIconButton
                  label="Play latest episode"
                  :disabled="activeId === podcast.ID"
                  @click="emit('play', podcast.ID)"
                >
                  <UiIcon name="play" />
                </UiIconButton>

                <UiIconButton
                  label="Download all episodes"
                  :disabled="activeId === podcast.ID"
                  @click="emit('download-all', podcast)"
                >
                  <UiIcon name="download" />
                </UiIconButton>

                <UiIconButton
                  label="Pause downloads"
                  :tooltip="podcast.IsPaused ? 'Downloads paused' : 'Pause downloads'"
                  :pressed="podcast.IsPaused"
                  :disabled="activeId === podcast.ID"
                  @click="emit('toggle-pause', podcast)"
                >
                  <UiIcon :name="podcast.IsPaused ? 'download-resume' : 'pause-downloads'" />
                </UiIconButton>

                <UiIconButton
                  label="Keep all episodes"
                  :tooltip="podcast.RetentionKeepAll ? 'Keeping all episodes' : 'Keep all episodes (now: global retention setting)'"
                  :pressed="podcast.RetentionKeepAll"
                  :disabled="activeId === podcast.ID"
                  @click="emit('toggle-retention', podcast)"
                >
                  <UiIcon name="archive" />
                </UiIconButton>

                <UiIconButton
                  label="Skip sponsor chapters"
                  :tooltip="podcast.AutoSkipSponsorChapters ? 'Sponsor skip: on' : 'Sponsor skip: off'"
                  :pressed="podcast.AutoSkipSponsorChapters"
                  :disabled="activeId === podcast.ID"
                  @click="emit('toggle-sponsor-skip', podcast)"
                >
                  <UiIcon name="skip-sponsor" />
                </UiIconButton>

                <UiIconButton
                  label="Sync to Briefpoint"
                  :tooltip="podcast.BriefpointEnabled ? 'Briefpoint: on' : 'Briefpoint: off'"
                  :pressed="podcast.BriefpointEnabled"
                  :disabled="activeId === podcast.ID"
                  @click="emit('toggle-briefpoint', podcast)"
                >
                  <UiIcon name="briefpoint" />
                </UiIconButton>

                <UiIconButton
                  label="Alternate feeds"
                  :disabled="activeId === podcast.ID"
                  @click="emit('edit-feeds', podcast)"
                >
                  <UiIcon name="feed" />
                </UiIconButton>

                <UiIconButton
                  label="Delete podcast"
                  tone="danger"
                  :disabled="activeId === podcast.ID"
                  @click="emit('delete', podcast)"
                >
                  <UiIcon name="trash" />
                </UiIconButton>
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
</style>
