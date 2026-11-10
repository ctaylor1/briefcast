import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount } from "@vue/test-utils";
import DashboardView from "./DashboardView.vue";
import { makePodcast } from "../test-support/fixtures";
import { FakeHttpError, fakeServer, flushPromises, type FakeServer } from "../test-support/http";
import { makeRouter } from "../test-support/router";
import { clearToasts, useToasts } from "../lib/toast";

let server: FakeServer | null = null;
afterEach(() => {
  server?.restore();
  clearToasts();
});

describe("DashboardView feedback", () => {
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
    const wrapper = mount(DashboardView, { global: { plugins: [router] } });
    await flushPromises();

    await wrapper.find('button[aria-label="Download all episodes"]').trigger("click");
    await flushPromises();
    await wrapper.find('button[aria-label="Pause downloads"]').trigger("click");
    await flushPromises();

    const tones = useToasts().toasts.map((t) => `${t.tone}: ${t.message}`);
    assert.equal(tones.length, 2, tones.join(" | "));
    assert.match(tones[0]!, /^success: /);
    assert.equal(tones[1], "danger: database is locked");
    assert.equal(wrapper.findAll(".ui-alert").length, 0, "no page-top alerts");
    wrapper.unmount();
  });
});
