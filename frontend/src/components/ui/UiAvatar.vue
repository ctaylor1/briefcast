<script setup lang="ts">
import { computed } from "vue";
import { cn } from "../../lib/cn";

const props = withDefaults(
  defineProps<{
    src?: string;
    alt?: string;
    fallback?: string;
    size?: "sm" | "md" | "lg";
  }>(),
  {
    src: "",
    alt: "Avatar",
    fallback: "",
    size: "md",
  },
);

const initials = computed(() => {
  const trimmed = props.fallback.trim();
  if (trimmed) {
    return trimmed.slice(0, 2).toUpperCase();
  }
  return "?";
});

const classes = computed(() => cn("ui-avatar", `ui-avatar--${props.size}`));
</script>

<template>
  <span :class="classes" role="img" :aria-label="alt">
    <img v-if="src" :src="src" :alt="alt" loading="lazy" />
    <span v-else>{{ initials }}</span>
  </span>
</template>
