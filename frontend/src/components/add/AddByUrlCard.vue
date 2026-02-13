<script setup lang="ts">
import UiButton from "../ui/UiButton.vue";
import UiCard from "../ui/UiCard.vue";

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
      <input
        :value="feedUrl"
        type="url"
        required
        placeholder="https://example.com/feed.xml"
        class="min-h-10 w-full rounded-md border border-slate-300 px-3 py-2 text-sm focus:border-cyan-500 focus:outline-none"
        @input="emit('update:feedUrl', ($event.target as HTMLInputElement).value)"
      />
      <UiButton type="submit" :disabled="working">
        Add
      </UiButton>
    </form>
  </UiCard>
</template>
