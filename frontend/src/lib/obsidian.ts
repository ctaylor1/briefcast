export const DEFAULT_OBSIDIAN_FOLDER = "Clippings";
export const DEFAULT_OBSIDIAN_VAULT = "Vault";

interface PodcastObsidianInput {
  vault?: string;
  folder?: string;
  episodeTitle: string;
  podcastTitle?: string;
  pubDate?: string;
}

export interface PodcastSummaryObsidianInput extends PodcastObsidianInput {
  model?: string;
  summary: string;
}

export interface PodcastTranscriptObsidianInput extends PodcastObsidianInput {
  transcript: string;
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

export function normalizeObsidianVault(vault?: string): string {
  const normalized = (vault || DEFAULT_OBSIDIAN_VAULT)
    .replace(/[\\/]/g, "")
    .replace(/[\x00-\x1f]/g, "")
    .replace(/\s+/g, " ")
    .trim();

  return normalized || DEFAULT_OBSIDIAN_VAULT;
}

export function sanitizeObsidianFileName(value: string): string {
  return normalizeObsidianPathPart(value).replace(/\//g, "").trim() || "Briefcast Summary";
}

function obsidianFilePath(input: PodcastObsidianInput, suffix = ""): string {
  const folder = normalizeObsidianFolder(input.folder);
  const baseName = sanitizeObsidianFileName(`${input.podcastTitle || "Unknown podcast"} - ${input.episodeTitle}`);
  const fileName = suffix ? sanitizeObsidianFileName(`${baseName} - ${suffix}`) : baseName;
  return `${folder}/${fileName}`;
}

function buildObsidianNewUrl(input: PodcastObsidianInput, file: string, content: string): string {
  const query = [
    `vault=${encodeURIComponent(normalizeObsidianVault(input.vault))}`,
    `file=${encodeURIComponent(file)}`,
    `content=${encodeURIComponent(content)}`,
  ].join("&");
  return `obsidian://new?${query}`;
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
  const file = obsidianFilePath(input);
  const content = buildPodcastSummaryObsidianContent(input);

  return buildObsidianNewUrl(input, file, content);
}

export function buildPodcastTranscriptObsidianContent(input: PodcastTranscriptObsidianInput): string {
  const title = input.episodeTitle.trim() || "Untitled episode";
  const podcast = input.podcastTitle?.trim() || "Unknown podcast";
  const date = isoDate(input.pubDate);

  const frontmatter = [
    "---",
    `title: ${yamlQuoted(title)}`,
    `podcast: ${yamlQuoted(podcast)}`,
    date ? `date: ${date}` : "",
    "tags: [briefcast, podcast-transcript]",
    "---",
  ]
    .filter(Boolean)
    .join("\n");

  return `${frontmatter}\n\n# ${title} Transcript\n\n${input.transcript.trim()}`;
}

export function buildPodcastTranscriptObsidianUrl(input: PodcastTranscriptObsidianInput): string {
  const file = obsidianFilePath(input, "Transcript");
  const content = buildPodcastTranscriptObsidianContent(input);

  return buildObsidianNewUrl(input, file, content);
}
