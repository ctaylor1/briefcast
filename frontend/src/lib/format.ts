export function formatDate(value?: string | null): string {
  if (!value) {
    return "Unknown";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return "Unknown";
  }
  return parsed.toLocaleDateString();
}

// Date and time to the minute. Seconds are noise in lists and tables.
export function formatDateTime(value: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return "Unknown";
  }
  return parsed.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
}

export function formatDuration(seconds: number): string {
  if (seconds <= 0) {
    return "--:--";
  }
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = seconds % 60;
  if (hours > 0) {
    return `${hours}:${String(minutes).padStart(2, "0")}:${String(secs).padStart(2, "0")}`;
  }
  return `${String(minutes).padStart(2, "0")}:${String(secs).padStart(2, "0")}`;
}

export function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) {
    return "0 B";
  }
  const units = ["B", "KB", "MB", "GB", "TB"];
  let idx = 0;
  let bytes = value;
  while (bytes >= 1024 && idx < units.length - 1) {
    bytes /= 1024;
    idx += 1;
  }
  const precision = bytes >= 100 || idx === 0 ? 0 : bytes >= 10 ? 1 : 2;
  return `${bytes.toFixed(precision)} ${units[idx]}`;
}

// Elapsed/remaining time for the player: whole seconds, never "--:--".
export function formatClock(seconds: number): string {
  const total = Number.isFinite(seconds) && seconds > 0 ? Math.floor(seconds) : 0;
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const secs = total % 60;
  const mm = hours > 0 ? String(minutes).padStart(2, "0") : String(minutes);
  return `${hours > 0 ? `${hours}:` : ""}${mm}:${String(secs).padStart(2, "0")}`;
}

// Spoken form for screen readers, e.g. "1 hour 2 minutes 5 seconds".
export function describeClock(seconds: number): string {
  const total = Number.isFinite(seconds) && seconds > 0 ? Math.floor(seconds) : 0;
  const parts: string[] = [];
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const secs = total % 60;
  if (hours) parts.push(`${hours} hour${hours === 1 ? "" : "s"}`);
  if (minutes) parts.push(`${minutes} minute${minutes === 1 ? "" : "s"}`);
  if (secs || parts.length === 0) parts.push(`${secs} second${secs === 1 ? "" : "s"}`);
  return parts.join(" ");
}
