import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount } from "@vue/test-utils";
import PlayerView from "./PlayerView.vue";
import { fakeServer, flushPromises, type FakeServer } from "../test-support/http";
import { makeRouter } from "../test-support/router";

let server: FakeServer | null = null;
afterEach(() => server?.restore());

describe("PlayerView", () => {
  it("shows a neutral empty state, not an error, when nothing is queued", async () => {
    server = fakeServer({});
    const router = await makeRouter("/player");
    const wrapper = mount(PlayerView, { global: { plugins: [router] } });
    await flushPromises();
    assert.match(wrapper.text(), /Nothing queued for playback/);
    assert.ok(!wrapper.find(".ui-alert--danger").exists(), "no error alert");
    assert.equal(server.requests.length, 0);
  });
});
