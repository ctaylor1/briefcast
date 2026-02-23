package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/model"
)

func TestTranscribePendingEpisodesWorkerBranches(t *testing.T) {
	setupRetentionTestDB(t)
	pythonPath := requireWorkingPython(t)

	tempDir := t.TempDir()
	scriptPath := writeTempScript(t, tempDir, "whisperx_branch_stub.py", `#!/usr/bin/env python3
import json
import os
import sys

name = os.path.basename(sys.argv[1])
if "fail" in name:
    print("forced failure", file=sys.stderr)
    sys.exit(2)
json.dump({"segments":[{"start":0.0,"end":1.0,"text":"ok"}]}, sys.stdout)
`)

	successAudio := filepath.Join(tempDir, "success.mp3")
	if err := os.WriteFile(successAudio, []byte("audio"), 0o644); err != nil {
		t.Fatalf("failed to create success audio: %v", err)
	}
	failAudio := filepath.Join(tempDir, "fail.mp3")
	if err := os.WriteFile(failAudio, []byte("audio"), 0o644); err != nil {
		t.Fatalf("failed to create fail audio: %v", err)
	}
	missingAudio := filepath.Join(tempDir, "missing.mp3")

	podcast := createPodcast(t, "whisperx-branches", false)

	successItem := createServicePodcastItem(t, podcast, "whisperx-success", db.Downloaded)
	successItem.DownloadPath = successAudio
	successItem.TranscriptStatus = "pending_whisperx"
	successItem.TranscriptJSON = ""
	if err := db.UpdatePodcastItem(&successItem); err != nil {
		t.Fatalf("failed to update success item: %v", err)
	}

	missingItem := createServicePodcastItem(t, podcast, "whisperx-missing", db.Downloaded)
	missingItem.DownloadPath = missingAudio
	missingItem.TranscriptStatus = "pending_whisperx"
	missingItem.TranscriptJSON = ""
	if err := db.UpdatePodcastItem(&missingItem); err != nil {
		t.Fatalf("failed to update missing item: %v", err)
	}

	failedItem := createServicePodcastItem(t, podcast, "whisperx-failed-retry", db.Downloaded)
	failedItem.DownloadPath = failAudio
	failedItem.TranscriptStatus = "failed"
	failedItem.TranscriptJSON = ""
	if err := db.UpdatePodcastItem(&failedItem); err != nil {
		t.Fatalf("failed to update failed item: %v", err)
	}

	t.Setenv(whisperxEnabledEnv, "true")
	t.Setenv(whisperxPythonEnv, pythonPath)
	t.Setenv(whisperxScriptEnv, scriptPath)
	t.Setenv("WHISPERX_RETRY_FAILED", "true")
	t.Setenv("WHISPERX_DIARIZATION", "true")
	t.Setenv("WHISPERX_MAX_CONCURRENCY", "3")
	t.Setenv("WHISPERX_THIRD_PARTY_LOG_LEVEL", "error")

	if err := TranscribePendingEpisodes(); err == nil {
		t.Fatalf("expected transcription run to return first processing error")
	}

	var refreshedSuccess db.PodcastItem
	if err := db.GetPodcastItemById(successItem.ID, &refreshedSuccess); err != nil {
		t.Fatalf("failed to reload success item: %v", err)
	}
	if refreshedSuccess.TranscriptStatus != "available" {
		t.Fatalf("expected success item transcript status available, got %q", refreshedSuccess.TranscriptStatus)
	}
	if !strings.Contains(refreshedSuccess.TranscriptJSON, "\"segments\"") {
		t.Fatalf("expected success item transcript JSON to contain segments, got %s", refreshedSuccess.TranscriptJSON)
	}
	if refreshedSuccess.TranscriptRetryCount != 0 || refreshedSuccess.TranscriptNextAttempt != nil {
		t.Fatalf(
			"expected success item retry metadata to be cleared, got retry_count=%d next_attempt=%v",
			refreshedSuccess.TranscriptRetryCount,
			refreshedSuccess.TranscriptNextAttempt,
		)
	}

	var refreshedMissing db.PodcastItem
	if err := db.GetPodcastItemById(missingItem.ID, &refreshedMissing); err != nil {
		t.Fatalf("failed to reload missing item: %v", err)
	}
	if refreshedMissing.TranscriptStatus != "failed" {
		t.Fatalf("expected missing item transcript status failed, got %q", refreshedMissing.TranscriptStatus)
	}
	if refreshedMissing.TranscriptRetryCount == 0 {
		t.Fatalf("expected missing item retry count to increment")
	}
	if refreshedMissing.TranscriptNextAttempt == nil {
		t.Fatalf("expected missing item next retry timestamp to be scheduled")
	}

	var refreshedFailed db.PodcastItem
	if err := db.GetPodcastItemById(failedItem.ID, &refreshedFailed); err != nil {
		t.Fatalf("failed to reload failed item: %v", err)
	}
	if refreshedFailed.TranscriptStatus != "failed" {
		t.Fatalf("expected failed retry item transcript status failed, got %q", refreshedFailed.TranscriptStatus)
	}
	if refreshedFailed.TranscriptRetryCount == 0 {
		t.Fatalf("expected failed retry item retry count to increment")
	}
	if refreshedFailed.TranscriptNextAttempt == nil {
		t.Fatalf("expected failed retry item next retry timestamp to be scheduled")
	}
}

