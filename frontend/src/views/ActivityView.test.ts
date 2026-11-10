import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount } from "@vue/test-utils";
import ActivityView from "./ActivityView.vue";
import SettingsView from "./SettingsView.vue";
import { fakeServer, flushPromises, type FakeServer } from "../test-support/http";
import { makeRouter } from "../test-support/router";

let server: FakeServer | null = null;
afterEach(() => {
  server?.restore();
  document.body.innerHTML = "";
});

function routes() {
  return fakeServer({
    "GET /downloads/queue": () => ({ paused: false, counts: { queued: 0, downloading: 0, downloaded: 0, paused: 0 }, items: [] }),
    "GET /settings/repair-work": () => ({ running: false, queue: { items: [] } }),
    "GET /settings/logs": () => ({ entries: [], impactEntries: [], sources: [], readErrors: [] }),
    "GET /settings/monitoring": () => ({}),
    "GET /settings": () => ({}),
    "GET /settings/summary-models": () => ({ models: [] }),
    "GET /settings/backfill-summaries": () => ({ running: false }),
    "GET /version": () => ({ version: "1.2.3", repoURL: "https://example.com/repo" }),
  });
}

describe("Activity", () => {
  it("opens on Downloads and keeps the chosen tab in the URL", async () => {
    server = routes();
    const router = await makeRouter("/activity");
    const wrapper = mount(ActivityView, { global: { plugins: [router] }, attachTo: document.body });
    await flushPromises();
    assert.deepEqual(wrapper.findAll('[role="tab"]').map((t) => t.text()), ["Downloads", "Transcripts & summaries", "Monitoring", "Logs"]);
    assert.equal(wrapper.get('[role="tab"][aria-selected="true"]').text(), "Downloads");
    assert.match(wrapper.text(), /No queued downloads/);
    assert.equal(wrapper.findAll("h2").filter((h) => h.text() === "Download queue").length, 0, "no second page title");
    await wrapper.findAll('[role="tab"]').find((t) => t.text() === "Logs")!.trigger("click");
    await flushPromises();
    assert.equal(router.currentRoute.value.query.tab, "logs");
    assert.ok(server.requests.some((r) => r.url.startsWith("/settings/logs")));
    wrapper.unmount();
  });
});

describe("Settings", () => {
  it("no longer has Work queue, Monitoring or Logs tabs, and includes About", async () => {
    server = routes();
    const router = await makeRouter("/settings");
    const wrapper = mount(SettingsView, { global: { plugins: [router] } });
    await flushPromises();
    assert.equal(wrapper.find(".settings-tabs").exists(), false);
    assert.ok(wrapper.find("#settings-about").exists());
    assert.match(wrapper.get("#settings-about").text(), /1\.2\.3/);
    wrapper.unmount();
  });
});
