import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount } from "@vue/test-utils";
import AppNav from "./AppNav.vue";
import router from "../router";
import { fakeServer, flushPromises, type FakeServer } from "../test-support/http";
import { makeRouter } from "../test-support/router";

let server: FakeServer | null = null;
afterEach(() => {
  server?.restore();
  document.body.innerHTML = "";
});

async function mountNav(path: string, width = 1280) {
  Object.defineProperty(window, "innerWidth", { value: width, configurable: true });
  server = fakeServer({ "GET /settings": () => ({}), "PATCH /settings": () => ({}) });
  const r = await makeRouter(path, [
    { path: "/", component: { template: "<div />" }, meta: { title: "Library" } },
    { path: "/activity", component: { template: "<div />" }, meta: { title: "Activity" } },
    { path: "/podcasts/:id", component: { template: "<div />" }, meta: { title: "Podcast" } },
  ]);
  const wrapper = mount(AppNav, { global: { plugins: [r] }, slots: { default: "<main>Page</main>" }, attachTo: document.body });
  await flushPromises();
  return { wrapper, router: r };
}

describe("AppNav", () => {
  it("shows the five destinations in the sidebar and the phone tabs, Most referenced under Library", async () => {
    const { wrapper } = await mountNav("/");
    const sidebar = wrapper.get(".app-sidebar nav").findAll("a").map((a) => a.text());
    assert.deepEqual(sidebar, ["Library", "Most referenced", "Episodes", "Summaries", "Activity", "Settings"]);
    const tabs = wrapper.get(".app-bottom-tabs").findAll("a").map((a) => a.text());
    assert.deepEqual(tabs, ["Library", "Episodes", "Summaries", "Activity", "Settings"]);
    wrapper.unmount();
  });

  it("has one heading per page, no breadcrumbs, and marks the current section", async () => {
    const { wrapper } = await mountNav("/podcasts/abc");
    const h1 = wrapper.get("h1");
    assert.equal(h1.text(), "Podcast");
    assert.ok(h1.classes().includes("sr-only"), "the visible title is the page's own");
    assert.equal(wrapper.find(".breadcrumbs").exists(), false);
    const current = wrapper.findAll('.app-sidebar a[aria-current="page"]').map((a) => a.text());
    assert.deepEqual(current, ["Library"], "a podcast page belongs to Library");
    wrapper.unmount();
  });

  it("replaces the Menu with a theme toggle that cycles the theme", async () => {
    const { wrapper } = await mountNav("/");
    assert.ok(!wrapper.text().includes("Menu"));
    const toggle = wrapper.get(".app-topbar__theme button");
    const before = toggle.attributes("aria-label");
    assert.match(before ?? "", /^Theme: /);
    await toggle.trigger("click");
    assert.notEqual(toggle.attributes("aria-label"), before);
    assert.ok(wrapper.find('a[href="/add"]').exists(), "Add podcast is a button in the top bar");
    wrapper.unmount();
  });
});

describe("old addresses", () => {
  it("redirect Downloads to Activity and About to Settings", async () => {
    await router.push("/downloads");
    assert.equal(router.currentRoute.value.path, "/activity");
    await router.push("/about");
    assert.equal(router.currentRoute.value.fullPath, "/settings?section=about");
    await router.push("/no/such/page");
    assert.equal(router.currentRoute.value.path, "/");
  });
});
