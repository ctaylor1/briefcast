// Static accessibility checks over every component template.
import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const srcDir = fileURLToPath(new URL(".", import.meta.url));

function vueFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) {
      return vueFiles(path);
    }
    return name.endsWith(".vue") ? [path] : [];
  });
}

function template(source: string): string {
  const start = source.indexOf("<template>");
  const end = source.lastIndexOf("</template>");
  return start >= 0 && end > start ? source.slice(start, end) : "";
}

// Interactive elements whose only content is an icon need a name from aria-label.
const INTERACTIVE = /<(button|a|summary|RouterLink)\b([^>]*)>([\s\S]*?)<\/\1>/g;
const GLYPHS_ONLY = /^[\s▶⏹↻×✕★☆⟳⏸⏵⏭⏮]*$/u;

function isIconOnly(inner: string): boolean {
  if (/<slot\b|\{\{/.test(inner)) {
    return false;
  }
  const text = inner
    .replace(/<svg[\s\S]*?<\/svg>/g, "")
    .replace(/<UiIcon\b[^>]*\/>/g, "")
    .replace(/<[^>]+>/g, "")
    .replace(/&\w+;/g, "");
  return GLYPHS_ONLY.test(text);
}

describe("component templates", () => {
  const files = vueFiles(srcDir);

  it("finds the components", () => {
    assert.ok(files.length > 20, `only found ${files.length} .vue files`);
  });

  it("give every icon-only control an accessible name", () => {
    const problems: string[] = [];
    for (const file of files) {
      const body = template(readFileSync(file, "utf8"));
      for (const match of body.matchAll(INTERACTIVE)) {
        const [, tag, attrs, inner] = match;
        if (/\baria-label(ledby)?=|:aria-label(ledby)?=/.test(attrs)) {
          continue;
        }
        // Hidden from assistive tech and the tab order on purpose: a duplicate of a named
        // control next to it (e.g. a cover image that repeats the title link).
        if (/\baria-hidden="true"/.test(attrs) && /\btabindex="-1"/.test(attrs)) {
          continue;
        }
        if (isIconOnly(inner)) {
          problems.push(`${relative(srcDir, file)}: <${tag}${attrs.replace(/\s+/g, " ").slice(0, 80)}>`);
        }
      }
    }
    assert.deepEqual(problems, []);
  });

  it("bind aria-pressed to a string, so false is not dropped from the DOM", () => {
    const problems: string[] = [];
    for (const file of files) {
      const body = template(readFileSync(file, "utf8"));
      for (const match of body.matchAll(/:aria-pressed="([^"]+)"/g)) {
        if (!/'true'|"true"|String\(/.test(match[1])) {
          problems.push(`${relative(srcDir, file)}: :aria-pressed="${match[1]}"`);
        }
      }
    }
    assert.deepEqual(problems, []);
  });

  // An unimported component only fails at runtime ("Failed to resolve component"), and
  // the type check does not catch it.
  it("import every component they use", () => {
    const GLOBALS = new Set(["RouterLink", "RouterView", "Transition", "TransitionGroup", "KeepAlive", "Teleport", "Suspense", "Component"]);
    const problems: string[] = [];
    for (const file of files) {
      const source = readFileSync(file, "utf8");
      const script = source.slice(0, source.indexOf("</script>"));
      const imported = new Set<string>();
      for (const m of script.matchAll(/import\s+(?:type\s+)?([^;]*?)\s+from\s/g)) {
        for (const name of m[1]!.replace(/[{}]/g, " ").split(/[\s,]+/)) {
          if (name && name !== "type" && name !== "as") imported.add(name);
        }
      }
      for (const m of template(source).matchAll(/<([A-Z][A-Za-z0-9]+)[\s/>]/g)) {
        const tag = m[1]!;
        if (!GLOBALS.has(tag) && !imported.has(tag)) {
          problems.push(`${relative(srcDir, file)}: <${tag}> is not imported`);
        }
      }
    }
    assert.deepEqual([...new Set(problems)], []);
  });
});
