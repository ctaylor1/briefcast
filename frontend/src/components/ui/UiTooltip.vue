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
    <span v-if="text" role="tooltip" class="ui-tooltip">{{ text }}</span>
  </span>
</template>
