package service

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
)

func reloadDownloadStatus(t *testing.T, id string) db.DownloadStatus {
	t.Helper()
	var item db.PodcastItem
	if err := db.GetPodcastItemByID(id, &item); err != nil {
		t.Fatalf("reload item %s: %v", id, err)
	}
	return item.DownloadStatus
}

func seedDownloadedItems(t *testing.T, podcast db.Podcast, dataDir string, count int, prefix string) []db.PodcastItem {
	t.Helper()
	items := make([]db.PodcastItem, 0, count)
	for i := 0; i < count; i++ {
		items = append(items, createDownloadedItem(t, podcast, fmt.Sprintf("%s-%02d", prefix, i), time.Now().Add(-time.Duration(i+1)*time.Hour), false, dataDir))
	}
	return items
}

func TestCheckMissingFilesRequeuesOnlyAbsentFiles(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	dataDir := filepath.Join(tempDir, "assets")
	podcast := createPodcast(t, "missing-files-basic", false)
	present := createDownloadedItem(t, podcast, "present", time.Now().Add(-2*time.Hour), false, dataDir)
	absent := createDownloadedItem(t, podcast, "absent", time.Now().Add(-time.Hour), false, dataDir)
	if err := os.Remove(absent.DownloadPath); err != nil {
		t.Fatalf("remove seeded file: %v", err)
	}

	if err := CheckMissingFiles(); err != nil {
		t.Fatalf("CheckMissingFiles: %v", err)
	}
	if got := reloadDownloadStatus(t, present.ID); got != db.Downloaded {
		t.Fatalf("present file changed state to %v", got)
	}
	if got := reloadDownloadStatus(t, absent.ID); got != db.NotDownloaded {
		t.Fatalf("absent file expected NotDownloaded, got %v", got)
	}
}

func TestCheckMissingFilesSkipsPassWhenDataDirIsUnavailable(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	dataDir := filepath.Join(tempDir, "assets")
	podcast := createPodcast(t, "missing-files-root", false)
	items := seedDownloadedItems(t, podcast, dataDir, 3, "root")

	// The volume "disappears": DATA points at a directory that does not exist.
	t.Setenv("DATA", filepath.Join(tempDir, "unmounted"))
	err := CheckMissingFiles()
	if err == nil || !strings.Contains(err.Error(), "data directory unavailable") {
		t.Fatalf("expected data directory error, got %v", err)
	}
	for _, item := range items {
		if got := reloadDownloadStatus(t, item.ID); got != db.Downloaded {
			t.Fatalf("item %s changed to %v although the pass was skipped", item.ID, got)
		}
	}
}

func TestCheckMissingFilesLeavesStateWhenStatFailsForOtherReasons(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	dataDir := filepath.Join(tempDir, "assets")
	podcast := createPodcast(t, "missing-files-staterr", false)
	faulty := createDownloadedItem(t, podcast, "faulty", time.Now().Add(-2*time.Hour), false, dataDir)
	absent := createDownloadedItem(t, podcast, "gone", time.Now().Add(-time.Hour), false, dataDir)
	if err := os.Remove(absent.DownloadPath); err != nil {
		t.Fatalf("remove seeded file: %v", err)
	}

	original := missingFileStat
	t.Cleanup(func() { missingFileStat = original })
	missingFileStat = func(name string) (os.FileInfo, error) {
		if name == faulty.DownloadPath {
			return nil, &fs.PathError{Op: "stat", Path: name, Err: errors.New("input/output error")}
		}
		return original(name)
	}

	if err := CheckMissingFiles(); err != nil {
		t.Fatalf("CheckMissingFiles: %v", err)
	}
	if got := reloadDownloadStatus(t, faulty.ID); got != db.Downloaded {
		t.Fatalf("stat error must not change state, got %v", got)
	}
	if got := reloadDownloadStatus(t, absent.ID); got != db.NotDownloaded {
		t.Fatalf("absent file expected NotDownloaded, got %v", got)
	}
}

func TestCheckMissingFilesBreakerStopsImplausibleMassReset(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	dataDir := filepath.Join(tempDir, "assets")
	podcast := createPodcast(t, "missing-files-breaker", false)
	items := seedDownloadedItems(t, podcast, dataDir, missingFilesBreakerMinItems, "breaker")
	// Just over half the library goes missing.
	for _, item := range items[:missingFilesBreakerMinItems/2+1] {
		if err := os.Remove(item.DownloadPath); err != nil {
			t.Fatalf("remove seeded file: %v", err)
		}
	}

	err := CheckMissingFiles()
	if err == nil || !strings.Contains(err.Error(), "aborted") {
		t.Fatalf("expected breaker error, got %v", err)
	}
	for _, item := range items {
		if got := reloadDownloadStatus(t, item.ID); got != db.Downloaded {
			t.Fatalf("breaker must not change any row; %s is %v", item.ID, got)
		}
	}

	// An operator who removed the files on purpose can raise the limit.
	t.Setenv(missingFilesMaxPercentEnv, "100")
	if err := CheckMissingFiles(); err != nil {
		t.Fatalf("CheckMissingFiles with raised limit: %v", err)
	}
	for i, item := range items {
		want := db.Downloaded
		if i <= missingFilesBreakerMinItems/2 {
			want = db.NotDownloaded
		}
		if got := reloadDownloadStatus(t, item.ID); got != want {
			t.Fatalf("item %d: want %v, got %v", i, want, got)
		}
	}
}

func TestCheckMissingFilesBreakerIgnoresSmallLibraries(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	dataDir := filepath.Join(tempDir, "assets")
	podcast := createPodcast(t, "missing-files-small", false)
	items := seedDownloadedItems(t, podcast, dataDir, 4, "small")
	for _, item := range items[:3] {
		if err := os.Remove(item.DownloadPath); err != nil {
			t.Fatalf("remove seeded file: %v", err)
		}
	}
	if err := CheckMissingFiles(); err != nil {
		t.Fatalf("CheckMissingFiles: %v", err)
	}
	if got := reloadDownloadStatus(t, items[0].ID); got != db.NotDownloaded {
		t.Fatalf("small library should still be updated, got %v", got)
	}
}

func TestMissingFilesMaxPercentParsing(t *testing.T) {
	cases := map[string]int{"": 50, "75": 75, "100": 100, "0": 50, "101": 50, "abc": 50}
	for raw, want := range cases {
		t.Setenv(missingFilesMaxPercentEnv, raw)
		if got := missingFilesMaxPercent(); got != want {
			t.Fatalf("%q: want %d, got %d", raw, want, got)
		}
	}
}
