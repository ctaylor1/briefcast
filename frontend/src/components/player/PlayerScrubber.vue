<script setup lang="ts">
import { computed, ref } from "vue";
import { describeClock } from "../../lib/format";
import type { SponsorSegment } from "../../lib/sponsor";
import type { Chapter } from "../../types/api";

// A native range input, so keyboard and screen readers work out of the box.
// While dragging it previews the position; the seek happens on release.
const props = withDefaults(
  defineProps<{
    currentTime: number;
    duration: number;
    chapters?: Chapter[];
    sponsorSegments?: SponsorSegment[];
    label?: string;
  }>(),
  {
    chapters: () => [],
    sponsorSegments: () => [],
    label: "Seek",
  },
);

const emit = defineEmits<{
  (event: "seek", seconds: number): void;
}>();

const dragging = ref<number | null>(null);
const max = computed(() => (props.duration > 0 ? Math.floor(props.duration) : 0));
const value = computed(() => Math.min(max.value, Math.floor(dragging.value ?? props.currentTime)));
const percent = computed(() => (max.value > 0 ? (value.value / max.value) * 100 : 0));

function share(seconds: number): string {
  return `${Math.min(100, Math.max(0, (seconds / max.value) * 100))}%`;
}

const chapterMarks = computed(() =>
  max.value > 0
    ? props.chapters
        .filter((chapter) => chapter.startSeconds > 0 && chapter.startSeconds < max.value)
        .map((chapter) => ({ key: `${chapter.startSeconds}-${chapter.title}`, left: share(chapter.startSeconds), title: chapter.title }))
    : [],
);

const sponsorBands = computed(() =>
  max.value > 0
    ? props.sponsorSegments.map((segment) => ({
        key: `${segment.start}-${segment.end}`,
        left: share(segment.start),
        width: `${Math.max(0, ((segment.end - segment.start) / max.value) * 100)}%`,
      }))
    : [],
);

const valueText = computed(() =>
  max.value > 0 ? `${describeClock(value.value)} of ${describeClock(max.value)}` : "Duration unknown",
);

function onInput(event: Event): void {
  dragging.value = Number((event.target as HTMLInputElement).value);
}

function onChange(event: Event): void {
  const seconds = Number((event.target as HTMLInputElement).value);
  dragging.value = null;
  emit("seek", seconds);
}
</script>

<template>
  <div class="player-scrubber" :style="{ '--scrubber-fill': `${percent}%` }">
    <div class="player-scrubber__track" aria-hidden="true">
      <span
        v-for="band in sponsorBands"
        :key="band.key"
        class="player-scrubber__sponsor"
        :style="{ left: band.left, width: band.width }"
      />
      <span class="player-scrubber__fill" />
      <span
        v-for="mark in chapterMarks"
        :key="mark.key"
        class="player-scrubber__chapter"
        :style="{ left: mark.left }"
        :title="mark.title"
      />
    </div>
    <input
      class="player-scrubber__input"
      type="range"
      min="0"
      :max="max"
      step="1"
      :value="value"
      :disabled="max === 0"
      :aria-label="label"
      :aria-valuetext="valueText"
      @input="onInput"
      @change="onChange"
    />
  </div>
</template>

<style scoped>
.player-scrubber {
  position: relative;
  flex: 1 1 auto;
  height: 20px;
  display: flex;
  align-items: center;
}

.player-scrubber__track {
  position: absolute;
  left: 0;
  right: 0;
  height: 4px;
  border-radius: 999px;
  background: var(--color-hover);
  overflow: hidden;
  pointer-events: none;
}

.player-scrubber__fill {
  position: absolute;
  inset: 0 auto 0 0;
  width: var(--scrubber-fill);
  background: var(--color-accent);
}

.player-scrubber__sponsor {
  position: absolute;
  top: 0;
  bottom: 0;
  background: color-mix(in srgb, var(--color-warning) 55%, transparent);
}

.player-scrubber__chapter {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 2px;
  margin-left: -1px;
  background: var(--color-bg-primary);
}

.player-scrubber__input {
  position: relative;
  width: 100%;
  height: 20px;
  margin: 0;
  background: transparent;
  appearance: none;
  -webkit-appearance: none;
  cursor: pointer;
}

.player-scrubber__input:disabled {
  cursor: default;
}

.player-scrubber__input::-webkit-slider-runnable-track {
  height: 4px;
  background: transparent;
}

.player-scrubber__input::-moz-range-track {
  height: 4px;
  background: transparent;
}

.player-scrubber__input::-webkit-slider-thumb {
  -webkit-appearance: none;
  width: 14px;
  height: 14px;
  margin-top: -5px;
  border: 2px solid var(--color-bg-primary);
  border-radius: 50%;
  background: var(--color-accent);
}

.player-scrubber__input::-moz-range-thumb {
  width: 14px;
  height: 14px;
  border: 2px solid var(--color-bg-primary);
  border-radius: 50%;
  background: var(--color-accent);
}

.player-scrubber__input:disabled::-webkit-slider-thumb {
  visibility: hidden;
}

.player-scrubber__input:disabled::-moz-range-thumb {
  visibility: hidden;
}

.player-scrubber__input:focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
  border-radius: var(--radius-1);
}
</style>
