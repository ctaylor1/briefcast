<script setup lang="ts">
import { Popover, PopoverButton, PopoverPanel } from "@headlessui/vue";
import { computed } from "vue";
import UiDropdownFit from "./UiDropdownFit.vue";

// A button that opens a panel of actions. Built on Headless UI's Popover, so it closes on
// Escape, on a click outside, and after any button or link inside it is used.
// Mark an element with data-keep-open to leave the panel open when it is clicked.
const props = withDefaults(
  defineProps<{
    label?: string;
    align?: "start" | "end";
    buttonClass?: string;
  }>(),
  {
    label: "More",
    align: "end",
    buttonClass: "ui-button ui-button--ghost ui-button--sm",
  },
);

const menuClasses = computed(() => [
  "ui-dropdown__menu",
  props.align === "start" ? "ui-dropdown__menu--start" : "ui-dropdown__menu--end",
]);

function closeAfterAction(event: MouseEvent, close: () => void): void {
  const target = event.target instanceof Element ? event.target : null;
  const action = target?.closest("button, a");
  if (action && !action.closest("[data-keep-open]")) {
    close();
  }
}
</script>

<template>
  <Popover v-slot="{ open }" class="ui-dropdown">
    <PopoverButton :class="buttonClass">
      <slot name="trigger" :open="open">{{ label }}</slot>
    </PopoverButton>
    <PopoverPanel v-slot="{ close }" :class="menuClasses">
      <UiDropdownFit>
        <div @click="closeAfterAction($event, close)">
          <slot :close="close" />
        </div>
      </UiDropdownFit>
    </PopoverPanel>
  </Popover>
</template>
