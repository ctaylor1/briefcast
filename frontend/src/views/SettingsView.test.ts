import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount } from "@vue/test-utils";
import SettingsView from "./SettingsView.vue";
import { FakeHttpError, fakeServer, flushPromises, type FakeServer } from "../test-support/http";
import { makeRouter } from "../test-support/router";

let server: FakeServer | null = null;
afterEach(() => server?.restore());

describe("SettingsView load guard", () => {
  it("hides every form, and offers a retry, when settings fail to load", async () => {
    server = fakeServer({
      "GET /settings": () => {
        throw new FakeHttpError(500, { error: "database is locked" });
      },
    });
    const router = await makeRouter("/settings");
    const wrapper = mount(SettingsView, { global: { plugins: [router] } });
    await flushPromises();
    assert.match(wrapper.text(), /database is locked/);
    assert.match(wrapper.text(), /Settings could not be loaded/);
    const saveButtons = wrapper.findAll("button").filter((b) => /^Save/.test(b.text()));
    assert.equal(saveButtons.length, 0, "nothing can be saved");
    assert.ok(server.requests.every((r) => r.method === "GET"), "nothing was written");
    wrapper.unmount();
  });
});
