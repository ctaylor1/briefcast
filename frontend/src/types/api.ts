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

export interface AppLogEntry {
  id: string;
  timestamp: string;
  level: "debug" | "info" | "warn" | "error" | "fatal";
  source: string;
  service?: string;
  caller?: string;
  message: string;
  humanMessage: string;
  category: string;
  userImpact: boolean;
  fields?: Record<string, unknown>;
  raw?: string;
}

export interface AppLogSource {
  name: string;
  updatedAt: string;
  sizeBytes: number;
}

export interface AppLogsResponse {
  entries: AppLogEntry[];
  impactEntries: AppLogEntry[];
  sources: AppLogSource[];
  readErrors?: string[];
  limit: number;
  totalDiscovered: number;
}

export interface WorkQueueSummaryCounts {
  complete: number;
  processing: number;
  failed: number;
  missing: number;
  eligibleForBackfill: number;
  blockedNoTranscript: number;
}

export interface WorkQueueTranscriptCounts {
  complete: number;
  queued: number;
  processing: number;
  failed: number;
  retryDue: number;
  retryScheduled: number;
  blocked: number;
}

export interface WorkQueueConfig {
  whisperxEnabled: boolean;
  llmEnabled: boolean;
  llmApiKeyConfigured: boolean;
  summarizationEnabled: boolean;
}

export interface WorkQueueItem {
  id: string;
  kind: "transcript" | "summary";
  status: string;
  statusLabel: string;
  category: "active" | "queued" | "failed" | "retry" | "blocked" | string;
  title: string;
  podcastTitle?: string;
  pubDate: string;
  updatedAt: string;
  progressPct?: number;
  progressStage?: string;
  retryCount?: number;
  nextAttempt?: string | null;
  lastError?: string;
  model?: string;
}

export interface WorkQueueSnapshot {
  summary: WorkQueueSummaryCounts;
  transcripts: WorkQueueTranscriptCounts;
  config: WorkQueueConfig;
  items: WorkQueueItem[];
  limit: number;
}

export interface RepairSummaryResult {
  eligible: number;
  started: boolean;
  succeeded: number;
  failed: number;
  error?: string;
}

export interface RepairTranscriptResult {
  readyNow: number;
  forcedDue: number;
  queued: number;
  workerStarted: boolean;
  workerLockHeld: boolean;
  error?: string;
}

export interface RepairWorkRun {
  startedAt: string;
  finishedAt?: string;
  summary: RepairSummaryResult;
  transcripts: RepairTranscriptResult;
  error?: string;
}

export interface RepairWorkResponse {
  running: boolean;
  startedAt?: string;
  lastRun?: RepairWorkRun;
  queue: WorkQueueSnapshot;
}

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
