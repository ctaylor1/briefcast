<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import UiAlert from "../components/ui/UiAlert.vue";
import UiBadge from "../components/ui/UiBadge.vue";
import UiButton from "../components/ui/UiButton.vue";
import UiCard from "../components/ui/UiCard.vue";
import UiSelect from "../components/ui/UiSelect.vue";
import { episodesApi, getErrorMessage, podcastsApi } from "../lib/api";
import { formatDateTime, formatDuration } from "../lib/format";
import { buildSponsorSegments } from "../lib/sponsor";
import type { Chapter, PodcastItem } from "../types/api";

const route = useRoute();
const isLoading = ref(true);
const errorMessage = ref("");
const items = ref<PodcastItem[]>([]);
const activeIndex = ref(0);
const isPlaying = ref(false);
const playbackRate = ref(1);
const audioRef = ref<HTMLAudioElement | null>(null);
const pendingStart = ref<number | null>(null);
const currentTime = ref(0);
const chapters = ref<Chapter[]>([]);
const lastAutoSkipStart = ref<number | null>(null);

const speedOptions = [
  0.75,
  0.9,
  1,
  1.1,
  1.2,
  1.3,
  1.4,
  1.5,
  1.7,
  2,
  2.2,
  2.5,
  2.7,
];

const activeItem = computed(() => items.value[activeIndex.value] ?? null);
const sponsorSegments = computed(() =>
  buildSponsorSegments(chapters.value, activeItem.value?.Duration),
);
const currentSponsorSegment = computed(() => {
  const time = currentTime.value;
  if (!Number.isFinite(time)) {
    return null;
  }
  return sponsorSegments.value.find((segment) => time >= segment.start && time < segment.end) ?? null;
});
const autoSkipEnabled = computed(() => activeItem.value?.Podcast?.AutoSkipSponsorChapters ?? false);
const currentSource = computed(() => {
  if (!activeItem.value) {
    return "";
  }
  return `/podcastitems/${activeItem.value.ID}/file`;
});

const activeImage = computed(() => {
  if (!activeItem.value) {
    return "";
  }
  return `/podcastitems/${activeItem.value.ID}/image`;
});

function parseItemIds(): string[] {
  const ids: string[] = [];
  const raw = route.query.itemIds;
  if (typeof raw === "string") {
    raw.split(",").forEach((value) => {
      const trimmed = value.trim();
      if (trimmed) {
        ids.push(trimmed);
      }
    });
  }
  if (Array.isArray(raw)) {
    raw.forEach((value) => {
      if (!value) {
        return;
      }
      value.split(",").forEach((inner) => {
        const trimmed = inner.trim();
        if (trimmed) {
          ids.push(trimmed);
        }
      });
    });
  }
  const single = route.query.itemId;
  if (typeof single === "string" && single.trim()) {
    ids.push(single.trim());
  }
  return Array.from(new Set(ids));
}

function parseStartSeconds(): number | null {
  const raw = route.query.start ?? route.query.t;
  if (typeof raw === "string") {
    const value = Number(raw);
    if (!Number.isNaN(value) && value >= 0) {
      return value;
    }
  }
  if (Array.isArray(raw) && raw.length > 0) {
    const value = Number(raw[0]);
    if (!Number.isNaN(value) && value >= 0) {
      return value;
    }
  }
  return null;
}

function currentQueueMatches(itemIds: string[]): boolean {
  if (itemIds.length === 0 || itemIds.length !== items.value.length) {
    return false;
  }
  return itemIds.every((id, index) => items.value[index]?.ID === id);
}

async function seekAndPlay(seconds: number): Promise<void> {
  await seekTo(seconds);
  const audio = audioRef.value;
  if (!audio) {
    return;
  }
  try {
    await audio.play();
  } catch {
    // Autoplay can be blocked until the user interacts with the page.
  }
}

async function loadItems(): Promise<void> {
  isLoading.value = true;
  errorMessage.value = "";
  items.value = [];

  const podcastId = typeof route.query.podcastId === "string" ? route.query.podcastId : "";
  const itemIds = parseItemIds();

  try {
    if (podcastId) {
      const data = await podcastsApi.items(podcastId);
      items.value = [...data].sort((left, right) => {
        const a = Date.parse(left.PubDate);
        const b = Date.parse(right.PubDate);
        if (Number.isNaN(a) || Number.isNaN(b)) {
          return 0;
        }
        return b - a;
      });
    } else if (itemIds.length > 0) {
      const responses = await Promise.all(
        itemIds.map((id) => episodesApi.getById(id).catch(() => null)),
      );
      const loaded = responses.filter(Boolean) as PodcastItem[];
      const order = new Map(itemIds.map((id, index) => [id, index]));
      items.value = [...loaded].sort(
        (left, right) => (order.get(left.ID) ?? 0) - (order.get(right.ID) ?? 0),
      );
    } else {
      errorMessage.value = "No items specified for playback.";
      return;
    }

    if (items.value.length === 0) {
      errorMessage.value = "No items available for playback.";
      return;
    }

    activeIndex.value = 0;
    pendingStart.value = parseStartSeconds();
    lastAutoSkipStart.value = null;
    await nextTick();
    await playAt(0, pendingStart.value ?? undefined);
  } catch (error) {
    errorMessage.value = getErrorMessage(error, "Failed to load items for playback.");
  } finally {
    isLoading.value = false;
  }
}

