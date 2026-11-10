<script setup lang="ts">
withDefaults(
  defineProps<{
    text: string;
  }>(),
  {
    text: "",
  },
);
</script>

<template>
  <span class="ui-tooltip-wrap">
    <slot />
    <!-- Visual hint only: the wrapped control carries its own accessible name. -->
    <span v-if="text" class="ui-tooltip" aria-hidden="true">{{ text }}</span>
  </span>
</template>
