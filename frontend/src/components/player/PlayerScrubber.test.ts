import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount } from "@vue/test-utils";
import PlayerScrubber from "./PlayerScrubber.vue";

describe("PlayerScrubber", () => {
  it("is a labelled range that speaks the position in words", () => {
    const wrapper = mount(PlayerScrubber, { props: { currentTime: 125.6, duration: 3600 } });
    const input = wrapper.get('input[type="range"]');
    assert.equal(input.attributes("aria-label"), "Seek");
    assert.equal(input.attributes("max"), "3600");
    assert.equal((input.element as HTMLInputElement).value, "125");
    assert.equal(input.attributes("aria-valuetext"), "2 minutes 5 seconds of 1 hour");
  });

  it("previews while dragging and seeks on release", async () => {
    const wrapper = mount(PlayerScrubber, { props: { currentTime: 0, duration: 600 } });
    const input = wrapper.get('input[type="range"]');
    (input.element as HTMLInputElement).value = "300";
    await input.trigger("input");
    assert.equal((input.element as HTMLInputElement).value, "300", "the thumb follows the drag");
    assert.equal(wrapper.emitted("seek"), undefined, "no seek while dragging");
    await input.trigger("change");
    assert.deepEqual(wrapper.emitted("seek")?.[0], [300]);
  });

  it("marks chapters and sponsor segments, and disables itself without a duration", () => {
    const wrapper = mount(PlayerScrubber, {
      props: {
        currentTime: 0,
        duration: 600,
        chapters: [
          { title: "Intro", startSeconds: 0 },
          { title: "Part two", startSeconds: 300 },
        ],
        sponsorSegments: [{ start: 60, end: 120, title: "Sponsor" }],
      },
    });
    assert.equal(wrapper.findAll(".player-scrubber__chapter").length, 1, "a chapter at 0 needs no mark");
    assert.equal(wrapper.get(".player-scrubber__chapter").attributes("style"), "left: 50%;");
    assert.equal(wrapper.findAll(".player-scrubber__sponsor").length, 1);
    const unknown = mount(PlayerScrubber, { props: { currentTime: 0, duration: 0 } });
    assert.equal(unknown.get("input").attributes("disabled"), "");
  });
});
