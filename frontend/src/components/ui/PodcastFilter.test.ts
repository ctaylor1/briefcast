import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount, type VueWrapper } from "@vue/test-utils";
import PodcastFilter from "./PodcastFilter.vue";
import { flushPromises } from "../../test-support/http";

const options = [
  { id: "b", title: "Beta Hour" },
  { id: "a", title: "Alpha Show" },
  { id: "c", title: "Gamma Weekly" },
];

let wrapper: VueWrapper | null = null;
afterEach(() => {
  wrapper?.unmount();
  document.body.innerHTML = "";
});

async function mountOpen(modelValue: string[] = []) {
  wrapper = mount(PodcastFilter, { attachTo: document.body, props: { options, modelValue } });
  await wrapper.get("button").trigger("click");
  await flushPromises();
  return wrapper;
}

describe("PodcastFilter", () => {
  it("summarises the selection on its trigger", async () => {
    wrapper = mount(PodcastFilter, { props: { options, modelValue: [] } });
    assert.match(wrapper.get("button").text(), /All podcasts\s*No filter/);
    await wrapper.setProps({ modelValue: ["c"] });
    assert.match(wrapper.get("button").text(), /Gamma Weekly\s*1 selected/);
    await wrapper.setProps({ modelValue: ["a", "c"] });
    assert.match(wrapper.get("button").text(), /2 podcasts\s*2 selected/);
  });

  it("lists podcasts alphabetically and filters them by search", async () => {
    const w = await mountOpen();
    const titles = () => w.findAll(".podcast-filter__option span").map((s) => s.text());
    assert.deepEqual(titles(), ["Alpha Show", "Beta Hour", "Gamma Weekly"]);
    await w.get('input[type="search"]').setValue("gam");
    assert.deepEqual(titles(), ["Gamma Weekly"]);
    await w.get('input[type="search"]').setValue("zzz");
    assert.match(w.text(), /No podcasts match/);
  });

  it("toggles podcasts and clears the selection", async () => {
    const w = await mountOpen(["a"]);
    const boxes = w.findAll('input[type="checkbox"]');
    assert.equal((boxes[0]!.element as HTMLInputElement).checked, true);
    await boxes[1]!.trigger("change");
    assert.deepEqual(w.emitted("update:modelValue")?.at(-1), [["a", "b"]]);
    await boxes[0]!.trigger("change");
    assert.deepEqual(w.emitted("update:modelValue")?.at(-1), [[]]);
    const clear = w.findAll("button").find((b) => b.text() === "Clear")!;
    await clear.trigger("click");
    assert.deepEqual(w.emitted("update:modelValue")?.at(-1), [[]]);
  });

  it("closes on Escape", async () => {
    const w = await mountOpen();
    assert.ok(document.querySelector(".podcast-filter__panel"));
    document.activeElement?.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await flushPromises();
    assert.equal(document.querySelector(".podcast-filter__panel"), null);
    assert.equal(w.get("button").attributes("aria-expanded"), "false");
  });
});
