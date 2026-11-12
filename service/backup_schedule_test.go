package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The scheduled backup ran "@every 48h", counted from the app's start, so the
// restarts of a normal week postponed it again and again: production went from
// 2026-11-09 to 2026-11-12 without one. It now runs when the newest archive is
// older than BackupInterval, however often the app restarted.
func TestCreateBackupIfDueBacksUpOnlyWhenTheNewestArchiveIsOld(t *testing.T) {
	setupRetentionTestDB(t)
	start := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	previous := backupNow
	t.Cleanup(func() { backupNow = previous })
	at := func(offset time.Duration) { backupNow = func() time.Time { return start.Add(offset) } }
	archives := func() int {
		files, err := GetAllBackupFiles()
		if err != nil {
			t.Fatal(err)
		}
		return len(files)
	}

	at(0)
	if name, err := CreateBackupIfDue(); err != nil || name == "" {
		t.Fatalf("expected a backup when there is none, got %q, %v", name, err)
	}
	at(BackupInterval - time.Minute)
	if name, err := CreateBackupIfDue(); err != nil || name != "" {
		t.Fatalf("expected no backup within the interval, got %q, %v", name, err)
	}
	if n := archives(); n != 1 {
		t.Fatalf("expected 1 archive, got %d", n)
	}
	at(BackupInterval + time.Minute)
	if name, err := CreateBackupIfDue(); err != nil || name == "" {
		t.Fatalf("expected a backup once the newest is older than the interval, got %q, %v", name, err)
	}
	if n := archives(); n != 2 {
		t.Fatalf("expected 2 archives, got %d", n)
	}
}

func TestNewestBackupTimeReadsArchiveNamesOnly(t *testing.T) {
	setupRetentionTestDB(t)
	backups := createConfigFolderIfNotExists("backups")
	older := time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 11, 9, 17, 23, 9, 0, time.UTC)
	for _, name := range []string{
		backupArchivePrefix + older.Format(backupArchiveTimeLayout) + backupArchiveSuffix,
		backupArchivePrefix + newer.Format(backupArchiveTimeLayout) + backupArchiveSuffix,
		backupArchivePrefix + "not-a-time" + backupArchiveSuffix,
		"keep-me.txt",
	} {
		if err := os.WriteFile(filepath.Join(backups, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := newestBackupTime()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(newer) {
		t.Fatalf("newest backup time %v, want %v", got, newer)
	}
}
