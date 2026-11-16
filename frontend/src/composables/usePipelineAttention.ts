import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { settingsApi } from "../lib/api";
import { attentionItems, type StatusItem } from "../lib/monitoring";

const POLL_MS = 60_000;

// One shared poller however many components use this. It asks for the
// monitoring snapshot without the LLM server check, which only the
// Monitoring panel requests.
const items = ref<StatusItem[]>([]);
let users = 0;
let timer: number | undefined;
let polling = false;

// The first poll runs even while the page reports itself hidden (as some
// browsers do while a page is still loading); later polls skip hidden tabs.
async function poll(initial = false): Promise<void> {
  if (polling || (!initial && document.visibilityState === "hidden")) {
    return;
  }
  polling = true;
  try {
    items.value = attentionItems(await settingsApi.getMonitoring());
  } catch {
    // A failed poll keeps the last state: the nav dot must never break the
    // page, and Monitoring shows the error when opened.
  } finally {
    polling = false;
  }
}

function onVisibilityChange(): void {
  if (document.visibilityState === "visible") {
    void poll();
  }
}

/** Problems serious enough for a dot on the Activity nav item ("bad" tone only). */
export function usePipelineAttention() {
  onMounted(() => {
    users += 1;
    if (users === 1) {
      void poll(true);
      timer = window.setInterval(() => void poll(), POLL_MS);
      document.addEventListener("visibilitychange", onVisibilityChange);
    }
  });
  onBeforeUnmount(() => {
    users -= 1;
    if (users === 0) {
      window.clearInterval(timer);
      timer = undefined;
      document.removeEventListener("visibilitychange", onVisibilityChange);
      items.value = [];
    }
  });
  const problems = computed(() => items.value.filter((item) => item.tone === "bad"));
  const summary = computed(() => problems.value.map((item) => item.label).join("; "));
  return { problems, summary };
}
