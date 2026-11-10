import type { TranscriptResponse } from "../types/api";

export type TranscriptSegment = { start: number; end: number; text: string; speaker?: string };
export type TranscriptAsset = { url?: string; type?: string; language?: string };

export interface ParsedTranscript {
  segments: TranscriptSegment[];
  text: string;
  assets: TranscriptAsset[];
}

// The transcript endpoint returns timed segments, a list of feed-provided assets, or plain
// text. One parser for the episode drawer and the Now playing view.
export function parseTranscript(response: Pick<TranscriptResponse, "transcript" | "canonicalTranscript">): ParsedTranscript {
  const transcript = response.transcript as unknown;
  const canonical = response.canonicalTranscript ?? "";
  if (transcript && typeof transcript === "object" && !Array.isArray(transcript)) {
    const segments = (transcript as { segments?: Array<Record<string, unknown>> }).segments;
    if (Array.isArray(segments)) {
      return {
        segments: segments
          .map((segment) => ({
            start: Number(segment.start ?? segment.start_time ?? 0),
            end: Number(segment.end ?? segment.end_time ?? 0),
            text: String(segment.text ?? segment.transcript ?? "").trim(),
            speaker: typeof segment.speaker === "string" ? segment.speaker : undefined,
          }))
          .filter((segment) => segment.text.length > 0),
        text: "",
        assets: [],
      };
    }
  }
  if (Array.isArray(transcript)) {
    const assets = transcript
      .filter((asset): asset is Record<string, unknown> => !!asset && typeof asset === "object")
      .map((asset) => asset);
    const withContent = assets.find((asset) => typeof asset.content === "string" && asset.content.trim().length > 0);
    return {
      segments: [],
      text: typeof withContent?.content === "string" ? withContent.content : canonical,
      assets: assets.map((asset) => ({
        url: typeof asset.url === "string" ? asset.url : undefined,
        type: typeof asset.type === "string" ? asset.type : undefined,
        language: typeof asset.language === "string" ? asset.language : undefined,
      })),
    };
  }
  return { segments: [], text: typeof transcript === "string" && transcript ? transcript : canonical, assets: [] };
}

// Index of the segment playing at `seconds`, or -1.
export function activeSegmentIndex(segments: TranscriptSegment[], seconds: number): number {
  for (let i = segments.length - 1; i >= 0; i -= 1) {
    if (seconds >= segments[i]!.start) {
      return i;
    }
  }
  return -1;
}
