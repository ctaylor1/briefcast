<script setup lang="ts">
import { RouterLink } from "vue-router";
import { podcastImageUrl } from "../../lib/api/paths";
import { formatDate } from "../../lib/format";
import { podcastPath } from "../../lib/routes";
import type { Podcast } from "../../types/api";
import UiIcon from "../ui/UiIcon.vue";
import UiIconButton from "../ui/UiIconButton.vue";
import PodcastActionsMenu from "./PodcastActionsMenu.vue";

// A library tile: cover and title open the podcast page; one primary action (play the
// latest episode) and everything else in the overflow menu.
defineProps<{
  podcast: Podcast;
  busy: boolean;
}>();

const emit = defineEmits<{
  (event: "play", podcastId: string): void;
  (event: "download-all", podcast: Podcast): void;
  (event: "toggle-pause", podcast: Podcast): void;
  (event: "toggle-retention", podcast: Podcast): void;
  (event: "toggle-sponsor-skip", podcast: Podcast): void;
  (event: "toggle-briefpoint", podcast: Podcast): void;
  (event: "edit-feeds", podcast: Podcast): void;
  (event: "delete", podcast: Podcast): void;
}>();
</script>

<template>
  <article class="podcast-tile">
    <!-- The title link is the one keyboard stop; the cover repeats it for pointer users. -->
    <RouterLink :to="podcastPath(podcast.ID)" class="podcast-tile__cover" tabindex="-1" aria-hidden="true">
      <img :src="podcastImageUrl(podcast.ID)" alt="" loading="lazy" />
    </RouterLink>
    <div class="podcast-tile__body">
      <h3 class="podcast-tile__title">
        <RouterLink :to="podcastPath(podcast.ID)">{{ podcast.Title }}</RouterLink>
      </h3>
      <p class="podcast-tile__meta">
        {{ podcast.AllEpisodesCount }} episodes · {{ formatDate(podcast.LastEpisode) }}
      </p>
      <p v-if="podcast.IsPaused || podcast.DownloadingEpisodesCount > 0" class="podcast-tile__state">
        <span v-if="podcast.IsPaused" class="podcast-tile__chip podcast-tile__chip--warning">Paused</span>
        <span v-if="podcast.DownloadingEpisodesCount > 0" class="podcast-tile__chip">
          {{ podcast.DownloadingEpisodesCount }} downloading
        </span>
      </p>
      <div class="podcast-tile__actions" role="group" :aria-label="`Actions for ${podcast.Title}`">
        <UiIconButton label="Play latest episode" :disabled="busy" @click="emit('play', podcast.ID)">
          <UiIcon name="play" />
        </UiIconButton>
        <PodcastActionsMenu
          :podcast="podcast"
          :busy="busy"
          @download-all="emit('download-all', $event)"
          @toggle-pause="emit('toggle-pause', $event)"
          @toggle-retention="emit('toggle-retention', $event)"
          @toggle-sponsor-skip="emit('toggle-sponsor-skip', $event)"
          @toggle-briefpoint="emit('toggle-briefpoint', $event)"
          @edit-feeds="emit('edit-feeds', $event)"
          @delete="emit('delete', $event)"
        />
      </div>
    </div>
  </article>
</template>

<style scoped>
.podcast-tile {
  display: grid;
  grid-template-rows: auto 1fr;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-3);
  background: var(--color-bg-primary);
  overflow: hidden;
}

.podcast-tile__cover {
  display: block;
  aspect-ratio: 1 / 1;
  background: var(--color-hover);
}

.podcast-tile__cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.podcast-tile__body {
  display: grid;
  align-content: start;
  gap: var(--space-1);
  padding: var(--space-3);
}

.podcast-tile__title {
  margin: 0;
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.podcast-tile__title a {
  color: var(--color-text-primary);
  text-decoration: none;
}

.podcast-tile__title a:hover {
  color: var(--color-accent-hover);
  text-decoration: underline;
}

.podcast-tile__meta,
.podcast-tile__state {
  margin: 0;
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
}

.podcast-tile__state {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
}

.podcast-tile__chip {
  padding: 0 var(--space-2);
  border-radius: 999px;
  background: var(--color-accent-subtle);
  color: var(--color-text-primary);
}

.podcast-tile__chip--warning {
  background: var(--color-warning-subtle);
  color: var(--color-warning-on-subtle);
}

.podcast-tile__actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  margin-top: var(--space-2);
}
</style>
