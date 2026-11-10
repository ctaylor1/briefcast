<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import PlayerControls from "../components/player/PlayerControls.vue";
import PlayerScrubber from "../components/player/PlayerScrubber.vue";
import PlayerSpeedMenu from "../components/player/PlayerSpeedMenu.vue";
import UiAlert from "../components/ui/UiAlert.vue";
import UiBadge from "../components/ui/UiBadge.vue";
import UiButton from "../components/ui/UiButton.vue";
import UiCard from "../components/ui/UiCard.vue";
import UiIcon from "../components/ui/UiIcon.vue";
import UiIconButton from "../components/ui/UiIconButton.vue";
import UiTabs from "../components/ui/UiTabs.vue";
import { usePlayer } from "../composables/usePlayer";
import { episodeImageUrl } from "../lib/api/paths";
import { formatClock, formatDateTime, formatDuration } from "../lib/format";
import { episodesApi, getErrorMessage } from "../lib/api";
import { isRecordId, parseRecordIds } from "../lib/ids";
import { createLatest } from "../lib/latest";
import { sanitizeHtml } from "../lib/sanitize";
import { activeSegmentIndex, parseTranscript, type TranscriptSegment } from "../lib/transcript";

// "Now playing": the expanded view of the persistent player. Old links that carried a
// queue in the URL (?itemIds=, ?podcastId=, ?start=) still work: they load that queue.
const route = useRoute();
const router = useRouter();
const player = usePlayer();

const item = computed(() => player.current.value);
const upNext = computed(() =>
  player.state.queue.map((entry, index) => ({ entry, index })).filter(({ index }) => index !== player.state.index),
);

type Section = "chapters" | "transcript" | "notes";
const sections = [
  { id: "chapters", label: "Chapters" },
  { id: "transcript", label: "Transcript" },
  { id: "notes", label: "Show notes" },
];
const section = ref<Section>("chapters");

const activeChapter = computed(() => {
  let index = -1;
  player.state.chapters.forEach((chapter, i) => {
    if (player.state.currentTime >= chapter.startSeconds) {
      index = i;
    }
  });
  return index;
});

// Transcript: fetched when its tab is first opened for an episode.
const transcriptRequest = createLatest();
const transcriptFor = ref("");
const transcriptLoading = ref(false);
const transcriptError = ref("");
const segments = ref<TranscriptSegment[]>([]);
const transcriptText = ref("");
const followAlong = ref(true);
const activeSegment = computed(() => activeSegmentIndex(segments.value, player.state.currentTime));
const transcriptList = ref<HTMLElement | null>(null);

async function loadTranscript(id: string): Promise<void> {
  transcriptFor.value = id;
  transcriptLoading.value = true;
  transcriptError.value = "";
  segments.value = [];
  transcriptText.value = "";
  try {
    const result = await transcriptRequest.run(() => episodesApi.getTranscript(id));
    if (result.stale) {
      return;
    }
    const parsed = parseTranscript(result.value);
    segments.value = parsed.segments;
    transcriptText.value = parsed.text;
  } catch (error) {
    transcriptError.value = getErrorMessage(error, "Failed to load the transcript.");
  } finally {
    if (transcriptFor.value === id) {
      transcriptLoading.value = false;
    }
  }
}

watch([section, () => item.value?.ID], ([open, id]) => {
  if (open === "transcript" && id && transcriptFor.value !== id) {
    void loadTranscript(id);
  }
});

watch(activeSegment, async (index) => {
  if (!followAlong.value || section.value !== "transcript" || index < 0) {
    return;
  }
  await nextTick();
  const row = transcriptList.value?.querySelector<HTMLElement>(`[data-segment="${index}"]`);
  row?.scrollIntoView?.({ block: "nearest" });
});

const showNotes = computed(() => (item.value?.Summary ? sanitizeHtml(item.value.Summary) : ""));

function startSeconds(): number | undefined {
  const raw = route.query.start ?? route.query.t;
  const value = Number(Array.isArray(raw) ? raw[0] : raw);
  return Number.isFinite(value) && value >= 0 ? value : undefined;
}

async function loadFromQuery(): Promise<void> {
  const podcastId = route.query.podcastId;
  // Only well-formed record IDs are accepted from the URL; anything else is ignored.
  const itemIds = parseRecordIds(route.query.itemIds, route.query.itemId);
  if (typeof podcastId === "string" && isRecordId(podcastId)) {
    await player.playPodcast(podcastId);
  } else if (itemIds.length > 0) {
    const start = startSeconds();
    const sameQueue =
      itemIds.length === player.state.queue.length && itemIds.every((id, i) => player.state.queue[i]?.ID === id);
    if (sameQueue && start !== undefined) {
      player.seek(start);
      await player.play();
    } else {
      await player.playEpisodeIds(itemIds, start);
    }
  } else {
    return;
  }
  // The queue now lives in the player; drop it from the URL so Back does not reload it.
  if (route.path === "/player") {
    void router.replace({ path: "/player" });
  }
}

