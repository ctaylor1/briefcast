export interface PodcastTagRef {
  ID: string;
  Label: string;
}

export type SearchSource = "itunes" | "podcastindex" | "gpodder";
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
  // Where the web player stopped; null until played there, 0 once finished.
  PlaybackPositionSeconds?: number | null;
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
  markdown?: string;
  markdownFileName?: string;
  markdownError?: string;
  status: string;
  isFavorited?: boolean;
  transcript?: unknown;
  canonicalTranscript?: string;
}

export interface SummaryResponse {
  markdown?: string;
  markdownFileName?: string;
  markdownError?: string;
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

export interface ShowNoteLink {
  ID: string;
  URL: string;
  Title: string;
  Domain: string;
  Position: number;
  ContextLabel: string;
  LinkTargetID: string | null;
}

export interface EpisodeLinksResponse {
  episodeId: string;
  podcastId: string;
  count: number;
  links: ShowNoteLink[] | null;
}

export type LinkKind = "profile" | "post" | "shortener" | "web";

export interface LinkTargetSummary {
  targetId: string;
  normalizedKey: string;
  canonicalUrl: string;
  kind: LinkKind;
  platform: string;
  handle?: string;
  domain: string;
  parentTargetId?: string;
}

export interface LinkTargetRank extends LinkTargetSummary {
  episodeCount: number;
  podcastCount: number;
  boilerplatePodcastCount: number;
  firstSeen?: string;
  lastSeen?: string;
}

export interface BoilerplateRule {
  minEpisodes: number;
  percent: number;
}

export interface TopLinksResponse {
  count: number;
  includeBoilerplate: boolean;
  boilerplateRule: BoilerplateRule;
  targets: LinkTargetRank[] | null;
}

export interface LinkTargetPodcastCount {
  podcastId: string;
  podcastTitle: string;
  episodeCount: number;
  podcastEpisodeCount: number;
  boilerplate: boolean;
}

export interface LinkTargetOccurrence {
  episodeId: string;
  episodeTitle: string;
  pubDate?: string;
  podcastId: string;
  podcastTitle: string;
  url: string;
  title: string;
  contextLabel: string;
}

export interface LinkTargetDetail {
  target: LinkTargetSummary;
  parent?: LinkTargetSummary;
  episodeCount: number;
  podcastCount: number;
  podcasts: LinkTargetPodcastCount[] | null;
  occurrences: LinkTargetOccurrence[] | null;
}

export interface SearchResult {
  url: string;
  title: string;
  image: string;
  already_saved: boolean;
  description: string;
  categories?: string[];
  author?: string;
  episode_count?: number;
  latest_episode_date?: string;
  subscribers?: number;
  subscribers_last_week?: number;
  apple_chart_rank?: number;
  apple_genre_rank?: number;
  apple_genre_name?: string;
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
  defaultProvider: string;
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
  /** Read-only: outbound User-Agent, configured with BRIEFCAST_USER_AGENT in .env. */
  userAgent?: string;
  userAgentSource?: string;
}

export type AppSettingsUpdate = Partial<AppSettings> & {
  briefpointAPIKey?: string;
};

export interface OpenRouterRecommendedModel {
  id: string;
  name: string;
  contextLength: number;
  maxCompletionTokens: number;
  promptPrice: string;
  completionPrice: string;
  recommendation: string;
}

export interface OpenRouterModelsResponse {
  provider: string;
  configured: boolean;
  models: OpenRouterRecommendedModel[];
  fetchedAt: string;
  stale: boolean;
}

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

export interface MonitoringWindow {
  label: string;
  hours: number;
  downloads: number;
  transcripts: number;
  transcribed_audio_seconds: number;
}

export interface MonitoringGPU {
  index: string;
  memory_used_bytes: number;
  memory_total_bytes: number;
  utilization: number;
}

export interface MonitoringWorker {
  configured: boolean;
  reachable: boolean;
  error?: string;
  healthy: boolean;
  accepting: boolean;
  running: number;
  queued: number;
  max_running: number;
  engine_profile?: string;
  device?: string;
  state_free_bytes: number;
  jobs_finished: Record<string, number>;
  avg_speed_ratio: number;
  avg_stage_seconds: Record<string, number>;
  transcribed_audio_seconds: number;
  gpus: MonitoringGPU[];
}

export interface MonitoringSinceStart {
  started_at: string;
  downloads: Record<string, number>;
  download_bytes: number;
  avg_download_seconds: number;
  transcriptions: Record<string, number>;
  transcribed_audio_seconds: number;
  avg_processing_seconds: number;
  avg_end_to_end_seconds: number;
  avg_worker_queue_seconds: number;
  avg_speed_ratio: number;
  scheduled_job_failures: Record<string, number>;
  transcript_export_failures: number;
}

export interface MonitoringSnapshot {
  generated_at: string;
  queue: {
    episodes_by_download_state: Record<string, number>;
    transcripts_by_state: Record<string, number>;
    oldest_pending_seconds: number;
  };
  windows: MonitoringWindow[];
  since_start: MonitoringSinceStart;
  worker: MonitoringWorker;
  assets_free_bytes: number | null;
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

export interface SummaryJobStatus {
  running: boolean;
  total: number;
  completed: number;
  failed: number;
  error?: string;
}

export interface SummaryJobStartResponse {
  message: string;
  status: SummaryJobStatus;
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
