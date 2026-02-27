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

const liveRole = computed(() =>
  props.tone === "danger" || props.tone === "warning" ? "alert" : "status",
);
</script>

<template>
  <div :role="liveRole" aria-atomic="true" :class="classes">
    <slot />
  </div>
</template>
