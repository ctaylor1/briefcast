<script setup lang="ts">
import { SKIP_BACK_SECONDS, SKIP_FORWARD_SECONDS, usePlayer } from "../../composables/usePlayer";
import UiIcon from "../ui/UiIcon.vue";
import UiIconButton from "../ui/UiIconButton.vue";

withDefaults(defineProps<{ size?: "md" | "lg" }>(), { size: "md" });

const player = usePlayer();
</script>

<template>
  <div class="player-controls" :class="`player-controls--${size}`" role="group" aria-label="Playback">
    <UiIconButton
      label="Previous episode"
      tooltip="Previous (or restart)"
      class="player-controls__secondary"
      @click="player.previous()"
    >
      <UiIcon name="previous" />
    </UiIconButton>
    <UiIconButton :label="`Back ${SKIP_BACK_SECONDS} seconds`" @click="player.skip(-SKIP_BACK_SECONDS)">
      <span class="player-controls__skip">
        <UiIcon name="skip-back" />
        <span class="player-controls__skip-number" aria-hidden="true">{{ SKIP_BACK_SECONDS }}</span>
      </span>
    </UiIconButton>
    <button
      type="button"
      class="player-controls__play"
      :aria-label="player.state.playing ? 'Pause' : 'Play'"
      @click="player.toggle()"
    >
      <UiIcon :name="player.state.playing ? 'pause' : 'play'" :size="size === 'lg' ? 28 : 20" />
    </button>
    <UiIconButton :label="`Forward ${SKIP_FORWARD_SECONDS} seconds`" @click="player.skip(SKIP_FORWARD_SECONDS)">
      <span class="player-controls__skip">
        <UiIcon name="skip-forward" />
        <span class="player-controls__skip-number" aria-hidden="true">{{ SKIP_FORWARD_SECONDS }}</span>
      </span>
    </UiIconButton>
    <UiIconButton
      label="Next episode"
      class="player-controls__secondary"
      :disabled="!player.hasNext.value"
      @click="player.next()"
    >
      <UiIcon name="next" />
    </UiIconButton>
  </div>
</template>

<style scoped>
.player-controls {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--space-1);
}

.player-controls :deep(.ui-icon-button) {
  border-color: transparent;
  background: transparent;
}

.player-controls__skip {
  position: relative;
  display: inline-flex;
}

.player-controls__skip-number {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  font-size: 7px;
  font-weight: 700;
  line-height: 1;
  padding-top: 1px;
}

.player-controls__play {
  width: 40px;
  height: 40px;
  border: 0;
  border-radius: 50%;
  background: var(--color-accent);
  color: var(--color-on-solid);
  display: inline-grid;
  place-items: center;
  cursor: pointer;
  margin-inline: var(--space-1);
}

.player-controls__play:hover {
  background: var(--color-accent-hover);
}

.player-controls--lg .player-controls__play {
  width: 56px;
  height: 56px;
}

.player-controls--lg {
  gap: var(--space-3);
}
</style>
