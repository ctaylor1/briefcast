import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { activeNavItem, commandNavItems, librarySubItems, paletteOnlyItems, primaryNavItems } from "./navigation.ts";
import { ICONS } from "./icons.ts";

describe("navigation", () => {
  it("has five destinations", () => {
    assert.deepEqual(primaryNavItems.map((item) => item.label), ["Library", "Episodes", "Summaries", "Activity", "Settings"]);
  });

  it("keeps Add, Player, Downloads and About out of the nav but in the command palette", () => {
    const navLabels = [...primaryNavItems, ...librarySubItems].map((item) => item.label);
    for (const label of ["Add podcast", "Now playing", "Downloads", "About"]) {
      assert.ok(!navLabels.includes(label), label);
      assert.ok(commandNavItems.some((item) => item.label === label), label);
    }
    assert.equal(paletteOnlyItems.length, 4);
  });

  it("puts Most referenced under Library", () => {
    assert.deepEqual(librarySubItems.map((item) => item.to), ["/links"]);
  });

  it("finds the active item, including detail pages", () => {
    assert.equal(activeNavItem("/")?.label, "Library");
    assert.equal(activeNavItem("/podcasts/abc")?.label, "Library");
    assert.equal(activeNavItem("/episodes/abc")?.label, "Episodes");
    assert.equal(activeNavItem("/activity")?.label, "Activity");
    assert.equal(activeNavItem("/links")?.label, "Most referenced");
  });

  it("uses icons that exist", () => {
    for (const item of commandNavItems) {
      assert.ok(item.icon in ICONS, item.icon);
    }
  });
});
