import { computed, ref, watch } from "vue";
import { settingsApi } from "../lib/api";
import {
  type EffectiveTheme,
  hourInTimezone,
  normalizeThemeMode,
  resolveTheme,
  STORAGE_KEYS,
  type ThemeMode,
  themeModeLabel as labelFor,
} from "../lib/theme";

export type { ThemeMode } from "../lib/theme";

// Storage can be unavailable (private windows, blocked site data). The theme still works.
function readStorage(key: string): string | null {
  try {
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
}

function writeStorage(key: string, value: string): void {
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // ignore
  }
}

function storedHour(key: string, fallback: number): number {
  const raw = readStorage(key);
  const value = raw === null ? NaN : Number(raw);
  return Number.isInteger(value) && value >= 0 && value < 24 ? value : fallback;
}

const themeMode = ref<ThemeMode>(normalizeThemeMode(readStorage(STORAGE_KEYS.mode)));
const timezone = ref(readStorage(STORAGE_KEYS.timezone) || "America/New_York");
const lightStartHour = ref(storedHour(STORAGE_KEYS.lightStart, 6));
const darkStartHour = ref(storedHour(STORAGE_KEYS.darkStart, 20));
const prefersDark = ref(false);
// Bumped on the hour so schedule mode re-evaluates.
const clockTick = ref(0);

let scheduleTimer: ReturnType<typeof setTimeout> | undefined;
let mediaQuery: MediaQueryList | null = null;

const effectiveTheme = computed<EffectiveTheme>(() => {
  void clockTick.value;
  return resolveTheme(themeMode.value, {
    prefersDark: prefersDark.value,
    hour: hourInTimezone(timezone.value),
    lightStartHour: lightStartHour.value,
    darkStartHour: darkStartHour.value,
  });
});

function resolveEffectiveTheme(): EffectiveTheme {
  return effectiveTheme.value;
}

function applyTheme(): void {
  document.documentElement.setAttribute("data-theme", effectiveTheme.value);
}

function scheduleNextCheck(): void {
  if (scheduleTimer) clearTimeout(scheduleTimer);
  if (themeMode.value !== "schedule") return;
  const now = new Date();
  const msUntilNextHour = (60 - now.getMinutes()) * 60_000 - now.getSeconds() * 1000;
  scheduleTimer = setTimeout(() => {
    clockTick.value += 1;
    applyTheme();
    scheduleNextCheck();
  }, msUntilNextHour);
}

function watchSystemPreference(): void {
  if (mediaQuery || typeof window.matchMedia !== "function") return;
  mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
  prefersDark.value = mediaQuery.matches;
  mediaQuery.addEventListener("change", (event) => {
    prefersDark.value = event.matches;
    applyTheme();
  });
}

function setThemeMode(mode: ThemeMode | string): void {
  themeMode.value = normalizeThemeMode(mode);
  writeStorage(STORAGE_KEYS.mode, themeMode.value);
  applyTheme();
  scheduleNextCheck();
}

// The quick toggle cycles the three everyday modes; the schedule is set in Settings.
function cycleTheme(): void {
  const order: ThemeMode[] = ["system", "light", "dark"];
  const index = order.indexOf(themeMode.value);
  setThemeMode(order[(index + 1) % order.length]!);
}

function setTimezone(tz: string): void {
  timezone.value = tz;
  writeStorage(STORAGE_KEYS.timezone, tz);
  applyTheme();
}

function setLightStartHour(hour: number): void {
  lightStartHour.value = hour;
  writeStorage(STORAGE_KEYS.lightStart, String(hour));
  applyTheme();
}

function setDarkStartHour(hour: number): void {
  darkStartHour.value = hour;
  writeStorage(STORAGE_KEYS.darkStart, String(hour));
  applyTheme();
}

function themeModeLabel(): string {
  return labelFor(themeMode.value, effectiveTheme.value);
}

async function loadFromServer(): Promise<void> {
  try {
    const settings = await settingsApi.get();
    if (settings.themeMode) setThemeMode(settings.themeMode);
    if (settings.timezone) setTimezone(settings.timezone);
    if (settings.lightStartHour != null) setLightStartHour(settings.lightStartHour);
    if (settings.darkStartHour != null) setDarkStartHour(settings.darkStartHour);
  } catch {
    // Fall back to the values in local storage.
  }
}

async function saveToServer(): Promise<void> {
  try {
    await settingsApi.update({
      themeMode: themeMode.value,
      timezone: timezone.value,
      lightStartHour: lightStartHour.value,
      darkStartHour: darkStartHour.value,
    });
  } catch {
    // Local storage keeps the choice on this device; the server copy syncs next time.
  }
}

function initTheme(): void {
  watchSystemPreference();
  applyTheme();
  scheduleNextCheck();
  void loadFromServer();
}

// Theme settings save as soon as they change; there is no Save button for them.
watch([themeMode, timezone, lightStartHour, darkStartHour], () => void saveToServer());

export function useTheme() {
  return {
    themeMode,
    timezone,
    lightStartHour,
    darkStartHour,
    effectiveTheme,
    setThemeMode,
    cycleTheme,
    setTimezone,
    setLightStartHour,
    setDarkStartHour,
    themeModeLabel,
    initTheme,
    resolveEffectiveTheme,
  };
}
