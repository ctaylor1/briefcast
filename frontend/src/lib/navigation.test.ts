import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { aboutNavItem, groupNavSections, linksNavItem, primaryNavItems, routeNavItems } from "./navigation.ts";

describe("Sidebar navigation", () => {
  it("lists About directly under Settings as the last sidebar link", () => {
    const sections = groupNavSections(routeNavItems);
    assert.deepEqual(
      sections.map((section) => section.title),
      ["Library", "Playback", "System"],
    );
    const system = sections[sections.length - 1];
    assert.deepEqual(
      system.items.map((item) => item.label),
      ["Settings", "About"],
    );
    const allLinks = sections.flatMap((section) => section.items);
    assert.equal(allLinks[allLinks.length - 1], aboutNavItem);
  });

  it("keeps About out of the mobile bottom tabs", () => {
    assert.equal(primaryNavItems.includes(aboutNavItem), false);
    assert.equal(primaryNavItems.length, 7);
  });

  it("lists Most referenced after Summaries, in the sidebar but not the mobile bottom tabs", () => {
    const library = groupNavSections(routeNavItems)[0];
    assert.deepEqual(
      library.items.map((item) => item.label),
      ["Podcasts", "Episodes", "Summaries", "Most referenced", "Downloads", "Add Podcast"],
    );
    assert.equal(primaryNavItems.includes(linksNavItem), false);
  });
});
