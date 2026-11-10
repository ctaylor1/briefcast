<script setup lang="ts">
import type { Podcast } from "../../types/api";
import UiDropdown from "../ui/UiDropdown.vue";
import UiIcon from "../ui/UiIcon.vue";

// The less common podcast actions behind one "More" button, so each podcast shows a
// single primary action (play) instead of eight equal icons. Settings are toggles with
// their state shown; delete sits apart at the bottom.
defineProps<{
  podcast: Podcast;
  busy?: boolean;
}>();

const emit = defineEmits<{
  (event: "download-all", podcast: Podcast): void;
  (event: "toggle-pause", podcast: Podcast): void;
  (event: "toggle-retention", podcast: Podcast): void;
  (event: "toggle-sponsor-skip", podcast: Podcast): void;
  (event: "toggle-briefpoint", podcast: Podcast): void;
  (event: "edit-feeds", podcast: Podcast): void;
  (event: "delete", podcast: Podcast): void;
}>();
</script>

<template>
  <UiDropdown align="end" button-class="ui-icon-button podcast-menu__trigger">
    <template #trigger>
      <UiIcon name="more" />
      <span class="sr-only">More actions for {{ podcast.Title }}</span>
    </template>
    <div class="podcast-menu" role="group" :aria-label="`More actions for ${podcast.Title}`">
      <button type="button" class="podcast-menu__item" :disabled="busy" @click="emit('download-all', podcast)">
        <UiIcon name="download" :size="16" />
        <span>Download all episodes</span>
      </button>
      <button type="button" class="podcast-menu__item" :disabled="busy" @click="emit('edit-feeds', podcast)">
        <UiIcon name="feed" :size="16" />
        <span>Alternate feeds…</span>
      </button>
      <p class="podcast-menu__heading">Settings</p>
      <button
        type="button"
        class="podcast-menu__item"
        :aria-pressed="podcast.IsPaused ? 'true' : 'false'"
        :disabled="busy"
        @click="emit('toggle-pause', podcast)"
      >
        <UiIcon :name="podcast.IsPaused ? 'check' : 'circle'" :size="16" />
        <span>Pause automatic downloads</span>
      </button>
      <button
        type="button"
        class="podcast-menu__item"
        :aria-pressed="podcast.RetentionKeepAll ? 'true' : 'false'"
        :disabled="busy"
        @click="emit('toggle-retention', podcast)"
      >
        <UiIcon :name="podcast.RetentionKeepAll ? 'check' : 'circle'" :size="16" />
        <span>Keep all episodes</span>
      </button>
      <button
        type="button"
        class="podcast-menu__item"
        :aria-pressed="podcast.AutoSkipSponsorChapters ? 'true' : 'false'"
        :disabled="busy"
        @click="emit('toggle-sponsor-skip', podcast)"
      >
        <UiIcon :name="podcast.AutoSkipSponsorChapters ? 'check' : 'circle'" :size="16" />
        <span>Skip sponsor chapters</span>
      </button>
      <button
        type="button"
        class="podcast-menu__item"
        :aria-pressed="podcast.BriefpointEnabled ? 'true' : 'false'"
        :disabled="busy"
        @click="emit('toggle-briefpoint', podcast)"
      >
        <UiIcon :name="podcast.BriefpointEnabled ? 'check' : 'circle'" :size="16" />
        <span>Sync to Briefpoint</span>
      </button>
      <hr class="podcast-menu__separator" />
      <button type="button" class="podcast-menu__item podcast-menu__item--danger" :disabled="busy" @click="emit('delete', podcast)">
        <UiIcon name="trash" :size="16" />
        <span>Delete podcast…</span>
      </button>
    </div>
  </UiDropdown>
</template>