func TestTranscribePendingEpisodesDisabledLockAndScriptErrors(t *testing.T) {
	setupRetentionTestDB(t)

	t.Setenv(whisperxEnabledEnv, "false")
	if err := TranscribePendingEpisodes(); err != nil {
		t.Fatalf("expected disabled whisperx run to no-op, got %v", err)
	}

	t.Setenv(whisperxEnabledEnv, "true")
	t.Setenv(whisperxPythonEnv, "python")
	t.Setenv(whisperxScriptEnv, filepath.Join(t.TempDir(), "missing_script.py"))
	if err := TranscribePendingEpisodes(); err == nil {
		t.Fatalf("expected missing script resolution failure")
	}

	pythonPath := requireWorkingPython(t)
	tempDir := t.TempDir()
	scriptPath := writeTempScript(t, tempDir, "whisperx_ok.py", `#!/usr/bin/env python3
import json
import sys
json.dump({"segments":[]}, sys.stdout)
`)

	podcast := createPodcast(t, "whisperx-lock", false)
	audioPath := filepath.Join(tempDir, "lock-audio.mp3")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("failed to create lock test audio: %v", err)
	}
	item := createServicePodcastItem(t, podcast, "whisperx-lock-item", db.Downloaded)
	item.DownloadPath = audioPath
	item.TranscriptStatus = "pending_whisperx"
	item.TranscriptJSON = ""
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("failed to update lock test item: %v", err)
	}

	t.Setenv(whisperxEnabledEnv, "true")
	t.Setenv(whisperxPythonEnv, pythonPath)
	t.Setenv(whisperxScriptEnv, scriptPath)

	db.Lock("TranscribePendingEpisodes", 120)
	defer db.Unlock("TranscribePendingEpisodes")
	if err := TranscribePendingEpisodes(); err != nil {
		t.Fatalf("expected lock-held whisperx run to skip without error, got %v", err)
	}

	var refreshed db.PodcastItem
	if err := db.GetPodcastItemById(item.ID, &refreshed); err != nil {
		t.Fatalf("failed to reload lock test item: %v", err)
	}
	if refreshed.TranscriptStatus != "pending_whisperx" {
		t.Fatalf("expected lock-held run to leave transcript status unchanged, got %q", refreshed.TranscriptStatus)
	}
}

