<script setup lang="ts">
import type { SearchResult, SearchSource } from "../../types/api";
import UiButton from "../ui/UiButton.vue";
import UiCard from "../ui/UiCard.vue";
import UiInput from "../ui/UiInput.vue";
import UiSelect from "../ui/UiSelect.vue";

defineProps<{
  query: string;
  source: SearchSource;
  working: boolean;
  results: SearchResult[];
}>();

const emit = defineEmits<{
  (event: "update:query", value: string): void;
  (event: "update:source", value: SearchSource): void;
  (event: "search"): void;
  (event: "add", url: string): void;
}>();
</script>

<template>
  <UiCard>
    <h2 class="text-base font-semibold text-slate-900">Search</h2>
    <form class="mt-[var(--space-2)] grid gap-[var(--space-2)] sm:grid-cols-[1fr_180px_auto]" @submit.prevent="emit('search')">
      <UiInput
        :model-value="query"
        type="search"
        required
        placeholder="Search podcast title"
        @update:model-value="emit('update:query', $event)"
      />
      <UiSelect
        :model-value="source"
        @update:model-value="emit('update:source', $event as SearchSource)"
      >
        <option value="itunes">iTunes</option>
        <option value="podcastindex">PodcastIndex</option>
      </UiSelect>
      <UiButton type="submit" :disabled="working">
        Search
      </UiButton>
    </form>

    <div v-if="working" class="mt-[var(--space-2)] text-sm text-slate-600">Working...</div>

    <div v-else-if="results.length > 0" class="mt-[var(--space-3)] stack-3">
      <UiCard
        v-for="item in results"
        :key="item.url"
        padding="sm"
        :elevated="false"
        class="grid gap-[var(--space-2)] border-dashed sm:grid-cols-[96px_1fr_auto]"
      >
        <img :src="item.image" :alt="item.title" class="h-24 w-full rounded-md bg-slate-100 object-cover" />
        <div class="stack-2">
          <h3 class="text-sm font-semibold text-slate-900">{{ item.title }}</h3>
          <p class="line-clamp-3 text-xs text-slate-600">{{ item.description || "No description provided." }}</p>
          <p v-if="item.categories?.length" class="text-xs text-slate-500">
            {{ item.categories.join(", ") }}
          </p>
        </div>
        <div class="self-start">
          <UiButton
            size="sm"
            variant="outline"
            :disabled="item.already_saved"
            @click="emit('add', item.url)"
          >
            {{ item.already_saved ? "Added" : "Add" }}
          </UiButton>
        </div>
      </UiCard>
    </div>
  </UiCard>
</template>
