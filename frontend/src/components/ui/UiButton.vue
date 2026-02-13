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
    "inline-flex items-center justify-center rounded-md font-semibold transition-colors",
    "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500 focus-visible:ring-offset-2",
    "disabled:cursor-not-allowed disabled:opacity-50",
    props.size === "sm" && "px-2.5 py-1.5 text-xs",
    props.size === "md" && "px-3.5 py-2 text-sm",
    props.size === "lg" && "px-4 py-2.5 text-sm",
    props.variant === "primary" && "bg-slate-900 text-white hover:bg-slate-800",
    props.variant === "secondary" && "bg-slate-100 text-slate-900 hover:bg-slate-200",
    props.variant === "outline" && "border border-slate-300 text-slate-700 hover:bg-slate-100",
    props.variant === "danger" && "border border-rose-300 text-rose-700 hover:bg-rose-50",
    props.variant === "ghost" && "text-slate-700 hover:bg-slate-100",
    props.block && "w-full",
  ),
);
</script>

<template>
  <button :type="type" :disabled="disabled" :class="classes">
    <slot />
  </button>
</template>
