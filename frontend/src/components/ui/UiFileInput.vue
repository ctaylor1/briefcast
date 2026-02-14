<script setup lang="ts">
import { computed, useAttrs } from "vue";
import { cn } from "../../lib/cn";

const props = withDefaults(
  defineProps<{
    disabled?: boolean;
    id?: string;
    name?: string;
    accept?: string;
    multiple?: boolean;
    inputClass?: string;
  }>(),
  {
    disabled: false,
    accept: "",
    multiple: false,
  },
);

const attrs = useAttrs();

const classes = computed(() =>
  cn(
    "min-h-10 w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm text-slate-700",
    "focus:border-cyan-500 focus:outline-none",
    "file:mr-3 file:rounded-md file:border-0 file:bg-slate-900 file:px-3 file:py-1.5 file:text-xs file:font-semibold file:text-white",
    "disabled:cursor-not-allowed disabled:bg-slate-100 disabled:text-slate-500",
    props.inputClass,
  ),
);
</script>

<template>
  <input
    v-bind="attrs"
    :id="id"
    :name="name"
    type="file"
    :accept="accept"
    :multiple="multiple"
    :disabled="disabled"
    :class="classes"
  />
</template>