watch(
  () => route.query,
  (query) => {
    if (Object.keys(query).length > 0) {
      void loadFromQuery();
    }
  },
);

onMounted(() => {
  void loadFromQuery();
});
</script>

<template>
  <section class="now-playing stack-4">
    <header class="page-header">
      <h2 class="section-title">Now playing</h2>
    </header>

    <UiAlert v-if="player.state.error" tone="danger">
      {{ player.state.error }}
    </UiAlert>

    <UiCard v-if="player.state.loadingQueue && !item" padding="lg" class="stack-2">
      <span class="skeleton now-playing__skeleton now-playing__skeleton--title"></span>
      <span class="skeleton now-playing__skeleton"></span>
    </UiCard>

    <UiCard v-else-if="!item" padding="lg" class="empty-state">
      <p class="empty-state__title">Nothing queued for playback</p>
      <p class="empty-state__copy">
        Press play on any episode in <RouterLink to="/episodes">Episodes</RouterLink> and it keeps playing while you
        browse.
      </p>
    </UiCard>

    <template v-else>
      <UiCard padding="lg" class="now-playing__main">
        <img :src="episodeImageUrl(item.ID)" alt="" class="now-playing__cover" />
        <div class="now-playing__body stack-3">
          <div class="stack-1">
            <h3 class="now-playing__title">{{ item.Title }}</h3>
            <p class="meta-text">
              {{ item.Podcast?.Title || "Unknown podcast" }} · {{ formatDateTime(item.PubDate) }}
            </p>
          </div>
          <div class="now-playing__timeline">
            <span class="player-bar__time">{{ formatClock(player.state.currentTime) }}</span>
            <PlayerScrubber
              :current-time="player.state.currentTime"
              :duration="player.state.duration"
              :chapters="player.state.chapters"
              :sponsor-segments="player.sponsorSegments.value"
              @seek="player.seek"
            />
            <span class="player-bar__time">{{ formatClock(player.state.duration) }}</span>
          </div>
          <PlayerControls size="lg" />
          <div class="now-playing__extras">
            <PlayerSpeedMenu />
            <UiButton
              v-if="player.currentSponsorSegment.value"
              size="sm"
              variant="secondary"
              @click="player.skipSponsor()"
            >
              Skip sponsor
            </UiButton>
            <UiBadge :tone="player.autoSkipEnabled.value ? 'info' : 'neutral'">
              {{ player.autoSkipEnabled.value ? "Sponsor auto-skip on" : "Sponsor auto-skip off" }}
            </UiBadge>
          </div>
        </div>
      </UiCard>

      <UiCard padding="md">
        <UiTabs v-model="section" :tabs="sections" label="Episode details">
          <template #default="{ active }">
            <template v-if="active === 'chapters'">
              <p v-if="player.state.chapters.length === 0" class="meta-text">This episode has no chapters.</p>
              <ol v-else class="now-playing__chapters">
                <li v-for="(chapter, index) in player.state.chapters" :key="`${chapter.startSeconds}-${chapter.title}`">
                  <button
                    type="button"
                    class="now-playing__row"
                    :aria-current="index === activeChapter ? 'true' : undefined"
                    @click="player.seek(chapter.startSeconds)"
                  >
                    <span class="now-playing__stamp">{{ formatClock(chapter.startSeconds) }}</span>
                    <span>{{ chapter.title }}</span>
                  </button>
                </li>
              </ol>
            </template>

            <template v-else-if="active === 'transcript'">
              <p v-if="transcriptLoading" class="meta-text">Loading the transcript...</p>
              <UiAlert v-else-if="transcriptError" tone="danger">{{ transcriptError }}</UiAlert>
              <template v-else-if="segments.length > 0">
                <label class="now-playing__follow">
                  <input v-model="followAlong" type="checkbox" />
                  Follow along
                </label>
                <ol ref="transcriptList" class="now-playing__transcript visually-scrollable">
                  <li v-for="(segment, index) in segments" :key="index" :data-segment="index">
                    <button
                      type="button"
                      class="now-playing__row"
                      :aria-current="index === activeSegment ? 'true' : undefined"
                      @click="player.seek(segment.start)"
                    >
                      <span class="now-playing__stamp">{{ formatClock(segment.start) }}</span>
                      <span><strong v-if="segment.speaker" class="now-playing__speaker">{{ segment.speaker }}: </strong>{{ segment.text }}</span>
                    </button>
                  </li>
                </ol>
              </template>
              <p v-else-if="transcriptText" class="now-playing__transcript-text">{{ transcriptText }}</p>
              <p v-else class="meta-text">No transcript yet.</p>
            </template>

            <template v-else>
              <!-- Feed HTML, sanitized with DOMPurify before rendering. -->
              <div v-if="showNotes" class="now-playing__notes" v-html="showNotes"></div>
              <p v-else class="meta-text">No show notes.</p>
            </template>
          </template>
        </UiTabs>
      </UiCard>

      <UiCard padding="md" class="stack-2">
        <h3 class="settings-section-title">Up next</h3>
        <p v-if="upNext.length === 0" class="meta-text">Nothing else is queued.</p>
        <ol v-else class="now-playing__queue">
          <li v-for="{ entry, index } in upNext" :key="entry.ID" class="now-playing__queue-item">
            <button type="button" class="now-playing__queue-play" @click="player.playFromQueue(index)">
              <img :src="episodeImageUrl(entry.ID)" alt="" class="now-playing__queue-cover" />
              <span class="now-playing__queue-text">
                <span class="now-playing__queue-title">{{ entry.Title }}</span>
                <span class="meta-text">{{ entry.Podcast?.Title || "Unknown podcast" }} · {{ formatDuration(entry.Duration) }}</span>
              </span>
            </button>
            <UiIconButton size="sm" :label="`Remove ${entry.Title} from the queue`" tooltip="Remove from queue" @click="player.removeFromQueue(index)">
              <UiIcon name="close" :size="16" />
            </UiIconButton>
          </li>
        </ol>
      </UiCard>
    </template>
  </section>
