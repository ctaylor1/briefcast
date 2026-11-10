import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount } from "@vue/test-utils";
import PodcastDetailView from "./PodcastDetailView.vue";
import { usePlayer } from "../composables/usePlayer";
import { makeEpisode, makePodcast } from "../test-support/fixtures";
import { fakeServer, flushPromises, type FakeServer } from "../test-support/http";
import { makeRouter } from "../test-support/router";
import { clearToasts } from "../lib/toast";

const ID = "dddddddd-0000-4000-8000-000000000001";
const player = usePlayer();
let server: FakeServer | null = null;
afterEach(() => {
  server?.restore();
  player.stop();
  clearToasts();
  document.body.innerHTML = "";
});

const routes = [{ path: "/podcasts/:id", component: PodcastDetailView }];

describe("Podcast page", () => {
  it("shows the podcast with its settings and newest episodes first", async () => {
    let paused = false;
    server = fakeServer({
      "GET /podcasts": () => [makePodcast({ ID, Title: "Deep Show", Author: "Ann", IsPaused: paused, AllEpisodesCount: 2 })],
      [`GET /podcasts/${ID}/items`]: () => [
        makeEpisode({ ID: "e1", Title: "Old one", PubDate: "2026-01-01T00:00:00Z" }),
        makeEpisode({ ID: "e2", Title: "New one", PubDate: "2026-11-01T00:00:00Z" }),
      ],
      [`PATCH /podcasts/${ID}/pause`]: () => {
        paused = true;
        return null;
      },
    });
    const router = await makeRouter(`/podcasts/${ID}`, routes);
    const wrapper = mount({ template: "<RouterView />" }, { global: { plugins: [router] }, attachTo: document.body });
    await flushPromises();

    assert.equal(wrapper.get(".podcast-page__title").text(), "Deep Show");
    assert.deepEqual(wrapper.findAll(".podcast-page__episode-title").map((e) => e.text()), ["New one", "Old one"]);
    const pause = wrapper.findAll(".podcast-setting").find((s) => s.text().includes("Pause automatic downloads"))!;
    const box = pause.get('input[type="checkbox"]');
    assert.equal((box.element as HTMLInputElement).checked, false);
    await box.trigger("change");
    await flushPromises();
    assert.ok(server.requests.some((r) => r.method === "PATCH" && r.url === `/podcasts/${ID}/pause`));
    assert.equal((wrapper.findAll(".podcast-setting")[0]!.get("input").element as HTMLInputElement).checked, true, "reloaded state");
    wrapper.unmount();
  });

  it("plays an episode from the list without leaving the page", async () => {
    server = fakeServer({
      "GET /podcasts": () => [makePodcast({ ID })],
      [`GET /podcasts/${ID}/items`]: () => [makeEpisode({ ID: "e9", Title: "Only one" })],
      "GET /podcastitems/e9/chapters": () => ({ source: "", chapters: [] }),
    });
    const router = await makeRouter(`/podcasts/${ID}`, routes);
    const wrapper = mount({ template: "<RouterView />" }, { global: { plugins: [router] } });
    await flushPromises();
    await wrapper.get('button[aria-label="Play Only one"]').trigger("click");
    await flushPromises();
    assert.equal(player.current.value?.ID, "e9");
    assert.equal(router.currentRoute.value.path, `/podcasts/${ID}`);
    wrapper.unmount();
  });

  it("says so when the podcast is not in the library", async () => {
    server = fakeServer({
      "GET /podcasts": () => [],
      [`GET /podcasts/${ID}/items`]: () => [],
    });
    const router = await makeRouter(`/podcasts/${ID}`, routes);
    const wrapper = mount({ template: "<RouterView />" }, { global: { plugins: [router] } });
    await flushPromises();
    assert.match(wrapper.text(), /not in your library/);
    wrapper.unmount();
  });
});
