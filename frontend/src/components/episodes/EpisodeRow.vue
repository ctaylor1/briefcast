<script setup lang="ts">
import { computed } from "vue";
import { RouterLink } from "vue-router";
import { episodeImageUrl } from "../../lib/api/paths";
import { isEpisodeFavorited } from "../../lib/bookmarks";
import {
  DownloadStatus,
  hasKnownTotal,
  isDownloaded,
  isDownloading,
  progressAriaText,
  progressAriaValue,
  progressPercent,
  transcriptPill,
} from "../../lib/episodeStatus";
import { formatClock, formatDate, formatDuration } from "../../lib/format";
import { episodePath } from "../../lib/routes";
import type { PodcastItem } from "../../types/api";
import UiDropdown from "../ui/UiDropdown.vue";
import UiIcon from "../ui/UiIcon.vue";
import UiIconButton from "../ui/UiIconButton.vue";

// One compact episode row, the same at every width: a small cover, the title (which opens
// the episode page), a meta line that only names state that is there, Play, and a menu.
const props = defineProps<{
  item: PodcastItem;
}>();

const emit = defineEmits<{
  (event: "play", item: PodcastItem): void;
  (event: "toggle-played", item: PodcastItem): void;
  (event: "toggle-bookmark", item: PodcastItem): void;
  (event: "queue-download", item: PodcastItem): void;
  (event: "cancel-download", item: PodcastItem): void;
  (event: "open-details", item: PodcastItem, tab?: "overview" | "chapters" | "transcript"): void;
}>();

const favorited = computed(() => isEpisodeFavorited(props.item));
const transcript = computed(() => transcriptPill(props.item.TranscriptStatus ?? ""));
const resumeAt = computed(() =>
  props.item.PlaybackPositionSeconds && !props.item.IsPlayed ? props.item.PlaybackPositionSeconds : 0,
);
const canCancel = computed(
  () => props.item.DownloadStatus === DownloadStatus.Queued || props.item.DownloadStatus === DownloadStatus.Downloading,
);
</script>

<template>
  <li class="episode-row" :class="{ 'episode-row--played': item.IsPlayed }">
    <img :src="episodeImageUrl(item.ID)" alt="" class="episode-row__cover" loading="lazy" />
    <div class="episode-row__body">
      <RouterLink :to="episodePath(item.ID)" class="episode-row__title">{{ item.Title }}</RouterLink>
      <p class="episode-row__meta">
        <span>{{ item.Podcast?.Title || "Unknown podcast" }}</span>
        <span>{{ formatDate(item.PubDate) }}</span>
        <span>{{ formatDuration(item.Duration) }}</span>
        <span v-if="resumeAt" class="episode-row__state">Stopped at {{ formatClock(resumeAt) }}</span>
        <span v-if="item.IsPlayed" class="episode-row__state">
          <UiIcon name="check" :size="14" />
          Played
        </span>
        <span v-if="favorited" class="episode-row__state episode-row__state--favorite">
          <UiIcon name="star-filled" :size="14" />
          Favorite
        </span>
        <span v-if="item.DownloadStatus === DownloadStatus.Queued" class="episode-row__state">Queued</span>
        <span v-if="item.DownloadStatus === DownloadStatus.Paused" class="episode-row__state">Download paused</span>
        <RouterLink
          v-if="transcript.visible"
          :to="{ path: episodePath(item.ID), query: { tab: 'transcript' } }"
          class="episode-row__state episode-row__state--link"
        >
          {{ transcript.label }}
        </RouterLink>
        <RouterLink
          v-if="item.HasChapters"
          :to="{ path: episodePath(item.ID), query: { tab: 'chapters' } }"
          class="episode-row__state episode-row__state--link"
        >
          Chapters
        </RouterLink>
      </p>
      <div
        v-if="isDownloading(item)"
        class="episode-row__progress"
        role="progressbar"
        :aria-label="`Download progress for ${item.Title}`"
        aria-valuemin="0"
        aria-valuemax="100"
        :aria-valuenow="progressAriaValue(item)"
        :aria-valuetext="progressAriaText(item)"
      >
        <span
          :class="{ 'episode-row__progress--unknown': !hasKnownTotal(item) }"
          :style="hasKnownTotal(item) ? { width: `${progressPercent(item)}%` } : undefined"
        />
      </div>
    </div>
    <div class="episode-row__actions" role="group" :aria-label="`Actions for ${item.Title}`">
      <UiIconButton :label="resumeAt ? `Resume ${item.Title}` : `Play ${item.Title}`" :tooltip="resumeAt ? 'Resume' : 'Play'" @click="emit('play', item)">
        <UiIcon name="play" />
      </UiIconButton>
      <UiDropdown align="end" button-class="ui-icon-button episode-row__menu-trigger">
        <template #trigger>
          <UiIcon name="more" />
          <span class="sr-only">More actions for {{ item.Title }}</span>
        </template>
        <div class="podcast-menu" role="group" :aria-label="`More actions for ${item.Title}`">
          <button
            type="button"
            class="podcast-menu__item"
            :aria-pressed="item.IsPlayed ? 'true' : 'false'"
            @click="emit('toggle-played', item)"
          >
            <UiIcon :name="item.IsPlayed ? 'check' : 'circle'" :size="16" />
            <span>Played</span>
          </button>
          <button
            type="button"
            class="podcast-menu__item"
            :aria-pressed="favorited ? 'true' : 'false'"
            @click="emit('toggle-bookmark', item)"
          >
            <UiIcon :name="favorited ? 'star-filled' : 'star'" :size="16" />
            <span>Favorite</span>
          </button>
          <button v-if="canCancel" type="button" class="podcast-menu__item" @click="emit('cancel-download', item)">
            <UiIcon name="close" :size="16" />
            <span>Cancel download</span>
          </button>
          <button v-else-if="!isDownloaded(item)" type="button" class="podcast-menu__item" @click="emit('queue-download', item)">
            <UiIcon name="download" :size="16" />
            <span>Download</span>
          </button>
          <button type="button" class="podcast-menu__item" @click="emit('open-details', item, 'overview')">
            <UiIcon name="info" :size="16" />
            <span>Episode details</span>
          </button>
        </div>
      </UiDropdown>
    </div>
  </li>
