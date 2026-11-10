import { describe, it } from "node:test";
import assert from "node:assert/strict";
import {
  DownloadStatus,
  downloadStatusLabel,
  downloadStatusTone,
  progressAriaValue,
  progressLabel,
  progressPercent,
  transcriptPill,
} from "./episodeStatus.ts";
import { makeEpisode } from "../test-support/fixtures.ts";

describe("episode status helpers", () => {
  it("label and colour every download state", () => {
    assert.deepEqual(
      Object.values(DownloadStatus).map((s) => [downloadStatusLabel(s), downloadStatusTone(s)]),
      [
        ["Queued", "warning"],
        ["Downloading", "info"],
        ["Downloaded", "success"],
        ["Not queued", "neutral"],
        ["Paused", "warning"],
      ],
    );
    assert.equal(downloadStatusLabel(99), "Unknown");
  });

  it("report progress with and without a known size", () => {
    const known = makeEpisode({ DownloadTotalBytes: 2048, DownloadedBytes: 512 });
    assert.equal(progressPercent(known), 25);
    assert.equal(progressAriaValue(known), 25);
    assert.match(progressLabel(known), /^25% \(512 B \/ 2\.00 KB\)$/);
    const unknown = makeEpisode({ DownloadTotalBytes: 0, DownloadedBytes: 0 });
    assert.equal(progressAriaValue(unknown), undefined);
    assert.equal(progressLabel(unknown), "Downloading...");
  });

  it("show transcript state only when there is one", () => {
    assert.equal(transcriptPill("available").label, "Transcript ready");
    assert.equal(transcriptPill("pending_download").tone, "warning");
    assert.equal(transcriptPill("missing").visible, false);
  });
});
