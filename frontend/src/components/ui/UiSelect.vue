<script setup lang="ts">
import { computed, useAttrs } from "vue";
import { cn } from "../../lib/cn";

const props = withDefaults(
  defineProps<{
    modelValue?: string | number;
    disabled?: boolean;
    id?: string;
    name?: string;
    inputClass?: string;
  }>(),
  {
    modelValue: "",
    disabled: false,
  },
);

const emit = defineEmits<{
  (event: "update:modelValue", value: string): void;
}>();

const attrs = useAttrs();

const classes = computed(() =>
  cn(
    "min-h-10 w-full rounded-md border border-slate-300 px-3 py-2 text-sm text-slate-900",
    "focus:border-cyan-500 focus:outline-none",
    "disabled:cursor-not-allowed disabled:bg-slate-100 disabled:text-slate-500",
    props.inputClass,
  ),
);
</script>

<template>
  <select
    v-bind="attrs"
    :id="id"
    :name="name"
    :disabled="disabled"
    :value="modelValue"
    :class="classes"
    @change="emit('update:modelValue', ($event.target as HTMLSelectElement).value)"
  >
    <slot />
  </select>
</template>
