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
  <UiCard padding="md" tone="subtle" class="episodes-pagination">
    <p class="meta-text">
      Showing page {{ page }} of {{ totalPages || 1 }} ({{ totalCount }} episodes)
    </p>
    <div class="episodes-pagination__actions">
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

<style scoped>
.episodes-pagination {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-3);
}

.episodes-pagination__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

@media (min-width: 768px) {
  .episodes-pagination {
    flex-direction: row;
    align-items: center;
  }
}
</style>
