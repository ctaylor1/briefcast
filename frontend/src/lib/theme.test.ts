import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { runInNewContext } from "node:vm";
import { hourInTimezone, normalizeThemeMode, resolveTheme, type ThemeMode } from "./theme.ts";

const at = (hour: number, prefersDark = false) => ({ prefersDark, hour, lightStartHour: 6, darkStartHour: 20 });

describe("theme modes", () => {
  it("treat the old 'auto' default and unknown values as system", () => {
    assert.equal(normalizeThemeMode("auto"), "system");
    assert.equal(normalizeThemeMode("sepia"), "system");
    assert.equal(normalizeThemeMode(null), "system");
    assert.equal(normalizeThemeMode("schedule"), "schedule");
  });

  it("resolve system, fixed and scheduled themes", () => {
    assert.equal(resolveTheme("system", at(12, true)), "dark");
    assert.equal(resolveTheme("system", at(12, false)), "light");
    assert.equal(resolveTheme("dark", at(12)), "dark");
    assert.equal(resolveTheme("light", at(23)), "light");
    assert.equal(resolveTheme("schedule", at(12)), "light");
    assert.equal(resolveTheme("schedule", at(21)), "dark");
    assert.equal(resolveTheme("schedule", at(5)), "dark");
    // A schedule that wraps past midnight: light 22:00 -> dark 04:00.
    const wrap = { prefersDark: false, lightStartHour: 22, darkStartHour: 4 };
    assert.equal(resolveTheme("schedule", { ...wrap, hour: 23 }), "light");
    assert.equal(resolveTheme("schedule", { ...wrap, hour: 2 }), "light");
    assert.equal(resolveTheme("schedule", { ...wrap, hour: 12 }), "dark");
  });
});

describe("index.html theme boot script", () => {
  const html = readFileSync(new URL("../../index.html", import.meta.url), "utf8");
  const script = /<script id="theme-boot">([\s\S]*?)<\/script>/.exec(html)?.[1] ?? "";

  function boot(storage: Record<string, string>, prefersDark: boolean): string {
    let theme = "";
    runInNewContext(script, {
      window: {
        localStorage: { getItem: (key: string) => (key in storage ? storage[key] : null) },
        matchMedia: () => ({ matches: prefersDark }),
      },
      document: { documentElement: { setAttribute: (_: string, value: string) => (theme = value) } },
      Intl,
      Date,
      Number,
      Math,
    });
    return theme;
  }

  it("is present and runs before the app bundle", () => {
    assert.ok(script.length > 100);
    assert.ok(html.indexOf('id="theme-boot"') < html.indexOf('type="module"'));
  });

  it("agrees with resolveTheme for every mode", () => {
    const hourUtc = hourInTimezone("UTC");
    const cases: Array<[string | undefined, boolean]> = [];
    for (const mode of [undefined, "auto", "system", "light", "dark", "schedule", "junk"]) {
      for (const prefersDark of [false, true]) {
        cases.push([mode, prefersDark]);
      }
    }
    for (const [mode, prefersDark] of cases) {
      for (const [light, dark] of [[6, 20], [22, 4], [hourUtc, (hourUtc + 1) % 24]]) {
        const storage: Record<string, string> = { "briefcast-timezone": "UTC", "briefcast-light-start": String(light), "briefcast-dark-start": String(dark) };
        if (mode !== undefined) storage["briefcast-theme"] = mode;
        const expected = resolveTheme(normalizeThemeMode(mode ?? null) as ThemeMode, {
          prefersDark,
          hour: hourUtc,
          lightStartHour: light!,
          darkStartHour: dark!,
        });
        assert.equal(boot(storage, prefersDark), expected, `mode=${mode} dark=${prefersDark} ${light}-${dark}`);
      }
    }
  });

  it("falls back to the system setting when storage is blocked", () => {
    let theme = "";
    runInNewContext(script, {
      window: {
        localStorage: { getItem: () => { throw new Error("SecurityError"); } },
        matchMedia: () => ({ matches: true }),
      },
      document: { documentElement: { setAttribute: (_: string, value: string) => (theme = value) } },
      Intl,
      Date,
      Number,
      Math,
    });
    assert.equal(theme, "dark");
  });
});
