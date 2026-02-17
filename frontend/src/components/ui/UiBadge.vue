<script setup lang="ts">
import { computed } from "vue";
import { cn } from "../../lib/cn";

type BadgeTone = "neutral" | "info" | "success" | "danger" | "warning";

const props = withDefaults(
  defineProps<{
    tone?: BadgeTone;
  }>(),
  {
    tone: "neutral",
  },
);

const classes = computed(() =>
  cn(
    "ui-badge",
    props.tone === "neutral" && "ui-badge--neutral",
    props.tone === "info" && "ui-badge--info",
    props.tone === "success" && "ui-badge--success",
    props.tone === "danger" && "ui-badge--danger",
    props.tone === "warning" && "ui-badge--warning",
  ),
);
</script>

<template>
  <span :class="classes">
    <slot />
  </span>
</template>
