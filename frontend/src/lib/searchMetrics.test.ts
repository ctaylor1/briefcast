import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { SEARCH_SOURCE_LABELS, searchResultMetrics } from "./searchMetrics.ts";
import type { SearchResult } from "../types/api";

const base: SearchResult = { url: "u", title: "t", image: "", already_saved: false, description: "" };
const now = new Date("2026-11-04T12:00:00Z");

describe("Search result metrics", () => {
  it("lists chart ranks, subscribers, episodes and recency in that order", () => {
    const metrics = searchResultMetrics(
      {
        ...base,
        apple_chart_rank: 12,
        apple_genre_rank: 3,
        apple_genre_name: "Business",
        subscribers: 1532,
        episode_count: 891,
        latest_episode_date: "2026-10-30T10:00:00Z",
      },
      now,
    );
    assert.deepEqual(
      metrics.map((m) => m.label),
      ["#12 Apple Top Podcasts", "#3 in Business", "1,532 gpodder subscribers", "891 episodes", "Latest episode 5 days ago"],
    );
    assert.equal(metrics[0].tone, "highlight");
    assert.equal(metrics[1].tone, "highlight");
  });

  it("needs a genre name to show a genre rank", () => {
    assert.deepEqual(searchResultMetrics({ ...base, apple_genre_rank: 4 }, now), []);
  });

  it("omits missing or zero values and handles singulars and old dates", () => {
    assert.deepEqual(searchResultMetrics(base, now), []);
    const metrics = searchResultMetrics(
      { ...base, apple_chart_rank: 0, subscribers: 1, episode_count: 1, latest_episode_date: "2021-07-10T01:37:00Z" },
      now,
    );
    assert.deepEqual(
      metrics.map((m) => m.label),
      ["1 gpodder subscriber", "1 episode", "Latest episode Jul 2021"],
    );
    assert.deepEqual(searchResultMetrics({ ...base, latest_episode_date: "not a date" }, now), []);
  });

  it("labels every search source", () => {
    assert.deepEqual(Object.keys(SEARCH_SOURCE_LABELS).sort(), ["gpodder", "itunes", "podcastindex"]);
  });
});
