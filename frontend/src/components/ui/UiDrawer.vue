<script setup lang="ts">
import {
  Dialog,
  DialogPanel,
  DialogTitle,
  TransitionChild,
  TransitionRoot,
} from "@headlessui/vue";
import UiButton from "./UiButton.vue";

const props = withDefaults(
  defineProps<{
    open: boolean;
    title: string;
    description?: string;
  }>(),
  {
    description: "",
  },
);

const emit = defineEmits<{
  (event: "close"): void;
}>();
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

      <div class="drawer-wrap">
        <TransitionChild
          as="template"
          enter="ui-transition-drawer-enter"
          enter-from="ui-transition-drawer-enter-from"
          enter-to="ui-transition-drawer-enter-to"
          leave="ui-transition-drawer-leave"
          leave-from="ui-transition-drawer-leave-from"
          leave-to="ui-transition-drawer-leave-to"
        >
          <DialogPanel class="drawer-panel">
            <div class="drawer-header">
              <div>
                <DialogTitle class="drawer-title">
                  {{ title }}
                </DialogTitle>
                <p v-if="description" class="drawer-description">
                  {{ description }}
                </p>
              </div>
              <UiButton variant="ghost" size="sm" @click="emit('close')">
                Close
              </UiButton>
            </div>
            <div class="drawer-body visually-scrollable">
              <slot />
            </div>
          </DialogPanel>
        </TransitionChild>
      </div>
    </Dialog>
  </TransitionRoot>
</template>
