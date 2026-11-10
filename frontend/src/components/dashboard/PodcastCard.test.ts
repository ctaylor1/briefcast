import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount } from "@vue/test-utils";
import PodcastCard from "./PodcastCard.vue";
import PodcastsTable from "./PodcastsTable.vue";
import { makePodcast } from "../../test-support/fixtures";

const EXPECTED_LABELS = [
  "Play latest episode",
  "Download all episodes",
  "Pause downloads",
  "Keep all episodes",
  "Skip sponsor chapters",
  "Sync to Briefpoint",
  "Alternate feeds",
  "Delete podcast",
];

describe("podcast action toolbars", () => {
  it("name every action in the card and group them under the podcast title", () => {
    const wrapper = mount(PodcastCard, { props: { podcast: makePodcast(), busy: false } });
    const group = wrapper.get('[role="group"]');
    assert.equal(group.attributes("aria-label"), "Actions for Example Show");
    const labels = group.findAll("button").map((b) => b.attributes("aria-label"));
    assert.deepEqual(labels, EXPECTED_LABELS);
  });

  it("expose the four settings as toggles that report their state", () => {
    const podcast = makePodcast({ IsPaused: true, RetentionKeepAll: false, AutoSkipSponsorChapters: true });
    const wrapper = mount(PodcastCard, { props: { podcast, busy: false } });
    const pressed = Object.fromEntries(
      wrapper.findAll('[role="group"] button').map((b) => [b.attributes("aria-label"), b.attributes("aria-pressed")]),
    );
    assert.equal(pressed["Pause downloads"], "true");
    assert.equal(pressed["Keep all episodes"], "false");
    assert.equal(pressed["Skip sponsor chapters"], "true");
    assert.equal(pressed["Sync to Briefpoint"], "false");
    assert.equal(pressed["Delete podcast"], undefined);
  });

  it("emit the same events as before", async () => {
    const podcast = makePodcast();
    const wrapper = mount(PodcastCard, { props: { podcast, busy: false } });
    const button = (label: string) => wrapper.get(`button[aria-label="${label}"]`);
    await button("Play latest episode").trigger("click");
    await button("Delete podcast").trigger("click");
    assert.deepEqual(wrapper.emitted("play")?.[0], [podcast.ID]);
    assert.deepEqual(wrapper.emitted("delete")?.[0], [podcast]);
  });

  it("use the same labels in the table view", () => {
    const wrapper = mount(PodcastsTable, {
      props: { podcasts: [makePodcast()], activeId: "" },
    });
    const labels = wrapper.get('[role="group"]').findAll("button").map((b) => b.attributes("aria-label"));
    assert.deepEqual(labels, EXPECTED_LABELS);
  });
});

describe("podcast table actions", () => {
  it("emit the podcast for every action", async () => {
    const podcast = makePodcast();
    const wrapper = mount(PodcastsTable, { props: { podcasts: [podcast], activeId: null } });
    await wrapper.get('button[aria-label="Download all episodes"]').trigger("click");
    await wrapper.get('button[aria-label="Pause downloads"]').trigger("click");
    assert.deepEqual(wrapper.emitted("download-all")?.[0], [podcast]);
    assert.deepEqual(wrapper.emitted("toggle-pause")?.[0], [podcast]);
  });
});
