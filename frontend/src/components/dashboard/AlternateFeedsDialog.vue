<script setup lang="ts">
import { ref, watch } from "vue";
import {
  Dialog,
  DialogPanel,
  DialogTitle,
  TransitionChild,
  TransitionRoot,
} from "@headlessui/vue";
import UiButton from "../ui/UiButton.vue";
import UiInput from "../ui/UiInput.vue";

const props = defineProps<{
  open: boolean;
  urls: string[];
  busy: boolean;
  podcastTitle: string;
}>();

const emit = defineEmits<{
  (event: "close"): void;
  (event: "save", urls: string[]): void;
}>();

const localUrls = ref<string[]>([]);

watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) {
      localUrls.value = props.urls.length > 0 ? [...props.urls] : [""];
    }
  },
);

function addRow(): void {
  if (localUrls.value.length < 3) {
    localUrls.value.push("");
  }
}

function removeRow(index: number): void {
  localUrls.value.splice(index, 1);
}

function handleSave(): void {
  const cleaned = localUrls.value
    .map((u) => u.trim())
    .filter((u) => u.length > 0);
  emit("save", cleaned);
}
</script>

<template>
  <TransitionRoot as="template" :show="open">
    <Dialog as="div" class="ui-layer" @close="emit('close')">
      <TransitionChild
        as="template"
        enter="ui-transition-fade-enter"
        enter-from="ui-transition-fade-enter-from"
        enter-to="ui-transition-fade-enter-to"
        leave="ui-transition-fade-leave"
        leave-from="ui-transition-fade-leave-from"
        leave-to="ui-transition-fade-leave-to"
      >
        <div class="dialog-overlay" />
      </TransitionChild>

      <div class="dialog-wrap">
        <div class="visually-scrollable">
          <TransitionChild
            as="template"
            enter="ui-transition-scale-enter"
            enter-from="ui-transition-scale-enter-from"
            enter-to="ui-transition-scale-enter-to"
            leave="ui-transition-scale-leave"
            leave-from="ui-transition-scale-leave-from"
            leave-to="ui-transition-scale-leave-to"
          >
            <DialogPanel class="dialog-panel dialog-panel--wide">
              <div class="dialog-header">
                <span class="dialog-icon" aria-hidden="true">i</span>
                <div>
                  <DialogTitle class="dialog-title">
                    Alternate Feeds
                  </DialogTitle>
                  <p class="dialog-description">
                    Add alternate RSS feed URLs for {{ podcastTitle }}.
                    When a download fails, the system will try these feeds to find a working URL.
                  </p>
                </div>
              </div>

              <div class="alt-feeds-list">
                <div
                  v-for="(url, index) in localUrls"
                  :key="index"
                  class="alt-feeds-row"
                >
                  <UiInput
                    :model-value="url"
                    type="url"
                    :placeholder="`Alternate feed URL ${index + 1}`"
                    :disabled="busy"
                    @update:model-value="localUrls[index] = $event"
                  />
                  <UiButton
                    size="sm"
                    variant="danger"
                    :disabled="busy || localUrls.length <= 1"
                    @click="removeRow(index)"
                  >
                    Remove
                  </UiButton>
                </div>
              </div>

              <UiButton
                v-if="localUrls.length < 3"
                size="sm"
                variant="secondary"
                :disabled="busy"
                @click="addRow"
              >
                + Add URL
              </UiButton>

              <div class="dialog-actions">
                <UiButton variant="secondary" :disabled="busy" @click="emit('close')">
                  Cancel
                </UiButton>
                <UiButton variant="primary" :disabled="busy" @click="handleSave">
                  Save
                </UiButton>
              </div>
            </DialogPanel>
          </TransitionChild>
        </div>
      </div>
    </Dialog>
  </TransitionRoot>
</template>

<style scoped>
.alt-feeds-list {
  display: grid;
  gap: var(--space-3);
  margin: var(--space-4) 0;
}

.alt-feeds-row {
  display: flex;
  align-items: flex-end;
  gap: var(--space-2);
}

.alt-feeds-row > :first-child {
  flex: 1;
}
</style>
