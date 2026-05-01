package service

import (
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
)

func TestShouldQueueInitialBackCatalogEpisodeCountMode(t *testing.T) {
	setting := &db.Setting{InitialDownloadCount: 2}
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	if !shouldQueueInitialBackCatalogEpisode(setting, InitialDownloadModeCount, 1, now, now) {
		t.Fatalf("expected episode inside count window to queue")
	}
	if shouldQueueInitialBackCatalogEpisode(setting, InitialDownloadModeCount, 2, now, now) {
		t.Fatalf("expected episode outside count window not to queue")
	}
}

func TestShouldQueueInitialBackCatalogEpisodeMonthsMode(t *testing.T) {
	setting := &db.Setting{InitialDownloadMonths: 6}
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	if !shouldQueueInitialBackCatalogEpisode(setting, InitialDownloadModeMonths, 10, now.AddDate(0, -5, 0), now) {
		t.Fatalf("expected episode inside month window to queue")
	}
	if shouldQueueInitialBackCatalogEpisode(setting, InitialDownloadModeMonths, 0, now.AddDate(0, -7, 0), now) {
		t.Fatalf("expected episode outside month window not to queue")
	}
}

func TestShouldQueueInitialBackCatalogEpisodeAllMode(t *testing.T) {
	setting := &db.Setting{InitialDownloadCount: 1, InitialDownloadMonths: 1}
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	if !shouldQueueInitialBackCatalogEpisode(setting, InitialDownloadModeAll, 500, now.AddDate(-10, 0, 0), now) {
		t.Fatalf("expected all mode to queue every feed episode")
	}
}
