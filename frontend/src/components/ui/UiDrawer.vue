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
    <Dialog as="div" class="relative z-50" @close="emit('close')">
      <TransitionChild
        as="template"
        enter="ease-out duration-200"
        enter-from="opacity-0"
        enter-to="opacity-100"
        leave="ease-in duration-150"
        leave-from="opacity-100"
        leave-to="opacity-0"
      >
        <div class="fixed inset-0 bg-slate-900/35" />
      </TransitionChild>

      <div class="fixed inset-0 overflow-hidden">
        <div class="absolute inset-0 overflow-hidden">
          <div class="pointer-events-none fixed inset-y-0 right-0 flex max-w-full pl-10">
            <TransitionChild
              as="template"
              enter="transform transition ease-out duration-200"
              enter-from="translate-x-full"
              enter-to="translate-x-0"
              leave="transform transition ease-in duration-150"
              leave-from="translate-x-0"
              leave-to="translate-x-full"
            >
              <DialogPanel class="pointer-events-auto w-screen max-w-lg">
                <div class="flex h-full flex-col bg-white shadow-xl">
                  <div class="flex items-start justify-between border-b border-slate-200 px-5 py-4">
                    <div>
                      <DialogTitle class="text-lg font-semibold text-slate-900">
                        {{ title }}
                      </DialogTitle>
                      <p v-if="description" class="text-sm text-slate-600">
                        {{ description }}
                      </p>
                    </div>
                    <UiButton variant="ghost" size="sm" @click="emit('close')">
                      Close
                    </UiButton>
                  </div>
                  <div class="flex-1 overflow-y-auto px-5 py-4">
                    <slot />
                  </div>
                </div>
              </DialogPanel>
            </TransitionChild>
          </div>
        </div>
      </div>
    </Dialog>
  </TransitionRoot>
</template>
