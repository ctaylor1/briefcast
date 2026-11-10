<script setup lang="ts">
import { computed } from "vue";
import { isEpisodeFavorited } from "../../lib/bookmarks";
import { DownloadStatus } from "../../lib/episodeStatus";
import type { PodcastItem } from "../../types/api";
import UiIcon from "../ui/UiIcon.vue";
import UiTooltip from "../ui/UiTooltip.vue";

const props = defineProps<{
  item: PodcastItem;
}>();

const emit = defineEmits<{
  (event: "play", item: PodcastItem): void;
  (event: "toggle-played", item: PodcastItem): void;
  (event: "toggle-bookmark", item: PodcastItem): void;
  (event: "queue-download", item: PodcastItem): void;
  (event: "cancel-download", item: PodcastItem): void;
}>();

const isPlayed = computed(() => props.item.IsPlayed);
const isBookmarked = computed(() => isEpisodeFavorited(props.item));
const isDownloading = computed(() => props.item.DownloadStatus === DownloadStatus.Downloading);
const isDownloaded = computed(() => props.item.DownloadStatus === DownloadStatus.Downloaded);
const canCancelDownload = computed(
  () => props.item.DownloadStatus === DownloadStatus.Queued || props.item.DownloadStatus === DownloadStatus.Downloading,
);

const playedLabel = computed(() => (isPlayed.value ? "Mark as unplayed" : "Mark as played"));
const bookmarkLabel = computed(() => (isBookmarked.value ? "Remove favorite" : "Favorite"));
const downloadLabel = computed(() => {
  if (isDownloading.value) {
    return "Downloading";
  }
  if (isDownloaded.value) {
    return "Downloaded";
  }
  return "Download";
});

function queueDownload(): void {
  if (isDownloading.value || isDownloaded.value) {
    return;
  }
  emit("queue-download", props.item);
}
</script>

<template>
  <div class="episode-track-controls">
    <UiTooltip text="Play">
      <button type="button" class="track-control" aria-label="Play" @click="emit('play', props.item)">
        <UiIcon name="play" :size="20" />
      </button>
    </UiTooltip>

    <UiTooltip :text="playedLabel">
      <button
        type="button"
        class="track-control"
        :class="{ 'track-control--active': isPlayed }"
        aria-label="Played"
        :aria-pressed="isPlayed ? 'true' : 'false'"
        @click="emit('toggle-played', props.item)"
      >
        <UiIcon :name="isPlayed ? 'check-circle' : 'circle'" :size="20" />
      </button>
    </UiTooltip>

    <UiTooltip :text="bookmarkLabel">
      <button
        type="button"
        class="track-control"
        :class="{ 'track-control--active': isBookmarked }"
        aria-label="Favorite"
        :aria-pressed="isBookmarked ? 'true' : 'false'"
        @click="emit('toggle-bookmark', props.item)"
      >
        <UiIcon :name="isBookmarked ? 'star-filled' : 'star'" :size="20" />
      </button>
    </UiTooltip>

    <UiTooltip :text="downloadLabel">
      <button
        type="button"
        class="track-control"
        :class="{ 'track-control--active': isDownloaded }"
        :aria-label="downloadLabel"
        :disabled="isDownloading || isDownloaded"
        @click="queueDownload"
      >
        <svg
          v-if="isDownloading"
          viewBox="0 0 24 24"
          class="track-control__icon track-control__spinner"
          aria-hidden="true"
          focusable="false"
        >
          <circle cx="12" cy="12" r="8" fill="none" stroke="currentColor" stroke-opacity="0.25" stroke-width="2.5" />
          <path d="M20 12a8 8 0 0 0-8-8" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" />
        </svg>
        <UiIcon v-else :name="isDownloaded ? 'check-circle' : 'download'" :size="20" />
      </button>
    </UiTooltip>

    <UiTooltip v-if="canCancelDownload" text="Cancel download">
      <button
        type="button"
        class="track-control"
        aria-label="Cancel download"
        @click="emit('cancel-download', props.item)"
      >
        <UiIcon name="close" :size="20" />
      </button>
    </UiTooltip>
  </div>
</template>

<style scoped>
.episode-track-controls {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1);
}

.track-control {
  width: 44px;
  min-width: 44px;
  height: 44px;
  min-height: 44px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-primary);
  color: var(--color-text-secondary);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  cursor: pointer;
  transition:
    background-color var(--duration-fast) var(--ease-enter),
    border-color var(--duration-fast) var(--ease-enter),
    color var(--duration-fast) var(--ease-enter);
}

.track-control:hover {
  background: var(--color-hover);
  color: var(--color-text-primary);
}

.track-control:focus-visible {
  outline: 2px solid var(--color-accent);
  outline-offset: 2px;
}

.track-control:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.track-control--active {
  border-color: color-mix(in srgb, var(--color-accent) 40%, var(--color-border));
  background: var(--color-accent-subtle);
  color: var(--color-accent-hover);
}

.track-control__icon {
  width: 20px;
  height: 20px;
}

.track-control__spinner {
  animation: track-control-spin 0.9s linear infinite;
}

@keyframes track-control-spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}
</style>
