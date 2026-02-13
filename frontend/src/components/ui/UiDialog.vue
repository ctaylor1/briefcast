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
    "inline-flex h-8 w-8 items-center justify-center rounded-full text-sm font-bold",
    props.tone === "danger" ? "bg-rose-100 text-rose-700" : "bg-slate-100 text-slate-700",
  ),
);
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

      <div class="fixed inset-0 overflow-y-auto p-4">
        <div class="flex min-h-full items-center justify-center">
          <TransitionChild
            as="template"
            enter="ease-out duration-200"
            enter-from="opacity-0 scale-95"
            enter-to="opacity-100 scale-100"
            leave="ease-in duration-150"
            leave-from="opacity-100 scale-100"
            leave-to="opacity-0 scale-95"
          >
            <DialogPanel class="w-full max-w-md rounded-xl border border-slate-200 bg-white p-5 shadow-xl">
              <div class="flex items-start gap-3">
                <span :class="iconClass" aria-hidden="true">
                  {{ tone === "danger" ? "!" : "i" }}
                </span>
                <div class="space-y-1">
                  <DialogTitle class="text-base font-semibold text-slate-900">
                    {{ title }}
                  </DialogTitle>
                  <p v-if="description" class="text-sm text-slate-600">
                    {{ description }}
                  </p>
                </div>
              </div>

              <div class="mt-5 flex justify-end gap-2">
                <UiButton variant="outline" :disabled="busy" @click="emit('close')">
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
