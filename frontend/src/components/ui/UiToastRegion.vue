<script setup lang="ts">
import { computed } from "vue";
import { dismissToast, useToasts } from "../../lib/toast";
import UiIcon from "./UiIcon.vue";
import UiIconButton from "./UiIconButton.vue";

// Two live regions: errors are announced assertively, everything else politely.
// Both regions are always in the DOM so screen readers register them before a message.
const state = useToasts();
const errors = computed(() => state.toasts.filter((toast) => toast.tone === "danger"));
const notices = computed(() => state.toasts.filter((toast) => toast.tone !== "danger"));
</script>

<template>
  <div class="ui-toast-region">
    <div role="status" aria-live="polite" class="ui-toast-stack">
      <div v-for="toast in notices" :key="toast.id" class="ui-toast" :class="`ui-toast--${toast.tone}`">
        <p class="ui-toast__message">{{ toast.message }}</p>
        <UiIconButton label="Dismiss" size="sm" class="ui-toast__dismiss" @click="dismissToast(toast.id)">
          <UiIcon name="close" :size="16" />
        </UiIconButton>
      </div>
    </div>
    <div role="alert" aria-live="assertive" class="ui-toast-stack">
      <div v-for="toast in errors" :key="toast.id" class="ui-toast ui-toast--danger">
        <p class="ui-toast__message">{{ toast.message }}</p>
        <UiIconButton label="Dismiss" size="sm" class="ui-toast__dismiss" @click="dismissToast(toast.id)">
          <UiIcon name="close" :size="16" />
        </UiIconButton>
      </div>
    </div>
  </div>
</template>
