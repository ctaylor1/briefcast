import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount } from "@vue/test-utils";
import SummariesView from "./SummariesView.vue";
import { clearReadingList, readingPosition } from "../lib/readingList";
import { fakeServer, flushPromises, type FakeServer } from "../test-support/http";
import { makeRouter } from "../test-support/router";

let server: FakeServer | null = null;
afterEach(() => {
  server?.restore();
  clearReadingList();
});

function summary(id: string, title: string) {
  return { id, episodeTitle: title, podcastTitle: "Show", readTime: 3, isPlayed: false, isFavorited: false, excerpt: "..." };
}

describe("Summaries", () => {
  it("links each summary to its episode page and hands over the list for J/K", async () => {
    server = fakeServer({
      "GET /summaries": () => ({
        summaries: [summary("s1", "One"), summary("s/2", "Two")],
        filter: { page: 1, count: 20, nextPage: 0, previousPage: 0, totalPages: 1, totalCount: 2 },
      }),
      "GET /podcasts": () => [],
    });
    const router = await makeRouter("/summaries");
    const wrapper = mount(SummariesView, { global: { plugins: [router] } });
    await flushPromises();
    const links = wrapper.findAll("a.summaries-list__content");
    assert.deepEqual(links.map((a) => a.attributes("href")), ["/episodes/s1?tab=summary", "/episodes/s%2F2?tab=summary"]);
    await links[1]!.trigger("click");
    await flushPromises();
    assert.deepEqual(readingPosition("s/2"), { previous: "s1", next: null, index: 1, total: 2, label: "Summary" });
    assert.equal(router.currentRoute.value.path, "/episodes/s%2F2");
    assert.equal(wrapper.find(".reader").exists(), false, "no in-page reader any more");
    wrapper.unmount();
  });
});
