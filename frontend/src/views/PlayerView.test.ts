import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount } from "@vue/test-utils";
import PlayerView from "./PlayerView.vue";
import { usePlayer } from "../composables/usePlayer";
import { fakeServer, flushPromises, type FakeServer } from "../test-support/http";
import { makeRouter } from "../test-support/router";

let server: FakeServer | null = null;
const player = usePlayer();
afterEach(() => {
  player.stop();
  server?.restore();
});

const ID = "3f2a8c1e-1b2c-4d5e-8f90-0123456789ab";
const PODCAST = "aaaaaaaa-1111-4222-8333-444444444444";

function episode(id: string, title: string, pubDate: string) {
  return { ID: id, Title: title, Podcast: { ID: PODCAST, Title: "Show" }, PubDate: pubDate, Duration: 10 };
}

describe("PlayerView (now playing)", () => {
  it("shows a neutral empty state, not an error, when nothing is queued", async () => {
    server = fakeServer({});
    const router = await makeRouter("/player");
    const wrapper = mount(PlayerView, { global: { plugins: [router] } });
    await flushPromises();
    assert.match(wrapper.text(), /Nothing queued for playback/);
    assert.ok(!wrapper.find(".ui-alert--danger").exists(), "no error alert");
    assert.equal(server.requests.length, 0);
  });

  it("loads old ?itemIds= links into the player, ignoring malformed IDs", async () => {
    server = fakeServer({
      [`GET /podcastitems/${ID}`]: () => episode(ID, "Valid", "2026-11-01T00:00:00Z"),
      [`GET /podcastitems/${ID}/chapters`]: () => ({ source: "", chapters: [] }),
    });
    const router = await makeRouter(`/player?itemIds=${ID},a%2Fdelete,..%2F..%2Fsettings`);
    const wrapper = mount(PlayerView, { global: { plugins: [router] } });
    await flushPromises();
    const paths = server.requests.map((r) => r.url);
    assert.ok(paths.includes(`/podcastitems/${ID}`), paths.join(", "));
    assert.ok(paths.every((p) => !p.includes("delete") && !p.includes("settings")), paths.join(", "));
    assert.equal(player.current.value?.ID, ID);
    assert.match(wrapper.text(), /Valid/);
    assert.equal(router.currentRoute.value.fullPath, "/player", "the queue moved out of the URL");
    wrapper.unmount();
  });

  it("queues a podcast newest first from ?podcastId=", async () => {
    const older = "bbbbbbbb-0000-4000-8000-000000000001";
    const newer = "bbbbbbbb-0000-4000-8000-000000000002";
    server = fakeServer({
      [`GET /podcasts/${PODCAST}/items`]: () => [
        episode(older, "Older", "2026-10-01T00:00:00Z"),
        episode(newer, "Newer", "2026-11-01T00:00:00Z"),
      ],
      [`GET /podcastitems/${newer}/chapters`]: () => ({ source: "", chapters: [] }),
    });
    const router = await makeRouter(`/player?podcastId=${PODCAST}`);
    const wrapper = mount(PlayerView, { global: { plugins: [router] } });
    await flushPromises();
    assert.deepEqual(player.state.queue.map((e) => e.Title), ["Newer", "Older"]);
    assert.match(wrapper.get(".now-playing__queue").text(), /Older/);
    wrapper.unmount();
  });
});
