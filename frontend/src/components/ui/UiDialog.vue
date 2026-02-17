<script setup lang="ts">
import {
  Dialog,
  DialogPanel,
  DialogTitle,
  TransitionChild,
  TransitionRoot,
} from "@headlessui/vue";
import { computed } from "vue";
import { cn } from "../../lib/cn";
import UiButton from "./UiButton.vue";

const props = withDefaults(
  defineProps<{
    open: boolean;
    title: string;
    description?: string;
    confirmLabel?: string;
    cancelLabel?: string;
    tone?: "neutral" | "danger";
    busy?: boolean;
  }>(),
  {
    description: "",
    confirmLabel: "Confirm",
    cancelLabel: "Cancel",
    tone: "neutral",
    busy: false,
  },
);

const emit = defineEmits<{
  (event: "close"): void;
  (event: "confirm"): void;
}>();

const iconClass = computed(() =>
  cn(
    "dialog-icon",
    props.tone === "danger" && "dialog-icon--danger",
  ),
);
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
            <DialogPanel class="dialog-panel">
              <div class="dialog-header">
                <span :class="iconClass" aria-hidden="true">
                  {{ tone === "danger" ? "!" : "i" }}
                </span>
                <div>
                  <DialogTitle class="dialog-title">
                    {{ title }}
                  </DialogTitle>
                  <p v-if="description" class="dialog-description">
                    {{ description }}
                  </p>
                </div>
              </div>

              <div class="dialog-actions">
                <UiButton variant="secondary" :disabled="busy" @click="emit('close')">
                  {{ cancelLabel }}
                </UiButton>
                <UiButton :variant="tone === 'danger' ? 'danger' : 'primary'" :disabled="busy" @click="emit('confirm')">
                  {{ confirmLabel }}
                </UiButton>
              </div>
            </DialogPanel>
          </TransitionChild>
        </div>
      </div>
    </Dialog>
  </TransitionRoot>
</template>
