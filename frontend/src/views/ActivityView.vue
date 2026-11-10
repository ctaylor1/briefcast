<script setup lang="ts">
import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";
import LogsPanel from "../components/activity/LogsPanel.vue";
import WorkQueuePanel from "../components/activity/WorkQueuePanel.vue";
import MonitoringPanel from "../components/settings/MonitoringPanel.vue";
import UiTabs from "../components/ui/UiTabs.vue";
import DownloadsView from "./DownloadsView.vue";

// Everything the pipeline is doing, in one place: downloads, transcription and summary
// work, monitoring and logs. These used to be split across Downloads and Settings.
type Tab = "downloads" | "work" | "monitoring" | "logs";
const TABS: Array<{ id: Tab; label: string }> = [
  { id: "downloads", label: "Downloads" },
  { id: "work", label: "Transcripts & summaries" },
  { id: "monitoring", label: "Monitoring" },
  { id: "logs", label: "Logs" },
];

const route = useRoute();
const router = useRouter();
const tab = computed<Tab>(() => {
  const value = route.query.tab;
  return TABS.some((t) => t.id === value) ? (value as Tab) : "downloads";
});

function select(next: string): void {
  void router.replace({ query: { ...route.query, tab: next === "downloads" ? undefined : next } });
}
</script>

<template>
  <section class="activity-page stack-4">
    <header class="page-header">
      <h2 class="section-title">Activity</h2>
      <p class="section-subtitle">Downloads, transcription and summaries as they happen.</p>
    </header>
    <UiTabs :model-value="tab" :tabs="TABS" label="Activity" @update:model-value="select">
      <DownloadsView v-if="tab === 'downloads'" embedded />
      <WorkQueuePanel v-else-if="tab === 'work'" />
      <MonitoringPanel v-else-if="tab === 'monitoring'" />
      <LogsPanel v-else />
    </UiTabs>
  </section>
</template>
