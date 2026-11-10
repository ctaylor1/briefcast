import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { formatDateTime, formatDuration } from "./format.ts";

describe("formatDateTime", () => {
  it("shows the time to the minute, without seconds", () => {
    const text = formatDateTime("2026-11-01T10:15:42Z");
    assert.ok(!/:42\b/.test(text), `seconds should be dropped: ${text}`);
    assert.match(text, /2026/);
  });

  it("reports unparseable values", () => {
    assert.equal(formatDateTime("not a date"), "Unknown");
  });
});

describe("formatDuration", () => {
  it("formats minutes and hours", () => {
    assert.equal(formatDuration(65), "01:05");
    assert.equal(formatDuration(3725), "1:02:05");
    assert.equal(formatDuration(0), "--:--");
  });
});
