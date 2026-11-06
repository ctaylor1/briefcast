import { describe, it } from "node:test";
import assert from "node:assert/strict";
import {
  buildTopLinksQuery,
  DEFAULT_LINK_FILTERS,
  groupLinksBySection,
  hasActiveLinkFilters,
  linkKindLabel,
  linkPlatformLabel,
  linkTargetLabel,
  pluralize,
  safeExternalUrl,
  showNoteLinkLabel,
} from "./links.ts";
import type { ShowNoteLink } from "../types/api";

function link(url: string, contextLabel = "", title = ""): ShowNoteLink {
  return { ID: url, URL: url, Title: title, Domain: "", Position: 0, ContextLabel: contextLabel, LinkTargetID: null };
}

describe("safeExternalUrl", () => {
  it("keeps http and https URLs", () => {
    assert.equal(safeExternalUrl("https://x.com/jason"), "https://x.com/jason");
    assert.equal(safeExternalUrl("http://example.com/a?b=1#c"), "http://example.com/a?b=1#c");
    assert.equal(safeExternalUrl("HTTPS://EXAMPLE.com/a"), "https://example.com/a");
  });

  it("rejects every other scheme, including obfuscated ones", () => {
    const unsafe = [
      "javascript:alert(1)",
      "JAVASCRIPT:alert(1)",
      " javascript:alert(1)",
      "java\tscript:alert(1)",
      "java\nscript:alert(1)",
      "data:text/html,<script>alert(1)</script>",
      "vbscript:msgbox(1)",
      "mailto:someone@example.com",
      "file:///etc/passwd",
    ];
    for (const url of unsafe) {
      assert.equal(safeExternalUrl(url), null, JSON.stringify(url));
    }
  });

  it("rejects relative, empty, and missing values", () => {
    for (const url of ["/podcastitems/1", "//example.com/a", "not a url", "", null, undefined]) {
      assert.equal(safeExternalUrl(url), null, JSON.stringify(url));
    }
  });
});

describe("linkTargetLabel", () => {
  const target = (kind: string, platform: string, handle: string | undefined, canonicalUrl: string) => ({
    kind: kind as "profile",
    platform,
    handle,
    canonicalUrl,
  });

  it("names social accounts and posts by handle", () => {
    assert.equal(linkTargetLabel(target("profile", "x", "jason", "https://x.com/jason")), "@jason");
    assert.equal(linkTargetLabel(target("post", "x", "jason", "https://x.com/jason/status/1")), "Post by @jason");
  });

  it("does not put an @ on LinkedIn slugs", () => {
    assert.equal(linkTargetLabel(target("profile", "linkedin", "jane-doe-123", "https://linkedin.com/in/jane-doe-123")), "jane-doe-123");
  });

  it("falls back to the URL without a scheme", () => {
    assert.equal(linkTargetLabel(target("web", "web", undefined, "https://www.reuters.com/business/story/")), "reuters.com/business/story");
    assert.equal(linkTargetLabel(target("post", "x", undefined, "https://x.com/i/status/1")), "x.com/i/status/1");
    assert.equal(linkTargetLabel(target("shortener", "web", undefined, "https://rb.gy/abc")), "rb.gy/abc");
  });
});

describe("showNoteLinkLabel", () => {
  it("prefers the link text and falls back to a readable URL", () => {
    assert.equal(showNoteLinkLabel(link("https://x.com/jason", "", " Jason ")), "Jason");
    assert.equal(showNoteLinkLabel(link("https://www.x.com/jason/")), "x.com/jason");
  });
});

describe("groupLinksBySection", () => {
  it("keeps first-seen order and merges repeated sections", () => {
    const sections = groupLinksBySection([
      link("https://a.test", "Follow on X"),
      link("https://b.test", "Referenced in the show"),
      link("https://c.test", " Follow on X "),
    ]);
    assert.deepEqual(
      sections.map((section) => [section.heading, section.links.map((item) => item.URL)]),
      [
        ["Follow on X", ["https://a.test", "https://c.test"]],
        ["Referenced in the show", ["https://b.test"]],
      ],
    );
  });

  it("names unlabeled links by whether other sections exist", () => {
    const mixed = groupLinksBySection([link("https://a.test"), link("https://b.test", "Sponsors")]);
    assert.deepEqual(mixed.map((section) => section.heading), ["Other links", "Sponsors"]);

    const onlyUnlabeled = groupLinksBySection([link("https://a.test"), link("https://b.test")]);
    assert.deepEqual(onlyUnlabeled.map((section) => section.heading), ["Links"]);
  });

  it("gives duplicate headings distinct keys", () => {
    const sections = groupLinksBySection([link("https://a.test"), link("https://b.test", "Other links")]);
    assert.notEqual(sections[0].key, sections[1].key);
  });

  it("returns nothing for no links", () => {
    assert.deepEqual(groupLinksBySection([]), []);
  });
});

describe("buildTopLinksQuery", () => {
  it("sends nothing for the default filters", () => {
    assert.deepEqual(buildTopLinksQuery({ ...DEFAULT_LINK_FILTERS }), {});
  });

  it("sends only the filters that are set", () => {
    assert.deepEqual(
      buildTopLinksQuery({
        kind: "post",
        platform: "x",
        podcastId: "abc",
        since: "2026-01-01",
        until: "2026-02-01",
        includeBoilerplate: true,
      }),
      { kind: "post", platform: "x", podcastId: "abc", since: "2026-01-01", until: "2026-02-01", includeBoilerplate: true },
    );
    assert.deepEqual(buildTopLinksQuery({ ...DEFAULT_LINK_FILTERS, platform: "youtube" }), { platform: "youtube" });
  });
});

describe("hasActiveLinkFilters", () => {
  it("detects any change from the defaults", () => {
    assert.equal(hasActiveLinkFilters({ ...DEFAULT_LINK_FILTERS }), false);
    assert.equal(hasActiveLinkFilters({ ...DEFAULT_LINK_FILTERS, since: "2026-01-01" }), true);
    assert.equal(hasActiveLinkFilters({ ...DEFAULT_LINK_FILTERS, includeBoilerplate: true }), true);
  });
});

describe("display labels", () => {
  it("names known kinds and platforms, and passes unknown ones through", () => {
    assert.equal(linkKindLabel("shortener"), "Short link");
    assert.equal(linkKindLabel("future"), "future");
    assert.equal(linkPlatformLabel("tiktok"), "TikTok");
    assert.equal(linkPlatformLabel("future"), "future");
  });

  it("pluralizes counts", () => {
    assert.equal(pluralize(1, "episode"), "1 episode");
    assert.equal(pluralize(0, "episode"), "0 episodes");
    assert.equal(pluralize(2, "podcast"), "2 podcasts");
  });
});
