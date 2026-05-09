import { describe, expect, it } from "vitest";
import { sanitizeHtml } from "./sanitize";

describe("sanitizeHtml", () => {
  it("strips <script> tags", () => {
    const dirty = '<p>Hello</p><script>alert("xss")</script>';
    const result = sanitizeHtml(dirty);
    expect(result).not.toContain("<script");
    expect(result).toContain("<p>Hello</p>");
  });

  it("strips event handler attributes", () => {
    const dirty = '<img src="x" onerror="alert(1)">';
    const result = sanitizeHtml(dirty);
    expect(result).not.toContain("onerror");
  });

  it("strips javascript: URLs", () => {
    const dirty = '<a href="javascript:alert(1)">click</a>';
    const result = sanitizeHtml(dirty);
    expect(result).not.toContain("javascript:");
  });

  it("preserves safe HTML from Markdown rendering", () => {
    const safe =
      '<h2 id="summary-heading-1">Overview</h2><p>This is a <strong>bold</strong> summary with a <a href="https://example.com">link</a>.</p><ul><li>Item 1</li><li>Item 2</li></ul>';
    const result = sanitizeHtml(safe);
    expect(result).toBe(safe);
  });

  it("preserves heading IDs used for TOC navigation", () => {
    const html = '<h2 id="summary-heading-3">Details</h2>';
    const result = sanitizeHtml(html);
    expect(result).toContain('id="summary-heading-3"');
  });

  it("strips nested script attempts", () => {
    const dirty = '<div><scr<script>ipt>alert(1)</scr</script>ipt></div>';
    const result = sanitizeHtml(dirty);
    expect(result).not.toContain("<script");
    expect(result).not.toContain("alert");
  });

  it("strips SVG-based XSS", () => {
    const dirty = '<svg onload="alert(1)"><circle r="10"/></svg>';
    const result = sanitizeHtml(dirty);
    expect(result).not.toContain("onload");
  });
});
