package db

import (
	"testing"
	"time"
)

// TestParseAddColumnIfNotExists handles the corresponding operation.
func TestParseAddColumnIfNotExists(t *testing.T) {
	query := "alter table settings add column if not exists retention_keep_all boolean default true"
	table, column, ok := parseAddColumnIfNotExists(query)
	if !ok {
		t.Fatalf("expected parse to succeed")
	}
	if table != "settings" || column != "retention_keep_all" {
		t.Fatalf("unexpected parse result table=%q column=%q", table, column)
	}

	if _, _, ok := parseAddColumnIfNotExists("update settings set x=1"); ok {
		t.Fatalf("expected parse to fail for non-alter query")
	}
}

// TestExecuteAndSaveMigrationRunsOnce handles the corresponding operation.
func TestExecuteAndSaveMigrationRunsOnce(t *testing.T) {
	setupDBForTest(t)

	name := "test_migration_" + time.Now().Format("150405")
	query := "create table if not exists migration_test_table (id text primary key)"
	if err := ExecuteAndSaveMigration(name, query); err != nil {
		t.Fatalf("ExecuteAndSaveMigration first run failed: %v", err)
	}
	if err := ExecuteAndSaveMigration(name, query); err != nil {
		t.Fatalf("ExecuteAndSaveMigration second run failed: %v", err)
	}

	var migrations []Migration
	if err := DB.Where("name=?", name).Find(&migrations).Error; err != nil {
		t.Fatalf("failed to query migrations: %v", err)
	}
	if len(migrations) != 1 {
		t.Fatalf("expected exactly one migration row, got %d", len(migrations))
	}
}

// TestExecuteMigrationQuerySQLiteSkipsExistingColumn handles the corresponding operation.
func TestExecuteMigrationQuerySQLiteSkipsExistingColumn(t *testing.T) {
	setupDBForTest(t)

	err := executeMigrationQuery(
		"test_skip_existing_column",
		"alter table settings add column if not exists retention_keep_all boolean default true",
	)
	if err != nil {
		t.Fatalf("expected existing-column migration to be skipped without error, got %v", err)
	}
}

// TestPodcastItemCanonicalTranscriptColumnsExistAfterMigrate handles the corresponding operation.
func TestPodcastItemCanonicalTranscriptColumnsExistAfterMigrate(t *testing.T) {
	setupDBForTest(t)

	if !DB.Migrator().HasColumn(&PodcastItem{}, "canonical_transcript") {
		t.Fatalf("expected canonical_transcript column to exist after migrate")
	}
	if !DB.Migrator().HasColumn(&PodcastItem{}, "canonical_transcript_version") {
		t.Fatalf("expected canonical_transcript_version column to exist after migrate")
	}
	if !DB.Migrator().HasColumn(&PodcastItem{}, "canonical_updated_at") {
		t.Fatalf("expected canonical_updated_at column to exist after migrate")
	}
}

func TestInitialDownloadPolicyColumnsExistAfterMigrate(t *testing.T) {
	setupDBForTest(t)

	if !DB.Migrator().HasColumn(&Setting{}, "initial_download_mode") {
		t.Fatalf("expected initial_download_mode column to exist after migrate")
	}
	if !DB.Migrator().HasColumn(&Setting{}, "initial_download_months") {
		t.Fatalf("expected initial_download_months column to exist after migrate")
	}
}

func TestObsidianColumnsExistAfterMigrate(t *testing.T) {
	setupDBForTest(t)

	if !DB.Migrator().HasColumn(&Setting{}, "obsidian_vault") {
		t.Fatalf("expected obsidian_vault column to exist after migrate")
	}
	if !DB.Migrator().HasColumn(&Setting{}, "obsidian_folder") {
		t.Fatalf("expected obsidian_folder column to exist after migrate")
	}

	setting := GetOrCreateSetting()
	if setting.ObsidianVault != DefaultObsidianVault {
		t.Fatalf("expected default obsidian vault %q, got %q", DefaultObsidianVault, setting.ObsidianVault)
	}
	if setting.ObsidianFolder != DefaultObsidianFolder {
		t.Fatalf("expected default obsidian folder %q, got %q", DefaultObsidianFolder, setting.ObsidianFolder)
	}
}
