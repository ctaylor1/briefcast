import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount } from "@vue/test-utils";
import EpisodeTrackControls from "./EpisodeTrackControls.vue";
import { makeEpisode } from "../../test-support/fixtures";

describe("EpisodeTrackControls", () => {
  it("has no Stop button", () => {
    const wrapper = mount(EpisodeTrackControls, { props: { item: makeEpisode() } });
    const labels = wrapper.findAll("button").map((b) => b.attributes("aria-label"));
    assert.ok(!labels.includes("Stop"), labels.join(", "));
    assert.ok(labels.includes("Play"));
  });

  it("reports played and favourite as toggles with stable names", () => {
    const off = mount(EpisodeTrackControls, { props: { item: makeEpisode({ IsPlayed: false }) } });
    assert.equal(off.get('button[aria-label="Played"]').attributes("aria-pressed"), "false");
    const on = mount(EpisodeTrackControls, { props: { item: makeEpisode({ IsPlayed: true }) } });
    assert.equal(on.get('button[aria-label="Played"]').attributes("aria-pressed"), "true");
    assert.ok(on.find('button[aria-label="Favorite"]').exists());
  });
});
