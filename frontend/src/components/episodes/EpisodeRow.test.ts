import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount, type VueWrapper } from "@vue/test-utils";
import EpisodeRow from "./EpisodeRow.vue";
import { makeEpisode } from "../../test-support/fixtures";
import { flushPromises } from "../../test-support/http";
import { makeRouter } from "../../test-support/router";

let wrapper: VueWrapper | null = null;
afterEach(() => {
  wrapper?.unmount();
  document.body.innerHTML = "";
});

async function mountRow(overrides = {}) {
  const router = await makeRouter("/episodes");
  wrapper = mount(
    { components: { EpisodeRow }, props: ["item"], template: '<ul><EpisodeRow :item="item" v-bind="$attrs" /></ul>' },
    { props: { item: makeEpisode({ ID: "ep/1", Title: "Deep Dive", ...overrides }) }, global: { plugins: [router] }, attachTo: document.body },
  );
  return wrapper.findComponent(EpisodeRow);
}

async function openMenu(row: ReturnType<typeof wrapper.findComponent>) {
  const trigger = row.findAll("button").find((b) => b.text().includes("More actions"))!;
  (trigger.element as HTMLButtonElement).focus();
  await trigger.trigger("click");
  await flushPromises();
  return [...document.querySelectorAll<HTMLButtonElement>(".podcast-menu__item")];
}

describe("EpisodeRow", () => {
  it("links the title to the episode page and says only what is true", async () => {
    const row = await mountRow({ IsPlayed: false, DownloadStatus: 3, TranscriptStatus: "missing" });
    assert.equal(row.get(".episode-row__title").attributes("href"), "/episodes/ep%2F1");
    const meta = row.get(".episode-row__meta").text();
    for (const absent of ["Unplayed", "No favorite", "Not queued", "Transcript"]) {
      assert.ok(!meta.includes(absent), `${absent} in "${meta}"`);
    }
  });

  it("shows present state, with transcript and chapters linking to their tabs", async () => {
    const row = await mountRow({ IsPlayed: true, BookmarkDate: "2026-11-01T00:00:00Z", TranscriptStatus: "available", HasChapters: true, DownloadStatus: 0 });
    const meta = row.get(".episode-row__meta").text();
    for (const present of ["Played", "Favorite", "Queued", "Transcript ready", "Chapters"]) {
      assert.ok(meta.includes(present), `${present} missing from "${meta}"`);
    }
    const hrefs = row.findAll(".episode-row__state--link").map((a) => a.attributes("href"));
    assert.deepEqual(hrefs, ["/episodes/ep%2F1?tab=transcript", "/episodes/ep%2F1?tab=chapters"]);
  });

  it("has one primary action and the rest in a named menu", async () => {
    const row = await mountRow({ DownloadStatus: 3 });
    const group = row.get('[role="group"]');
    assert.equal(group.attributes("aria-label"), "Actions for Deep Dive");
    assert.ok(group.find('button[aria-label="Play Deep Dive"]').exists());
    const items = (await openMenu(row)).map((b) => b.textContent?.trim());
    assert.deepEqual(items, ["Played", "Favorite", "Download", "Episode details"]);
  });

  it("offers Resume when there is a saved position", async () => {
    const row = await mountRow({ PlaybackPositionSeconds: 125, IsPlayed: false });
    assert.ok(row.find('button[aria-label="Resume Deep Dive"]').exists());
    assert.match(row.get(".episode-row__meta").text(), /Stopped at 2:05/);
  });

  it("reports toggles with aria-pressed and emits the episode", async () => {
    const row = await mountRow({ IsPlayed: true, DownloadStatus: 1, DownloadTotalBytes: 100, DownloadedBytes: 25 });
    assert.equal(row.get('[role="progressbar"]').attributes("aria-valuenow"), "25");
    const items = await openMenu(row);
    assert.equal(items.find((b) => b.textContent?.includes("Played"))!.getAttribute("aria-pressed"), "true");
    assert.ok(items.some((b) => b.textContent?.includes("Cancel download")));
    items.find((b) => b.textContent?.includes("Played"))!.click();
    await flushPromises();
    assert.equal(row.emitted("toggle-played")?.length, 1);
  });
});
