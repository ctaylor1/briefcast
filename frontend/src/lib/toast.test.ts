import { afterEach, beforeEach, describe, it, mock } from "node:test";
import assert from "node:assert/strict";
import { clearToasts, dismissToast, TOAST_TIMEOUT_MS, toast, useToasts } from "./toast.ts";

describe("toast store", () => {
  beforeEach(() => mock.timers.enable({ apis: ["setTimeout"] }));
  afterEach(() => {
    clearToasts();
    mock.timers.reset();
  });

  it("times out notices but keeps errors until dismissed", () => {
    toast.success("Saved.");
    const errorId = toast.error("Could not save.");
    assert.deepEqual(useToasts().toasts.map((t) => t.tone), ["success", "danger"]);
    mock.timers.tick(TOAST_TIMEOUT_MS + 1);
    assert.deepEqual(useToasts().toasts.map((t) => t.message), ["Could not save."]);
    dismissToast(errorId);
    assert.equal(useToasts().toasts.length, 0);
  });

  it("ignores empty messages, collapses repeats and keeps at most four", () => {
    toast.info("   ");
    assert.equal(useToasts().toasts.length, 0);
    toast.success("Saved.");
    toast.success("Saved.");
    assert.equal(useToasts().toasts.length, 1);
    for (const n of [1, 2, 3, 4, 5]) {
      toast.error(`Error ${n}`);
    }
    assert.deepEqual(useToasts().toasts.map((t) => t.message), ["Error 2", "Error 3", "Error 4", "Error 5"]);
  });
});
