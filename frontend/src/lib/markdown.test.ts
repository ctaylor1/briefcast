import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { normalizeSummaryMarkdown, renderSummary } from "./markdown.ts";
import { clearReadingList, readingPosition, setReadingList } from "./readingList.ts";

describe("renderSummary", () => {
  it("builds a table of contents from headings, with plain text", () => {
    const { html, toc } = renderSummary("# **Big** idea\n\nText\n\n## Part `one`\n\nMore");
    assert.deepEqual(toc, [
      { id: "summary-heading-1", text: "Big idea", level: 1 },
      { id: "summary-heading-2", text: "Part one", level: 2 },
    ]);
    assert.match(html, /<h1 id="summary-heading-1">Big idea<\/h1>/);
  });

  it("sanitizes what the model wrote", () => {
    const { html } = renderSummary("Hi <img src=x onerror=alert(1)> <script>alert(2)</script>");
    assert.ok(!/onerror|<script/.test(html), html);
  });

  it("turns labelled outline summaries into sections", () => {
    const normalized = normalizeSummaryMarkdown("- Title: A talk\n- Key Points: one");
    assert.match(normalized, /#/);
    assert.ok(renderSummary("- Title: A talk\n- Key Points: one").toc.length >= 1);
  });
});

describe("reading list", () => {
  it("knows the neighbours of an item", () => {
    setReadingList(["a", "b", "c"], "Summary");
    assert.deepEqual(readingPosition("b"), { previous: "a", next: "c", index: 1, total: 3, label: "Summary" });
    assert.equal(readingPosition("a")?.previous, null);
    assert.equal(readingPosition("c")?.next, null);
    assert.equal(readingPosition("x"), null);
    clearReadingList();
    assert.equal(readingPosition("b"), null);
  });
});
