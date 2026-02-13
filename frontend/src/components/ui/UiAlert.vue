<script setup lang="ts">
import { computed } from "vue";
import { cn } from "../../lib/cn";

type AlertTone = "info" | "success" | "danger" | "warning";

const props = withDefaults(
  defineProps<{
    tone?: AlertTone;
  }>(),
  {
    tone: "info",
  },
);

const classes = computed(() =>
  cn(
    "rounded-md border px-4 py-3 text-sm",
    props.tone === "info" && "border-cyan-300 bg-cyan-50 text-cyan-800",
    props.tone === "success" && "border-emerald-300 bg-emerald-50 text-emerald-800",
    props.tone === "danger" && "border-rose-300 bg-rose-50 text-rose-800",
    props.tone === "warning" && "border-amber-300 bg-amber-50 text-amber-800",
  ),
);
</script>

<template>
  <div role="status" :class="classes">
    <slot />
  </div>
</template>
