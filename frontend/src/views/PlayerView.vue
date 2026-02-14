<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import UiAlert from "../components/ui/UiAlert.vue";
import UiButton from "../components/ui/UiButton.vue";
import UiCard from "../components/ui/UiCard.vue";
import UiSelect from "../components/ui/UiSelect.vue";
import { episodesApi, getErrorMessage, podcastsApi } from "../lib/api";
import { formatDateTime, formatDuration } from "../lib/format";
import type { PodcastItem } from "../types/api";

const route = useRoute();
const isLoading = ref(true);
const errorMessage = ref("");
const items = ref<PodcastItem[]>([]);
const activeIndex = ref(0);
const isPlaying = ref(false);
const playbackRate = ref(1);
const audioRef = ref<HTMLAudioElement | null>(null);
const pendingStart = ref<number | null>(null);

const speedOptions = [
  0.75, 0.9, 1, 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8, 1.9, 2,
];

const activeItem = computed(() => items.value[activeIndex.value] ?? null);
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

watch(playbackRate, (rate) => {
  if (audioRef.value) {
    audioRef.value.playbackRate = rate;
  }
});

watch(
  () => route.query,
  () => {
    void loadItems();
  },
  { deep: true },
);

onMounted(loadItems);
</script>

<template>
  <section class="stack-4">
    <div class="stack-2">
      <h1 class="fluid-title-xl font-semibold tracking-tight text-slate-900">Player</h1>
      <p class="fluid-subtle text-slate-600">Queue-based playback for a podcast or selected episodes.</p>
    </div>

    <UiAlert v-if="errorMessage" tone="danger">
      {{ errorMessage }}
    </UiAlert>

    <UiCard v-if="isLoading" padding="lg" class="text-sm text-slate-600">
      Loading player...
    </UiCard>

    <template v-else>
      <UiCard v-if="!activeItem" padding="lg" class="text-sm text-slate-600">
        Nothing to play yet.
      </UiCard>

      <UiCard v-else padding="lg" class="stack-4">
        <div class="flex flex-wrap items-start gap-[var(--space-3)]">
          <img
            :src="activeImage"
            :alt="activeItem.Title"
            class="h-28 w-28 rounded-md bg-slate-100 object-cover"
          />
          <div class="min-w-[220px] flex-1">
            <h2 class="text-lg font-semibold text-slate-900">{{ activeItem.Title }}</h2>
            <p class="text-sm text-slate-600">
              {{ activeItem.Podcast?.Title || "Unknown Podcast" }} •
              {{ formatDateTime(activeItem.PubDate) }} • {{ formatDuration(activeItem.Duration) }}
            </p>
            <p class="mt-2 text-sm text-slate-600 line-clamp-3">
              {{ activeItem.Summary || "No summary available." }}
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <UiButton variant="outline" size="sm" @click="playPrevious" :disabled="activeIndex === 0">
              Prev
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
            <label class="flex items-center gap-2 text-sm text-slate-600">
              Speed
              <UiSelect
                :model-value="playbackRate"
                input-class="min-h-9 w-auto bg-white px-2 py-1 text-sm"
                @update:model-value="playbackRate = Number($event)"
              >
                <option v-for="speed in speedOptions" :key="speed" :value="speed">
                  {{ speed }}x
                </option>
              </UiSelect>
            </label>
          </div>
        </div>

        <audio
          ref="audioRef"
          class="w-full"
          controls
          :src="currentSource"
          @ended="handleEnded"
          @play="isPlaying = true"
          @pause="isPlaying = false"
        />
      </UiCard>

      <UiCard padding="none">
        <div class="divide-y divide-slate-200">
          <button
            v-for="(item, index) in items"
            :key="item.ID"
            type="button"
            class="flex w-full items-start gap-3 px-[var(--space-3)] py-[var(--space-2)] text-left transition hover:bg-slate-50"
            :class="index === activeIndex ? 'bg-slate-100' : ''"
            @click="playAt(index)"
          >
            <img
              :src="`/podcastitems/${item.ID}/image`"
              :alt="item.Title"
              class="h-12 w-12 rounded-md bg-slate-100 object-cover"
            />
            <div class="flex-1">
              <p class="text-sm font-semibold text-slate-900">{{ item.Title }}</p>
              <p class="text-xs text-slate-500">
                {{ item.Podcast?.Title || "Unknown Podcast" }} • {{ formatDateTime(item.PubDate) }}
              </p>
            </div>
            <span class="text-xs text-slate-500">{{ formatDuration(item.Duration) }}</span>
          </button>
        </div>
      </UiCard>
    </template>
  </section>
</template>
