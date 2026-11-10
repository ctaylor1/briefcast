import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount } from "@vue/test-utils";
import UiButton from "./UiButton.vue";

describe("UiButton", () => {
  it("renders a type=button with variant and size classes by default", () => {
    const wrapper = mount(UiButton, { slots: { default: "Save" } });
    const button = wrapper.get("button");
    assert.equal(button.attributes("type"), "button");
    assert.equal(button.text(), "Save");
    assert.ok(button.classes().includes("ui-button--primary"));
    assert.ok(button.classes().includes("ui-button--md"));
  });

  it("emits clicks unless disabled", async () => {
    const enabled = mount(UiButton, { slots: { default: "Go" } });
    await enabled.get("button").trigger("click");
    assert.equal(enabled.emitted("click")?.length, 1);

    const disabled = mount(UiButton, { props: { disabled: true }, slots: { default: "Go" } });
    assert.equal(disabled.get("button").attributes("disabled"), "");
  });
});
