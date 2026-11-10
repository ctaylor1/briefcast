// Preloaded by `npm run test:unit` (node --import). It gives every test a jsdom
// window and document, and registers the hooks that load .ts and .vue files.
import { register } from "node:module";
import { JSDOM } from "jsdom";

const dom = new JSDOM("<!doctype html><html><head></head><body></body></html>", {
  url: "http://localhost/app/",
  pretendToBeVisual: true,
});
const { window } = dom;

globalThis.window = window;
globalThis.document = window.document;
for (const key of Object.getOwnPropertyNames(window)) {
  if (key in globalThis) {
    continue;
  }
  try {
    globalThis[key] = window[key];
  } catch {
    // Some window properties are read-only accessors; skip them.
  }
}

// Node has its own Event and CustomEvent, which jsdom refuses to dispatch. DOM tests need
// jsdom's.
globalThis.Event = window.Event;
globalThis.CustomEvent = window.CustomEvent;

// jsdom has no media playback. Tests that need different behaviour override these.
window.HTMLMediaElement.prototype.play = function play() {
  return Promise.resolve();
};
window.HTMLMediaElement.prototype.pause = function pause() {};
window.HTMLMediaElement.prototype.load = function load() {};

register("./hooks.mjs", import.meta.url);
