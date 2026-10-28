import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { buildPodcastMarkdownObsidianUrl, normalizeObsidianFolder, normalizeObsidianVault } from "./obsidian.ts";

describe("Obsidian destination", () => {
  it("normalizes a vault-relative folder and vault name", () => {
    assert.equal(normalizeObsidianFolder(" Research\\Podcasts//Summaries "), "Research/Podcasts/Summaries");
    assert.equal(normalizeObsidianFolder(".././"), "Clippings");
    assert.equal(normalizeObsidianVault(" Research\\Vault/One "), "ResearchVaultOne");
  });
  it("passes the exact server Markdown without changing IDs or provenance", () => {
    const markdown = '---\nmetadata_schema: "podcast-markdown"\nschema_version: "1.2"\n---\n\n# A note\n';
    const url = buildPodcastMarkdownObsidianUrl({vault:"Research Vault", folder:"Podcasts", fileName:"episode-r2.md", markdown});
    const params = new URLSearchParams(url.slice("obsidian://new?".length));
    assert.equal(params.get("vault"), "Research Vault");
    assert.equal(params.get("file"), "Podcasts/episode-r2");
    assert.equal(params.get("content"), markdown);
    assert.equal(params.has("overwrite"), false);
  });
});
