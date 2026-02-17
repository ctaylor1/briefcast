<script setup lang="ts">
import UiButton from "../ui/UiButton.vue";
import UiCard from "../ui/UiCard.vue";
import UiFileInput from "../ui/UiFileInput.vue";

defineProps<{
  working: boolean;
  hasFile: boolean;
}>();

const emit = defineEmits<{
  (event: "file-change", value: Event): void;
  (event: "upload"): void;
}>();
</script>

<template>
  <UiCard padding="lg" class="opml-card stack-3">
    <div class="stack-1">
      <h3 class="opml-card__title">Import OPML</h3>
      <p class="meta-text">Upload your existing podcast subscription export.</p>
    </div>

    <div class="opml-card__controls">
      <UiFileInput
        label="OPML file"
        accept=".opml,text/xml"
        @change="emit('file-change', $event)"
      />
      <UiButton class="opml-card__submit" variant="outline" :disabled="working || !hasFile" @click="emit('upload')">
        {{ working ? "Uploading..." : "Upload file" }}
      </UiButton>
    </div>
  </UiCard>
</template>

<style scoped>
.opml-card__title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: var(--font-card-title-size);
  line-height: var(--font-card-title-line-height);
  font-weight: 600;
}

.opml-card__controls {
  display: grid;
  gap: var(--space-3);
}

.opml-card__submit {
  width: 100%;
}

@media (min-width: 768px) {
  .opml-card__controls {
    grid-template-columns: 1fr auto;
    align-items: end;
  }

  .opml-card__submit {
    width: auto;
  }
}
</style>
