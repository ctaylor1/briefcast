import { ref } from "vue";
import { getErrorMessage, searchApi } from "../lib/api";
import type { LocalSearchResult } from "../types/api";

export function useGlobalSearch(limit = 50) {
  const query = ref("");
  const results = ref<LocalSearchResult[]>([]);
  const loading = ref(false);
  const error = ref("");

  async function run(): Promise<void> {
    const term = query.value.trim();
    if (!term) {
      results.value = [];
      error.value = "";
      loading.value = false;
      return;
    }
    loading.value = true;
    error.value = "";
    try {
      results.value = await searchApi.local(term, limit);
    } catch (err) {
      error.value = getErrorMessage(err, "Failed to search.");
    } finally {
      loading.value = false;
    }
  }

  function typeLabel(result: LocalSearchResult): string {
    switch (result.type) {
      case "podcast":
        return "Podcast";
      case "chapter":
        return "Chapter";
      case "transcript":
        return "Transcript";
      default:
        return "Episode";
    }
  }

  function summary(result: LocalSearchResult): string {
    if (result.type === "chapter") {
      return result.chapterTitle || "Chapter match";
    }
    if (result.type === "transcript") {
      return result.transcriptSnippet || "Transcript match";
    }
    return result.summarySnippet || "";
  }

  return {
    query,
    results,
    loading,
    error,
    run,
    typeLabel,
    summary,
  };
}
