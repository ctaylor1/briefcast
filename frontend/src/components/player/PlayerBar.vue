<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { RouterLink } from "vue-router";
import { usePlayer } from "../../composables/usePlayer";
import { episodeImageUrl } from "../../lib/api/paths";
import { formatClock } from "../../lib/format";
import UiIcon from "../ui/UiIcon.vue";
import UiIconButton from "../ui/UiIconButton.vue";
import PlayerControls from "./PlayerControls.vue";
import PlayerScrubber from "./PlayerScrubber.vue";
import PlayerSpeedMenu from "./PlayerSpeedMenu.vue";

// The app-wide player. The <audio> element is always mounted here, so playback survives
// navigation; the bar itself shows once something is queued.
const player = usePlayer();
const audioEl = ref<HTMLAudioElement | null>(null);
let detach: (() => void) | null = null;

onMounted(() => {
  if (audioEl.value) {
    detach = player.attach(audioEl.value);
  }
});

onBeforeUnmount(() => detach?.());

const item = computed(() => player.current.value);
const remaining = computed(() => Math.max(0, player.state.duration - player.state.currentTime));
const progressPercent = computed(() =>
  player.state.duration > 0 ? Math.min(100, (player.state.currentTime / player.state.duration) * 100) : 0,
);
</script>

<template>
  <audio ref="audioEl" class="player-audio" preload="metadata" />
  <section v-if="item" class="player-bar" aria-label="Player">
    <div class="player-bar__mobile-progress" aria-hidden="true">
      <span :style="{ width: `${progressPercent}%` }" />
    </div>

    <RouterLink to="/player" class="player-bar__info" :title="`Open now playing: ${item.Title}`">
      <img :src="episodeImageUrl(item.ID)" alt="" class="player-bar__cover" />
      <span class="player-bar__text">
        <span class="player-bar__title">{{ item.Title }}</span>
        <span class="player-bar__podcast">{{ item.Podcast?.Title }}</span>
      </span>
    </RouterLink>

    <div class="player-bar__center">
      <PlayerControls />
      <div class="player-bar__timeline">
        <span class="player-bar__time">{{ formatClock(player.state.currentTime) }}</span>
        <PlayerScrubber
          :current-time="player.state.currentTime"
          :duration="player.state.duration"
          :chapters="player.state.chapters"
          :sponsor-segments="player.sponsorSegments.value"
          @seek="player.seek"
        />
        <span class="player-bar__time">-{{ formatClock(remaining) }}</span>
      </div>
    </div>

    <div class="player-bar__side">
      <UiIconButton
        v-if="player.currentSponsorSegment.value"
        label="Skip sponsor segment"
        :tooltip="`Skip: ${player.currentSponsorSegment.value.title}`"
        @click="player.skipSponsor()"
      >
        <UiIcon name="skip-sponsor" />
      </UiIconButton>
      <PlayerSpeedMenu />
      <button
        type="button"
        class="player-bar__mobile-play"
        :aria-label="player.state.playing ? 'Pause' : 'Play'"
        @click="player.toggle()"
      >
        <UiIcon :name="player.state.playing ? 'pause' : 'play'" :size="22" />
      </button>
      <RouterLink to="/player" class="player-bar__expand ui-button ui-button--ghost ui-button--sm" aria-label="Open now playing">
        <UiIcon name="expand" />
      </RouterLink>
    </div>

    <p v-if="player.state.error || player.state.autoplayBlocked" class="player-bar__notice" role="status">
      {{ player.state.error || "Press play to start listening." }}
    </p>
  </section>
</template>
