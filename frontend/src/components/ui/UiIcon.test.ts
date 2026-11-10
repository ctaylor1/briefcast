import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount } from "@vue/test-utils";
import UiIcon from "./UiIcon.vue";
import { ICONS, type IconName } from "../../lib/icons";

describe("UiIcon", () => {
  it("renders a decorative, sized SVG", () => {
    const wrapper = mount(UiIcon, { props: { name: "download", size: 20 } });
    const svg = wrapper.get("svg");
    assert.equal(svg.attributes("aria-hidden"), "true");
    assert.equal(svg.attributes("focusable"), "false");
    assert.equal(svg.attributes("width"), "20");
    assert.equal(svg.attributes("fill"), "none");
    assert.equal(svg.findAll("path").length, ICONS.download.paths.length);
  });

  it("fills solid icons", () => {
    const svg = mount(UiIcon, { props: { name: "play" } }).get("svg");
    assert.equal(svg.attributes("fill"), "currentColor");
    assert.equal(svg.attributes("stroke"), "none");
  });

  it("has well-formed path data for every icon", () => {
    for (const [name, icon] of Object.entries(ICONS) as Array<[IconName, { paths: string[] }]>) {
      assert.ok(icon.paths.length > 0, name);
      for (const d of icon.paths) {
        assert.match(d, /^M[\d.\s,-]/, `${name}: ${d}`);
      }
    }
  });
});
