package service

import (
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
)

func TestDownloadStateTransitions(t *testing.T) {
	setupRetentionTestDB(t)

	podcast := createPodcast(t, "download-state", false)
	item := db.PodcastItem{
		PodcastID:          podcast.ID,
		Title:              "episode",
		DownloadPath:       "/tmp/file.mp3",
		DownloadStatus:     db.Downloaded,
		DownloadDate:       time.Now().UTC(),
		DownloadedBytes:    123,
		DownloadTotalBytes: 456,
	}
	if err := db.CreatePodcastItem(&item); err != nil {
		t.Fatalf("create podcast item failed: %v", err)
	}

	if err := SetPodcastItemAsQueuedForDownload(item.ID); err != nil {
		t.Fatalf("queue for download failed: %v", err)
	}
	var queued db.PodcastItem
	if err := db.GetPodcastItemById(item.ID, &queued); err != nil {
		t.Fatalf("reload item failed: %v", err)
	}
	if queued.DownloadStatus != db.NotDownloaded {
		t.Fatalf("expected status not downloaded, got %v", queued.DownloadStatus)
	}
	if queued.DownloadedBytes != 0 || queued.DownloadTotalBytes != 0 {
		t.Fatalf("expected progress reset, got %d/%d", queued.DownloadedBytes, queued.DownloadTotalBytes)
	}

	if err := SetPodcastItemAsPaused(item.ID); err != nil {
		t.Fatalf("pause failed: %v", err)
	}
	var paused db.PodcastItem
	if err := db.GetPodcastItemById(item.ID, &paused); err != nil {
		t.Fatalf("reload item failed: %v", err)
	}
	if paused.DownloadStatus != db.Paused {
		t.Fatalf("expected paused status, got %v", paused.DownloadStatus)
	}

	if err := SetPodcastItemAsNotDownloaded(item.ID, db.Deleted); err != nil {
		t.Fatalf("set not downloaded failed: %v", err)
	}
	var cleared db.PodcastItem
	if err := db.GetPodcastItemById(item.ID, &cleared); err != nil {
		t.Fatalf("reload item failed: %v", err)
	}
	if cleared.DownloadStatus != db.Deleted {
		t.Fatalf("expected deleted status, got %v", cleared.DownloadStatus)
	}
	if cleared.DownloadPath != "" {
		t.Fatalf("expected download path cleared, got %q", cleared.DownloadPath)
	}
	if !cleared.DownloadDate.IsZero() {
		t.Fatalf("expected download date reset")
	}
	if cleared.DownloadedBytes != 0 || cleared.DownloadTotalBytes != 0 {
		t.Fatalf("expected progress cleared, got %d/%d", cleared.DownloadedBytes, cleared.DownloadTotalBytes)
	}
}
