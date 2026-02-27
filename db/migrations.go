package db

import (
	"errors"
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
}

var addColumnIfNotExistsRe = regexp.MustCompile(`(?i)alter\s+table\s+(\S+)\s+add\s+column\s+if\s+not\s+exists\s+(\S+)`)

// RunMigrations handles the corresponding operation.
func RunMigrations() {
	for _, mig := range migrations {
		if err := ExecuteAndSaveMigration(mig.Name, mig.Query); err != nil {
			logging.Sugar().Errorw("database migration failed", "name", mig.Name, "error", err)
		}
	}
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
