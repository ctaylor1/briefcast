package db

import (
	"strings"
	"testing"
)

func TestRunMigrationsStopsAtTheFirstFailure(t *testing.T) {
	setupDBForTest(t)
	previous := migrations
	t.Cleanup(func() { migrations = previous })
	migrations = []localMigration{
		{Name: "2099_01_01_00_00_Broken", Query: "this is not sql"},
		{Name: "2099_01_01_00_01_DependsOnBroken", Query: "create table depends_on_broken (id integer)"},
	}

	err := RunMigrations()
	if err == nil || !strings.Contains(err.Error(), "2099_01_01_00_00_Broken") {
		t.Fatalf("expected the failing migration's error, got %v", err)
	}
	if DB.Migrator().HasTable("depends_on_broken") {
		t.Fatal("a migration after the failure ran anyway")
	}
	var recorded int64
	if err := DB.Model(&Migration{}).Where("name LIKE ?", "2099_%").Count(&recorded).Error; err != nil {
		t.Fatal(err)
	}
	if recorded != 0 {
		t.Fatalf("%d failed or skipped migrations were recorded as done", recorded)
	}
}

func TestMigrateReturnsNilOnAHealthyDatabase(t *testing.T) {
	setupDBForTest(t)
	if err := Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
}

func TestSQLiteFilePathStripsDSNDecorations(t *testing.T) {
	cases := map[string]string{
		"/config/briefcast.db":                         "/config/briefcast.db",
		"file:/config/other.db?_busy_timeout=5000":     "/config/other.db",
		"sqlite:///config/custom.db?_journal_mode=WAL": "/config/custom.db",
	}
	for url, want := range cases {
		t.Setenv("DATABASE_URL", url)
		t.Setenv("DB_DRIVER", "")
		t.Setenv("DATABASE_DRIVER", "")
		got, err := SQLiteFilePath()
		if err != nil || got != want {
			t.Fatalf("%q: got %q %v, want %q", url, got, err, want)
		}
	}
}

func TestSQLiteSnapshotRefusesOtherDrivers(t *testing.T) {
	previous := activeDriver
	t.Cleanup(func() { activeDriver = previous })
	activeDriver = DriverPostgres
	if err := SQLiteSnapshot(t.TempDir() + "/x.db"); err == nil {
		t.Fatal("expected an error on postgres")
	}
}