</template>

<style scoped>
.now-playing__main {
  display: grid;
  gap: var(--space-6);
}

.now-playing__cover {
  width: min(100%, 280px);
  aspect-ratio: 1 / 1;
  border-radius: var(--radius-4);
  object-fit: cover;
  background: var(--color-hover);
  justify-self: center;
}

.now-playing__title {
  margin: 0;
  font-size: var(--font-section-size);
  line-height: var(--font-section-line-height);
  font-weight: var(--font-section-weight);
}

.now-playing__timeline {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.now-playing__extras {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: var(--space-2);
}

.now-playing__queue {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: var(--space-1);
}

.now-playing__queue-item {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.now-playing__queue-play {
  flex: 1 1 auto;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2);
  border: 0;
  border-radius: var(--radius-2);
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.now-playing__queue-play:hover {
  background: var(--color-hover);
}

.now-playing__queue-cover {
  width: 40px;
  height: 40px;
  flex: 0 0 auto;
  border-radius: var(--radius-2);
  object-fit: cover;
  background: var(--color-hover);
}

.now-playing__queue-text {
  display: grid;
  min-width: 0;
}

.now-playing__queue-title {
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  font-weight: 600;
}

.now-playing__skeleton {
  display: block;
  height: 14px;
  width: 60%;
}

.now-playing__skeleton--title {
  height: 22px;
  width: 80%;
}

@media (min-width: 768px) {
  .now-playing__main {
    grid-template-columns: 240px minmax(0, 1fr);
    align-items: center;
  }
}

.now-playing__chapters,
.now-playing__transcript {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: 2px;
}

.now-playing__transcript {
  max-height: 420px;
  overflow: auto;
}

.now-playing__row {
  width: 100%;
  display: grid;
  grid-template-columns: 56px minmax(0, 1fr);
  gap: var(--space-3);
  align-items: baseline;
  padding: var(--space-2);
  border: 0;
  border-radius: var(--radius-2);
  background: transparent;
  color: var(--color-text-primary);
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.now-playing__row:hover {
  background: var(--color-hover);
}

.now-playing__row[aria-current="true"] {
  background: var(--color-accent-subtle);
}

.now-playing__stamp {
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
  font-variant-numeric: tabular-nums;
}

.now-playing__speaker {
  font-weight: 600;
}

.now-playing__follow {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  margin-bottom: var(--space-2);
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
}

.now-playing__transcript-text {
  white-space: pre-wrap;
}

.now-playing__notes {
  overflow-wrap: anywhere;
}

.now-playing__notes :deep(a) {
  color: var(--color-accent);
}
</style>
