<script setup lang="ts">
import UiButton from "../ui/UiButton.vue";
import UiCard from "../ui/UiCard.vue";
import UiInput from "../ui/UiInput.vue";

defineProps<{
  feedUrl: string;
  working: boolean;
}>();

const emit = defineEmits<{
  (event: "update:feedUrl", value: string): void;
  (event: "submit"): void;
}>();
</script>

<template>
  <UiCard>
    <h2 class="text-base font-semibold text-slate-900">Add by RSS URL</h2>
    <form class="mt-[var(--space-2)] flex flex-col gap-[var(--space-2)] sm:flex-row" @submit.prevent="emit('submit')">
      <UiInput
        :model-value="feedUrl"
        type="url"
        required
        placeholder="https://example.com/feed.xml"
        @update:model-value="emit('update:feedUrl', $event)"
      />
      <UiButton type="submit" :disabled="working">
        Add
      </UiButton>
    </form>
  </UiCard>
</template>
