import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { mount, type VueWrapper } from "@vue/test-utils";
import { nextTick } from "vue";
import UiDropdown from "./UiDropdown.vue";
import { flushPromises } from "../../test-support/http";

let wrapper: VueWrapper | null = null;
afterEach(() => {
  wrapper?.unmount();
  document.body.innerHTML = "";
});

// Like the app: the menu lives inside one root element (#app), next to other content.
function appRoot(): { host: HTMLElement; sibling: HTMLElement } {
  const root = document.createElement("div");
  const host = document.createElement("div");
  const sibling = document.createElement("p");
  root.append(host, sibling);
  document.body.appendChild(root);
  return { host, sibling };
}

let sibling: HTMLElement;

function mountMenu() {
  const nodes = appRoot();
  sibling = nodes.sibling;
  wrapper = mount(UiDropdown, {
    attachTo: nodes.host,
    props: { label: "Menu" },
    slots: {
      default: `<button type="button" class="go">Open settings</button>
                <button type="button" class="stay" data-keep-open>Theme</button>`,
    },
  });
  return wrapper;
}

// Browsers focus a button when it is clicked; jsdom does not, so do it here.
async function open(w: VueWrapper) {
  (w.get("button").element as HTMLButtonElement).focus();
  await w.get("button").trigger("click");
  await flushPromises();
}

const panel = () => document.querySelector(".ui-dropdown__menu");

describe("UiDropdown", () => {
  it("opens from its button and reports aria-expanded", async () => {
    const w = mountMenu();
    const trigger = w.get("button");
    assert.equal(trigger.attributes("aria-expanded"), "false");
    assert.equal(panel(), null);
    await open(w);
    assert.equal(trigger.attributes("aria-expanded"), "true");
    assert.ok(panel());
  });

  it("closes on Escape", async () => {
    const w = mountMenu();
    await open(w);
    document.activeElement?.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await flushPromises();
    assert.equal(panel(), null);
  });

  it("closes on a click outside", async () => {
    const w = mountMenu();
    await open(w);
    const outside = sibling;
    for (const type of ["pointerdown", "mousedown", "pointerup", "mouseup", "click"]) {
      outside.dispatchEvent(new MouseEvent(type, { bubbles: true }));
    }
    await flushPromises();
    assert.equal(panel(), null);
  });

  it("closes after an action is used, unless the action keeps it open", async () => {
    const w = mountMenu();
    await open(w);
    (document.querySelector(".stay") as HTMLButtonElement).click();
    await nextTick();
    assert.ok(panel(), "data-keep-open leaves it open");
    (document.querySelector(".go") as HTMLButtonElement).click();
    await flushPromises();
    assert.equal(panel(), null);
  });
});
