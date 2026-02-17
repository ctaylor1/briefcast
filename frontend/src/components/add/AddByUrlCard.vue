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
  <UiCard padding="lg" class="add-url-card stack-3">
    <div class="stack-1">
      <h3 class="add-url-card__title">Add by RSS URL</h3>
      <p class="meta-text">Paste a feed URL to subscribe instantly.</p>
    </div>

    <form class="add-url-card__form" @submit.prevent="emit('submit')">
      <UiInput
        :model-value="feedUrl"
        type="url"
        required
        label="Feed URL"
        input-class="add-url-card__input"
        placeholder="https://example.com/feed.xml"
        @update:model-value="emit('update:feedUrl', $event)"
      />
      <UiButton type="submit" :disabled="working" class="add-url-card__submit">
        {{ working ? "Adding..." : "Add podcast" }}
      </UiButton>
    </form>
  </UiCard>
</template>

<style scoped>
.add-url-card__title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
}

.add-url-card__form {
  display: grid;
  gap: var(--space-3);
}

.add-url-card__input {
  min-height: 56px;
}

.add-url-card__submit {
  width: 100%;
}

@media (min-width: 768px) {
  .add-url-card__submit {
    width: auto;
    justify-self: start;
  }
}
</style>
