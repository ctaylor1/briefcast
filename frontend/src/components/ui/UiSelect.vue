<script setup lang="ts">
import { computed, useAttrs } from "vue";
import { cn } from "../../lib/cn";

const props = withDefaults(
  defineProps<{
    modelValue?: string | number;
    disabled?: boolean;
    id?: string;
    name?: string;
    label?: string;
    hint?: string;
    error?: string;
    inputClass?: string;
  }>(),
  {
    modelValue: "",
    disabled: false,
    label: "",
    hint: "",
    error: "",
  },
);

const emit = defineEmits<{
  (event: "update:modelValue", value: string): void;
}>();

const attrs = useAttrs();

const classes = computed(() =>
  cn(
    "ui-select",
    props.error && "ui-select--error",
    props.inputClass,
  ),
);
</script>

<template>
  <div class="ui-field">
    <label v-if="label" class="ui-label" :for="id">{{ label }}</label>
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
    <p v-if="error" class="ui-error">{{ error }}</p>
    <p v-else-if="hint" class="ui-hint">{{ hint }}</p>
  </div>
</template>
