import { ref, watch } from "vue";
import { settingsApi } from "../lib/api";

export type ThemeMode = "light" | "dark" | "auto";

const STORAGE_KEY = "briefcast-theme";
const TIMEZONE_KEY = "briefcast-timezone";
const LIGHT_START_KEY = "briefcast-light-start";
const DARK_START_KEY = "briefcast-dark-start";

const themeMode = ref<ThemeMode>(
  (localStorage.getItem(STORAGE_KEY) as ThemeMode) || "auto",
);
const timezone = ref(localStorage.getItem(TIMEZONE_KEY) || "America/New_York");
const lightStartHour = ref(Number(localStorage.getItem(LIGHT_START_KEY)) || 6);
const darkStartHour = ref(Number(localStorage.getItem(DARK_START_KEY)) || 20);

let scheduleTimer: ReturnType<typeof setTimeout> | undefined;

function currentHourInTimezone(tz: string): number {
  try {
    const parts = new Intl.DateTimeFormat("en-US", {
      timeZone: tz,
      hour: "numeric",
      hour12: false,
    }).formatToParts(new Date());
    const hourPart = parts.find((p) => p.type === "hour");
    return hourPart ? Number(hourPart.value) : new Date().getHours();
  } catch {
    return new Date().getHours();
  }
}

function resolveEffectiveTheme(): "light" | "dark" {
  if (themeMode.value !== "auto") return themeMode.value;
  const hour = currentHourInTimezone(timezone.value);
  if (lightStartHour.value <= darkStartHour.value) {
    return hour >= lightStartHour.value && hour < darkStartHour.value ? "light" : "dark";
  }
  return hour >= lightStartHour.value || hour < darkStartHour.value ? "light" : "dark";
}

function applyTheme(): void {
  const effective = resolveEffectiveTheme();
  document.documentElement.setAttribute("data-theme", effective);
}

function scheduleNextCheck(): void {
  if (scheduleTimer) clearTimeout(scheduleTimer);
  if (themeMode.value !== "auto") return;
  const msUntilNextHour =
    (60 - new Date().getMinutes()) * 60_000 - new Date().getSeconds() * 1000;
  scheduleTimer = setTimeout(() => {
    applyTheme();
    scheduleNextCheck();
  }, msUntilNextHour);
}

function setThemeMode(mode: ThemeMode): void {
  themeMode.value = mode;
  localStorage.setItem(STORAGE_KEY, mode);
  applyTheme();
  scheduleNextCheck();
}

function cycleTheme(): void {
  const order: ThemeMode[] = ["auto", "light", "dark"];
  const idx = order.indexOf(themeMode.value);
  setThemeMode(order[(idx + 1) % order.length]!);
}

function setTimezone(tz: string): void {
  timezone.value = tz;
  localStorage.setItem(TIMEZONE_KEY, tz);
  applyTheme();
}

function setLightStartHour(hour: number): void {
  lightStartHour.value = hour;
  localStorage.setItem(LIGHT_START_KEY, String(hour));
  applyTheme();
}

function setDarkStartHour(hour: number): void {
  darkStartHour.value = hour;
  localStorage.setItem(DARK_START_KEY, String(hour));
  applyTheme();
}

function themeModeLabel(): string {
  if (themeMode.value === "auto") {
    return `Auto (${resolveEffectiveTheme()})`;
  }
  return themeMode.value === "dark" ? "Dark" : "Light";
}

async function loadFromServer(): Promise<void> {
  try {
    const settings = await settingsApi.get();
    if (settings.themeMode) setThemeMode(settings.themeMode as ThemeMode);
    if (settings.timezone) setTimezone(settings.timezone);
    if (settings.lightStartHour != null) setLightStartHour(settings.lightStartHour);
    if (settings.darkStartHour != null) setDarkStartHour(settings.darkStartHour);
  } catch {
    // Fall back to localStorage values
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
    // Silent fail — localStorage is the primary store
  }
}

function initTheme(): void {
  applyTheme();
  scheduleNextCheck();
  void loadFromServer();
}

watch(
  [themeMode, timezone, lightStartHour, darkStartHour],
  () => void saveToServer(),
);

export function useTheme() {
  return {
    themeMode,
    timezone,
    lightStartHour,
    darkStartHour,
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
