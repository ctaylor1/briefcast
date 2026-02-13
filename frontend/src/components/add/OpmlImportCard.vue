<script setup lang="ts">
import UiButton from "../ui/UiButton.vue";
import UiCard from "../ui/UiCard.vue";

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
  <UiCard>
    <h2 class="text-base font-semibold text-slate-900">Import OPML</h2>
    <div class="mt-[var(--space-2)] flex flex-col gap-[var(--space-2)] sm:flex-row sm:items-center">
      <input
        type="file"
        accept=".opml,text/xml"
        class="min-h-10 text-sm text-slate-700"
        @change="emit('file-change', $event)"
      />
      <UiButton variant="outline" :disabled="working || !hasFile" @click="emit('upload')">
        Upload
      </UiButton>
    </div>
  </UiCard>
</template>
