package db

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ctaylor1/briefcast/internal/logging"
	"gorm.io/gorm"
)

type localMigration struct {
	Name  string
	Query string
}

var migrations = []localMigration{
	{
		Name:  "2020_11_03_04_42_SetDefaultDownloadStatus",
		Query: "update podcast_items set download_status=2 where download_path!='' and download_status=0",
	},
	{
		Name:  "2026_02_14_01_00_AddRetentionColumnsSettingsKeepAll",
		Query: "alter table settings add column if not exists retention_keep_all boolean default true",
	},
	{
		Name:  "2026_02_14_01_01_AddRetentionColumnsSettingsKeepLatest",
		Query: "alter table settings add column if not exists retention_keep_latest integer default 0",
	},
	{
		Name:  "2026_02_14_01_02_AddRetentionColumnsSettingsDeleteAfterDays",
		Query: "alter table settings add column if not exists retention_delete_after_days integer default 0",
	},
	{
		Name:  "2026_02_14_01_03_AddRetentionColumnsSettingsDeleteOnlyPlayed",
		Query: "alter table settings add column if not exists retention_delete_only_played boolean default true",
	},
	{
		Name:  "2026_02_14_01_04_AddRetentionColumnsPodcastKeepAll",
		Query: "alter table podcasts add column if not exists retention_keep_all boolean default false",
	},
	{
		Name:  "2026_02_14_01_05_BackfillRetentionKeepAll",
		Query: "update settings set retention_keep_all = true where retention_keep_all is null",
	},
	{
		Name:  "2026_02_14_01_06_BackfillRetentionDeleteOnlyPlayed",
		Query: "update settings set retention_delete_only_played = true where retention_delete_only_played is null",
	},
	{
		Name:  "2026_02_14_01_07_BackfillPodcastRetentionKeepAll",
		Query: "update podcasts set retention_keep_all = false where retention_keep_all is null",
	},
	{
		Name:  "2026_02_17_01_00_AddAutoSkipSponsorChapters",
		Query: "alter table podcasts add column if not exists auto_skip_sponsor_chapters boolean default false",
	},
	{
		Name:  "2026_02_17_01_01_BackfillAutoSkipSponsorChapters",
		Query: "update podcasts set auto_skip_sponsor_chapters = false where auto_skip_sponsor_chapters is null",
	},
	{
		Name:  "2026_02_23_01_00_AddTranscriptRetryCount",
		Query: "alter table podcast_items add column if not exists transcript_retry_count integer default 0",
	},
	{
		Name:  "2026_02_23_01_01_AddTranscriptNextAttempt",
		Query: "alter table podcast_items add column if not exists transcript_next_attempt timestamp",
	},
	{
		Name:  "2026_02_23_01_02_AddTranscriptLastError",
		Query: "alter table podcast_items add column if not exists transcript_last_error text",
	},
	{
		Name:  "2026_02_23_01_03_BackfillTranscriptRetryCount",
		Query: "update podcast_items set transcript_retry_count = 0 where transcript_retry_count is null",
	},
	{
		Name: "2026_02_23_01_04_DedupeJobLocksByName",
		Query: `delete from job_locks
where id in (
	select id
	from (
		select
			id,
			row_number() over (
				partition by name
				order by date desc, updated_at desc, created_at desc, id desc
			) as rn
		from job_locks
	) dedupe
	where dedupe.rn > 1
)`,
	},
	{
		Name:  "2026_02_23_01_05_AddUniqueIndexJobLocksName",
		Query: "create unique index if not exists idx_job_locks_name on job_locks(name)",
	},
	{
		Name:  "2026_02_25_01_00_AddTranscriptProgressPct",
		Query: "alter table podcast_items add column if not exists transcript_progress_pct integer default 0",
	},
	{
		Name:  "2026_02_25_01_01_AddTranscriptProgressStage",
		Query: "alter table podcast_items add column if not exists transcript_progress_stage text",
	},
	{
		Name:  "2026_02_25_01_02_AddTranscriptCheckpointJSON",
		Query: "alter table podcast_items add column if not exists transcript_checkpoint_json text",
	},
	{
		Name:  "2026_02_25_01_03_BackfillTranscriptProgressPct",
		Query: "update podcast_items set transcript_progress_pct = 0 where transcript_progress_pct is null",
	},
	{
		Name:  "2026_02_27_01_00_AddCanonicalTranscript",
		Query: "alter table podcast_items add column if not exists canonical_transcript text",
	},
	{
		Name:  "2026_02_27_01_01_AddCanonicalTranscriptVersion",
		Query: "alter table podcast_items add column if not exists canonical_transcript_version integer",
	},
	{
		Name:  "2026_02_27_01_02_AddCanonicalUpdatedAt",
		Query: "alter table podcast_items add column if not exists canonical_updated_at timestamp",
	},
	// Transcription lineage.
	{
		Name:  "2026_03_05_01_00_AddTranscriptModel",
		Query: "alter table podcast_items add column if not exists transcript_model text",
	},
	// LLM summarization columns on podcast_items.
	{
		Name:  "2026_03_05_01_01_AddLLMSummary",
		Query: "alter table podcast_items add column if not exists llm_summary text",
	},
	{
		Name:  "2026_03_05_01_02_AddLLMSummaryStatus",
		Query: "alter table podcast_items add column if not exists llm_summary_status text",
	},
	{
		Name:  "2026_03_05_01_03_AddLLMSummaryError",
		Query: "alter table podcast_items add column if not exists llm_summary_error text",
	},
	{
		Name:  "2026_03_05_01_04_AddLLMSummaryDate",
		Query: "alter table podcast_items add column if not exists llm_summary_date timestamp",
	},
	{
		Name:  "2026_03_05_01_05_AddLLMSummaryModel",
		Query: "alter table podcast_items add column if not exists llm_summary_model text",
	},
	{
		Name:  "2026_03_05_01_06_AddLLMSummaryPrompt",
		Query: "alter table podcast_items add column if not exists llm_summary_prompt text",
	},
	// Summarization settings on settings table.
	{
		Name:  "2026_03_05_01_07_AddSummarizationEnabled",
		Query: "alter table settings add column if not exists summarization_enabled boolean default false",
	},
	{
		Name:  "2026_03_05_01_08_AddSummarizationPrompt",
		Query: "alter table settings add column if not exists summarization_prompt text",
	},
	{
		Name:  "2026_03_05_01_09_BackfillSummarizationEnabled",
		Query: "update settings set summarization_enabled = false where summarization_enabled is null",
	},
	{
		Name:  "2026_03_08_01_00_AddIsSummaryFavorited",
		Query: "alter table podcast_items add column if not exists is_summary_favorited boolean default false",
	},
	{
		Name:  "2026_03_08_01_01_BackfillIsSummaryFavorited",
		Query: "update podcast_items set is_summary_favorited = false where is_summary_favorited is null",
	},
	{
		Name:  "2026_04_23_01_00_AddThemeMode",
		Query: "alter table settings add column if not exists theme_mode text default 'auto'",
	},
	{
		Name:  "2026_04_23_01_01_AddTimezone",
		Query: "alter table settings add column if not exists timezone text default 'America/New_York'",
	},
	{
		Name:  "2026_04_23_01_02_AddLightStartHour",
		Query: "alter table settings add column if not exists light_start_hour integer default 6",
	},
	{
		Name:  "2026_04_23_01_03_AddDarkStartHour",
		Query: "alter table settings add column if not exists dark_start_hour integer default 20",
	},
	{
		Name:  "2026_05_01_01_00_AddInitialDownloadMode",
		Query: "alter table settings add column if not exists initial_download_mode text default 'count'",
	},
	{
		Name:  "2026_05_01_01_01_AddInitialDownloadMonths",
		Query: "alter table settings add column if not exists initial_download_months integer default 0",
	},
	{
		Name:  "2026_05_01_01_02_BackfillInitialDownloadMode",
		Query: "update settings set initial_download_mode = 'count' where initial_download_mode is null or initial_download_mode = ''",
	},
	{
		Name:  "2026_05_08_01_00_AddSummarizationModel",
		Query: "alter table settings add column if not exists summarization_model text",
	},
	{
		Name:  "2026_05_09_01_00_AddLLMConcurrency",
		Query: "alter table settings add column if not exists llm_concurrency integer default 1",
	},
	{
		Name:  "2026_05_03_01_00_AddPodcastItemsSummaryListIndex",
		Query: "create index if not exists idx_podcast_items_summary_list on podcast_items(llm_summary_status, pub_date)",
	},
	{
		Name:  "2026_05_03_01_01_AddPodcastItemsDownloadQueueIndex",
		Query: "create index if not exists idx_podcast_items_download_queue on podcast_items(download_status, download_date, created_at)",
	},
	{
		Name:  "2026_05_03_01_02_AddPodcastItemsPodcastPubDateIndex",
		Query: "create index if not exists idx_podcast_items_podcast_pub_date on podcast_items(podcast_id, pub_date)",
	},
	{
		Name:  "2026_05_24_01_00_AddAlternateFeedURLs",
		Query: "alter table podcasts add column if not exists alternate_feed_urls text",
	},
	{
		Name:  "2026_05_24_01_01_AddAlternateFileURLs",
		Query: "alter table podcast_items add column if not exists alternate_file_urls text",
	},
	{
		Name:  "2026_05_24_02_00_AddPodcastBriefpointEnabled",
		Query: "alter table podcasts add column if not exists briefpoint_enabled boolean default false",
	},
	{
		Name:  "2026_05_24_02_01_BackfillPodcastBriefpointEnabled",
		Query: "update podcasts set briefpoint_enabled = false where briefpoint_enabled is null",
	},
	{
		Name:  "2026_05_24_02_02_AddSettingsBriefpointEnabled",
		Query: "alter table settings add column if not exists briefpoint_enabled boolean default false",
	},
	{
		Name:  "2026_05_24_02_03_AddSettingsBriefpointServerURL",
		Query: "alter table settings add column if not exists briefpoint_server_url text",
	},
	{
		Name:  "2026_05_24_02_04_AddSettingsBriefpointAPIKey",
		Query: "alter table settings add column if not exists briefpoint_api_key text",
	},
	{
		Name:  "2026_06_02_01_00_AddSettingsObsidianFolder",
		Query: "alter table settings add column if not exists obsidian_folder text default 'Clippings'",
	},
	{
		Name:  "2026_06_02_01_01_BackfillSettingsObsidianFolder",
		Query: "update settings set obsidian_folder = 'Clippings' where obsidian_folder is null or trim(obsidian_folder) = ''",
	},
	{
		Name:  "2026_06_03_01_00_AddSettingsObsidianVault",
		Query: "alter table settings add column if not exists obsidian_vault text default 'Vault'",
	},
	{
		Name:  "2026_06_03_01_01_BackfillSettingsObsidianVault",
		Query: "update settings set obsidian_vault = 'Vault' where obsidian_vault is null or trim(obsidian_vault) = ''",
	},
	{
		// Episode deletes previously left show note links behind.
		Name:  "2026_11_05_01_00_DeleteOrphanedShowNoteLinks",
		Query: "delete from show_note_links where podcast_item_id not in (select id from podcast_items)",
	},
	{
		// Concurrent ingest and backfill could store the same URL twice for
		// one episode. Keep the first occurrence so the unique index below
		// can be created.
		Name: "2026_11_05_02_00_DeleteDuplicateShowNoteLinks",
		Query: "delete from show_note_links where id in (" +
			"select id from (select id, row_number() over (" +
			"partition by podcast_item_id, url order by position, created_at, id) as rn " +
			"from show_note_links) ranked where rn > 1)",
	},
	{
		// Makes link writes idempotent (ON CONFLICT target for SaveShowNoteLinks).
		Name:  "2026_11_05_02_01_AddShowNoteLinksItemURLUniqueIndex",
		Query: "create unique index if not exists idx_show_note_links_item_url on show_note_links (podcast_item_id, url)",
	},
	{
		// Covers the per-podcast, per-target episode counts used by link
		// rankings and boilerplate detection (TopLinkTargets).
		Name:  "2026_11_05_03_00_AddShowNoteLinksPodcastTargetIndex",
		Query: "create index if not exists idx_show_note_links_podcast_target_item on show_note_links (podcast_id, link_target_id, podcast_item_id)",
	},
	{
		// Without this, the once-a-minute worker queue query reads through every
		// stored transcript, because transcript_status sits after them in the row.
		Name:  "2026_11_06_01_00_AddPodcastItemsTranscriptQueueIndex",
		Query: "create index if not exists idx_podcast_items_transcript_queue on podcast_items(download_status, transcript_status)",
	},
	{
		// Same reason, for the once-a-minute receipt query. Partial, because few
		// rows have an open checkpoint. The query must keep the matching condition.
		Name:  "2026_11_06_01_01_AddPodcastItemsOpenCheckpointIndex",
		Query: "create index if not exists idx_podcast_items_open_checkpoint on podcast_items(transcript_status) where transcript_checkpoint_json <> ''",
	},
	{
		// Downloads check whether another episode already owns a file path.
		// Without an index that check would scan every row, transcripts included.
		Name:  "2026_11_09_01_00_AddPodcastItemsDownloadPathIndex",
		Query: "create index if not exists idx_podcast_items_download_path on podcast_items(download_path)",
	},
	{
		// Upgrade note: the next three migrations delete duplicate episodes
		// (same podcast and GUID) that a refresh racing "download all" could
		// create. Take a backup first; restoring it is the way back. The kept
		// row is the one with a transcript, then a download, then a summary,
		// then the oldest. Links and Markdown revisions of the removed rows go
		// first, because SQLite does not enforce the foreign keys.
		Name: "2026_11_09_02_00_DeleteLinksOfDuplicatePodcastItems",
		Query: "delete from show_note_links where podcast_item_id in (" +
			"select id from (select id, " +
			"row_number() over (partition by podcast_id, " +
			"guid order by case when transcript_json <> '' then 0 else 1 end, " +
			"case when download_status = 2 then 0 else 1 end, " +
			"case when llm_summary <> '' then 0 else 1 end, " +
			"created_at, " +
			"id) as rn from podcast_items where guid <> '') ranked where rn > 1" + ")",
	},
	{
		Name: "2026_11_09_02_01_DeleteMarkdownRevisionsOfDuplicatePodcastItems",
		Query: "delete from markdown_revisions where episode_id in (" +
			"select id from (select id, " +
			"row_number() over (partition by podcast_id, " +
			"guid order by case when transcript_json <> '' then 0 else 1 end, " +
			"case when download_status = 2 then 0 else 1 end, " +
			"case when llm_summary <> '' then 0 else 1 end, " +
			"created_at, " +
			"id) as rn from podcast_items where guid <> '') ranked where rn > 1" + ")",
	},
	{
		Name: "2026_11_09_02_02_DeleteDuplicatePodcastItems",
		Query: "delete from podcast_items where id in (" +
			"select id from (select id, " +
			"row_number() over (partition by podcast_id, " +
			"guid order by case when transcript_json <> '' then 0 else 1 end, " +
			"case when download_status = 2 then 0 else 1 end, " +
			"case when llm_summary <> '' then 0 else 1 end, " +
			"created_at, " +
			"id) as rn from podcast_items where guid <> '') ranked where rn > 1" + ")",
	},
	{
		// One row per episode GUID within a podcast. Episodes without a GUID
		// are not covered.
		Name:  "2026_11_09_02_03_AddPodcastItemsPodcastGUIDUniqueIndex",
		Query: "create unique index if not exists idx_podcast_items_podcast_guid on podcast_items (podcast_id, guid) where guid <> ''",
	},
	{
		// The work-queue snapshot (Activity page) counts and lists episodes by
		// whether they have a transcript or a summary. Testing transcript_json,
		// canonical_transcript or llm_summary for emptiness reads every stored
		// transcript: 48 s on a production copy. These partial indexes let those
		// queries answer from the index. Each query must keep its index's
		// condition word for word (see service/repair_work.go).
		Name:  "2026_11_12_01_00_AddPodcastItemsNoTranscriptWorkIndex",
		Query: "create index if not exists idx_podcast_items_no_transcript_work on podcast_items(transcript_status, download_status, download_path, transcript_next_attempt, updated_at, id) where transcript_json is null or transcript_json = ''",
	},
	{
		// Partial on "has a summary", not "has none": an index on "has none"
		// also matches the summary-missing queries, and SQLite picks it over
		// idx_podcast_items_summary_missing because it is narrower, then reads
		// every transcript to test the rest.
		Name:  "2026_11_12_01_01_AddPodcastItemsWithSummaryIndex",
		Query: "create index if not exists idx_podcast_items_with_summary on podcast_items(llm_summary_status) where llm_summary is not null and llm_summary <> ''",
	},
	{
		// Also holds the columns the queue shows for these episodes, so listing
		// them never reads the columns stored after the transcript.
		Name:  "2026_11_12_01_02_AddPodcastItemsSummaryMissingIndex",
		Query: "create index if not exists idx_podcast_items_summary_missing on podcast_items(llm_summary_status, updated_at, id, transcript_status, llm_summary_error, llm_summary_model) where canonical_transcript is not null and canonical_transcript <> '' and (llm_summary is null or llm_summary = '')",
	},
}

