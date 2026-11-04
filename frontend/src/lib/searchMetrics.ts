import type { SearchResult, SearchSource } from "../types/api";

export const SEARCH_SOURCE_LABELS: Record<SearchSource, string> = {
  itunes: "iTunes",
  podcastindex: "PodcastIndex",
  gpodder: "gpodder.net",
};

export type SearchMetricTone = "highlight" | "neutral";

export interface SearchMetric {
  key: string;
  label: string;
  tone: SearchMetricTone;
  title?: string;
}

const numberFormat = new Intl.NumberFormat("en-US");

function formatLatest(iso: string, now: Date): string | null {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return null;
  }
  const days = Math.floor((now.getTime() - date.getTime()) / 86_400_000);
  if (days <= 0) {
    return "Latest episode today";
  }
  if (days === 1) {
    return "Latest episode yesterday";
  }
  if (days < 45) {
    return `Latest episode ${days} days ago`;
  }
  return `Latest episode ${date.toLocaleDateString("en-US", { month: "short", year: "numeric", timeZone: "UTC" })}`;
}

/** Popularity and activity signals for a directory search result, most telling first. */
export function searchResultMetrics(result: SearchResult, now: Date = new Date()): SearchMetric[] {
  const metrics: SearchMetric[] = [];
  if (result.apple_chart_rank && result.apple_chart_rank > 0) {
    metrics.push({
      key: "chart",
      label: `#${result.apple_chart_rank} Apple Top Podcasts`,
      tone: "highlight",
      title: "Position in Apple's US Top Podcasts chart",
    });
  }
  if (result.apple_genre_rank && result.apple_genre_rank > 0 && result.apple_genre_name) {
    metrics.push({
      key: "genre-chart",
      label: `#${result.apple_genre_rank} in ${result.apple_genre_name}`,
      tone: "highlight",
      title: `Position in Apple's US ${result.apple_genre_name} chart`,
    });
  }
  if (result.subscribers && result.subscribers > 0) {
    const noun = result.subscribers === 1 ? "subscriber" : "subscribers";
    metrics.push({
      key: "subscribers",
      label: `${numberFormat.format(result.subscribers)} gpodder ${noun}`,
      tone: "neutral",
      title: "Subscribers on gpodder.net (a small open directory; useful for comparison, not total audience)",
    });
  }
  if (result.episode_count && result.episode_count > 0) {
    const noun = result.episode_count === 1 ? "episode" : "episodes";
    metrics.push({ key: "episodes", label: `${numberFormat.format(result.episode_count)} ${noun}`, tone: "neutral" });
  }
  if (result.latest_episode_date) {
    const latest = formatLatest(result.latest_episode_date, now);
    if (latest) {
      metrics.push({ key: "latest", label: latest, tone: "neutral" });
    }
  }
  return metrics;
}