async function playAt(index: number, startSeconds?: number): Promise<void> {
  if (index < 0 || index >= items.value.length) {
    return;
  }
  activeIndex.value = index;
  await nextTick();
  const audio = audioRef.value;
  if (!audio) {
    return;
  }
  audio.playbackRate = playbackRate.value;
  if (typeof startSeconds === "number" && startSeconds >= 0) {
    await seekTo(startSeconds);
    pendingStart.value = null;
  }
  try {
    await audio.play();
  } catch {
    // Autoplay can be blocked until the user interacts with the page.
  }
}

async function loadChapters(id: string): Promise<void> {
  try {
    const response = await episodesApi.getChapters(id);
    chapters.value = response.chapters ?? [];
  } catch {
    chapters.value = [];
  }
}

async function seekTo(seconds: number): Promise<void> {
  const audio = audioRef.value;
  if (!audio) {
    return;
  }
  const applySeek = () => {
    audio.currentTime = Math.max(0, seconds);
  };
  if (audio.readyState >= 1) {
    applySeek();
    return;
  }
  await new Promise<void>((resolve) => {
    const handle = () => {
      audio.removeEventListener("loadedmetadata", handle);
      resolve();
    };
    audio.addEventListener("loadedmetadata", handle);
  });
  applySeek();
}

function playNext(): void {
  if (activeIndex.value + 1 < items.value.length) {
    void playAt(activeIndex.value + 1);
  }
}

function playPrevious(): void {
  if (activeIndex.value - 1 >= 0) {
    void playAt(activeIndex.value - 1);
  }
}

function togglePlay(): void {
  const audio = audioRef.value;
  if (!audio) {
    return;
  }
  if (audio.paused) {
    void audio.play();
  } else {
    audio.pause();
  }
}

function handleEnded(): void {
  const item = activeItem.value;
  if (item && !item.IsPlayed) {
    item.IsPlayed = true;
    episodesApi.setPlayed(item.ID, true).catch(() => {});
  }
  playNext();
}

function handleTimeUpdate(): void {
  const audio = audioRef.value;
  if (!audio) {
    return;
  }
  currentTime.value = audio.currentTime;
  if (!autoSkipEnabled.value) {
    return;
  }
  const segment = currentSponsorSegment.value;
  if (!segment) {
    lastAutoSkipStart.value = null;
    return;
  }
  if (lastAutoSkipStart.value === segment.start) {
    return;
  }
  lastAutoSkipStart.value = segment.start;
  void seekTo(segment.end);
}

function skipSponsor(): void {
  const segment = currentSponsorSegment.value;
  if (!segment) {
    return;
  }
  void seekTo(segment.end);
}

watch(playbackRate, (rate) => {
  if (audioRef.value) {
    audioRef.value.playbackRate = rate;
  }
});

watch(
  () => activeItem.value?.ID,
  (id) => {
    lastAutoSkipStart.value = null;
    if (id) {
      void loadChapters(id);
    } else {
      chapters.value = [];
    }
  },
  { immediate: true },
);

watch(() => route.fullPath, () => {
  const itemIds = parseItemIds();
  const startSeconds = parseStartSeconds();
  if (startSeconds !== null && currentQueueMatches(itemIds)) {
    void seekAndPlay(startSeconds);
    return;
  }
  void loadItems();
});

onMounted(loadItems);
</script>

