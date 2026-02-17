package db

import (
	"testing"
	"time"
)

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
