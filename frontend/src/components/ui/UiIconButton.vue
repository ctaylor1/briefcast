<script setup lang="ts">
import { computed, useId } from "vue";

// An icon-only button. `label` is required and becomes the accessible name, so an
// icon button can never be silent to a screen reader. Pass `pressed` to make it a
// toggle (aria-pressed); leave it undefined for a plain action.
const props = withDefaults(
  defineProps<{
    label: string;
    tooltip?: string;
    pressed?: boolean;
    disabled?: boolean;
    tone?: "default" | "danger";
    size?: "sm" | "md";
    type?: "button" | "submit";
  }>(),
  {
    tooltip: "",
    pressed: undefined,
    disabled: false,
    tone: "default",
    size: "md",
    type: "button",
  },
);

defineEmits<{
  (event: "click", value: MouseEvent): void;
}>();

const tooltipId = useId();
const tooltipText = computed(() => props.tooltip || props.label);
// The tooltip repeats the label unless it adds something, such as the current state.
const describesMore = computed(() => tooltipText.value !== props.label);
</script>

<template>
  <span class="ui-tooltip-wrap">
    <button
      :type="type"
      class="ui-icon-button"
      :class="[
        `ui-icon-button--${size}`,
        {
          'ui-icon-button--pressed': pressed === true,
          'ui-icon-button--danger': tone === 'danger',
        },
      ]"
      :aria-label="label"
      :aria-pressed="pressed === undefined ? undefined : pressed ? 'true' : 'false'"
      :aria-describedby="describesMore ? tooltipId : undefined"
      :disabled="disabled"
      @click="$emit('click', $event)"
    >
      <slot />
    </button>
    <span
      :id="tooltipId"
      role="tooltip"
      class="ui-tooltip"
      :aria-hidden="describesMore ? undefined : 'true'"
    >{{ tooltipText }}</span>
  </span>
</template>
