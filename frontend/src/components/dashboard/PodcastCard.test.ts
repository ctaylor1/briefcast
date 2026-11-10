import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount } from "@vue/test-utils";
import PodcastCard from "./PodcastCard.vue";
import { makePodcast } from "../../test-support/fixtures";
import { flushPromises } from "../../test-support/http";
import { makeRouter } from "../../test-support/router";

afterEach(() => {
  document.body.innerHTML = "";
});

async function mountCard(podcast = makePodcast()) {
  const router = await makeRouter("/");
  return mount(PodcastCard, { props: { podcast, busy: false }, global: { plugins: [router] }, attachTo: document.body });
}

describe("PodcastCard", () => {
  it("links the title to the podcast page and keeps the cover out of the tab order", async () => {
    const wrapper = await mountCard(makePodcast({ ID: "pod/1", Title: "Example Show" }));
    const title = wrapper.get(".podcast-tile__title a");
    assert.equal(title.text(), "Example Show");
    assert.equal(title.attributes("href"), "/podcasts/pod%2F1");
    const cover = wrapper.get(".podcast-tile__cover");
    assert.equal(cover.attributes("tabindex"), "-1");
    assert.equal(cover.attributes("aria-hidden"), "true");
    wrapper.unmount();
  });

  it("shows one primary action and puts the rest in a named menu", async () => {
    const wrapper = await mountCard();
    const group = wrapper.get('[role="group"]');
    assert.equal(group.attributes("aria-label"), "Actions for Example Show");
    assert.ok(group.find('button[aria-label="Play latest episode"]').exists());
    const trigger = group.findAll("button").find((b) => b.text().includes("More actions for Example Show"));
    assert.ok(trigger, "menu trigger with an accessible name");
    trigger!.element.focus();
    await trigger!.trigger("click");
    await flushPromises();
    const items = [...document.querySelectorAll(".podcast-menu__item")].map((b) => b.textContent?.trim());
    assert.deepEqual(items, [
      "Download all episodes",
      "Alternate feeds…",
      "Pause automatic downloads",
      "Keep all episodes",
      "Skip sponsor chapters",
      "Sync to Briefpoint",
      "Delete podcast…",
    ]);
    wrapper.unmount();
  });

  it("shows settings as toggles that report their state, and only shows state that is present", async () => {
    const podcast = makePodcast({ IsPaused: true, AutoSkipSponsorChapters: true });
    const wrapper = await mountCard(podcast);
    assert.match(wrapper.get(".podcast-tile__state").text(), /Paused/);
    const trigger = wrapper.findAll("button").find((b) => b.text().includes("More actions"))!;
    trigger.element.focus();
    await trigger.trigger("click");
    await flushPromises();
    const pressed = Object.fromEntries(
      [...document.querySelectorAll(".podcast-menu__item[aria-pressed]")].map((b) => [b.textContent?.trim(), b.getAttribute("aria-pressed")]),
    );
    assert.deepEqual(pressed, {
      "Pause automatic downloads": "true",
      "Keep all episodes": "false",
      "Skip sponsor chapters": "true",
      "Sync to Briefpoint": "false",
    });
    const quiet = await mountCard(makePodcast({ IsPaused: false, DownloadingEpisodesCount: 0 }));
    assert.equal(quiet.find(".podcast-tile__state").exists(), false);
    wrapper.unmount();
    quiet.unmount();
  });

  it("emits the podcast for menu actions", async () => {
    const podcast = makePodcast();
    const wrapper = await mountCard(podcast);
    await wrapper.get('button[aria-label="Play latest episode"]').trigger("click");
    assert.deepEqual(wrapper.emitted("play")?.[0], [podcast.ID]);
    const trigger = wrapper.findAll("button").find((b) => b.text().includes("More actions"))!;
    trigger.element.focus();
    await trigger.trigger("click");
    await flushPromises();
    const item = [...document.querySelectorAll<HTMLButtonElement>(".podcast-menu__item")].find((b) => b.textContent?.includes("Delete"))!;
    item.click();
    await flushPromises();
    assert.deepEqual(wrapper.emitted("delete")?.[0], [podcast]);
    wrapper.unmount();
  });
});
