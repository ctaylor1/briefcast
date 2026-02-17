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
    label?: string;
    hint?: string;
    error?: string;
    inputClass?: string;
  }>(),
  {
    disabled: false,
    accept: "",
    multiple: false,
    label: "",
    hint: "",
    error: "",
  },
);

const attrs = useAttrs();

const classes = computed(() =>
  cn(
    "ui-file",
    props.error && "ui-file--error",
    props.inputClass,
  ),
);
</script>

<template>
  <div class="ui-field">
    <label v-if="label" class="ui-label" :for="id">{{ label }}</label>
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
    <p v-if="error" class="ui-error">{{ error }}</p>
    <p v-else-if="hint" class="ui-hint">{{ hint }}</p>
  </div>
</template>