var addColumnIfNotExistsRe = regexp.MustCompile(`(?i)alter\s+table\s+(\S+)\s+add\s+column\s+if\s+not\s+exists\s+(\S+)`)

// RunMigrations handles the corresponding operation.
func RunMigrations() error {
	for _, mig := range migrations {
		if err := ExecuteAndSaveMigration(mig.Name, mig.Query); err != nil {
			logging.Sugar().Errorw("database migration failed; later migrations were not run", "name", mig.Name, "error", err)
			return fmt.Errorf("migration %s: %w", mig.Name, err)
		}
	}
	return nil
}

// ExecuteAndSaveMigration handles the corresponding operation.
func ExecuteAndSaveMigration(name string, query string) error {
	var migration Migration
	result := DB.Where("name=?", name).First(&migration)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		logging.Sugar().Infow("running database migration", "name", name)
		execErr := executeMigrationQuery(name, query)
		if execErr == nil {
			DB.Save(&Migration{
				Date: time.Now().UTC(),
				Name: name,
			})
		}
		return execErr
	}
	return nil
}

func executeMigrationQuery(name string, query string) error {
	if CurrentDriver() == DriverSQLite {
		if table, column, ok := parseAddColumnIfNotExists(query); ok {
			if DB.Migrator().HasColumn(table, column) {
				logging.Sugar().Infow("migration skipped; column exists", "name", name, "table", table, "column", column)
				return nil
			}
			query = strings.Replace(query, "add column if not exists", "add column", 1)
		}
	}
	result := DB.Exec(query)
	return result.Error
}

func parseAddColumnIfNotExists(query string) (string, string, bool) {
	matches := addColumnIfNotExistsRe.FindStringSubmatch(query)
	if len(matches) < 3 {
		return "", "", false
	}
	return matches[1], matches[2], true
}
