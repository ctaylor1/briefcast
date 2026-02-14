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

export function formatDateTime(value: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return "Unknown";
  }
  return parsed.toLocaleString();
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
