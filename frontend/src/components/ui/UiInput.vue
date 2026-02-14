<script setup lang="ts">
import { computed, useAttrs } from "vue";
import { cn } from "../../lib/cn";

const props = withDefaults(
  defineProps<{
    modelValue?: string | number;
    type?: string;
    placeholder?: string;
    disabled?: boolean;
    required?: boolean;
    id?: string;
    name?: string;
    autocomplete?: string;
    inputClass?: string;
  }>(),
  {
    modelValue: "",
    type: "text",
    placeholder: "",
    disabled: false,
    required: false,
  },
);

const emit = defineEmits<{
  (event: "update:modelValue", value: string): void;
}>();

const attrs = useAttrs();

const classes = computed(() =>
  cn(
    "min-h-10 w-full rounded-md border border-slate-300 px-3 py-2 text-sm text-slate-900",
    "placeholder:text-slate-400 focus:border-cyan-500 focus:outline-none",
    "disabled:cursor-not-allowed disabled:bg-slate-100 disabled:text-slate-500",
    props.inputClass,
  ),
);

const inputValue = computed(() => (props.type === "file" ? undefined : props.modelValue));
</script>

<template>
  <input
    v-bind="attrs"
    :id="id"
    :name="name"
    :type="type"
    :placeholder="placeholder"
    :disabled="disabled"
    :required="required"
    :autocomplete="autocomplete"
    :value="inputValue"
    :class="classes"
    @input="emit('update:modelValue', ($event.target as HTMLInputElement).value)"
  />
</template>
