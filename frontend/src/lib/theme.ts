// Theme modes. "system" follows the operating system; "schedule" switches at set hours.
// The inline script in index.html repeats resolveTheme so the first paint already has the
// right theme; lib/theme.test.ts runs that script to keep the two in step.
export type ThemeMode = "system" | "light" | "dark" | "schedule";
export type EffectiveTheme = "light" | "dark";

export const THEME_MODES: readonly ThemeMode[] = ["system", "light", "dark", "schedule"];

export const STORAGE_KEYS = {
  mode: "briefcast-theme",
  timezone: "briefcast-timezone",
  lightStart: "briefcast-light-start",
  darkStart: "briefcast-dark-start",
} as const;

// "auto" was the clock schedule and the database default, so nearly every install has it.
// It now means "follow the system"; the schedule is an explicit choice.
export function normalizeThemeMode(value: unknown): ThemeMode {
  if (value === "auto") {
    return "system";
  }
  return THEME_MODES.includes(value as ThemeMode) ? (value as ThemeMode) : "system";
}

export interface ThemeContext {
  prefersDark: boolean;
  hour: number;
  lightStartHour: number;
  darkStartHour: number;
}

export function resolveTheme(mode: ThemeMode, context: ThemeContext): EffectiveTheme {
  if (mode === "light" || mode === "dark") {
    return mode;
  }
  if (mode === "system") {
    return context.prefersDark ? "dark" : "light";
  }
  const { hour, lightStartHour: light, darkStartHour: dark } = context;
  if (light <= dark) {
    return hour >= light && hour < dark ? "light" : "dark";
  }
  return hour >= light || hour < dark ? "light" : "dark";
}

export function hourInTimezone(timezone: string, now = new Date()): number {
  try {
    const parts = new Intl.DateTimeFormat("en-US", { timeZone: timezone, hour: "numeric", hour12: false }).formatToParts(now);
    const hour = Number(parts.find((part) => part.type === "hour")?.value);
    return Number.isFinite(hour) ? hour % 24 : now.getHours();
  } catch {
    return now.getHours();
  }
}

export function themeModeLabel(mode: ThemeMode, effective: EffectiveTheme): string {
  switch (mode) {
    case "system":
      return `System (${effective})`;
    case "schedule":
      return `Schedule (${effective})`;
    case "dark":
      return "Dark";
    default:
      return "Light";
  }
}
