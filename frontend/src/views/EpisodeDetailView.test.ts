import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount, type VueWrapper } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { RouterView } from "vue-router";
import EpisodeDetailView from "./EpisodeDetailView.vue";
import { usePlayer } from "../composables/usePlayer";
import { clearReadingList, setReadingList } from "../lib/readingList";
import { clearToasts } from "../lib/toast";
import { makeEpisode } from "../test-support/fixtures";
import { fakeServer, flushPromises, type FakeServer } from "../test-support/http";
import { makeRouter } from "../test-support/router";

const A = "eeeeeeee-0000-4000-8000-00000000000a";
const B = "eeeeeeee-0000-4000-8000-00000000000b";
const player = usePlayer();
let server: FakeServer | null = null;
let wrapper: VueWrapper | null = null;

afterEach(() => {
  wrapper?.unmount();
  server?.restore();
  player.stop();
  clearReadingList();
  clearToasts();
  document.body.innerHTML = "";
});

function episodeRoutes(id: string, title: string) {
  return {
    [`GET /podcastitems/${id}`]: () =>
      makeEpisode({ ID: id, Title: title, Summary: '<p>Notes<script>alert(1)</script></p>', DownloadStatus: 2 }),
    [`GET /podcastitems/${id}/chapters`]: () => ({ source: "file", chapters: [{ title: "Intro", startSeconds: 0 }, { title: "Ads", startSeconds: 90 }] }),
    [`GET /podcastitems/${id}/transcript`]: () => ({
      status: "available",
      canonicalTranscript: "Hello there.",
      transcript: { segments: [{ start: 0, end: 4, text: "Hello there." }, { start: 4, end: 9, text: "Second line.", speaker: "Guest" }] },
    }),
    [`GET /podcastitems/${id}/summary`]: () => ({
      status: "available",
      summary: "# Big idea\n\nText.\n\n## Part one\n\nMore.\n\n## Part two\n\n<img src=x onerror=alert(1)>Last.",
      model: "test-model",
    }),
    [`GET /podcastitems/${id}/links`]: () => ({ links: [] }),
  };
}

const Shell = defineComponent({ render: () => h(RouterView) });

async function open(path: string, extra: Record<string, () => unknown> = {}) {
  server = fakeServer({
    ...episodeRoutes(A, "Episode A"),
    ...episodeRoutes(B, "Episode B"),
    "GET /settings": () => ({}),
    ...extra,
  });
  const router = await makeRouter(path, [{ path: "/episodes/:id", component: EpisodeDetailView }]);
  wrapper = mount(Shell, { global: { plugins: [router] }, attachTo: document.body });
  await flushPromises();
  return router;
}

describe("Episode page", () => {
  it("shows the episode with sanitized show notes on the overview", async () => {
    await open(`/episodes/${A}`);
    assert.equal(wrapper!.get(".episode-page__title").text(), "Episode A");
    const notes = wrapper!.get(".episode-page__prose");
    assert.match(notes.text(), /Notes/);
    assert.ok(!notes.html().includes("<script"), notes.html());
    assert.equal(wrapper!.get('[role="tab"][aria-selected="true"]').text(), "Overview");
  });

  it("opens the tab named in the URL and renders the summary as Markdown with contents", async () => {
    await open(`/episodes/${A}?tab=summary`);
    assert.equal(wrapper!.get('[role="tab"][aria-selected="true"]').text(), "Summary");
    const article = wrapper!.get("article.episode-page__prose");
    assert.equal(article.findAll("h2").length, 2);
    assert.ok(!article.html().includes("onerror"), article.html());
    assert.deepEqual(wrapper!.findAll(".episode-page__toc button").map((b) => b.text()), ["Big idea", "Part one", "Part two"]);
  });

  it("keeps the tab in the URL, so Back and links work", async () => {
    const router = await open(`/episodes/${A}`);
    const transcript = wrapper!.findAll('[role="tab"]').find((t) => t.text() === "Transcript")!;
    await transcript.trigger("click");
    await flushPromises();
    assert.equal(router.currentRoute.value.query.tab, "transcript");
    assert.equal(wrapper!.findAll(".episode-page__line").length, 2);
  });

  it("plays from a transcript line or chapter in the player, staying on the page", async () => {
    const router = await open(`/episodes/${A}?tab=chapters`, {
      [`GET /podcastitems/${A}/chapters`]: () => ({ source: "file", chapters: [{ title: "Intro", startSeconds: 0 }, { title: "Ads", startSeconds: 90 }] }),
    });
    const ads = wrapper!.findAll(".episode-page__line").find((b) => b.text().includes("Ads"))!;
    await ads.trigger("click");
    await flushPromises();
    assert.equal(player.current.value?.ID, A);
    assert.equal(player.state.currentTime, 90);
    assert.equal(router.currentRoute.value.path, `/episodes/${A}`);
  });

  it("moves through the reading list with J/K and Previous/Next", async () => {
    setReadingList([A, B], "Summary");
    const router = await open(`/episodes/${A}?tab=summary`);
    assert.match(wrapper!.get(".episode-page__reading").text(), /Summary 1 of 2/);
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "j" }));
    await flushPromises();
    assert.equal(router.currentRoute.value.path, `/episodes/${B}`);
    assert.equal(router.currentRoute.value.query.tab, "summary", "stays on the same tab");
    assert.equal(wrapper!.get(".episode-page__title").text(), "Episode B");
    const previous = wrapper!.findAll(".episode-page__reading button").find((b) => b.text().includes("Previous"))!;
    await previous.trigger("click");
    await flushPromises();
    assert.equal(router.currentRoute.value.path, `/episodes/${A}`);
  });

  it("ignores J/K without a reading list", async () => {
    const router = await open(`/episodes/${A}`);
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "j" }));
    await flushPromises();
    assert.equal(router.currentRoute.value.path, `/episodes/${A}`);
    assert.equal(wrapper!.find(".episode-page__reading").exists(), false);
  });

  it("toggles played and favorite through the API", async () => {
    let played = "";
    let bookmarked = "";
    await open(`/episodes/${A}`, {
      [`PATCH /podcastitems/${A}/markPlayed`]: () => (played = "yes"),
      [`PATCH /podcastitems/${A}/bookmark`]: () => (bookmarked = "yes"),
    });
    const button = (label: string) => wrapper!.findAll(".episode-page__actions button").find((b) => b.text().includes(label))!;
    assert.equal(button("Played").attributes("aria-pressed"), "false");
    await button("Played").trigger("click");
    await flushPromises();
    assert.equal(played, "yes");
    assert.equal(button("Played").attributes("aria-pressed"), "true");
    await button("Favorite").trigger("click");
    await flushPromises();
    assert.equal(bookmarked, "yes");
    assert.equal(button("Favorite").attributes("aria-pressed"), "true");
  });
});
