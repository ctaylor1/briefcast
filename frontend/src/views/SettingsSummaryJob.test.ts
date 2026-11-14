import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount, type VueWrapper } from "@vue/test-utils";
import SettingsView from "./SettingsView.vue";
import { clearToasts } from "../lib/toast";
import { fakeServer, flushPromises, type FakeRoute, type FakeServer, type RecordedRequest } from "../test-support/http";
import { makeRouter } from "../test-support/router";

let server: FakeServer | null = null;
let wrapper: VueWrapper | null = null;
afterEach(() => {
  wrapper?.unmount();
  server?.restore();
  clearToasts();
  document.body.innerHTML = "";
});

const SETTINGS = {
  keepAllEpisodes: true,
  keepLatestEpisodes: 0,
  deleteAfterDays: 0,
  deleteOnlyPlayed: true,
  autoDownload: true,
  downloadOnAdd: true,
  initialDownloadMode: "count",
  initialDownloadCount: 3,
  initialDownloadMonths: 0,
  summarizationEnabled: true,
  summarizationModel: "",
  summarizationPrompt: "",
  summarizationUserPrompt: "",
  obsidianVault: "Vault",
  obsidianFolder: "Clippings",
  llmConcurrency: 1,
};

async function mountSettings(routes: Record<string, FakeRoute> = {}) {
  server = fakeServer({
    "GET /settings": () => ({ ...SETTINGS }),
    "GET /settings/summary-models": () => ({ models: [] }),
    "GET /settings/backfill-summaries": () => ({ running: false, total: 0, completed: 0, failed: 0 }),
    "GET /version": () => ({ version: "1.0.0", repoURL: "https://example.com" }),
    ...routes,
  });
  const router = await makeRouter("/settings");
  wrapper = mount(SettingsView, { global: { plugins: [router] }, attachTo: document.body });
  await flushPromises();
}

function input(label: string) {
  const field = wrapper!.findAll("label").find((l) => l.text().trim() === label);
  assert.ok(field, `label ${label}`);
  return wrapper!.get(`#${field!.attributes("for")}`);
}

function button(text: string) {
  const found = wrapper!.findAll("button").find((b) => b.text().trim() === text);
  assert.ok(found, `button ${text}`);
  return found!;
}

describe("Settings summary jobs", () => {
  it("summarizes only the chosen number of waiting episodes", async () => {
    const starts: RecordedRequest[] = [];
    await mountSettings({
      "POST /settings/backfill-summaries": (request) => {
        starts.push(request);
        return { message: "summary backfill started", status: { running: true, total: 0, completed: 0, failed: 0 } };
      },
    });
    await input("Episodes to summarize").setValue("3");
    await button("Summarize").trigger("click");
    await flushPromises();

    assert.equal(starts.length, 1);
    assert.deepEqual(starts[0]!.data, { limit: 3 });
    assert.match(wrapper!.text(), /Summary job running\. Finding episodes\.\.\./);
    assert.equal(wrapper!.find(".settings-savebar").exists(), false, "the count is not a setting to save");
  });

  it("will not start with a count outside 1 to 500", async () => {
    await mountSettings();
    for (const value of ["0", "501", "2.5", ""]) {
      await input("Episodes to summarize").setValue(value);
      assert.equal(button("Summarize").attributes("disabled"), "", `disabled for ${JSON.stringify(value)}`);
    }
    assert.match(wrapper!.text(), /Enter a whole number from 1 to 500\./);
  });

  it("stops a running summary job", async () => {
    const stops: RecordedRequest[] = [];
    await mountSettings({
      "GET /settings/backfill-summaries": () => ({ running: true, total: 3, completed: 1, failed: 0 }),
      "POST /settings/backfill-summaries/stop": (request) => {
        stops.push(request);
        return { message: "summary job stopping", status: { running: true, stopping: true, total: 3, completed: 1, failed: 0 } };
      },
    });
    assert.match(wrapper!.text(), /Summary job running\. 1 of 3 processed, 0 failed\./);
    assert.equal(button("Summarize").attributes("disabled"), "", "no second job while one runs");

    await button("Stop").trigger("click");
    await flushPromises();

    assert.equal(stops.length, 1);
    assert.match(wrapper!.text(), /Stopping after the episode in progress\. 1 of 3 processed, 0 failed\./);
    assert.equal(button("Stopping...").attributes("disabled"), "");
  });

  it("says when a finished job was stopped or found nothing", async () => {
    await mountSettings({
      "GET /settings/backfill-summaries": () => ({ running: false, stopped: true, total: 3, completed: 1, failed: 0 }),
    });
    assert.match(wrapper!.text(), /Last summary job \(stopped\): 1 of 3 processed,\s+0 failed\./);
  });
});
