import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { aboutNavItem, groupNavSections, primaryNavItems, routeNavItems } from "./navigation.ts";

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
});
