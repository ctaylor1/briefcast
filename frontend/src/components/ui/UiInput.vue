<script setup lang="ts">
import { computed, useAttrs } from "vue";
import { cn } from "../../lib/cn";

let nextUiInputId = 0;

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

const generatedId = `ui-input-${++nextUiInputId}`;

const controlId = computed(() => {
  if (props.id && props.id.trim().length > 0) {
    return props.id;
  }
  if (props.label || props.hint || props.error) {
    return generatedId;
  }
  return undefined;
});

const hintId = computed(() =>
  controlId.value && props.hint ? `${controlId.value}-hint` : undefined,
);

const errorId = computed(() =>
  controlId.value && props.error ? `${controlId.value}-error` : undefined,
);

const describedBy = computed(() => {
  const ids = [hintId.value, errorId.value].filter((value): value is string => Boolean(value));
  if (ids.length === 0) {
    return undefined;
  }
  return ids.join(" ");
});

const ariaInvalid = computed(() => (props.error ? "true" : undefined));

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
    <label v-if="label" class="ui-label" :for="controlId">{{ label }}</label>
    <input
      v-bind="attrs"
      :id="controlId"
      :name="name"
      :type="type"
      :placeholder="placeholder"
      :disabled="disabled"
      :required="required"
      :autocomplete="autocomplete"
      :value="inputValue"
      :class="classes"
      :aria-invalid="ariaInvalid"
      :aria-describedby="describedBy"
      @input="emit('update:modelValue', ($event.target as HTMLInputElement).value)"
    />
    <p v-if="error" :id="errorId" class="ui-error">{{ error }}</p>
    <p v-if="hint" :id="hintId" class="ui-hint">{{ hint }}</p>
  </div>
</template>
