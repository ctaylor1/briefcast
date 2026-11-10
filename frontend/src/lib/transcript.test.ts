import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { activeSegmentIndex, parseTranscript } from "./transcript.ts";

describe("parseTranscript", () => {
  it("reads timed segments in either field style and drops empty ones", () => {
    const parsed = parseTranscript({
      transcript: {
        segments: [
          { start: 0, end: 4, text: " Hello ", speaker: "A" },
          { start_time: 4, end_time: 8, transcript: "World" },
          { start: 8, end: 9, text: "   " },
        ],
      },
    } as never);
    assert.deepEqual(parsed.segments, [
      { start: 0, end: 4, text: "Hello", speaker: "A" },
      { start: 4, end: 8, text: "World", speaker: undefined },
    ]);
    assert.equal(parsed.text, "");
  });

  it("uses an asset's text, or falls back to the stored transcript", () => {
    const withContent = parseTranscript({ transcript: [{ url: "https://x/t.vtt", type: "text/vtt", content: "Spoken text" }] } as never);
    assert.equal(withContent.text, "Spoken text");
    assert.deepEqual(withContent.assets, [{ url: "https://x/t.vtt", type: "text/vtt", language: undefined }]);
    const linkOnly = parseTranscript({ transcript: [{ url: "https://x/t.srt" }], canonicalTranscript: "Canonical" } as never);
    assert.equal(linkOnly.text, "Canonical");
  });

  it("reads plain text and handles nothing at all", () => {
    assert.equal(parseTranscript({ transcript: "Plain" } as never).text, "Plain");
    assert.deepEqual(parseTranscript({} as never), { segments: [], text: "", assets: [] });
  });
});

describe("activeSegmentIndex", () => {
  const segments = [
    { start: 0, end: 4, text: "a" },
    { start: 4, end: 8, text: "b" },
    { start: 10, end: 12, text: "c" },
  ];
  it("finds the segment being spoken", () => {
    assert.equal(activeSegmentIndex(segments, 0), 0);
    assert.equal(activeSegmentIndex(segments, 5), 1);
    assert.equal(activeSegmentIndex(segments, 9), 1);
    assert.equal(activeSegmentIndex(segments, 100), 2);
    assert.equal(activeSegmentIndex([], 5), -1);
  });
});
