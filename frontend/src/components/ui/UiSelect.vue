<script setup lang="ts">
import { computed, useAttrs } from "vue";
import { cn } from "../../lib/cn";

let nextUiSelectId = 0;

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

const generatedId = `ui-select-${++nextUiSelectId}`;

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
    "ui-select",
    props.error && "ui-select--error",
    props.inputClass,
  ),
);
</script>

<template>
  <div class="ui-field">
    <label v-if="label" class="ui-label" :for="controlId">{{ label }}</label>
    <select
      v-bind="attrs"
      :id="controlId"
      :name="name"
      :disabled="disabled"
      :value="modelValue"
      :class="classes"
      :aria-invalid="ariaInvalid"
      :aria-describedby="describedBy"
      @change="emit('update:modelValue', ($event.target as HTMLSelectElement).value)"
    >
      <slot />
    </select>
    <p v-if="error" :id="errorId" class="ui-error">{{ error }}</p>
    <p v-if="hint" :id="hintId" class="ui-hint">{{ hint }}</p>
  </div>
</template>
