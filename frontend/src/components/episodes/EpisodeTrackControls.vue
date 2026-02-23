<script setup lang="ts">
import { computed } from "vue";
import { isBookmarkedDate } from "../../lib/bookmarks";
import type { PodcastItem } from "../../types/api";
import UiTooltip from "../ui/UiTooltip.vue";

const props = defineProps<{
  item: PodcastItem;
  isPlayingTrack: boolean;
}>();

const emit = defineEmits<{
  (event: "play", item: PodcastItem): void;
  (event: "stop-playback", item: PodcastItem): void;
  (event: "toggle-played", item: PodcastItem): void;
  (event: "toggle-bookmark", item: PodcastItem): void;
  (event: "queue-download", item: PodcastItem): void;
  (event: "cancel-download", item: PodcastItem): void;
}>();

const isPlayed = computed(() => props.item.IsPlayed);
const isBookmarked = computed(() => isBookmarkedDate(props.item.BookmarkDate));
const isDownloading = computed(() => props.item.DownloadStatus === 1);
const isDownloaded = computed(() => props.item.DownloadStatus === 2);
const canCancelDownload = computed(() => props.item.DownloadStatus === 0 || props.item.DownloadStatus === 1);

const playedLabel = computed(() => (isPlayed.value ? "Mark as unplayed" : "Mark as played"));
const bookmarkLabel = computed(() => (isBookmarked.value ? "Remove bookmark" : "Bookmark"));
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
      <button
        type="button"
        class="track-control"
        title="Play"
        aria-label="Play"
        @click="emit('play', props.item)"
      >
        <span class="track-control__glyph" aria-hidden="true">▶</span>
      </button>
    </UiTooltip>

    <UiTooltip text="Stop">
      <button
        type="button"
        class="track-control"
        :class="{ 'track-control--active': isPlayingTrack }"
        title="Stop"
        aria-label="Stop"
        :disabled="!isPlayingTrack"
        @click="emit('stop-playback', props.item)"
      >
        <span class="track-control__glyph" aria-hidden="true">⏹</span>
      </button>
    </UiTooltip>

    <UiTooltip :text="playedLabel">
      <button
        type="button"
        class="track-control"
        :class="{ 'track-control--active': isPlayed }"
        :title="playedLabel"
        :aria-label="playedLabel"
        :aria-pressed="isPlayed"
        @click="emit('toggle-played', props.item)"
      >
        <svg
          v-if="isPlayed"
          viewBox="0 0 24 24"
          class="track-control__icon"
          aria-hidden="true"
          focusable="false"
        >
          <circle cx="12" cy="12" r="10" fill="currentColor" />
          <path
            d="M7 12.5L10.5 16L17 9.5"
            fill="none"
            stroke="var(--color-bg-primary)"
            stroke-width="2.2"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </svg>
        <svg
          v-else
          viewBox="0 0 24 24"
          class="track-control__icon"
          aria-hidden="true"
          focusable="false"
        >
          <circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" stroke-width="2" />
        </svg>
      </button>
    </UiTooltip>

    <UiTooltip :text="bookmarkLabel">
      <button
        type="button"
        class="track-control"
        :class="{ 'track-control--active': isBookmarked }"
        :title="bookmarkLabel"
        :aria-label="bookmarkLabel"
        :aria-pressed="isBookmarked"
        @click="emit('toggle-bookmark', props.item)"
      >
        <svg
          v-if="isBookmarked"
          viewBox="0 0 24 24"
          class="track-control__icon"
          aria-hidden="true"
          focusable="false"
        >
          <path
            d="M6 3h12a1 1 0 0 1 1 1v16l-7-4-7 4V4a1 1 0 0 1 1-1z"
            fill="currentColor"
          />
        </svg>
        <svg
          v-else
          viewBox="0 0 24 24"
          class="track-control__icon"
          aria-hidden="true"
          focusable="false"
        >
          <path
            d="M6 3h12a1 1 0 0 1 1 1v16l-7-4-7 4V4a1 1 0 0 1 1-1z"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </svg>
      </button>
    </UiTooltip>

    <UiTooltip :text="downloadLabel">
      <button
        type="button"
        class="track-control"
        :class="{ 'track-control--active': isDownloaded }"
        :title="downloadLabel"
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
          <path
            d="M20 12a8 8 0 0 0-8-8"
            fill="none"
            stroke="currentColor"
            stroke-width="2.5"
            stroke-linecap="round"
          />
        </svg>
        <svg
          v-else-if="isDownloaded"
          viewBox="0 0 24 24"
          class="track-control__icon"
          aria-hidden="true"
          focusable="false"
        >
          <circle cx="12" cy="12" r="10" fill="currentColor" />
          <path
            d="M7 12.5L10.5 16L17 9.5"
            fill="none"
            stroke="var(--color-bg-primary)"
            stroke-width="2.2"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </svg>
        <svg
          v-else
          viewBox="0 0 24 24"
          class="track-control__icon"
          aria-hidden="true"
          focusable="false"
        >
          <path
            d="M12 3v11m0 0l4-4m-4 4l-4-4M5 19h14"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </svg>
      </button>
    </UiTooltip>

    <UiTooltip v-if="canCancelDownload" text="Cancel download">
      <button
        type="button"
        class="track-control"
        title="Cancel download"
        aria-label="Cancel download"
        @click="emit('cancel-download', props.item)"
      >
        <svg viewBox="0 0 24 24" class="track-control__icon" aria-hidden="true" focusable="false">
          <path
            d="M6 6l12 12M18 6L6 18"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </svg>
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

.track-control__glyph {
  font-size: 18px;
  line-height: 1;
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
