package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
	glebarezsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupRetentionTestDB(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "assets")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("failed to create data dir: %v", err)
	}
	t.Setenv("CONFIG", tempDir)
	t.Setenv("DATA", dataDir)
	t.Setenv("DATABASE_URL", filepath.Join(tempDir, "briefcast.db"))

	var err error
	db.DB, err = db.Init()
	if err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") || strings.Contains(err.Error(), "requires cgo") {
			db.DB, err = gorm.Open(glebarezsqlite.Open(filepath.Join(tempDir, "briefcast.db")), &gorm.Config{})
			if err != nil {
				t.Fatalf("db init fallback failed: %v", err)
			}
		} else {
			t.Fatalf("db init failed: %v", err)
		}
	}
	sqlDB, sqlErr := db.DB.DB()
	if sqlErr == nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}
	db.Migrate()
	return tempDir
}

func createPodcast(t *testing.T, title string, keepAll bool) db.Podcast {
	t.Helper()
	podcast := db.Podcast{
		Title:            title,
		URL:              "https://example.com/" + title,
		RetentionKeepAll: keepAll,
	}
	if err := db.CreatePodcast(&podcast); err != nil {
		t.Fatalf("create podcast failed: %v", err)
	}
	return podcast
}

func createDownloadedItem(t *testing.T, podcast db.Podcast, title string, pubDate time.Time, played bool, dir string) db.PodcastItem {
	t.Helper()
	filePath := filepath.Join(dir, title+".mp3")
	if err := os.WriteFile(filePath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("failed to write audio file: %v", err)
	}
	item := db.PodcastItem{
		PodcastID:      podcast.ID,
		Title:          title,
		PubDate:        pubDate,
		DownloadDate:   pubDate,
		DownloadPath:   filePath,
		DownloadStatus: db.Downloaded,
		IsPlayed:       played,
	}
	if err := db.CreatePodcastItem(&item); err != nil {
		t.Fatalf("create podcast item failed: %v", err)
	}
	return item
}

func TestRetentionKeepAllSkipsDeletion(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	dataDir := filepath.Join(tempDir, "assets")

	podcast := createPodcast(t, "keep-all", false)
	item := createDownloadedItem(t, podcast, "episode-1", time.Now().Add(-48*time.Hour), true, dataDir)

	setting := db.GetOrCreateSetting()
	setting.RetentionKeepAll = true
	setting.RetentionKeepLatest = 1
	setting.RetentionDeleteAfterDays = 1
	setting.RetentionDeleteOnlyPlayed = false
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatalf("update settings failed: %v", err)
	}

	if err := ApplyRetentionPolicies(); err != nil {
		t.Fatalf("apply retention failed: %v", err)
	}

	var refreshed db.PodcastItem
	if err := db.GetPodcastItemById(item.ID, &refreshed); err != nil {
		t.Fatalf("reload item failed: %v", err)
	}
	if refreshed.DownloadStatus != db.Downloaded {
		t.Fatalf("expected download status to remain downloaded, got %v", refreshed.DownloadStatus)
	}
	if _, err := os.Stat(item.DownloadPath); err != nil {
		t.Fatalf("expected file to remain, got error %v", err)
	}
}

func TestRetentionKeepLatestDeletesOldest(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	dataDir := filepath.Join(tempDir, "assets")

	podcast := createPodcast(t, "keep-latest", false)
	now := time.Now()
	newest := createDownloadedItem(t, podcast, "episode-new", now, false, dataDir)
	middle := createDownloadedItem(t, podcast, "episode-mid", now.Add(-24*time.Hour), false, dataDir)
	oldest := createDownloadedItem(t, podcast, "episode-old", now.Add(-48*time.Hour), false, dataDir)

	setting := db.GetOrCreateSetting()
	setting.RetentionKeepAll = false
	setting.RetentionKeepLatest = 2
	setting.RetentionDeleteAfterDays = 0
	setting.RetentionDeleteOnlyPlayed = true
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatalf("update settings failed: %v", err)
	}

	if err := ApplyRetentionPolicies(); err != nil {
		t.Fatalf("apply retention failed: %v", err)
	}

	assertStatus := func(item db.PodcastItem, expected db.DownloadStatus) {
		t.Helper()
		var refreshed db.PodcastItem
		if err := db.GetPodcastItemById(item.ID, &refreshed); err != nil {
			t.Fatalf("reload item failed: %v", err)
		}
		if refreshed.DownloadStatus != expected {
			t.Fatalf("expected %s to be %v, got %v", item.Title, expected, refreshed.DownloadStatus)
		}
	}

	assertStatus(newest, db.Downloaded)
	assertStatus(middle, db.Downloaded)
	assertStatus(oldest, db.Deleted)

	if _, err := os.Stat(oldest.DownloadPath); !os.IsNotExist(err) {
		t.Fatalf("expected oldest file to be deleted")
	}
}

