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
  BriefpointEnabled: boolean;
  AlternateFeedURLs?: string[];
}

export interface PodcastItemPodcast {
  ID: string;
  Title: string;
  AutoSkipSponsorChapters?: boolean;
}

export interface PodcastItem {
  ID: string;
  CreatedAt?: string;
  UpdatedAt?: string;
  PodcastID: string;
  Podcast: PodcastItemPodcast;
  Title: string;
  Summary: string;
  Duration: number;
  PubDate: string;
  FileURL: string;
  Image: string;
  LocalImage: string;
  DownloadDate: string;
  DownloadPath: string;
  DownloadStatus: number;
  DownloadedBytes: number;
  DownloadTotalBytes: number;
  TranscriptStatus: string;
  HasChapters: boolean;
  HasTranscript: boolean;
  HasSummary: boolean;
  LLMSummaryStatus: string;
  IsSummaryFavorited?: boolean;
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
  isFavorited?: boolean;
  transcript?: unknown;
  canonicalTranscript?: string;
}

export interface SummaryResponse {
  status: string;
  isFavorited?: boolean;
  summary?: string;
  generatedAt?: string;
  model?: string;
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
  isBookmarked?: EpisodeTriState | null;
  sorting?: EpisodeSorting;
  q?: string;
  podcastIds?: string[];
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

export type LocalSearchRecordType = "podcast" | "episode" | "chapter" | "transcript" | "summary";

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

export interface AppSettings {
  autoDownload: boolean;
  downloadOnAdd: boolean;
  initialDownloadCount: number;
  initialDownloadMode: "count" | "months" | "all";
  initialDownloadMonths: number;
  keepAllEpisodes: boolean;
  keepLatestEpisodes: number;
  deleteAfterDays: number;
  deleteOnlyPlayed: boolean;
  summarizationEnabled: boolean;
  summarizationModel: string;
  summarizationPrompt: string;
  summarizationUserPrompt: string;
  effectiveSystemPrompt: string;
  effectiveUserPrompt: string;
  llmConcurrency: number;
  defaultModel: string;
  defaultSystemPrompt: string;
  defaultUserPrompt: string;
  themeMode: string;
  timezone: string;
  lightStartHour: number;
  darkStartHour: number;
  briefpointEnabled: boolean;
  briefpointServerURL: string;
  briefpointAPIKeyConfigured: boolean;
  obsidianVault: string;
  obsidianFolder: string;
}

export type AppSettingsUpdate = Partial<AppSettings> & {
  briefpointAPIKey?: string;
};

export interface RuntimeVersionInfo {
  version: string;
  repoUrl: string;
}

export type SummarySorting =
  | "newest"
  | "oldest"
  | "title_asc"
  | "title_desc"
  | "shortest"
  | "longest";

export interface SummaryListItem {
  id: string;
  episodeTitle: string;
  podcastId: string;
  podcastTitle: string;
  podcastImage: string;
  duration: number;
  pubDate: string;
  generatedAt: string;
  model: string;
  excerpt: string;
  readTime: number;
  isPlayed: boolean;
  hasSummary: boolean;
  isFavorited: boolean;
}

export interface PromptVersion {
  ID: string;
  CreatedAt: string;
  PromptType: "system" | "user";
  Content: string;
}

export interface ResummarizeFilter {
  model?: string;
  before?: string;
  podcastId?: string;
  dryRun?: boolean;
}

export interface ResummarizeResult {
  total: number;
  succeeded?: number;
  failed?: number;
}

export interface SummariesFilter {
  page: number;
  count: number;
  totalCount: number;
  totalPages: number;
  nextPage: number;
  previousPage: number;
}

export interface SummariesResponse {
  summaries: SummaryListItem[];
  filter: SummariesFilter;
}
