<script setup lang="ts">
import { SPEED_OPTIONS, usePlayer } from "../../composables/usePlayer";
import UiDropdown from "../ui/UiDropdown.vue";
import UiIcon from "../ui/UiIcon.vue";

const player = usePlayer();

function label(rate: number): string {
  return `${rate}×`;
}
</script>

<template>
  <UiDropdown align="end" button-class="ui-button ui-button--ghost ui-button--sm player-speed__trigger">
    <template #trigger>
      <span aria-hidden="true">{{ label(player.state.rate) }}</span>
      <span class="sr-only">Playback speed {{ player.state.rate }} times</span>
    </template>
    <div class="player-speed__menu" role="group" aria-label="Playback speed">
      <button
        v-for="rate in SPEED_OPTIONS"
        :key="rate"
        type="button"
        class="player-speed__option"
        :aria-pressed="player.state.rate === rate ? 'true' : 'false'"
        @click="player.setRate(rate)"
      >
        <span>{{ label(rate) }}</span>
        <UiIcon v-if="player.state.rate === rate" name="check" :size="16" />
      </button>
    </div>
  </UiDropdown>
</template>

<style scoped>
.player-speed__menu {
  display: grid;
  min-width: 96px;
}

.player-speed__option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  min-height: var(--control-height-sm);
  padding: 0 var(--space-3);
  border: 0;
  border-radius: var(--radius-2);
  background: transparent;
  color: var(--color-text-primary);
  font: inherit;
  cursor: pointer;
}

.player-speed__option:hover,
.player-speed__option[aria-pressed="true"] {
  background: var(--color-hover);
}
</style>