func TestRetentionDeleteAfterDaysPlayedOnly(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	dataDir := filepath.Join(tempDir, "assets")

	podcast := createPodcast(t, "delete-after", false)
	oldPlayed := createDownloadedItem(t, podcast, "episode-played", time.Now().Add(-15*24*time.Hour), true, dataDir)
	oldUnplayed := createDownloadedItem(t, podcast, "episode-unplayed", time.Now().Add(-15*24*time.Hour), false, dataDir)

	setting := db.GetOrCreateSetting()
	setting.RetentionKeepAll = false
	setting.RetentionKeepLatest = 0
	setting.RetentionDeleteAfterDays = 10
	setting.RetentionDeleteOnlyPlayed = true
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatalf("update settings failed: %v", err)
	}

	if err := ApplyRetentionPolicies(); err != nil {
		t.Fatalf("apply retention failed: %v", err)
	}

	var refreshedPlayed db.PodcastItem
	if err := db.GetPodcastItemById(oldPlayed.ID, &refreshedPlayed); err != nil {
		t.Fatalf("reload item failed: %v", err)
	}
	if refreshedPlayed.DownloadStatus != db.Deleted {
		t.Fatalf("expected played episode to be deleted")
	}

	var refreshedUnplayed db.PodcastItem
	if err := db.GetPodcastItemById(oldUnplayed.ID, &refreshedUnplayed); err != nil {
		t.Fatalf("reload item failed: %v", err)
	}
	if refreshedUnplayed.DownloadStatus != db.Downloaded {
		t.Fatalf("expected unplayed episode to remain downloaded")
	}
}

func TestRetentionPerPodcastOverride(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	dataDir := filepath.Join(tempDir, "assets")

	keepAllPodcast := createPodcast(t, "override-keep", true)
	normalPodcast := createPodcast(t, "override-normal", false)
	now := time.Now()

	keepAllItemOld := createDownloadedItem(t, keepAllPodcast, "keep-old", now.Add(-48*time.Hour), false, dataDir)
	keepAllItemNew := createDownloadedItem(t, keepAllPodcast, "keep-new", now, false, dataDir)

	normalItemOld := createDownloadedItem(t, normalPodcast, "delete-old", now.Add(-48*time.Hour), false, dataDir)
	normalItemNew := createDownloadedItem(t, normalPodcast, "keep-new", now, false, dataDir)

	setting := db.GetOrCreateSetting()
	setting.RetentionKeepAll = false
	setting.RetentionKeepLatest = 1
	setting.RetentionDeleteAfterDays = 0
	setting.RetentionDeleteOnlyPlayed = true
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatalf("update settings failed: %v", err)
	}

	if err := ApplyRetentionPolicies(); err != nil {
		t.Fatalf("apply retention failed: %v", err)
	}

	var refreshedKeepOld db.PodcastItem
	if err := db.GetPodcastItemById(keepAllItemOld.ID, &refreshedKeepOld); err != nil {
		t.Fatalf("reload item failed: %v", err)
	}
	if refreshedKeepOld.DownloadStatus != db.Downloaded {
		t.Fatalf("expected keep-all podcast to retain old episode")
	}

	var refreshedNormalOld db.PodcastItem
	if err := db.GetPodcastItemById(normalItemOld.ID, &refreshedNormalOld); err != nil {
		t.Fatalf("reload item failed: %v", err)
	}
	if refreshedNormalOld.DownloadStatus != db.Deleted {
		t.Fatalf("expected normal podcast old episode to be deleted")
	}

	var refreshedKeepNew db.PodcastItem
	if err := db.GetPodcastItemById(keepAllItemNew.ID, &refreshedKeepNew); err != nil {
		t.Fatalf("reload item failed: %v", err)
	}
	if refreshedKeepNew.DownloadStatus != db.Downloaded {
		t.Fatalf("expected keep-all podcast new episode to remain")
	}

	var refreshedNormalNew db.PodcastItem
	if err := db.GetPodcastItemById(normalItemNew.ID, &refreshedNormalNew); err != nil {
		t.Fatalf("reload item failed: %v", err)
	}
	if refreshedNormalNew.DownloadStatus != db.Downloaded {
		t.Fatalf("expected normal podcast newest episode to remain")
	}
}
