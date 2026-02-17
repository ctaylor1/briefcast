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
    label?: string;
    hint?: string;
    error?: string;
    inputClass?: string;
  }>(),
  {
    modelValue: "",
    type: "text",
    placeholder: "",
    disabled: false,
    required: false,
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
    "ui-input",
    props.error && "ui-input--error",
    props.inputClass,
  ),
);

const inputValue = computed(() => (props.type === "file" ? undefined : props.modelValue));
</script>

<template>
  <div class="ui-field">
    <label v-if="label" class="ui-label" :for="id">{{ label }}</label>
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
    <p v-if="error" class="ui-error">{{ error }}</p>
    <p v-else-if="hint" class="ui-hint">{{ hint }}</p>
  </div>
</template>
