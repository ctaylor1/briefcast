package service

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
)

func holdJobLock(t *testing.T, name string) *db.JobLock {
	t.Helper()

	jobLock, acquired, err := db.TryLock(name, 120)
	if err != nil {
		t.Fatalf("acquire %s test lock: %v", name, err)
	}
	if !acquired || jobLock == nil || jobLock.ID == "" {
		t.Fatalf("expected to acquire %s test lock", name)
	}
	t.Cleanup(func() {
		db.UnlockByID(jobLock.ID)
	})
	return jobLock
}

func TestRefreshEpisodesSkipsWorkWhenLockIsHeld(t *testing.T) {
	setupRetentionTestDB(t)

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	lastEpisode := time.Now().UTC()
	podcast := db.Podcast{
		Title:       "refresh-lock-held",
		URL:         server.URL + "/feed",
		LastEpisode: &lastEpisode,
	}
	if err := db.CreatePodcast(&podcast); err != nil {
		t.Fatalf("create podcast: %v", err)
	}

	holdJobLock(t, "RefreshEpisodes")
	if err := RefreshEpisodes(); err != nil {
		t.Fatalf("expected held refresh lock to skip without error, got %v", err)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("expected held refresh lock to prevent feed requests, got %d", got)
	}
}

func TestDownloadMissingEpisodesSkipsWorkWhenLockIsHeld(t *testing.T) {
	setupRetentionTestDB(t)
	ResumeDownloads()
	t.Cleanup(ResumeDownloads)

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("audio"))
	}))
	t.Cleanup(server.Close)

	podcast := createPodcast(t, "download-lock-held", false)
	item := createServicePodcastItem(t, podcast, "queued episode", db.NotDownloaded)
	item.FileURL = server.URL + "/episode.mp3"
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("update queued item URL: %v", err)
	}

	holdJobLock(t, "DownloadMissingEpisodes")
	if err := DownloadMissingEpisodes(); err != nil {
		t.Fatalf("expected held download lock to skip without error, got %v", err)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("expected held download lock to prevent HTTP requests, got %d", got)
	}

	var refreshed db.PodcastItem
	if err := db.GetPodcastItemByID(item.ID, &refreshed); err != nil {
		t.Fatalf("reload queued item: %v", err)
	}
	if refreshed.DownloadStatus != db.NotDownloaded {
		t.Fatalf("expected queued item status to remain unchanged, got %v", refreshed.DownloadStatus)
	}
}

func TestApplyRetentionPoliciesSkipsWorkWhenLockIsHeld(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	dataDir := filepath.Join(tempDir, "assets")

	podcast := createPodcast(t, "retention-lock-held", false)
	item := createDownloadedItem(t, podcast, "old episode", time.Now().UTC().Add(-48*time.Hour), true, dataDir)

	setting := db.GetOrCreateSetting()
	setting.RetentionKeepAll = false
	setting.RetentionKeepLatest = 0
	setting.RetentionDeleteAfterDays = 1
	setting.RetentionDeleteOnlyPlayed = false
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatalf("configure retention: %v", err)
	}

	holdJobLock(t, "RetentionCleanup")
	if err := ApplyRetentionPolicies(); err != nil {
		t.Fatalf("expected held retention lock to skip without error, got %v", err)
	}

	var refreshed db.PodcastItem
	if err := db.GetPodcastItemByID(item.ID, &refreshed); err != nil {
		t.Fatalf("reload retained item: %v", err)
	}
	if refreshed.DownloadStatus != db.Downloaded {
		t.Fatalf("expected held retention lock to preserve download status, got %v", refreshed.DownloadStatus)
	}
	if _, err := os.Stat(item.DownloadPath); err != nil {
		t.Fatalf("expected held retention lock to preserve episode file: %v", err)
	}
}

func TestRefreshEpisodesIsSingleFlightWhileFeedRequestIsActive(t *testing.T) {
	setupRetentionTestDB(t)
	pythonPath := requireWorkingPython(t)

	PauseDownloads()
	t.Cleanup(ResumeDownloads)

	scriptPath := writeTempScript(t, t.TempDir(), "feedparser_lock_stub.py", `#!/usr/bin/env python3
import json
import sys

sys.stdin.read()
json.dump({"feed": {"title": "Lock Test"}, "entries": []}, sys.stdout)
`)
	t.Setenv(feedparserPythonEnv, pythonPath)
	t.Setenv(feedparserScriptEnv, scriptPath)

	firstRequest := make(chan struct{})
	secondRequest := make(chan struct{})
	releaseFirstRequest := make(chan struct{})
	var firstRequestOnce sync.Once
	var secondRequestOnce sync.Once
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestNumber := requests.Add(1)
		firstRequestOnce.Do(func() { close(firstRequest) })
		if requestNumber > 1 {
			secondRequestOnce.Do(func() { close(secondRequest) })
		}
		<-releaseFirstRequest
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>Lock Test</title></channel></rss>`))
	}))
	t.Cleanup(server.Close)

	lastEpisode := time.Now().UTC()
	podcast := db.Podcast{
		Title:       "single-flight-refresh",
		URL:         server.URL + "/feed",
		LastEpisode: &lastEpisode,
	}
	if err := db.CreatePodcast(&podcast); err != nil {
		t.Fatalf("create podcast: %v", err)
	}

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- RefreshEpisodes()
	}()

	select {
	case <-firstRequest:
	case <-time.After(5 * time.Second):
		close(releaseFirstRequest)
		t.Fatal("timed out waiting for the first refresh feed request")
	}

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- RefreshEpisodes()
	}()

	select {
	case err := <-secondDone:
		if err != nil {
			close(releaseFirstRequest)
			t.Fatalf("second refresh should skip without error: %v", err)
		}
	case <-secondRequest:
		close(releaseFirstRequest)
		<-firstDone
		<-secondDone
		t.Fatal("second refresh fetched the feed while the first refresh was active")
	case <-time.After(5 * time.Second):
		close(releaseFirstRequest)
		t.Fatal("timed out waiting for the second refresh to skip")
	}

	close(releaseFirstRequest)
	if err := <-firstDone; err != nil {
		t.Fatalf("first refresh failed: %v", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("expected exactly one feed request across overlapping refreshes, got %d", got)
	}
}
