<script setup lang="ts">
import { computed } from "vue";
import { cn } from "../../lib/cn";

const props = withDefaults(
  defineProps<{
    label?: string;
    align?: "start" | "end";
  }>(),
  {
    label: "More",
    align: "end",
  },
);

const menuClasses = computed(() =>
  cn(
    "ui-dropdown__menu",
    props.align === "start" ? "ui-dropdown__menu--start" : "ui-dropdown__menu--end",
  ),
);
</script>

<template>
  <details class="ui-dropdown">
    <summary>
      <slot name="trigger">
        <button type="button" class="ui-button ui-button--ghost ui-button--sm">
          {{ label }}
        </button>
      </slot>
    </summary>
    <div :class="menuClasses">
      <slot />
    </div>
  </details>
</template>
