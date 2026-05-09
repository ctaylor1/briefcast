import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import DOMPurify from "dompurify";

const window = new JSDOM("").window;
const purify = DOMPurify(window as unknown as Window);

function sanitizeHtml(dirty: string): string {
  return purify.sanitize(dirty);
}

describe("sanitizeHtml", () => {
  it("strips <script> tags", () => {
    const dirty = '<p>Hello</p><script>alert("xss")</script>';
    const result = sanitizeHtml(dirty);
    assert.ok(!result.includes("<script"), "should not contain <script");
    assert.ok(result.includes("<p>Hello</p>"), "should preserve safe HTML");
  });

  it("strips event handler attributes", () => {
    const dirty = '<img src="x" onerror="alert(1)">';
    const result = sanitizeHtml(dirty);
    assert.ok(!result.includes("onerror"), "should not contain onerror");
  });

  it("strips javascript: URLs", () => {
    const dirty = '<a href="javascript:alert(1)">click</a>';
    const result = sanitizeHtml(dirty);
    assert.ok(!result.includes("javascript:"), "should not contain javascript:");
  });

  it("preserves safe HTML from Markdown rendering", () => {
    const safe =
      '<h2 id="summary-heading-1">Overview</h2><p>This is a <strong>bold</strong> summary with a <a href="https://example.com">link</a>.</p><ul><li>Item 1</li><li>Item 2</li></ul>';
    const result = sanitizeHtml(safe);
    assert.equal(result, safe);
  });

  it("preserves heading IDs used for TOC navigation", () => {
    const html = '<h2 id="summary-heading-3">Details</h2>';
    const result = sanitizeHtml(html);
    assert.ok(result.includes('id="summary-heading-3"'), "should keep heading IDs");
  });

  it("strips nested script attempts", () => {
    const dirty = '<div><scr<script>ipt>alert(1)</scr</script>ipt></div>';
    const result = sanitizeHtml(dirty);
    assert.ok(!result.includes("<script"), "should not contain <script");
  });

  it("strips SVG-based XSS", () => {
    const dirty = '<svg onload="alert(1)"><circle r="10"/></svg>';
    const result = sanitizeHtml(dirty);
    assert.ok(!result.includes("onload"), "should not contain onload");
  });
});
