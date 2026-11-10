package service

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
	glebarezsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// extractSingleEntry unpacks the only file in a backup archive to dir.
func extractSingleEntry(t *testing.T, archive, dir string) (string, string) {
	t.Helper()
	f, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	header, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "restored.db")
	w, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(w, tr); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Next(); err != io.EOF {
		t.Fatalf("expected one archive entry, got another: %v", err)
	}
	return header.Name, out
}

func TestCreateBackupProducesARestorableSnapshot(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	podcast := createPodcast(t, "backup-restore", false)

	name, err := CreateBackup()
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	// A change after the snapshot must not appear in the archive.
	createPodcast(t, "after-backup", false)

	entry, restored := extractSingleEntry(t, filepath.Join(tempDir, "backups", name), t.TempDir())
	if want := filepath.Join(tempDir, "briefcast.db"); entry != want {
		t.Fatalf("archive entry %q, want the database path %q", entry, want)
	}
	conn, err := gorm.Open(glebarezsqlite.Open(restored), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := conn.DB()
	defer sqlDB.Close()
	var check string
	if err := conn.Raw("PRAGMA integrity_check").Scan(&check).Error; err != nil || check != "ok" {
		t.Fatalf("restored database integrity: %q %v", check, err)
	}
	var titles []string
	if err := conn.Table("podcasts").Order("title").Pluck("title", &titles).Error; err != nil {
		t.Fatal(err)
	}
	if len(titles) != 1 || titles[0] != podcast.Title {
		t.Fatalf("restored podcasts %v, want only %q", titles, podcast.Title)
	}
	leftovers, _ := filepath.Glob(filepath.Join(tempDir, "backups", "*.partial"))
	if len(leftovers) != 0 {
		t.Fatalf("partial files left behind: %v", leftovers)
	}
}

func TestCreateBackupFailureLeavesNoArchive(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	fixed := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	previous := backupNow
	t.Cleanup(func() { backupNow = previous })
	backupNow = func() time.Time { return fixed }

	backups := createConfigFolderIfNotExists("backups")
	archive := filepath.Join(backups, backupArchivePrefix+fixed.Format("2006.01.02_150405")+backupArchiveSuffix)
	// VACUUM INTO refuses to overwrite a non-empty file.
	if err := os.WriteFile(archive+".db.partial", []byte("occupied"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateBackup(); err == nil {
		t.Fatal("expected the snapshot to fail")
	}
	if _, err := os.Stat(archive); !os.IsNotExist(err) {
		t.Fatalf("a failed backup must not leave an archive: %v", err)
	}
	_ = tempDir
}

func TestBackupListingAndPruningIgnoreOtherFiles(t *testing.T) {
	setupRetentionTestDB(t)
	backups := createConfigFolderIfNotExists("backups")
	operatorFile := filepath.Join(backups, "keep-me.txt")
	if err := os.WriteFile(operatorFile, []byte("operator notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		name := filepath.Join(backups, backupArchivePrefix+"2026.01.0"+string(rune('1'+i))+"_120000"+backupArchiveSuffix)
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	deleteOldBackup()
	files, err := GetAllBackupFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 5 {
		t.Fatalf("expected 5 archives after pruning, got %d", len(files))
	}
	if _, err := os.Stat(operatorFile); err != nil {
		t.Fatalf("pruning removed a file that is not a backup archive: %v", err)
	}
	_ = db.DB
}
