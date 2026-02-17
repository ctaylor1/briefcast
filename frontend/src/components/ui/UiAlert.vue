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
    "ui-alert",
    props.tone === "info" && "ui-alert--info",
    props.tone === "success" && "ui-alert--success",
    props.tone === "danger" && "ui-alert--danger",
    props.tone === "warning" && "ui-alert--warning",
  ),
);
</script>

<template>
  <div role="status" :class="classes">
    <slot />
  </div>
</template>
