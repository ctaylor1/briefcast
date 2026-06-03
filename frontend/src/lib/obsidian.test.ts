import { describe, it } from "node:test";
import assert from "node:assert/strict";
import {
  DEFAULT_OBSIDIAN_FOLDER,
  DEFAULT_OBSIDIAN_VAULT,
  buildPodcastSummaryObsidianContent,
  buildPodcastSummaryObsidianUrl,
  buildPodcastTranscriptObsidianContent,
  buildPodcastTranscriptObsidianUrl,
  normalizeObsidianFolder,
  normalizeObsidianVault,
} from "./obsidian.ts";

describe("normalizeObsidianFolder", () => {
  it("defaults blank folders to Clippings", () => {
    assert.equal(normalizeObsidianFolder("  "), DEFAULT_OBSIDIAN_FOLDER);
  });

  it("normalizes nested vault-relative folders", () => {
    assert.equal(normalizeObsidianFolder(" Research\\Podcasts//Summaries "), "Research/Podcasts/Summaries");
  });
});

describe("normalizeObsidianVault", () => {
  it("defaults blank vaults to Vault", () => {
    assert.equal(normalizeObsidianVault("  "), DEFAULT_OBSIDIAN_VAULT);
  });

  it("normalizes vault names", () => {
    assert.equal(normalizeObsidianVault(" Research\\Vault/One "), "ResearchVaultOne");
  });
});

describe("buildPodcastSummaryObsidianContent", () => {
  it("builds markdown with frontmatter for an episode summary", () => {
    const content = buildPodcastSummaryObsidianContent({
      episodeTitle: 'Episode "One"',
      podcastTitle: "Briefcast Daily",
      pubDate: "2026-06-02T12:00:00Z",
      model: "gpt-test",
      summary: "Summary body",
    });

    assert.ok(content.includes('title: "Episode \\"One\\""'));
    assert.ok(content.includes('podcast: "Briefcast Daily"'));
    assert.ok(content.includes("date: 2026-06-02"));
    assert.ok(content.includes('model: "gpt-test"'));
    assert.ok(content.endsWith("# Episode \"One\"\n\nSummary body"));
  });
});

describe("buildPodcastSummaryObsidianUrl", () => {
  it("uses the configured vault and folder in the target file path", () => {
    const url = buildPodcastSummaryObsidianUrl({
      vault: "Research Vault",
      folder: "Clippings/Podcasts",
      episodeTitle: "A/B Episode",
      podcastTitle: "Briefcast",
      summary: "Summary body",
    });

    const query = url.slice("obsidian://new?".length);
    const params = new URLSearchParams(query);

    assert.equal(params.get("vault"), "Research Vault");
    assert.equal(params.get("file"), "Clippings/Podcasts/Briefcast - AB Episode");
    assert.equal(params.get("content")?.includes("Summary body"), true);
  });
});

describe("buildPodcastTranscriptObsidianContent", () => {
  it("builds markdown with frontmatter for an episode transcript", () => {
    const content = buildPodcastTranscriptObsidianContent({
      episodeTitle: "Episode One",
      podcastTitle: "Briefcast Daily",
      pubDate: "2026-06-02T12:00:00Z",
      transcript: "Transcript body",
    });

    assert.ok(content.includes('title: "Episode One"'));
    assert.ok(content.includes('podcast: "Briefcast Daily"'));
    assert.ok(content.includes("date: 2026-06-02"));
    assert.ok(content.includes("tags: [briefcast, podcast-transcript]"));
    assert.ok(content.endsWith("# Episode One Transcript\n\nTranscript body"));
  });
});

describe("buildPodcastTranscriptObsidianUrl", () => {
  it("uses the configured vault and a transcript-specific file path", () => {
    const url = buildPodcastTranscriptObsidianUrl({
      vault: "Research Vault",
      folder: "Clippings/Podcasts",
      episodeTitle: "A/B Episode",
      podcastTitle: "Briefcast",
      transcript: "Transcript body",
    });

    const query = url.slice("obsidian://new?".length);
    const params = new URLSearchParams(query);

    assert.equal(params.get("vault"), "Research Vault");
    assert.equal(params.get("file"), "Clippings/Podcasts/Briefcast - AB Episode - Transcript");
    assert.equal(params.get("content")?.includes("Transcript body"), true);
  });
});
