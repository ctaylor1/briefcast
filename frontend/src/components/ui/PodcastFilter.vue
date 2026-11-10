<script setup lang="ts">
import { Popover, PopoverButton, PopoverPanel } from "@headlessui/vue";
import { computed, ref, useId } from "vue";
import UiButton from "./UiButton.vue";

// Multi-select podcast filter shared by Episodes and Summaries. A Headless UI popover,
// so it closes on Escape and on a click outside, and returns focus to its trigger.
export interface PodcastFilterOption {
  id: string;
  title: string;
}

const props = withDefaults(
  defineProps<{
    options: PodcastFilterOption[];
    modelValue: string[];
    label?: string;
    allLabel?: string;
  }>(),
  {
    label: "Podcasts",
    allLabel: "All podcasts",
  },
);

const emit = defineEmits<{
  (event: "update:modelValue", value: string[]): void;
}>();

const labelId = useId();
const triggerId = useId();
const search = ref("");

const sortedOptions = computed(() => [...props.options].sort((a, b) => a.title.localeCompare(b.title)));

const visibleOptions = computed(() => {
  const term = search.value.trim().toLowerCase();
  if (!term) {
    return sortedOptions.value;
  }
  return sortedOptions.value.filter((option) => option.title.toLowerCase().includes(term));
});

const selectedLabel = computed(() => {
  const count = props.modelValue.length;
  if (count === 0) {
    return props.allLabel;
  }
  if (count === 1) {
    return props.options.find((option) => option.id === props.modelValue[0])?.title ?? "1 podcast";
  }
  return `${count} podcasts`;
});

const countLabel = computed(() => (props.modelValue.length === 0 ? "No filter" : `${props.modelValue.length} selected`));

function isSelected(id: string): boolean {
  return props.modelValue.includes(id);
}

function toggle(id: string): void {
  const next = isSelected(id) ? props.modelValue.filter((value) => value !== id) : [...props.modelValue, id];
  emit("update:modelValue", next);
}

function clear(): void {
  emit("update:modelValue", []);
}
</script>

<template>
  <div class="podcast-filter">
    <span :id="labelId" class="ui-label">{{ label }}</span>
    <Popover class="podcast-filter__popover">
      <PopoverButton
        :id="triggerId"
        class="podcast-filter__trigger"
        :aria-labelledby="`${labelId} ${triggerId}`"
      >
        <span class="podcast-filter__value">{{ selectedLabel }}</span>
        <span class="meta-text">{{ countLabel }}</span>
      </PopoverButton>
      <PopoverPanel class="podcast-filter__panel" focus>
        <input
          v-model="search"
          type="search"
          class="ui-input podcast-filter__search"
          placeholder="Search podcasts"
          aria-label="Search podcasts"
        />
        <p v-if="visibleOptions.length === 0" class="meta-text">No podcasts match.</p>
        <ul v-else class="podcast-filter__list visually-scrollable" :aria-labelledby="labelId">
          <li v-for="option in visibleOptions" :key="option.id">
            <label class="podcast-filter__option">
              <input type="checkbox" :checked="isSelected(option.id)" @change="toggle(option.id)" />
              <span>{{ option.title }}</span>
            </label>
          </li>
        </ul>
        <div class="podcast-filter__actions">
          <UiButton size="sm" variant="ghost" :disabled="modelValue.length === 0" @click="clear">
            Clear
          </UiButton>
        </div>
      </PopoverPanel>
    </Popover>
  </div>
</template>

<style scoped>
.podcast-filter {
  display: grid;
  gap: var(--space-2);
}

.podcast-filter__popover {
  position: relative;
}

.podcast-filter__trigger {
  width: 100%;
  min-height: 40px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-primary);
  color: var(--color-text-primary);
  font: inherit;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  text-align: left;
  cursor: pointer;
}

.podcast-filter__value {
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.podcast-filter__panel {
  position: absolute;
  top: calc(100% + var(--space-2));
  left: 0;
  right: 0;
  min-width: 240px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-3);
  background: var(--color-bg-primary);
  box-shadow: var(--shadow-popover);
  padding: var(--space-3);
  z-index: 30;
  display: grid;
  gap: var(--space-2);
}

.podcast-filter__search {
  min-height: 36px;
}

.podcast-filter__list {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: var(--space-1);
  max-height: 240px;
  overflow: auto;
}

.podcast-filter__option {
  min-height: 36px;
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-2);
  color: var(--color-text-primary);
  cursor: pointer;
}

.podcast-filter__option:hover,
.podcast-filter__option:focus-within {
  background: var(--color-hover);
}

.podcast-filter__option input[type="checkbox"] {
  width: 18px;
  height: 18px;
  margin: 0;
  flex: 0 0 18px;
  accent-color: var(--color-accent);
}

.podcast-filter__actions {
  display: flex;
  justify-content: flex-end;
}
</style>