func TestSearchProviderErrorBranches(t *testing.T) {
	origGpodder := gpodderBaseURL
	origItunes := itunesBaseURL
	t.Cleanup(func() {
		gpodderBaseURL = origGpodder
		itunesBaseURL = origItunes
	})

	invalidJSONServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer invalidJSONServer.Close()

	gpodderBaseURL = invalidJSONServer.URL
	itunesBaseURL = invalidJSONServer.URL

	if got := Query("briefcast"); len(got) != 0 {
		t.Fatalf("expected empty gpodder query results for invalid json, got %+v", got)
	}
	if got := ByTag("tech", 5); len(got) != 0 {
		t.Fatalf("expected empty gpodder by-tag results for invalid json, got %+v", got)
	}
	if got := Top(5); len(got) != 0 {
		t.Fatalf("expected empty gpodder top results for invalid json, got %+v", got)
	}
	if got := Tags(5); len(got) != 0 {
		t.Fatalf("expected empty gpodder tags results for invalid json, got %+v", got)
	}
	if got := (ItunesService{}).Query("briefcast"); len(got) != 0 {
		t.Fatalf("expected empty itunes query results for invalid json, got %+v", got)
	}

	gpodderBaseURL = "http://127.0.0.1:1"
	itunesBaseURL = "http://127.0.0.1:1"
	if got := Query("briefcast"); len(got) != 0 {
		t.Fatalf("expected empty gpodder query results for request error, got %+v", got)
	}
	if got := (ItunesService{}).Query("briefcast"); len(got) != 0 {
		t.Fatalf("expected empty itunes query results for request error, got %+v", got)
	}
}

func TestDeletePodcastWithoutDeletingExternalFiles(t *testing.T) {
	setupRetentionTestDB(t)
	podcast := createPodcast(t, "delete-podcast-no-files", false)
	item := createServicePodcastItem(t, podcast, "delete-no-files-item", db.Downloaded)

	externalDir := t.TempDir()
	audioPath := filepath.Join(externalDir, "external-audio.mp3")
	imagePath := filepath.Join(externalDir, "external-image.jpg")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("failed to create external audio file: %v", err)
	}
	if err := os.WriteFile(imagePath, []byte("image"), 0o644); err != nil {
		t.Fatalf("failed to create external image file: %v", err)
	}

	item.DownloadPath = audioPath
	item.LocalImage = imagePath
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("failed to persist external file paths: %v", err)
	}

	if err := DeletePodcast(podcast.ID, false); err != nil {
		t.Fatalf("delete podcast without deleting files failed: %v", err)
	}

	if _, err := os.Stat(audioPath); err != nil {
		t.Fatalf("expected external audio to remain after deleteFiles=false, got %v", err)
	}
	if _, err := os.Stat(imagePath); err != nil {
		t.Fatalf("expected external image to remain after deleteFiles=false, got %v", err)
	}

	var removedPodcast db.Podcast
	if err := db.GetPodcastById(podcast.ID, &removedPodcast); err == nil {
		t.Fatalf("expected podcast to be deleted")
	}
}

func TestSetPodcastItemAsDownloadedTranscriptDefaults(t *testing.T) {
	setupRetentionTestDB(t)
	podcast := createPodcast(t, "set-downloaded-state", false)

	audioPath := filepath.Join(t.TempDir(), "episode.mp3")
	if err := os.WriteFile(audioPath, []byte("audio-bytes"), 0o644); err != nil {
		t.Fatalf("failed to create episode audio file: %v", err)
	}

	pendingItem := createServicePodcastItem(t, podcast, "pending-transcript-default", db.NotDownloaded)
	pendingItem.TranscriptStatus = ""
	pendingItem.TranscriptJSON = ""
	if err := db.UpdatePodcastItem(&pendingItem); err != nil {
		t.Fatalf("failed to persist pending item: %v", err)
	}
	if err := SetPodcastItemAsDownloaded(pendingItem.ID, audioPath); err != nil {
		t.Fatalf("set podcast item downloaded failed: %v", err)
	}

	var refreshedPending db.PodcastItem
	if err := db.GetPodcastItemById(pendingItem.ID, &refreshedPending); err != nil {
		t.Fatalf("failed to reload pending item: %v", err)
	}
	if refreshedPending.DownloadStatus != db.Downloaded {
		t.Fatalf("expected pending item to be downloaded, got %v", refreshedPending.DownloadStatus)
	}
	if refreshedPending.TranscriptStatus != "pending_whisperx" {
		t.Fatalf("expected pending whisperx default transcript status, got %q", refreshedPending.TranscriptStatus)
	}
	if refreshedPending.DownloadedBytes == 0 || refreshedPending.DownloadTotalBytes == 0 {
		t.Fatalf("expected downloaded byte counters to be populated, got downloaded=%d total=%d", refreshedPending.DownloadedBytes, refreshedPending.DownloadTotalBytes)
	}

	availableItem := createServicePodcastItem(t, podcast, "already-has-transcript", db.NotDownloaded)
	availableItem.TranscriptStatus = "available"
	availableItem.TranscriptJSON = `{"segments":[]}`
	if err := db.UpdatePodcastItem(&availableItem); err != nil {
		t.Fatalf("failed to persist available item: %v", err)
	}
	if err := SetPodcastItemAsDownloaded(availableItem.ID, audioPath); err != nil {
		t.Fatalf("set downloaded for available transcript item failed: %v", err)
	}

	var refreshedAvailable db.PodcastItem
	if err := db.GetPodcastItemById(availableItem.ID, &refreshedAvailable); err != nil {
		t.Fatalf("failed to reload available item: %v", err)
	}
	if refreshedAvailable.TranscriptStatus != "available" {
		t.Fatalf("expected existing transcript status to remain available, got %q", refreshedAvailable.TranscriptStatus)
	}
}

