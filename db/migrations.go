package db

import (
	"errors"
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
}

func RunMigrations() {
	for _, mig := range migrations {
		ExecuteAndSaveMigration(mig.Name, mig.Query)
	}
}
func ExecuteAndSaveMigration(name string, query string) error {
	var migration Migration
	result := DB.Where("name=?", name).First(&migration)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		logging.Sugar().Infow("running database migration", "name", name)
		result = DB.Debug().Exec(query)
		if result.Error == nil {
			DB.Save(&Migration{
				Date: time.Now(),
				Name: name,
			})
		}
		return result.Error
	}
	return nil
}
