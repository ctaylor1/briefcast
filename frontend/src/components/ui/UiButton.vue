<script setup lang="ts">
import { computed } from "vue";
import { cn } from "../../lib/cn";

type ButtonVariant = "primary" | "secondary" | "outline" | "danger" | "ghost";
type ButtonSize = "sm" | "md" | "lg";

const props = withDefaults(
  defineProps<{
    type?: "button" | "submit" | "reset";
    variant?: ButtonVariant;
    size?: ButtonSize;
    disabled?: boolean;
    block?: boolean;
  }>(),
  {
    type: "button",
    variant: "primary",
    size: "md",
    disabled: false,
    block: false,
  },
);

const classes = computed(() =>
  cn(
    "ui-button",
    `ui-button--${props.size}`,
    `ui-button--${props.variant}`,
    props.block && "ui-button--block",
  ),
);
</script>

<template>
  <button :type="type" :disabled="disabled" :class="classes">
    <slot />
  </button>
</template>
