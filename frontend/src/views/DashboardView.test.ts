import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount, type VueWrapper } from "@vue/test-utils";
import DashboardView from "./DashboardView.vue";
import { makePodcast } from "../test-support/fixtures";
import { FakeHttpError, fakeServer, flushPromises, type FakeServer } from "../test-support/http";
import { makeRouter } from "../test-support/router";
import { clearToasts, useToasts } from "../lib/toast";

let server: FakeServer | null = null;
afterEach(() => {
  server?.restore();
  clearToasts();
  document.body.innerHTML = "";
});

async function useMenuItem(wrapper: VueWrapper, label: string): Promise<void> {
  const trigger = wrapper.findAll("button").find((b) => b.text().includes("More actions"))!;
  (trigger.element as HTMLButtonElement).focus();
  await trigger.trigger("click");
  await flushPromises();
  const item = [...document.querySelectorAll<HTMLButtonElement>(".podcast-menu__item")].find((b) => b.textContent?.includes(label));
  assert.ok(item, `menu item ${label}`);
  item!.click();
  await flushPromises();
}

describe("Library", () => {
  it("reports action results as toasts, not as alerts at the top of the page", async () => {
    const podcast = makePodcast({ ID: "3f2a8c1e-1b2c-4d5e-8f90-0123456789ab" });
    server = fakeServer({
      "GET /podcasts": () => [podcast],
      [`POST /podcasts/${podcast.ID}/download`]: () => null,
      [`PATCH /podcasts/${podcast.ID}/pause`]: () => {
        throw new FakeHttpError(500, { error: "database is locked" });
      },
    });
    const router = await makeRouter("/");
    const wrapper = mount(DashboardView, { global: { plugins: [router] }, attachTo: document.body });
    await flushPromises();

    await useMenuItem(wrapper, "Download all episodes");
    await useMenuItem(wrapper, "Pause automatic downloads");

    const tones = useToasts().toasts.map((t) => `${t.tone}: ${t.message}`);
    assert.equal(tones.length, 2, tones.join(" | "));
    assert.match(tones[0]!, /^success: /);
    assert.equal(tones[1], "danger: database is locked");
    assert.equal(wrapper.findAll(".ui-alert").length, 0, "no page-top alerts");
    wrapper.unmount();
  });

  it("lists podcasts as tiles, newest activity first, with a summary line", async () => {
    server = fakeServer({
      "GET /podcasts": () => [
        makePodcast({ ID: "a", Title: "Older", LastEpisode: "2026-01-01T00:00:00Z", AllEpisodesCount: 3 }),
        makePodcast({ ID: "b", Title: "Newer", LastEpisode: "2026-11-01T00:00:00Z", AllEpisodesCount: 4 }),
      ],
    });
    const router = await makeRouter("/");
    const wrapper = mount(DashboardView, { global: { plugins: [router] } });
    await flushPromises();
    assert.deepEqual(wrapper.findAll(".podcast-tile__title").map((t) => t.text()), ["Newer", "Older"]);
    assert.match(wrapper.get(".section-subtitle").text(), /2 podcasts · 7 episodes/);
    assert.equal(wrapper.findAll("table").length, 0, "no table view any more");
    wrapper.unmount();
  });
});