<template>
  <section class="player-page stack-4">
    <header class="page-header">
      <h2 class="section-title">Player</h2>
      <p class="section-subtitle">
        Play an entire podcast queue or a custom list of episode IDs with transcript-aware sponsor skipping.
      </p>
    </header>

    <UiAlert v-if="errorMessage" tone="danger">
      {{ errorMessage }}
    </UiAlert>

    <div v-if="isLoading" class="player-skeleton">
      <UiCard padding="lg" class="stack-2">
        <span class="skeleton player-skeleton__line player-skeleton__line--title"></span>
        <span class="skeleton player-skeleton__line"></span>
        <span class="skeleton player-skeleton__line"></span>
      </UiCard>
      <UiCard padding="lg" class="stack-2">
        <span class="skeleton player-skeleton__line"></span>
        <span class="skeleton player-skeleton__line"></span>
      </UiCard>
    </div>

    <template v-else>
      <UiCard v-if="!activeItem" padding="lg" class="empty-state">
        <p class="empty-state__title">Nothing queued for playback</p>
        <p class="empty-state__copy">Open the Episodes screen and send one or more episodes to the player.</p>
      </UiCard>

      <template v-else>
        <UiCard padding="lg" class="stack-3">
          <div class="player-now">
            <img
              :src="activeImage"
              :alt="activeItem.Title"
              class="player-now__image"
            />
            <div class="player-now__content">
              <h3 class="player-now__title">{{ activeItem.Title }}</h3>
              <p class="meta-text">
                {{ activeItem.Podcast?.Title || "Unknown podcast" }} •
                {{ formatDateTime(activeItem.PubDate) }} • {{ formatDuration(activeItem.Duration) }}
              </p>
              <p class="player-now__summary">
                {{ activeItem.Summary || "No summary available." }}
              </p>
              <div class="player-now__badges">
                <UiBadge :tone="isPlaying ? 'success' : 'neutral'">
                  {{ isPlaying ? "Playing" : "Paused" }}
                </UiBadge>
                <UiBadge :tone="autoSkipEnabled ? 'info' : 'neutral'">
                  {{ autoSkipEnabled ? "Auto skip sponsor on" : "Auto skip sponsor off" }}
                </UiBadge>
                <UiBadge v-if="currentSponsorSegment" tone="warning">
                  Sponsor segment detected
                </UiBadge>
              </div>
            </div>
          </div>

          <div class="player-controls">
            <UiButton variant="outline" size="sm" :disabled="activeIndex === 0" @click="playPrevious">
              Previous
            </UiButton>
            <UiButton variant="primary" size="sm" @click="togglePlay">
              {{ isPlaying ? "Pause" : "Play" }}
            </UiButton>
            <UiButton
              variant="outline"
              size="sm"
              @click="playNext"
              :disabled="activeIndex >= items.length - 1"
            >
              Next
            </UiButton>
            <UiButton
              v-if="currentSponsorSegment"
              variant="secondary"
              size="sm"
              @click="skipSponsor"
            >
              Skip sponsor
            </UiButton>
            <UiSelect
              :model-value="playbackRate"
              label="Playback speed"
              class="player-controls__speed"
              @update:model-value="playbackRate = Number($event)"
            >
              <option v-for="speed in speedOptions" :key="speed" :value="speed">
                {{ speed }}x
              </option>
            </UiSelect>
          </div>

          <audio
            ref="audioRef"
            class="player-audio"
            controls
            :src="currentSource"
            @ended="handleEnded"
            @timeupdate="handleTimeUpdate"
            @play="isPlaying = true"
            @pause="isPlaying = false"
          />
        </UiCard>

        <UiCard padding="none">
          <div class="player-queue">
            <button
              v-for="(item, index) in items"
              :key="item.ID"
              type="button"
              class="player-queue__item"
              :class="index === activeIndex ? 'player-queue__item--active' : ''"
              @click="playAt(index)"
            >
              <img
                :src="`/podcastitems/${item.ID}/image`"
                :alt="item.Title"
                class="player-queue__image"
              />
              <div class="player-queue__meta">
                <p class="player-queue__title">{{ item.Title }}</p>
                <p class="meta-text">
                  {{ item.Podcast?.Title || "Unknown podcast" }} • {{ formatDateTime(item.PubDate) }}
                </p>
              </div>
              <span class="meta-text">{{ formatDuration(item.Duration) }}</span>
            </button>
          </div>
        </UiCard>
      </template>
    </template>
  </section>
</template>

<style scoped>
.player-skeleton {
  display: grid;
  gap: var(--space-3);
}

.player-skeleton__line {
  height: 14px;
}

.player-skeleton__line--title {
  width: 58%;
  height: 20px;
}

.player-now {
  display: grid;
  gap: var(--space-3);
  grid-template-columns: 1fr;
}

.player-now__image {
  width: 100%;
  max-width: 170px;
  aspect-ratio: 1 / 1;
  border-radius: var(--radius-3);
  background: var(--color-hover);
  object-fit: cover;
}

.player-now__content {
  min-width: 0;
}

.player-now__title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-section-size);
  line-height: var(--font-section-line-height);
  font-weight: var(--font-section-weight);
}

.player-now__summary {
  margin: var(--space-2) 0 0;
  color: var(--color-text-secondary);
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.player-now__badges {
  margin-top: var(--space-2);
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.player-controls {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: var(--space-2);
}

.player-controls__speed {
  min-width: 140px;
}

.player-audio {
  width: 100%;
}

.player-queue {
  display: grid;
}

.player-queue__item {
  width: 100%;
  min-height: 72px;
  border: 0;
  border-bottom: 1px solid var(--color-border);
  background: transparent;
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  text-align: left;
  cursor: pointer;
}

.player-queue__item:hover {
  background: var(--color-hover);
}

.player-queue__item--active {
  background: var(--color-accent-subtle);
}

.player-queue__image {
  width: 48px;
  height: 48px;
  border-radius: var(--radius-2);
  background: var(--color-hover);
  object-fit: cover;
  flex: 0 0 auto;
}

.player-queue__meta {
  min-width: 0;
  flex: 1;
}

.player-queue__title {
  margin: 0;
  color: var(--color-text-primary);
  font-weight: 600;
}

.empty-state__title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
}

.empty-state__copy {
  margin: var(--space-2) auto 0;
  max-width: 50ch;
}

@media (min-width: 768px) {
  .player-now {
    grid-template-columns: 170px 1fr;
  }
}
</style>
