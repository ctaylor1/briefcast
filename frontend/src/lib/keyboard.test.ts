import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readerKeyAction } from "./keyboard.ts";

describe("readerKeyAction", () => {
  it("maps J, K and Escape", () => {
    assert.equal(readerKeyAction({ key: "j" }), "next");
    assert.equal(readerKeyAction({ key: "J" }), "next");
    assert.equal(readerKeyAction({ key: "k" }), "previous");
    assert.equal(readerKeyAction({ key: "Escape" }), "close");
  });

  it("leaves the arrow keys to scroll the page", () => {
    for (const key of ["ArrowDown", "ArrowUp", "ArrowLeft", "ArrowRight", "PageDown", " "]) {
      assert.equal(readerKeyAction({ key }), null, key);
    }
  });

  it("ignores modified keys, handled events and typing in fields", () => {
    assert.equal(readerKeyAction({ key: "j", ctrlKey: true }), null);
    assert.equal(readerKeyAction({ key: "j", metaKey: true }), null);
    assert.equal(readerKeyAction({ key: "j", defaultPrevented: true }), null);
    const input = document.createElement("input");
    assert.equal(readerKeyAction({ key: "j", target: input }), null);
    const div = document.createElement("div");
    assert.equal(readerKeyAction({ key: "j", target: div }), "next");
  });
});
