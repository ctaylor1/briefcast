<script setup lang="ts">
import { computed } from "vue";
import { cn } from "../../lib/cn";

type CardPadding = "none" | "sm" | "md" | "lg";
type CardTone = "default" | "subtle";

const props = withDefaults(
  defineProps<{
    padding?: CardPadding;
    tone?: CardTone;
  }>(),
  {
    padding: "md",
    tone: "default",
  },
);

const classes = computed(() =>
  cn(
    "ui-card",
    props.padding === "none" && "ui-card--padding-none",
    props.padding === "sm" && "ui-card--padding-sm",
    props.padding === "md" && "ui-card--padding-md",
    props.padding === "lg" && "ui-card--padding-lg",
    props.tone === "subtle" && "ui-card--subtle",
  ),
);
</script>

<template>
  <div :class="classes">
    <slot />
  </div>
</template>
