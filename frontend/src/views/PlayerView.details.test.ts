import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount } from "@vue/test-utils";
import PlayerView from "./PlayerView.vue";
import { usePlayer } from "../composables/usePlayer";
import { makeEpisode } from "../test-support/fixtures";
import { fakeServer, flushPromises, type FakeServer } from "../test-support/http";
import { makeRouter } from "../test-support/router";

const ID = "cccccccc-0000-4000-8000-000000000001";
const player = usePlayer();
let server: FakeServer | null = null;
afterEach(() => {
  player.stop();
  server?.restore();
  document.body.innerHTML = "";
});

async function mountPlaying(summary = "<p>Notes <a href=\"https://example.com\">link</a><img src=x onerror=alert(1)></p>") {
  server = fakeServer({
    [`GET /podcastitems/${ID}/chapters`]: () => ({
      source: "file",
      chapters: [
        { title: "Intro", startSeconds: 0 },
        { title: "Interview", startSeconds: 300 },
      ],
    }),
    [`GET /podcastitems/${ID}/transcript`]: () => ({
      status: "available",
      transcript: { segments: [{ start: 0, end: 5, text: "Welcome back." }, { start: 5, end: 9, text: "Today we talk.", speaker: "Host" }] },
    }),
  });
  await player.playQueue([makeEpisode({ ID, Title: "Deep Dive", Duration: 1800, Summary: summary })]);
  const router = await makeRouter("/player");
  const wrapper = mount(PlayerView, { global: { plugins: [router] }, attachTo: document.body });
  await flushPromises();
  return wrapper;
}

describe("Now playing details", () => {
  it("lists chapters, marks the current one and seeks on click", async () => {
    const wrapper = await mountPlaying();
    const rows = wrapper.findAll(".now-playing__chapters button");
    assert.deepEqual(rows.map((r) => r.text()), ["0:00Intro", "5:00Interview"]);
    assert.equal(rows[0]!.attributes("aria-current"), "true");
    await rows[1]!.trigger("click");
    assert.equal(player.state.currentTime, 300);
    assert.equal(wrapper.findAll(".now-playing__chapters button")[1]!.attributes("aria-current"), "true");
    wrapper.unmount();
  });

  it("loads the transcript when its tab opens, and seeks from a line", async () => {
    const wrapper = await mountPlaying();
    assert.ok(!server!.requests.some((r) => r.url.endsWith("/transcript")), "not fetched before it is needed");
    const tab = wrapper.findAll('[role="tab"]').find((t) => t.text() === "Transcript")!;
    await tab.trigger("click");
    await flushPromises();
    const lines = wrapper.findAll(".now-playing__transcript button");
    assert.equal(lines.length, 2);
    assert.match(lines[1]!.text(), /Host: Today we talk\./);
    await lines[1]!.trigger("click");
    assert.equal(player.state.currentTime, 5);
    wrapper.unmount();
  });

  it("renders show notes sanitized", async () => {
    const wrapper = await mountPlaying();
    const tab = wrapper.findAll('[role="tab"]').find((t) => t.text() === "Show notes")!;
    await tab.trigger("click");
    const notes = wrapper.get(".now-playing__notes");
    assert.ok(notes.find('a[href="https://example.com"]').exists());
    assert.ok(!notes.html().includes("onerror"), notes.html());
    wrapper.unmount();
  });
});