func TestAddPodcastAndAddPodcastItemsCoverage(t *testing.T) {
	setupRetentionTestDB(t)
	pythonPath := requireWorkingPython(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed":
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd">
  <channel>
    <title>Coverage Feed</title>
    <description>Coverage feed summary</description>
    <itunes:image href="http://` + r.Host + `/cover.jpg" />
  </channel>
</rss>`))
		case "/cover.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("cover"))
		case "/chapters1.json":
			_, _ = w.Write([]byte(`{"chapters":[{"title":"Intro","startTime":0}]}`))
		case "/transcript1.json":
			_, _ = w.Write([]byte(`hello transcript`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	feedXMLURL := strings.ReplaceAll(server.URL, "http://", "")

	feedScript := writeTempScript(t, t.TempDir(), "feedparser_rich_stub.py", `#!/usr/bin/env python3
import json
import os
import sys

base = os.environ.get("TEST_BASE_URL", "")
payload = {
  "feed": {
    "title": "Coverage Feed",
    "itunes_author": "Coverage Author",
    "summary": "<p>Coverage summary</p>",
    "image": {}
  },
  "entries": [
    {
      "title": "Episode 1",
      "guid": "ep-1",
      "published": "Mon, 01 Jan 2024 00:00:00 GMT",
      "itunes_duration": "123",
      "summary": "<p>Episode one summary</p>",
      "enclosures": [{"href": base + "/audio1.mp3"}],
      "chapters": {"url": base + "/chapters1.json", "type": "application/json"},
      "transcripts": [{"url": base + "/transcript1.json", "type": "text/plain", "language": "en"}],
      "image": {"href": base + "/episode1.jpg"}
    },
    {
      "title": "Episode 2",
      "guid": "ep-2",
      "published": "bad-date",
      "itunes_duration": "1:02",
      "summary": "Episode two summary",
      "enclosures": [{"href": base + "/audio2.mp3"}]
    },
    {
      "title": "Episode without guid",
      "published": "Mon, 01 Jan 2024 00:00:00 GMT",
      "enclosures": [{"href": base + "/audio3.mp3"}]
    }
  ]
}
json.dump(payload, sys.stdout)
`)

	t.Setenv("TEST_BASE_URL", server.URL)
	t.Setenv(feedparserPythonEnv, pythonPath)
	t.Setenv(feedparserScriptEnv, feedScript)

	setting := db.GetOrCreateSetting()
	setting.GenerateNFOFile = true
	setting.AutoDownload = true
	setting.DownloadOnAdd = true
	setting.InitialDownloadCount = 1
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatalf("failed to update settings for add podcast coverage: %v", err)
	}

	feedURL := "http://" + feedXMLURL + "/feed"
	podcast, err := AddPodcast(feedURL)
	if err != nil {
		t.Fatalf("add podcast failed: %v", err)
	}
	if podcast.Image != server.URL+"/cover.jpg" {
		t.Fatalf("expected fallback image from raw xml body, got %q", podcast.Image)
	}

	nfoPath := filepath.Join(createDataFolderIfNotExists(podcast.Title), "album.nfo")
	if _, err := os.Stat(nfoPath); err != nil {
		t.Fatalf("expected generated nfo file, got %v", err)
	}

	if _, err := AddPodcast(feedURL); err == nil {
		t.Fatalf("expected duplicate add podcast call to fail")
	} else {
		var duplicateErr *model.PodcastAlreadyExistsError
		if !errors.As(err, &duplicateErr) {
			t.Fatalf("expected duplicate podcast error type, got %v", err)
		}
	}

	if err := AddPodcastItems(&podcast, true); err != nil {
		t.Fatalf("add podcast items (new podcast) failed: %v", err)
	}

	var items []db.PodcastItem
	if err := db.GetAllPodcastItemsByPodcastId(podcast.ID, &items); err != nil {
		t.Fatalf("failed to load podcast items: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 podcast items after skipping no-guid entry, got %d", len(items))
	}

	var ep1 db.PodcastItem
	var ep2 db.PodcastItem
	for _, item := range items {
		switch item.GUID {
		case "ep-1":
			ep1 = item
		case "ep-2":
			ep2 = item
		}
	}
	if ep1.GUID == "" || ep2.GUID == "" {
		t.Fatalf("expected ep-1 and ep-2 items to exist, got %+v", items)
	}
	if ep1.DownloadStatus != db.NotDownloaded {
		t.Fatalf("expected ep-1 to be queued, got %v", ep1.DownloadStatus)
	}
	if ep2.DownloadStatus != db.Deleted {
		t.Fatalf("expected ep-2 to be deleted for initial download limit, got %v", ep2.DownloadStatus)
	}
	if ep1.TranscriptStatus != "available" || !strings.Contains(ep1.TranscriptJSON, "hello transcript") {
		t.Fatalf("expected ep-1 transcript to be available with fetched content, got status=%q json=%s", ep1.TranscriptStatus, ep1.TranscriptJSON)
	}
	if ep2.TranscriptStatus != "pending_whisperx" {
		t.Fatalf("expected ep-2 transcript to be queued for whisperx, got %q", ep2.TranscriptStatus)
	}
	if !strings.Contains(ep1.ChaptersJSON, "Intro") {
		t.Fatalf("expected ep-1 chapters json to contain fetched chapter content, got %s", ep1.ChaptersJSON)
	}

	if err := AddPodcastItems(&podcast, false); err != nil {
		t.Fatalf("add podcast items (existing podcast) failed: %v", err)
	}
	var itemsAfterSecondRun []db.PodcastItem
	if err := db.GetAllPodcastItemsByPodcastId(podcast.ID, &itemsAfterSecondRun); err != nil {
		t.Fatalf("failed to load podcast items after second run: %v", err)
	}
	if len(itemsAfterSecondRun) != 2 {
		t.Fatalf("expected no duplicate episodes after second add run, got %d items", len(itemsAfterSecondRun))
	}
}

func TestDeletePodcastAndEpisodeDeleteBranches(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	dataDir := filepath.Join(tempDir, "assets")

	podcast := createPodcast(t, "delete-episodes-coverage", false)
	item := createServicePodcastItem(t, podcast, "delete-episodes-item", db.Downloaded)
	mediaPath := filepath.Join(dataDir, cleanFileName(podcast.Title), "episode.mp3")
	imagePath := filepath.Join(dataDir, cleanFileName(podcast.Title), "episode.jpg")
	if err := os.MkdirAll(filepath.Dir(mediaPath), 0o755); err != nil {
		t.Fatalf("failed to create episode file dir: %v", err)
	}
	if err := os.WriteFile(mediaPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("failed to create episode media file: %v", err)
	}
	if err := os.WriteFile(imagePath, []byte("image"), 0o644); err != nil {
		t.Fatalf("failed to create episode image file: %v", err)
	}
	item.DownloadPath = mediaPath
	item.LocalImage = imagePath
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("failed to persist delete-episodes paths: %v", err)
	}

	if err := DeletePodcastEpisodes(podcast.ID); err != nil {
		t.Fatalf("delete podcast episodes failed: %v", err)
	}
	var refreshed db.PodcastItem
	if err := db.GetPodcastItemById(item.ID, &refreshed); err != nil {
		t.Fatalf("failed to reload deleted episode item: %v", err)
	}
	if refreshed.DownloadStatus != db.Deleted {
		t.Fatalf("expected delete podcast episodes to mark item deleted, got %v", refreshed.DownloadStatus)
	}
	if _, err := os.Stat(mediaPath); !os.IsNotExist(err) {
		t.Fatalf("expected episode media file to be deleted")
	}
	if _, err := os.Stat(imagePath); !os.IsNotExist(err) {
		t.Fatalf("expected episode image file to be deleted")
	}

	podcastForDelete := createPodcast(t, "delete-podcast-coverage", false)
	deleteItem := createServicePodcastItem(t, podcastForDelete, "delete-podcast-item", db.Downloaded)
	deleteMedia := filepath.Join(dataDir, cleanFileName(podcastForDelete.Title), "delete.mp3")
	deleteImage := filepath.Join(dataDir, cleanFileName(podcastForDelete.Title), "delete.jpg")
	if err := os.MkdirAll(filepath.Dir(deleteMedia), 0o755); err != nil {
		t.Fatalf("failed to create delete podcast dir: %v", err)
	}
	if err := os.WriteFile(deleteMedia, []byte("audio"), 0o644); err != nil {
		t.Fatalf("failed to create delete podcast media file: %v", err)
	}
	if err := os.WriteFile(deleteImage, []byte("image"), 0o644); err != nil {
		t.Fatalf("failed to create delete podcast image file: %v", err)
	}
	deleteItem.DownloadPath = deleteMedia
	deleteItem.LocalImage = deleteImage
	if err := db.UpdatePodcastItem(&deleteItem); err != nil {
		t.Fatalf("failed to persist delete podcast item paths: %v", err)
	}

	if err := DeletePodcast(podcastForDelete.ID, true); err != nil {
		t.Fatalf("delete podcast with file cleanup failed: %v", err)
	}
	if _, err := os.Stat(deleteMedia); !os.IsNotExist(err) {
		t.Fatalf("expected delete podcast media file to be removed")
	}
	if _, err := os.Stat(deleteImage); !os.IsNotExist(err) {
		t.Fatalf("expected delete podcast image file to be removed")
	}

	var removedPodcast db.Podcast
	if err := db.GetPodcastById(podcastForDelete.ID, &removedPodcast); err == nil {
		t.Fatalf("expected podcast record to be deleted")
	}

	if err := DeletePodcastEpisodes("missing-podcast-id"); err == nil {
		t.Fatalf("expected delete podcast episodes to fail for missing podcast id")
	}
	if err := DeletePodcast("missing-podcast-id", true); err == nil {
		t.Fatalf("expected delete podcast to fail for missing podcast id")
	}
}

func TestSetPodcastItemAsDownloadedExtractsID3Metadata(t *testing.T) {
	setupRetentionTestDB(t)
	pythonPath := requireWorkingPython(t)

	mutagenScript := writeTempScript(t, t.TempDir(), "mutagen_stub.py", `#!/usr/bin/env python3
import json
import sys

payload = {
  "tags": {"artist": ["Tester"], "title": ["Episode"]},
  "chapters": [{"title": "Intro", "startTime": 1.25}]
}
json.dump(payload, sys.stdout)
`)

	t.Setenv(mutagenPythonEnv, pythonPath)
	t.Setenv(mutagenScriptEnv, mutagenScript)

	audioPath := filepath.Join(t.TempDir(), "id3-audio.mp3")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("failed to create id3 test audio file: %v", err)
	}

	podcast := createPodcast(t, "id3-coverage", false)
	item := createServicePodcastItem(t, podcast, "id3-episode", db.NotDownloaded)
	if err := SetPodcastItemAsDownloaded(item.ID, audioPath); err != nil {
		t.Fatalf("set podcast item downloaded with id3 extraction failed: %v", err)
	}

	var refreshed db.PodcastItem
	if err := db.GetPodcastItemById(item.ID, &refreshed); err != nil {
		t.Fatalf("failed to reload id3-updated item: %v", err)
	}
	if strings.TrimSpace(refreshed.ID3TagsJSON) == "" {
		t.Fatalf("expected id3 tags json to be populated")
	}
	if strings.TrimSpace(refreshed.ID3ChaptersJSON) == "" {
		t.Fatalf("expected id3 chapters json to be populated")
	}
	if strings.TrimSpace(refreshed.ChaptersJSON) == "" || refreshed.ChaptersType != "id3" {
		t.Fatalf("expected chapters json/type to be populated from id3 extraction, got chapters=%q type=%q", refreshed.ChaptersJSON, refreshed.ChaptersType)
	}
}
