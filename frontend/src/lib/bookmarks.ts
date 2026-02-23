export const UNBOOKMARKED_DATE = "0001-01-01T00:00:00Z";

export function isBookmarkedDate(value: string | null | undefined): boolean {
  if (!value) {
    return false;
  }

  const trimmed = value.trim();
  if (!trimmed) {
    return false;
  }

  // Go zero-time can appear with small format differences; treat all as unbookmarked.
  if (trimmed.startsWith("0001-01-01T00:00:00")) {
    return false;
  }

  const parsed = new Date(trimmed);
  if (Number.isNaN(parsed.getTime())) {
    return false;
  }

  return parsed.getUTCFullYear() > 1;
}
