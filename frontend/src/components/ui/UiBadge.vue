<script setup lang="ts">
import { computed } from "vue";
import { cn } from "../../lib/cn";

type BadgeTone = "neutral" | "info" | "success" | "danger";

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
    "inline-flex items-center rounded-full px-2 py-1 text-xs font-medium",
    props.tone === "neutral" && "bg-slate-100 text-slate-700",
    props.tone === "info" && "bg-cyan-100 text-cyan-800",
    props.tone === "success" && "bg-emerald-100 text-emerald-800",
    props.tone === "danger" && "bg-rose-100 text-rose-800",
  ),
);
</script>

<template>
  <span :class="classes">
    <slot />
  </span>
</template>
