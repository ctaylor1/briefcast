import { computed, ref } from "vue";
import { episodesApi, getErrorMessage } from "../lib/api";
import type { Chapter, ChaptersResponse, PodcastItem, TranscriptResponse } from "../types/api";

type DrawerTab = "overview" | "chapters" | "transcript";
type TranscriptSegment = { start: number; end: number; text: string; speaker?: string };
type TranscriptAsset = { url?: string; type?: string; language?: string };

export function useEpisodeDrawer() {
  const drawerTabs: Array<{ id: DrawerTab; label: string }> = [
    { id: "overview", label: "Overview" },
    { id: "chapters", label: "Chapters" },
    { id: "transcript", label: "Transcript" },
  ];
  const drawerOpen = ref(false);
  const drawerItem = ref<PodcastItem | null>(null);
  const drawerTab = ref<DrawerTab>("overview");
  const drawerChapters = ref<Chapter[]>([]);
  const drawerChaptersSource = ref("");
  const drawerTranscriptStatus = ref("missing");
  const drawerTranscriptSegments = ref<TranscriptSegment[]>([]);
  const drawerTranscriptText = ref("");
  const drawerTranscriptAssets = ref<TranscriptAsset[]>([]);
  const drawerLoadingChapters = ref(false);
  const drawerLoadingTranscript = ref(false);
  const drawerLoadError = ref("");
  const chaptersSearch = ref("");
  const transcriptSearch = ref("");

  const chaptersSearchQuery = computed(() => chaptersSearch.value.trim().toLowerCase());
  const transcriptSearchQuery = computed(() => transcriptSearch.value.trim().toLowerCase());

  const filteredChapters = computed(() => {
    const query = chaptersSearchQuery.value;
    if (!query) {
      return drawerChapters.value;
    }
    return drawerChapters.value.filter((chapter) => chapter.title.toLowerCase().includes(query));
  });

  const filteredTranscriptSegments = computed(() => {
    const query = transcriptSearchQuery.value;
    if (!query) {
      return drawerTranscriptSegments.value;
    }
    return drawerTranscriptSegments.value.filter((segment) => {
      const text = segment.text.toLowerCase();
      const speaker = segment.speaker ? segment.speaker.toLowerCase() : "";
      return text.includes(query) || (speaker && speaker.includes(query));
    });
  });

  const transcriptLines = computed(() => {
    if (!drawerTranscriptText.value) {
      return [];
    }
    return drawerTranscriptText.value
      .split(/\r?\n/)
      .map((line) => line.trimEnd())
      .filter((line) => line.trim().length > 0);
  });

  const filteredTranscriptLines = computed(() => {
    const query = transcriptSearchQuery.value;
    if (!query) {
      return transcriptLines.value;
    }
    return transcriptLines.value.filter((line) => line.toLowerCase().includes(query));
  });

  const transcriptDisplayText = computed(() => {
    if (!drawerTranscriptText.value) {
      return "";
    }
    const query = transcriptSearchQuery.value;
    if (!query) {
      return drawerTranscriptText.value;
    }
    return filteredTranscriptLines.value.join("\n");
  });

  function setDrawerTab(tab: DrawerTab): void {
    drawerTab.value = tab;
  }

  function syncDrawerSearch(tab: DrawerTab, searchTerm: string): void {
    chaptersSearch.value = tab === "chapters" ? searchTerm : "";
    transcriptSearch.value = tab === "transcript" ? searchTerm : "";
  }

  function resetDrawerData(): void {
    drawerLoadError.value = "";
    drawerChapters.value = [];
    drawerChaptersSource.value = "";
    drawerTranscriptSegments.value = [];
    drawerTranscriptText.value = "";
    drawerTranscriptAssets.value = [];
  }

  function openDrawer(item: PodcastItem, tab: DrawerTab = "overview", searchTerm = ""): void {
    drawerItem.value = item;
    setDrawerTab(tab);
    drawerOpen.value = true;
    syncDrawerSearch(tab, searchTerm);
    void fetchDrawerData(item.ID);
  }

  function closeDrawer(): void {
    drawerOpen.value = false;
  }

  async function fetchDrawerData(id: string): Promise<void> {
    resetDrawerData();
    drawerLoadingChapters.value = true;
    drawerLoadingTranscript.value = true;

    await Promise.all([fetchChapters(id), fetchTranscript(id)]);
  }

  async function fetchChapters(id: string): Promise<void> {
    drawerLoadingChapters.value = true;
    try {
      const response = await episodesApi.getChapters(id);
      applyChaptersResponse(response);
    } catch (error) {
      drawerLoadError.value = getErrorMessage(error, "Failed to load chapters.");
    } finally {
      drawerLoadingChapters.value = false;
    }
  }

  function applyChaptersResponse(response: ChaptersResponse): void {
    drawerChaptersSource.value = response.source || "unknown";
    drawerChapters.value = response.chapters ?? [];
  }

  async function fetchTranscript(id: string): Promise<void> {
    drawerLoadingTranscript.value = true;
    try {
      const response = await episodesApi.getTranscript(id);
      applyTranscriptResponse(response);
    } catch (error) {
      drawerLoadError.value = getErrorMessage(error, "Failed to load transcript.");
    } finally {
      drawerLoadingTranscript.value = false;
    }
  }

  function applyTranscriptResponse(response: TranscriptResponse): void {
    drawerTranscriptStatus.value = response.status || "missing";
    const transcript = response.transcript;
    if (transcript && typeof transcript === "object" && !Array.isArray(transcript)) {
      const maybeSegments = (transcript as { segments?: Array<Record<string, unknown>> }).segments;
      if (Array.isArray(maybeSegments)) {
        drawerTranscriptSegments.value = maybeSegments
          .map((segment) => ({
            start: Number(segment.start ?? segment.start_time ?? 0),
            end: Number(segment.end ?? segment.end_time ?? 0),
            text: String(segment.text ?? segment.transcript ?? "").trim(),
            speaker: typeof segment.speaker === "string" ? segment.speaker : undefined,
          }))
          .filter((segment) => segment.text.length > 0);
        return;
      }
    }

    if (Array.isArray(transcript)) {
      const assets = transcript
        .filter((asset) => asset && typeof asset === "object")
        .map((asset) => asset as Record<string, unknown>);
      const contentAsset = assets.find((asset) => typeof asset.content === "string" && asset.content.trim().length > 0);
      if (contentAsset && typeof contentAsset.content === "string") {
        drawerTranscriptText.value = contentAsset.content;
      }
      drawerTranscriptAssets.value = assets.map((asset) => ({
        url: typeof asset.url === "string" ? asset.url : undefined,
        type: typeof asset.type === "string" ? asset.type : undefined,
        language: typeof asset.language === "string" ? asset.language : undefined,
      }));
      return;
    }

    if (typeof transcript === "string") {
      drawerTranscriptText.value = transcript;
    }
  }

  function drawerTranscriptSummary(): string {
    switch (drawerTranscriptStatus.value) {
      case "available":
        return "Transcript is ready.";
      case "processing":
        return "WhisperX is transcribing this episode.";
      case "pending_whisperx":
        return "Waiting for WhisperX transcription.";
      case "failed":
        return "Transcript failed to generate.";
      default:
        return "No transcript available.";
    }
  }

  function drawerChaptersSummary(): string {
    if (drawerChapters.value.length === 0) {
      return "No chapters available.";
    }
    return `${drawerChapters.value.length} chapters available.`;
  }

  return {
    drawerOpen,
    drawerItem,
    drawerTab,
    drawerChapters,
    drawerChaptersSource,
    drawerTranscriptStatus,
    drawerTranscriptSegments,
    drawerTranscriptText,
    drawerTranscriptAssets,
    drawerLoadingChapters,
    drawerLoadingTranscript,
    drawerLoadError,
    chaptersSearch,
    transcriptSearch,
    filteredChapters,
    filteredTranscriptSegments,
    transcriptLines,
    filteredTranscriptLines,
    transcriptDisplayText,
    openDrawer,
    setDrawerTab,
    closeDrawer,
    fetchDrawerData,
    fetchChapters,
    fetchTranscript,
    drawerTranscriptSummary,
    drawerChaptersSummary,
    drawerTabs,
  };
}
