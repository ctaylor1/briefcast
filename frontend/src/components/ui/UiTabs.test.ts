import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount, type VueWrapper } from "@vue/test-utils";
import { defineComponent, h, ref } from "vue";
import UiTabs from "./UiTabs.vue";

const tabs = [
  { id: "a", label: "Alpha" },
  { id: "b", label: "Beta" },
  { id: "c", label: "Gamma" },
];

let wrapper: VueWrapper | null = null;
afterEach(() => {
  wrapper?.unmount();
  document.body.innerHTML = "";
});

const Host = defineComponent({
  setup() {
    const active = ref("a");
    return () =>
      h(UiTabs, { tabs, label: "Sections", modelValue: active.value, "onUpdate:modelValue": (v: string) => (active.value = v) }, {
        default: ({ active: current }: { active: string }) => h("p", { class: "panel" }, `Panel ${current}`),
      });
  },
});

describe("UiTabs", () => {
  it("links tabs to the panel and marks the selected one", () => {
    wrapper = mount(Host, { attachTo: document.body });
    const [first, second] = wrapper.findAll('[role="tab"]');
    assert.equal(wrapper.get('[role="tablist"]').attributes("aria-label"), "Sections");
    assert.equal(first!.attributes("aria-selected"), "true");
    assert.equal(first!.attributes("tabindex"), "0");
    assert.equal(second!.attributes("tabindex"), "-1");
    const panel = wrapper.get('[role="tabpanel"]');
    assert.equal(panel.attributes("aria-labelledby"), first!.attributes("id"));
    assert.equal(first!.attributes("aria-controls"), panel.attributes("id"));
    assert.equal(wrapper.get(".panel").text(), "Panel a");
  });

  it("moves with the arrow keys, Home and End, and follows focus", async () => {
    wrapper = mount(Host, { attachTo: document.body });
    const tab = (i: number) => wrapper!.findAll('[role="tab"]')[i]!;
    await tab(0).trigger("keydown", { key: "ArrowRight" });
    assert.equal(wrapper.get(".panel").text(), "Panel b");
    assert.equal(document.activeElement, tab(1).element);
    await tab(1).trigger("keydown", { key: "End" });
    assert.equal(wrapper.get(".panel").text(), "Panel c");
    await tab(2).trigger("keydown", { key: "ArrowRight" });
    assert.equal(wrapper.get(".panel").text(), "Panel a", "wraps around");
    await tab(0).trigger("keydown", { key: "ArrowLeft" });
    assert.equal(wrapper.get(".panel").text(), "Panel c");
    await tab(2).trigger("keydown", { key: "Home" });
    assert.equal(wrapper.get(".panel").text(), "Panel a");
  });
});
