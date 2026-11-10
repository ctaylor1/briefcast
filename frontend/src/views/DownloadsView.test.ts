import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount } from "@vue/test-utils";
import DownloadsView from "./DownloadsView.vue";
import { makeEpisode } from "../test-support/fixtures";
import { fakeServer, flushPromises, type FakeServer } from "../test-support/http";
import { makeRouter } from "../test-support/router";

let server: FakeServer | null = null;
afterEach(() => server?.restore());

function queue() {
  return {
    paused: false,
    counts: { queued: 1, downloading: 1, downloaded: 0, paused: 1 },
    items: [
      makeEpisode({ ID: "a", Title: "Downloading one", DownloadStatus: 1, DownloadTotalBytes: 100, DownloadedBytes: 40 }),
      makeEpisode({ ID: "b", Title: "Paused one", DownloadStatus: 4 }),
    ],
  };
}

describe("DownloadsView queue", () => {
  it("labels each cell so rows can render as cards on phones", async () => {
    server = fakeServer({ "GET /downloads/queue": queue });
    const router = await makeRouter("/downloads");
    const wrapper = mount(DownloadsView, { global: { plugins: [router] } });
    await flushPromises();

    const table = wrapper.get("table.downloads-table");
    assert.equal(table.attributes("role"), "table");
    const rows = table.findAll("tbody tr");
    assert.equal(rows.length, 2);
    for (const row of rows) {
      assert.equal(row.attributes("role"), "row");
      const labels = row.findAll("td[data-label]").map((cell) => cell.attributes("data-label"));
      assert.deepEqual(labels, ["Podcast", "Download date", "Status", "Progress"]);
      assert.ok(row.findAll("td").every((cell) => cell.attributes("role") === "cell"));
    }
    const headers = table.findAll("th").map((th) => th.text());
    assert.deepEqual(headers, ["Episode", "Podcast", "Download date", "Status", "Progress", "Actions"]);
    wrapper.unmount();
  });

  it("uses sentence case for row actions", async () => {
    server = fakeServer({ "GET /downloads/queue": queue });
    const router = await makeRouter("/downloads");
    const wrapper = mount(DownloadsView, { global: { plugins: [router] } });
    await flushPromises();
    assert.match(wrapper.text(), /Resume download/);
    assert.doesNotMatch(wrapper.text(), /Resume Download/);
    wrapper.unmount();
  });
});
