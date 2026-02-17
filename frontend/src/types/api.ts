export interface PodcastTagRef {
  ID: string;
  Label: string;
}

export type SearchSource = "itunes" | "podcastindex";
export type EpisodeTriState = "nil" | "true" | "false";
export type EpisodeSorting =
  | "release_desc"
  | "release_asc"
  | "duration_desc"
  | "duration_asc";

export interface Podcast {
  ID: string;
  Title: string;
  Summary: string;
  Author: string;
  Image: string;
  URL: string;
  LastEpisode?: string | null;
  Tags: PodcastTagRef[];
  DownloadedEpisodesCount: number;
  DownloadingEpisodesCount: number;
  AllEpisodesCount: number;
  DownloadedEpisodesSize: number;
  DownloadingEpisodesSize: number;
  AllEpisodesSize: number;
  IsPaused: boolean;
  RetentionKeepAll: boolean;
  AutoSkipSponsorChapters: boolean;
}

export interface PodcastItemPodcast {
  ID: string;
  Title: string;
  AutoSkipSponsorChapters?: boolean;
}

export interface PodcastItem {
  ID: string;
  PodcastID: string;
  Podcast: PodcastItemPodcast;
  Title: string;
  Summary: string;
  Duration: number;
  PubDate: string;
  FileURL: string;
  Image: string;
  LocalImage: string;
  DownloadPath: string;
  DownloadStatus: number;
  DownloadedBytes: number;
  DownloadTotalBytes: number;
  TranscriptStatus: string;
  HasChapters: boolean;
  HasTranscript: boolean;
  IsPlayed: boolean;
  BookmarkDate: string;
}

export interface DownloadCounts {
  queued: number;
  downloading: number;
  downloaded: number;
  paused: number;
}

export interface DownloadQueueResponse {
  paused: boolean;
  counts: DownloadCounts;
  items: PodcastItem[];
}

export interface Chapter {
  title: string;
  startSeconds: number;
  endSeconds?: number;
}

export interface ChaptersResponse {
  source: string;
  chapters: Chapter[];
}

export interface TranscriptResponse {
  status: string;
  transcript?: unknown;
}

export interface EpisodesFilter {
  page: number;
  count: number;
  nextPage: number;
  previousPage: number;
  totalCount: number;
  totalPages: number;
  isDownloaded?: EpisodeTriState | null;
  isPlayed?: EpisodeTriState | null;
  sorting?: EpisodeSorting;
  q?: string;
}

export interface EpisodesResponse {
  podcastItems: PodcastItem[];
  filter: EpisodesFilter;
}

export interface SearchResult {
  url: string;
  title: string;
  image: string;
  already_saved: boolean;
  description: string;
  categories?: string[];
}

export type LocalSearchRecordType = "podcast" | "episode" | "chapter" | "transcript";

export interface LocalSearchResult {
  type: LocalSearchRecordType;
  podcastId?: string;
  podcastTitle?: string;
  episodeId?: string;
  episodeTitle?: string;
  chapterTitle?: string;
  transcriptSnippet?: string;
  summarySnippet?: string;
  startSeconds?: number;
}

export interface RetentionSettings {
  keepAllEpisodes: boolean;
  keepLatestEpisodes: number;
  deleteAfterDays: number;
  deleteOnlyPlayed: boolean;
}
