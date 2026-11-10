// Shared test fixtures. Not imported by app code.
import type { Podcast, PodcastItem } from "../types/api";

export function makePodcast(overrides: Partial<Podcast> = {}): Podcast {
  return {
    ID: "pod-1",
    Title: "Example Show",
    Summary: "A show about examples.",
    Author: "Example Author",
    Image: "",
    URL: "https://example.com/feed.xml",
    LastEpisode: "2026-11-01T10:00:00Z",
    Tags: [],
    DownloadedEpisodesCount: 3,
    DownloadingEpisodesCount: 0,
    AllEpisodesCount: 10,
    DownloadedEpisodesSize: 0,
    DownloadingEpisodesSize: 0,
    AllEpisodesSize: 0,
    IsPaused: false,
    RetentionKeepAll: false,
    AutoSkipSponsorChapters: false,
    BriefpointEnabled: false,
    ...overrides,
  };
}

export function makeEpisode(overrides: Partial<PodcastItem> = {}): PodcastItem {
  return {
    ID: "ep-1",
    PodcastID: "pod-1",
    Podcast: { ID: "pod-1", Title: "Example Show" },
    Title: "Episode One",
    Summary: "<p>Show notes</p>",
    Duration: 3600,
    PubDate: "2026-11-01T10:00:00Z",
    FileURL: "https://example.com/ep1.mp3",
    Image: "",
    LocalImage: "",
    ...overrides,
  } as PodcastItem;
}
