package db

import (
	"path/filepath"
	"strings"
	"testing"
)

func openForJournalModeTest(t *testing.T, dir string) string {
	t.Helper()
	t.Setenv("CONFIG", dir)
	t.Setenv("DB_DRIVER", "")
	t.Setenv("DATABASE_DRIVER", "")
	t.Setenv("DATABASE_URL", filepath.Join(dir, "briefcast.db"))
	conn, err := Init()
	if err != nil {
		if strings.Contains(err.Error(), "cgo") {
			t.Skip("journal mode is applied in Init; needs a driver Init can open")
		}
		t.Fatal(err)
	}
	sqlDB, err := conn.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	var mode string
	if err := sqlDB.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	return strings.ToLower(mode)
}

func TestSQLiteJournalModeIsOptIn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(sqliteJournalModeEnv, "")
	if got := openForJournalModeTest(t, dir); got != "delete" {
		t.Fatalf("default journal mode changed: %q", got)
	}
	t.Setenv(sqliteJournalModeEnv, "WAL")
	if got := openForJournalModeTest(t, dir); got != "wal" {
		t.Fatalf("opt-in WAL not applied: %q", got)
	}
	// Unset leaves the stored mode alone.
	t.Setenv(sqliteJournalModeEnv, "")
	if got := openForJournalModeTest(t, dir); got != "wal" {
		t.Fatalf("unset must not change the stored mode: %q", got)
	}
	// Switching back is one restart with SQLITE_JOURNAL_MODE=delete.
	t.Setenv(sqliteJournalModeEnv, "delete")
	if got := openForJournalModeTest(t, dir); got != "delete" {
		t.Fatalf("switching back to delete failed: %q", got)
	}
}

func TestConfiguredSQLiteJournalModeRejectsOtherValues(t *testing.T) {
	for raw, want := range map[string]string{"": "", "wal": "wal", " Delete ": "delete", "truncate": "", "wal; drop table x": ""} {
		t.Setenv(sqliteJournalModeEnv, raw)
		if got := configuredSQLiteJournalMode(); got != want {
			t.Fatalf("%q: got %q, want %q", raw, got, want)
		}
	}
}
