<script setup lang="ts">
import { nextTick, onMounted, ref } from "vue";

// Wraps a dropdown panel and keeps it on screen: it shifts sideways when it would cross the
// viewport edge, and opens upwards when there is no room below but there is above.
const MARGIN = 8;
const root = ref<HTMLElement | null>(null);

function fit(): void {
  const panel = root.value?.parentElement;
  if (!panel) {
    return;
  }
  panel.style.transform = "";
  panel.classList.remove("ui-dropdown__menu--up");
  const rect = panel.getBoundingClientRect();
  const viewportWidth = document.documentElement.clientWidth || window.innerWidth;
  const reservedBottom =
    Number.parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--player-bar-height")) || 0;
  const viewportBottom = window.innerHeight - reservedBottom;
  let dx = 0;
  if (rect.left < MARGIN) {
    dx = MARGIN - rect.left;
  } else if (rect.right > viewportWidth - MARGIN) {
    dx = viewportWidth - MARGIN - rect.right;
  }
  if (dx !== 0) {
    panel.style.transform = `translateX(${Math.round(dx)}px)`;
  }
  const trigger = panel.parentElement?.getBoundingClientRect();
  if (rect.bottom > viewportBottom - MARGIN && trigger && trigger.top - rect.height - MARGIN > 0) {
    panel.classList.add("ui-dropdown__menu--up");
  }
}

onMounted(async () => {
  await nextTick();
  fit();
});
</script>

<template>
  <div ref="root" class="ui-dropdown-fit">
    <slot />
  </div>
</template>