</template>

<style scoped>
.episode-row {
  display: grid;
  grid-template-columns: 48px minmax(0, 1fr) auto;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  border-top: 1px solid var(--color-border);
}

.episode-row:first-child {
  border-top: 0;
}

.episode-row:hover {
  background: var(--color-hover);
}

.episode-row__cover {
  width: 48px;
  height: 48px;
  border-radius: var(--radius-2);
  object-fit: cover;
  background: var(--color-hover);
}

.episode-row__body {
  display: grid;
  gap: 2px;
  min-width: 0;
}

.episode-row__title {
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  color: var(--color-text-primary);
  font-weight: 600;
  text-decoration: none;
}

.episode-row__title:hover {
  text-decoration: underline;
}

.episode-row--played .episode-row__title {
  color: var(--color-text-secondary);
  font-weight: 500;
}

.episode-row__meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0 var(--space-2);
  margin: 0;
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
}

.episode-row__meta > span:not(.episode-row__state) + span:not(.episode-row__state)::before {
  content: "·";
  margin-right: var(--space-2);
}

.episode-row__state {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  color: var(--color-text-primary);
}

.episode-row__state--favorite .ui-icon {
  color: var(--color-favorite);
}

.episode-row__state--link {
  color: var(--color-accent);
  text-decoration: none;
}

.episode-row__state--link:hover {
  text-decoration: underline;
}

.episode-row__progress {
  height: 3px;
  max-width: 240px;
  border-radius: 999px;
  background: var(--color-hover);
  overflow: hidden;
}

.episode-row__progress > span {
  display: block;
  height: 100%;
  background: var(--color-accent);
}

.episode-row__progress--unknown {
  width: 40%;
}

.episode-row__actions {
  display: flex;
  align-items: center;
  gap: var(--space-1);
}

.episode-row__actions :deep(.ui-icon-button) {
  border-color: transparent;
  background: transparent;
}

@media (max-width: 767px) {
  .episode-row {
    grid-template-columns: 40px minmax(0, 1fr) auto;
    padding-inline: var(--space-2);
  }

  .episode-row__cover {
    width: 40px;
    height: 40px;
  }
}
</style>
