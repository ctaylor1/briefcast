<script setup lang="ts">
import { nextTick, ref, useId } from "vue";

// Accessible tabs: a tablist of buttons with arrow-key, Home and End navigation.
// The selected panel is rendered through the default slot ({ active }).
export interface UiTab {
  id: string;
  label: string;
}

const props = defineProps<{
  tabs: UiTab[];
  modelValue: string;
  label: string;
}>();

const emit = defineEmits<{
  (event: "update:modelValue", value: string): void;
}>();

const baseId = useId();
const tabRefs = ref<HTMLButtonElement[]>([]);

function tabId(id: string): string {
  return `${baseId}-tab-${id}`;
}

function panelId(id: string): string {
  return `${baseId}-panel-${id}`;
}

async function select(index: number, focus = false): Promise<void> {
  const tab = props.tabs[index];
  if (!tab) {
    return;
  }
  emit("update:modelValue", tab.id);
  if (focus) {
    await nextTick();
    tabRefs.value[index]?.focus();
  }
}

function onKeydown(event: KeyboardEvent, index: number): void {
  const last = props.tabs.length - 1;
  const target =
    event.key === "ArrowRight" ? (index === last ? 0 : index + 1)
    : event.key === "ArrowLeft" ? (index === 0 ? last : index - 1)
    : event.key === "Home" ? 0
    : event.key === "End" ? last
    : -1;
  if (target >= 0) {
    event.preventDefault();
    void select(target, true);
  }
}
</script>

<template>
  <div class="ui-tabs">
    <div class="ui-tabs__list" role="tablist" :aria-label="label">
      <button
        v-for="(tab, index) in tabs"
        :id="tabId(tab.id)"
        :key="tab.id"
        ref="tabRefs"
        type="button"
        role="tab"
        class="ui-tabs__tab"
        :aria-selected="tab.id === modelValue ? 'true' : 'false'"
        :aria-controls="panelId(tab.id)"
        :tabindex="tab.id === modelValue ? 0 : -1"
        @click="select(index)"
        @keydown="onKeydown($event, index)"
      >
        {{ tab.label }}
      </button>
    </div>
    <div :id="panelId(modelValue)" class="ui-tabs__panel" role="tabpanel" :aria-labelledby="tabId(modelValue)" tabindex="0">
      <slot :active="modelValue" />
    </div>
  </div>
</template>
