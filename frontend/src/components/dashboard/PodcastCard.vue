<script setup lang="ts">
import type { Podcast } from "../../types/api";
import { formatDate } from "../../lib/format";
import { podcastImageUrl } from "../../lib/api/paths";
import UiBadge from "../ui/UiBadge.vue";
import UiCard from "../ui/UiCard.vue";
import UiIcon from "../ui/UiIcon.vue";
import UiIconButton from "../ui/UiIconButton.vue";

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
  (event: "toggle-briefpoint", podcast: Podcast): void;
  (event: "edit-feeds", podcast: Podcast): void;
  (event: "delete", podcast: Podcast): void;
}>();

function getPodcastImage(id: string): string {
  return podcastImageUrl(id);
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
      <div class="podcast-card__toolbar" role="group" :aria-label="`Actions for ${podcast.Title}`">
        <UiIconButton
          label="Play latest episode"
          :disabled="busy"
          @click="emit('play', podcast.ID)"
        >
          <UiIcon name="play" />
        </UiIconButton>

        <UiIconButton
          label="Download all episodes"
          :disabled="busy"
          @click="emit('download-all', podcast)"
        >
          <UiIcon name="download" />
        </UiIconButton>

        <UiIconButton
          label="Pause downloads"
          :tooltip="podcast.IsPaused ? 'Downloads paused' : 'Pause downloads'"
          :pressed="podcast.IsPaused"
          :disabled="busy"
          @click="emit('toggle-pause', podcast)"
        >
          <UiIcon :name="podcast.IsPaused ? 'download-resume' : 'pause-downloads'" />
        </UiIconButton>

        <UiIconButton
          label="Keep all episodes"
          :tooltip="podcast.RetentionKeepAll ? 'Keeping all episodes' : 'Keep all episodes (now: global retention setting)'"
          :pressed="podcast.RetentionKeepAll"
          :disabled="busy"
          @click="emit('toggle-retention', podcast)"
        >
          <UiIcon name="archive" />
        </UiIconButton>

        <UiIconButton
          label="Skip sponsor chapters"
          :tooltip="podcast.AutoSkipSponsorChapters ? 'Sponsor skip: on' : 'Sponsor skip: off'"
          :pressed="podcast.AutoSkipSponsorChapters"
          :disabled="busy"
          @click="emit('toggle-sponsor-skip', podcast)"
        >
          <UiIcon name="skip-sponsor" />
        </UiIconButton>

        <UiIconButton
          label="Sync to Briefpoint"
          :tooltip="podcast.BriefpointEnabled ? 'Briefpoint: on' : 'Briefpoint: off'"
          :pressed="podcast.BriefpointEnabled"
          :disabled="busy"
          @click="emit('toggle-briefpoint', podcast)"
        >
          <UiIcon name="briefpoint" />
        </UiIconButton>

        <UiIconButton
          label="Alternate feeds"
          :disabled="busy"
          @click="emit('edit-feeds', podcast)"
        >
          <UiIcon name="feed" />
        </UiIconButton>

        <UiIconButton
          label="Delete podcast"
          tone="danger"
          :disabled="busy"
          @click="emit('delete', podcast)"
        >
          <UiIcon name="trash" />
        </UiIconButton>
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

/* The card clips its overflow, so the focus ring has to sit inside the cover. */
.podcast-card__cover-trigger:focus-visible {
  outline-offset: -2px;
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

.podcast-card__stats {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.podcast-card__toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}
</style>
