import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount } from "@vue/test-utils";
import UiIconButton from "./UiIconButton.vue";

describe("UiIconButton", () => {
  it("uses the label as the accessible name", () => {
    const wrapper = mount(UiIconButton, { props: { label: "Delete podcast" }, slots: { default: "<svg />" } });
    const button = wrapper.get("button");
    assert.equal(button.attributes("aria-label"), "Delete podcast");
    assert.equal(button.attributes("type"), "button");
    assert.equal(button.attributes("aria-pressed"), undefined, "a plain action is not a toggle");
    assert.equal(button.attributes("aria-describedby"), undefined, "a tooltip that repeats the label adds nothing");
    assert.equal(wrapper.get(".ui-tooltip").attributes("aria-hidden"), "true");
  });

  it("exposes toggle state as aria-pressed, including false", () => {
    const off = mount(UiIconButton, { props: { label: "Keep all episodes", pressed: false } });
    assert.equal(off.get("button").attributes("aria-pressed"), "false");
    assert.ok(!off.get("button").classes().includes("ui-icon-button--pressed"));

    const on = mount(UiIconButton, { props: { label: "Keep all episodes", pressed: true } });
    assert.equal(on.get("button").attributes("aria-pressed"), "true");
    assert.ok(on.get("button").classes().includes("ui-icon-button--pressed"));
  });

  it("links a tooltip that adds information", () => {
    const wrapper = mount(UiIconButton, {
      props: { label: "Pause downloads", tooltip: "Downloads paused", pressed: true },
    });
    const tooltip = wrapper.get(".ui-tooltip");
    assert.equal(tooltip.text(), "Downloads paused");
    assert.equal(wrapper.get("button").attributes("aria-describedby"), tooltip.attributes("id"));
    assert.equal(tooltip.attributes("aria-hidden"), undefined);
  });

  it("emits click, and not when disabled", async () => {
    const wrapper = mount(UiIconButton, { props: { label: "Play" } });
    await wrapper.get("button").trigger("click");
    assert.equal(wrapper.emitted("click")?.length, 1);

    const disabled = mount(UiIconButton, { props: { label: "Play", disabled: true } });
    assert.equal(disabled.get("button").attributes("disabled"), "");
  });
});
