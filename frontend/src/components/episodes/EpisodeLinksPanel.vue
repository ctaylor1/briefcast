<script setup lang="ts">
import { computed } from "vue";
import { groupLinksBySection, safeExternalUrl, showNoteLinkLabel } from "../../lib/links";
import type { ShowNoteLink } from "../../types/api";

const props = defineProps<{
  links: ShowNoteLink[];
  loading: boolean;
}>();

const sections = computed(() =>
  groupLinksBySection(props.links).map((section) => ({
    key: section.key,
    heading: section.heading,
    chips: section.links.map((link) => ({
      id: link.ID,
      label: showNoteLinkLabel(link),
      href: safeExternalUrl(link.URL),
      url: link.URL,
    })),
  })),
);
</script>

<template>
  <div class="stack-3">
    <p v-if="loading" class="meta-text">Loading links...</p>
    <p v-else-if="links.length === 0" class="meta-text">No links found in this episode's show notes.</p>
    <template v-else>
      <section v-for="section in sections" :key="section.key" class="episode-links__section">
        <h3 class="episode-links__heading">{{ section.heading }}</h3>
        <ul class="episode-links__chips">
          <li v-for="chip in section.chips" :key="chip.id" class="episode-links__item">
            <a
              v-if="chip.href"
              class="episode-links__chip"
              :href="chip.href"
              :title="chip.url"
              target="_blank"
              rel="noopener noreferrer"
            >
              {{ chip.label }}
            </a>
            <span v-else class="episode-links__chip episode-links__chip--plain" :title="chip.url">
              {{ chip.label }}
            </span>
          </li>
        </ul>
      </section>
    </template>
  </div>
</template>

<style scoped>
.episode-links__section {
  display: grid;
  gap: var(--space-2);
}

.episode-links__heading {
  margin: 0;
  color: var(--color-text-secondary);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
  font-weight: 600;
}

.episode-links__chips {
  margin: 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.episode-links__item {
  min-width: 0;
  max-width: 100%;
}

.episode-links__chip {
  display: block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-2);
  background: var(--color-bg-secondary);
  padding: var(--space-1) var(--space-2);
  color: var(--color-accent);
  font-size: var(--font-caption-size);
  line-height: var(--font-caption-line-height);
  text-decoration: none;
}

a.episode-links__chip:hover {
  background: var(--color-hover);
  text-decoration: underline;
}

a.episode-links__chip:focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
}

.episode-links__chip--plain {
  color: var(--color-text-secondary);
}
</style>
