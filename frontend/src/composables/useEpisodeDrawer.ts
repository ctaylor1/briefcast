import { computed, ref } from "vue";
import { episodesApi, getErrorMessage } from "../lib/api";
import type { Chapter, ChaptersResponse, PodcastItem, SummaryResponse, TranscriptResponse } from "../types/api";

type DrawerTab = "overview" | "chapters" | "transcript" | "summary";
type TranscriptSegment = { start: number; end: number; text: string; speaker?: string };
type TranscriptAsset = { url?: string; type?: string; language?: string };

export function useEpisodeDrawer() {
  const drawerTabs: Array<{ id: DrawerTab; label: string }> = [
    { id: "overview", label: "Overview" },
    { id: "chapters", label: "Chapters" },
    { id: "transcript", label: "Transcript" },
    { id: "summary", label: "Summary" },
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

  const drawerCanonicalTranscript = ref("");

  // Summary state
  const drawerSummaryStatus = ref("not_attempted");
  const drawerSummaryText = ref("");
  const drawerSummaryDate = ref("");
  const drawerSummaryModel = ref("");
  const drawerLoadingSummary = ref(false);

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
    drawerCanonicalTranscript.value = "";
    drawerSummaryStatus.value = "not_attempted";
    drawerSummaryText.value = "";
    drawerSummaryDate.value = "";
    drawerSummaryModel.value = "";
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
    drawerLoadingSummary.value = true;

    await Promise.all([fetchChapters(id), fetchTranscript(id), fetchSummary(id)]);
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
    drawerCanonicalTranscript.value = response.canonicalTranscript ?? "";
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

  async function fetchSummary(id: string): Promise<void> {
    drawerLoadingSummary.value = true;
    drawerSummaryText.value = "";
    drawerSummaryStatus.value = "not_attempted";
    drawerSummaryDate.value = "";
    drawerSummaryModel.value = "";
    try {
      const response: SummaryResponse = await episodesApi.getSummary(id);
      drawerSummaryStatus.value = response.status || "not_attempted";
      drawerSummaryText.value = response.summary ?? "";
      drawerSummaryDate.value = response.generatedAt ?? "";
      drawerSummaryModel.value = response.model ?? "";
    } catch (error) {
      drawerLoadError.value = getErrorMessage(error, "Failed to load summary.");
    } finally {
      drawerLoadingSummary.value = false;
    }
  }

  function drawerTranscriptSummary(): string {
    if (drawerTranscriptStatus.value === "available") {
      return "Transcript is ready.";
    }
    if (drawerTranscriptStatus.value === "processing") {
      return "Briefcast transcription in progress.";
    }
    if (drawerTranscriptStatus.value.startsWith("pending_")) {
      return "Briefcast transcription queued.";
    }
    if (drawerTranscriptStatus.value === "failed") {
      return "Transcript failed to generate.";
    }
    return "No transcript available.";
  }

  function drawerChaptersSummary(): string {
    if (drawerChapters.value.length === 0) {
      return "No chapters available.";
    }
    return `${drawerChapters.value.length} chapters available.`;
  }

  function transcriptFileName(): string {
    const title = drawerItem.value?.Title ?? "transcript";
    const safe = title.replace(/[^a-zA-Z0-9_\- ]/g, "").trim().replace(/\s+/g, "_");
    return `${safe || "transcript"}.txt`;
  }

  function transcriptBlob(): Blob {
    return new Blob([drawerCanonicalTranscript.value], { type: "text/plain" });
  }

  function downloadCanonicalTranscript(): void {
    const url = URL.createObjectURL(transcriptBlob());
    const a = document.createElement("a");
    a.href = url;
    a.download = transcriptFileName();
    a.click();
    URL.revokeObjectURL(url);
  }

  async function sendToApp(scheme: string, appName: string): Promise<void> {
    const file = new File([drawerCanonicalTranscript.value], transcriptFileName(), { type: "text/plain" });
    if (navigator.share) {
      try {
        await navigator.share({ files: [file], title: transcriptFileName() });
        return;
      } catch {
        // share cancelled or unsupported — fall through to custom-scheme attempt
      }
    }
    const url = `${scheme}`;
    const w = window.open(url, "_blank");
    if (!w) {
      alert(`Could not open ${appName}. Make sure the ${appName} desktop app is installed.`);
    } else {
      setTimeout(() => {
        try {
          if (w.closed) return;
          w.close();
          alert(`Could not open ${appName}. Make sure the ${appName} desktop app is installed.`);
        } catch {
          // cross-origin — app likely opened
        }
      }, 2000);
    }
  }

  function sendToChatGPT(): void {
    const text = drawerCanonicalTranscript.value;
    const encoded = encodeURIComponent(text);
    const maxURLLen = 8000;
    if (encoded.length > maxURLLen) {
      downloadCanonicalTranscript();
      alert("The transcript is too large to send directly. It has been downloaded as a file — please upload it to ChatGPT manually.");
      return;
    }
    void sendToApp(`chatgpt://chat?prompt=${encoded}`, "ChatGPT");
  }

  function sendToClaude(): void {
    const text = drawerCanonicalTranscript.value;
    const encoded = encodeURIComponent(text);
    const maxURLLen = 8000;
    if (encoded.length > maxURLLen) {
      downloadCanonicalTranscript();
      alert("The transcript is too large to send directly. It has been downloaded as a file — please upload it to Claude manually.");
      return;
    }
    void sendToApp(`claude://chat?prompt=${encoded}`, "Claude");
  }

  function drawerSummarySummary(): string {
    if (drawerSummaryStatus.value === "available") {
      return "AI summary is ready.";
    }
    if (drawerSummaryStatus.value === "processing") {
      return "Summary is being generated.";
    }
    if (drawerSummaryStatus.value === "pending") {
      return "Summary generation queued.";
    }
    if (drawerSummaryStatus.value === "failed") {
      return "Summary generation failed.";
    }
    return "No AI summary available.";
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
    drawerCanonicalTranscript,
    downloadCanonicalTranscript,
    sendToChatGPT,
    sendToClaude,
    // Summary
    drawerSummaryStatus,
    drawerSummaryText,
    drawerSummaryDate,
    drawerSummaryModel,
    drawerLoadingSummary,
    openDrawer,
    setDrawerTab,
    closeDrawer,
    fetchDrawerData,
    fetchChapters,
    fetchTranscript,
    fetchSummary,
    drawerTranscriptSummary,
    drawerChaptersSummary,
    drawerSummarySummary,
    drawerTabs,
  };
}
