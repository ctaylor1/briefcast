import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import UiInput from "./UiInput.vue";
import UiSelect from "./UiSelect.vue";

describe("form controls", () => {
  it("give every instance its own id, so each label points at its own field", () => {
    const Form = defineComponent({
      render: () => [
        h(UiInput, { label: "First" }),
        h(UiInput, { label: "Second" }),
        h(UiSelect, { label: "Third" }, () => [h("option", { value: "a" }, "A")]),
        h(UiSelect, { label: "Fourth" }, () => [h("option", { value: "b" }, "B")]),
      ],
    });
    const wrapper = mount(Form);
    const controls = wrapper.findAll("input, select");
    const ids = controls.map((c) => c.attributes("id"));
    assert.equal(new Set(ids).size, 4, `ids: ${ids.join(", ")}`);
    const labels = wrapper.findAll("label").map((l) => l.attributes("for"));
    assert.deepEqual(labels, ids);
  });
});
