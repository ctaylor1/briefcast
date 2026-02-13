<script setup lang="ts">
import UiButton from "../ui/UiButton.vue";
import UiCard from "../ui/UiCard.vue";

defineProps<{
  page: number;
  totalPages: number;
  totalCount: number;
  hasPrevious: boolean;
  hasNext: boolean;
}>();

const emit = defineEmits<{
  (event: "first"): void;
  (event: "previous"): void;
  (event: "next"): void;
}>();
</script>

<template>
  <UiCard padding="sm" class="flex flex-col items-start justify-between gap-[var(--space-2)] sm:flex-row sm:items-center">
    <p class="fluid-subtle text-slate-600">
      Showing page {{ page }} of {{ totalPages || 1 }} ({{ totalCount }} episodes)
    </p>
    <div class="flex w-full flex-wrap items-center gap-[var(--space-1)] sm:w-auto">
      <UiButton size="sm" variant="outline" :disabled="!hasPrevious" @click="emit('first')">
        First
      </UiButton>
      <UiButton size="sm" variant="outline" :disabled="!hasPrevious" @click="emit('previous')">
        Prev
      </UiButton>
      <UiButton size="sm" variant="outline" :disabled="!hasNext" @click="emit('next')">
        Next
      </UiButton>
    </div>
  </UiCard>
</template>
