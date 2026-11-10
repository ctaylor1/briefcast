import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount, type VueWrapper } from "@vue/test-utils";
import SettingsView from "./SettingsView.vue";
import { clearToasts, useToasts } from "../lib/toast";
import { fakeServer, flushPromises, type FakeServer, type RecordedRequest } from "../test-support/http";
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
  keepAllEpisodes: false,
  keepLatestEpisodes: 5,
  deleteAfterDays: 0,
  deleteOnlyPlayed: true,
  autoDownload: true,
  downloadOnAdd: true,
  initialDownloadMode: "count",
  initialDownloadCount: 3,
  initialDownloadMonths: 0,
  summarizationEnabled: false,
  summarizationModel: "",
  summarizationPrompt: "",
  summarizationUserPrompt: "",
  obsidianVault: "Vault",
  obsidianFolder: "Clippings",
  llmConcurrency: 1,
};

async function mountSettings() {
  const patches: RecordedRequest[] = [];
  server = fakeServer({
    "GET /settings": () => ({ ...SETTINGS }),
    "PATCH /settings": (request) => {
      patches.push(request);
      return { ...SETTINGS, ...(request.data as object) };
    },
    "GET /settings/summary-models": () => ({ models: [] }),
    "GET /settings/backfill-summaries": () => ({ running: false }),
    "GET /version": () => ({ version: "1.0.0", repoURL: "https://example.com" }),
  });
  const router = await makeRouter("/settings");
  wrapper = mount(SettingsView, { global: { plugins: [router] }, attachTo: document.body });
  await flushPromises();
  return patches;
}

function input(label: string) {
  const field = wrapper!.findAll("label").find((l) => l.text().trim() === label);
  assert.ok(field, `label ${label}`);
  return wrapper!.get(`#${field!.attributes("for")}`);
}

describe("Settings save model", () => {
  it("has section links and no per-card Save buttons", async () => {
    await mountSettings();
    assert.deepEqual(wrapper!.findAll(".settings-nav__link").map((b) => b.text()), [
      "Appearance", "New episodes", "Retention", "AI summaries", "Obsidian", "Briefpoint", "About",
    ]);
    assert.equal(wrapper!.findAll("button").filter((b) => /^Save .* settings$/.test(b.text())).length, 0);
    assert.equal(wrapper!.find(".settings-savebar").exists(), false, "nothing to save yet");
    assert.equal(wrapper!.findAll("select:not([id])").length, 0, "every select comes from UiSelect with a label");
  });

  it("collects changes across sections into one save with only the changed fields", async () => {
    const patches = await mountSettings();
    await input("Keep latest episodes").setValue("12");
    await input("Vault").setValue("Work");
    const bar = wrapper!.get(".settings-savebar");
    assert.match(bar.text(), /unsaved changes in Retention, Obsidian/);
    await bar.findAll("button").find((b) => b.text() === "Save changes")!.trigger("click");
    await flushPromises();
    assert.equal(patches.length, 1, "one request");
    const body = patches[0]!.data as Record<string, unknown>;
    assert.equal(body.keepLatestEpisodes, 12);
    assert.equal(body.obsidianVault, "Work");
    assert.ok(!("summarizationEnabled" in body), "unchanged sections are not sent");
    assert.ok(!("autoDownload" in body));
    assert.equal(wrapper!.find(".settings-savebar").exists(), false, "saved");
    assert.deepEqual(useToasts().toasts.map((t) => t.message), ["Settings saved."]);
  });

  it("discards unsaved changes", async () => {
    const patches = await mountSettings();
    await input("Vault").setValue("Scratch");
    await wrapper!.get(".settings-savebar").findAll("button").find((b) => b.text() === "Discard")!.trigger("click");
    await flushPromises();
    assert.equal((input("Vault").element as HTMLInputElement).value, "Vault");
    assert.equal(wrapper!.find(".settings-savebar").exists(), false);
    assert.equal(patches.length, 0);
  });
});
