import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount } from "@vue/test-utils";
import { nextTick } from "vue";
import UiToastRegion from "./UiToastRegion.vue";
import { clearToasts, toast, useToasts } from "../../lib/toast";

afterEach(() => clearToasts());

describe("UiToastRegion", () => {
  it("announces errors assertively and other notices politely", async () => {
    const wrapper = mount(UiToastRegion);
    assert.ok(wrapper.find('[role="status"][aria-live="polite"]').exists(), "polite region is always present");
    assert.ok(wrapper.find('[role="alert"][aria-live="assertive"]').exists(), "assertive region is always present");
    toast.success("Episode download queued.");
    toast.error("Could not cancel download.");
    await nextTick();
    assert.match(wrapper.get('[role="status"]').text(), /Episode download queued/);
    assert.match(wrapper.get('[role="alert"]').text(), /Could not cancel download/);
    assert.doesNotMatch(wrapper.get('[role="status"]').text(), /Could not cancel/);
  });

  it("dismisses a toast from its button", async () => {
    const wrapper = mount(UiToastRegion);
    toast.error("Could not save.");
    await nextTick();
    await wrapper.get('button[aria-label="Dismiss"]').trigger("click");
    assert.equal(useToasts().toasts.length, 0);
  });
});
