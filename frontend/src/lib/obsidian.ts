export const DEFAULT_OBSIDIAN_FOLDER = "Clippings";
export const DEFAULT_OBSIDIAN_VAULT = "Vault";

interface PodcastMarkdownInput {
  vault?: string;
  folder?: string;
  fileName: string;
  markdown: string;
}

function normalizeObsidianPathPart(value: string): string {
  return value.replace(/[\\:*?"<>|#^[\]\x00-\x1f]/g, "").replace(/\s+/g, " ").trim();
}

export function normalizeObsidianFolder(folder?: string): string {
  const parts = (folder || DEFAULT_OBSIDIAN_FOLDER).replace(/\\/g, "/").trim()
    .split("/").map(normalizeObsidianPathPart)
    .filter((part) => part.length > 0 && part !== "." && part !== "..");
  return parts.length > 0 ? parts.join("/") : DEFAULT_OBSIDIAN_FOLDER;
}

export function normalizeObsidianVault(vault?: string): string {
  return (vault || DEFAULT_OBSIDIAN_VAULT).replace(/[\\/\x00-\x1f]/g, "").replace(/\s+/g, " ").trim() || DEFAULT_OBSIDIAN_VAULT;
}

export function buildPodcastMarkdownObsidianUrl(input: PodcastMarkdownInput): string {
  const fileName = normalizeObsidianPathPart(input.fileName).replace(/\//g, "").replace(/\.md$/i, "");
  const query = [
    `vault=${encodeURIComponent(normalizeObsidianVault(input.vault))}`,
    `file=${encodeURIComponent(`${normalizeObsidianFolder(input.folder)}/${fileName}`)}`,
    `content=${encodeURIComponent(input.markdown)}`,
  ].join("&");
  return `obsidian://new?${query}`;
}

// The server's document is authoritative. Never reconstruct historical metadata
// from UI defaults, the current date, or the currently configured model.
export function sendPodcastMarkdownToObsidian(input: PodcastMarkdownInput): void {
  if (!input.markdown) {
    alert("Markdown export is unavailable. Reload the episode or check the server logs.");
    return;
  }
  const url = buildPodcastMarkdownObsidianUrl(input);
  if (url.length > 8000) {
    const blobUrl = URL.createObjectURL(new Blob([input.markdown], { type: "text/markdown;charset=utf-8" }));
    const link = document.createElement("a");
    link.href = blobUrl;
    link.download = input.fileName;
    link.click();
    setTimeout(() => URL.revokeObjectURL(blobUrl), 1000);
    alert("This note is too large for an Obsidian link. The Markdown file has been downloaded; move it into your vault.");
    return;
  }
  window.location.href = url;
}
