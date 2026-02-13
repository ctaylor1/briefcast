<script setup lang="ts">
import { computed } from "vue";
import { cn } from "../../lib/cn";

type CardPadding = "none" | "sm" | "md" | "lg";

const props = withDefaults(
  defineProps<{
    padding?: CardPadding;
    elevated?: boolean;
  }>(),
  {
    padding: "md",
    elevated: true,
  },
);

const classes = computed(() =>
  cn(
    "rounded-[var(--radius-lg)] border border-slate-200 bg-white",
    props.elevated && "shadow-sm",
    props.padding === "none" && "p-0",
    props.padding === "sm" && "p-[var(--space-2)]",
    props.padding === "md" && "p-[var(--space-3)]",
    props.padding === "lg" && "p-[var(--space-4)]",
  ),
);
</script>

<template>
  <div :class="classes">
    <slot />
  </div>
</template>
