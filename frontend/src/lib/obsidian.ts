export const DEFAULT_OBSIDIAN_FOLDER = "Clippings";

export interface PodcastSummaryObsidianInput {
  folder?: string;
  episodeTitle: string;
  podcastTitle?: string;
  pubDate?: string;
  model?: string;
  summary: string;
}

function yamlQuoted(value: string): string {
  return JSON.stringify(value);
}

function isoDate(value?: string): string {
  if (!value) {
    return "";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return "";
  }
  return parsed.toISOString().split("T")[0] ?? "";
}

function normalizeObsidianPathPart(value: string): string {
  return value
    .replace(/[\\:*?"<>|#^[\]\x00-\x1f]/g, "")
    .replace(/\s+/g, " ")
    .trim();
}

export function normalizeObsidianFolder(folder?: string): string {
  const raw = (folder || DEFAULT_OBSIDIAN_FOLDER).replace(/\\/g, "/").trim();
  const parts = raw
    .split("/")
    .map(normalizeObsidianPathPart)
    .filter((part) => part.length > 0 && part !== "." && part !== "..");

  return parts.length > 0 ? parts.join("/") : DEFAULT_OBSIDIAN_FOLDER;
}

export function sanitizeObsidianFileName(value: string): string {
  return normalizeObsidianPathPart(value).replace(/\//g, "").trim() || "Briefcast Summary";
}

export function buildPodcastSummaryObsidianContent(input: PodcastSummaryObsidianInput): string {
  const title = input.episodeTitle.trim() || "Untitled episode";
  const podcast = input.podcastTitle?.trim() || "Unknown podcast";
  const date = isoDate(input.pubDate);
  const model = input.model?.trim() || "";

  const frontmatter = [
    "---",
    `title: ${yamlQuoted(title)}`,
    `podcast: ${yamlQuoted(podcast)}`,
    date ? `date: ${date}` : "",
    model ? `model: ${yamlQuoted(model)}` : "",
    "tags: [briefcast, podcast-summary]",
    "---",
  ]
    .filter(Boolean)
    .join("\n");

  return `${frontmatter}\n\n# ${title}\n\n${input.summary.trim()}`;
}

export function buildPodcastSummaryObsidianUrl(input: PodcastSummaryObsidianInput): string {
  const folder = normalizeObsidianFolder(input.folder);
  const fileName = sanitizeObsidianFileName(`${input.podcastTitle || "Unknown podcast"} - ${input.episodeTitle}`);
  const file = `${folder}/${fileName}`;
  const content = buildPodcastSummaryObsidianContent(input);

  return `obsidian://new?file=${encodeURIComponent(file)}&content=${encodeURIComponent(content)}`;
}
