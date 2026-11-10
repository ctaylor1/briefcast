// Contrast of the colour tokens in style.css, per theme (WCAG 2.x). Guards the theme work:
// any token change that breaks a pairing the stylesheet relies on fails here.
import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const css = readFileSync(new URL("./style.css", import.meta.url), "utf8");

function block(selector: string): string {
  const start = css.indexOf(`${selector} {`);
  assert.ok(start >= 0, `missing ${selector}`);
  return css.slice(start, css.indexOf("\n}", start));
}

function tokens(body: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const m of body.matchAll(/(--color-[\w-]+):\s*([^;]+);/g)) {
    out[m[1]!] = m[2]!.trim();
  }
  return out;
}

const light = tokens(block(":root"));
const dark = { ...light, ...tokens(block('[data-theme="dark"]')) };
type Rgb = [number, number, number];

function resolve(value: string, theme: Record<string, string>, under: Rgb): Rgb {
  const hex = /^#([0-9a-f]{6})$/i.exec(value);
  if (hex) {
    const h = hex[1]!;
    return [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16)) as Rgb;
  }
  const ref = /^var\((--[\w-]+)\)$/.exec(value);
  if (ref) {
    return resolve(theme[ref[1]!]!, theme, under);
  }
  const rgba = /^rgba\((\d+),\s*(\d+),\s*(\d+),\s*([\d.]+)\)$/.exec(value);
  if (rgba) {
    const alpha = Number(rgba[4]);
    return [1, 2, 3].map((i, k) => Math.round(Number(rgba[i]) * alpha + under[k]! * (1 - alpha))) as Rgb;
  }
  throw new Error(`unsupported colour ${value}`);
}

function luminance([r, g, b]: Rgb): number {
  const c = [r, g, b].map((v) => {
    const s = v / 255;
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * c[0]! + 0.7152 * c[1]! + 0.0722 * c[2]!;
}

function ratio(fg: string, bg: string, theme: Record<string, string>): number {
  const surface = resolve(theme["--color-bg-primary"]!, theme, [255, 255, 255]);
  const back = resolve(theme[bg]!, theme, surface);
  const front = resolve(theme[fg]!, theme, back);
  const [a, b] = [luminance(front), luminance(back)].sort((x, y) => y - x);
  return (a! + 0.05) / (b! + 0.05);
}

const SURFACES = ["--color-bg-primary", "--color-bg-secondary", "--color-hover"];
const TEXT_ON_SURFACES = ["--color-text-primary", "--color-text-secondary", "--color-accent", "--color-danger", "--color-success", "--color-warning"];

for (const [name, theme] of [["light", light], ["dark", dark]] as const) {
  describe(`${name} theme contrast`, () => {
    it("keeps text and accent colours at 4.5:1 on every surface", () => {
      const failures: string[] = [];
      for (const fg of TEXT_ON_SURFACES) {
        for (const bg of SURFACES) {
          const value = ratio(fg, bg, theme);
          if (value < 4.5) failures.push(`${fg} on ${bg}: ${value.toFixed(2)}`);
        }
      }
      assert.deepEqual(failures, []);
    });

    it("keeps text on solid accent fills and tinted accents at 4.5:1", () => {
      assert.ok(ratio("--color-on-solid", "--color-accent", theme) >= 4.5, "button text on accent");
      assert.ok(ratio("--color-accent", "--color-accent-subtle", theme) >= 4.5, "accent on its tint");
      assert.ok(ratio("--color-accent-hover", "--color-accent-subtle", theme) >= 4.5, "hover accent on its tint");
    });
  });
}

describe("brand accent", () => {
  it("comes from the logo gradient", () => {
    assert.equal(light["--color-accent"], "#1370ab");
    assert.equal(dark["--color-accent"], "#0ea7e0");
  });
});
