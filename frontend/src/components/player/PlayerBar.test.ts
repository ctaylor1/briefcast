import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount } from "@vue/test-utils";
import { defineComponent, h, nextTick } from "vue";
import { RouterView } from "vue-router";
import PlayerBar from "./PlayerBar.vue";
import { usePlayer } from "../../composables/usePlayer";
import { makeEpisode } from "../../test-support/fixtures";
import { fakeServer, flushPromises, type FakeServer } from "../../test-support/http";
import { makeRouter } from "../../test-support/router";

const A = makeEpisode({ ID: "aaaaaaaa-0000-4000-8000-000000000001", Title: "First", Duration: 600 });
const player = usePlayer();
let server: FakeServer | null = null;

afterEach(() => {
  player.stop();
  server?.restore();
  document.body.innerHTML = "";
});

const Shell = defineComponent({
  render: () => [h(RouterView), h(PlayerBar)],
});

function chaptersRoute() {
  return fakeServer({ [`GET /podcastitems/${A.ID}/chapters`]: () => ({ source: "", chapters: [] }) });
}

describe("PlayerBar", () => {
  it("stays hidden until something is queued, then shows the episode and controls", async () => {
    server = chaptersRoute();
    const router = await makeRouter("/episodes");
    const wrapper = mount(PlayerBar, { global: { plugins: [router] }, attachTo: document.body });
    assert.equal(wrapper.find(".player-bar").exists(), false);
    assert.ok(wrapper.find("audio").exists(), "the audio element is always mounted");
    await player.playQueue([A]);
    await nextTick();
    assert.match(wrapper.get(".player-bar").text(), /First/);
    for (const label of ["Previous episode", "Back 15 seconds", "Forward 30 seconds", "Next episode"]) {
      assert.ok(wrapper.find(`button[aria-label="${label}"]`).exists(), label);
    }
    assert.ok(wrapper.find('input[type="range"][aria-label="Seek"]').exists());
    wrapper.unmount();
  });

  it("keeps playing across navigation", async () => {
    server = chaptersRoute();
    const router = await makeRouter("/episodes");
    const wrapper = mount(Shell, { global: { plugins: [router] }, attachTo: document.body });
    const audio = wrapper.get("audio").element as HTMLAudioElement;
    let pauses = 0;
    audio.addEventListener("pause", () => (pauses += 1));
    await player.playQueue([A]);
    await router.push("/settings");
    await router.push("/summaries");
    await flushPromises();
    assert.equal(wrapper.get("audio").element, audio, "same element after navigating");
    assert.equal(audio.getAttribute("src"), `/podcastitems/${A.ID}/file`);
    assert.equal(pauses, 0);
    assert.ok(wrapper.find(".player-bar").exists());
    wrapper.unmount();
  });

  it("toggles between Play and Pause", async () => {
    server = chaptersRoute();
    const router = await makeRouter("/");
    const wrapper = mount(PlayerBar, { global: { plugins: [router] }, attachTo: document.body });
    await player.playQueue([A]);
    const audio = wrapper.get("audio").element as HTMLAudioElement;
    audio.dispatchEvent(new Event("play"));
    await nextTick();
    const toggle = wrapper.get(".player-controls__play");
    assert.equal(toggle.attributes("aria-label"), "Pause");
    audio.dispatchEvent(new Event("pause"));
    await nextTick();
    assert.equal(toggle.attributes("aria-label"), "Play");
    wrapper.unmount();
  });
});
