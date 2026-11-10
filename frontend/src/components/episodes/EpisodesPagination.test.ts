import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount } from "@vue/test-utils";
import EpisodesPagination from "./EpisodesPagination.vue";

const base = { page: 2, totalPages: 5, totalCount: 48, hasPrevious: true, hasNext: true };

describe("EpisodesPagination", () => {
  it("counts episodes by default", () => {
    const wrapper = mount(EpisodesPagination, { props: base });
    assert.match(wrapper.text(), /Showing page 2 of 5 \(48 episodes\)/);
  });

  it("uses the noun it is given", () => {
    const wrapper = mount(EpisodesPagination, { props: { ...base, noun: "summaries" } });
    assert.match(wrapper.text(), /\(48 summaries\)/);
  });
});
