<script setup lang="ts">
import { computed } from "vue";
import { ICONS, type IconDefinition, type IconName } from "../../lib/icons";

// Decorative by default (aria-hidden). Give the surrounding control a label instead.
const props = withDefaults(
  defineProps<{
    name: IconName;
    size?: number;
    strokeWidth?: number;
  }>(),
  {
    size: 18,
    strokeWidth: 1.75,
  },
);

const icon = computed<IconDefinition>(() => ICONS[props.name]);
</script>

<template>
  <svg
    class="ui-icon"
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    :fill="icon.fill ? 'currentColor' : 'none'"
    :stroke="icon.fill ? 'none' : 'currentColor'"
    :stroke-width="strokeWidth"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
    focusable="false"
  >
    <path v-for="d in icon.paths" :key="d" :d="d" />
  </svg>
</template>
