import { marked } from "marked";
import { sanitizeHtml } from "./sanitize";

// AI summaries arrive as Markdown, sometimes as a bulleted outline with labelled sections.
// renderSummary turns either into sanitized HTML plus a table of contents.

export interface TocEntry {
  id: string;
  text: string;
  level: number;
}

export interface RenderedSummary {
  html: string;
  toc: TocEntry[];
}

// Older summaries are "- Title: ...", "- Key Points: ..." outlines; give them headings.
export function normalizeSummaryMarkdown(raw: string): string {
  if (/^#{1,3}\s+/m.test(raw)) return raw;

  const sectionRe =
    /^-\s+(Title|Core Thesis|Key Points|Notable Details|Actionable Takeaways|Open Questions(?:\s+or\s+Uncertainties)?)\s*:\s*(.*)/i;

  const lines = raw.split("\n");
  const out: string[] = [];

  for (const line of lines) {
    const m = line.match(sectionRe);
    if (m && m[1] && m[2] !== undefined) {
      const label = m[1].trim();
      const rest = m[2].trim();
      if (/^title$/i.test(label) && rest) {
        out.push(`## ${rest}`);
      } else {
        out.push(`## ${label}`);
        if (rest) {
          out.push("");
          out.push(rest);
        }
      }
    } else {
      out.push(line);
    }
  }
  return out.join("\n");
}

// Plain text of a heading, for the table of contents.
export function stripMarkdown(text: string): string {
  return text
    .replace(/\*\*(.+?)\*\*/g, "$1")  // **bold**
    .replace(/__(.+?)__/g, "$1")       // __bold__
    .replace(/\*(.+?)\*/g, "$1")       // *italic*
    .replace(/_(.+?)_/g, "$1")         // _italic_
    .replace(/~~(.+?)~~/g, "$1")       // ~~strikethrough~~
    .replace(/`(.+?)`/g, "$1")         // `code`
    .replace(/\[([^\]]+)\]\([^)]+\)/g, "$1") // [link](url)
    .replace(/^#{1,6}\s+/gm, "")       // leading # headings
    .replace(/---+/g, "")              // horizontal rules
    .trim();
}

export function renderSummary(text: string, idPrefix = "summary-heading"): RenderedSummary {
  const toc: TocEntry[] = [];
  let headingIndex = 0;
  const renderer = new marked.Renderer();
  renderer.heading = function ({ text: headingText, depth }: { text: string; depth: number }) {
    headingIndex += 1;
    const id = `${idPrefix}-${headingIndex}`;
    const clean = stripMarkdown(headingText);
    toc.push({ id, text: clean, level: depth });
    return `<h${depth} id="${id}">${clean}</h${depth}>`;
  };
  const html = marked.parse(normalizeSummaryMarkdown(text), { renderer, async: false }) as string;
  return { html: sanitizeHtml(html), toc };
}
