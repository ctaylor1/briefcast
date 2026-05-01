package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TheHippo/podcastindex"
	"github.com/ctaylor1/briefcast/db"
)

// TestFileServiceErrorAndExistingFileBranches handles the corresponding operation.
func TestFileServiceErrorAndExistingFileBranches(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	dataDir := filepath.Join(tempDir, "assets")
	podcast := createPodcast(t, "file-service-branches", false)

	if _, err := DownloadPodcastCoverImage("", podcast.Title); err == nil {
		t.Fatalf("expected empty cover image link to fail")
	}
	if _, err := DownloadImage("", "episode-id", podcast.Title); err == nil {
		t.Fatalf("expected empty episode image link to fail")
	}
	if _, err := DownloadPodcastCoverImage("://bad-url", podcast.Title); err == nil {
		t.Fatalf("expected malformed cover image URL to fail")
	}
	if _, err := DownloadImage("://bad-url", "episode-id", podcast.Title); err == nil {
		t.Fatalf("expected malformed episode image URL to fail")
	}

	contentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "10")
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write([]byte("image-data"))
	}))
	defer contentServer.Close()

	coverURL := contentServer.URL + "/cover.jpg"
	coverPath := GetPodcastLocalImagePath(coverURL, podcast.Title)
	if err := os.MkdirAll(filepath.Dir(coverPath), 0o755); err != nil {
		t.Fatalf("failed to create cover dir: %v", err)
	}
	if err := os.WriteFile(coverPath, []byte("existing-cover"), 0o644); err != nil {
		t.Fatalf("failed to seed existing cover file: %v", err)
	}

	gotCoverPath, err := DownloadPodcastCoverImage(coverURL, podcast.Title)
	if err != nil {
		t.Fatalf("expected existing cover branch to succeed, got %v", err)
	}
	if gotCoverPath != coverPath {
		t.Fatalf("expected existing cover path %q, got %q", coverPath, gotCoverPath)
	}

	imageURL := contentServer.URL + "/episode.jpg"
	imageFileName := getFileName(imageURL, "episode-id", ".jpg")
	imagePath := path.Join(createFolder("images", createDataFolderIfNotExists(podcast.Title)), imageFileName)
	if err := os.MkdirAll(filepath.Dir(imagePath), 0o755); err != nil {
		t.Fatalf("failed to create image dir: %v", err)
	}
	if err := os.WriteFile(imagePath, []byte("existing-episode-image"), 0o644); err != nil {
		t.Fatalf("failed to seed existing episode image: %v", err)
	}

	gotImagePath, err := DownloadImage(imageURL, "episode-id", podcast.Title)
	if err != nil {
		t.Fatalf("expected existing episode image branch to succeed, got %v", err)
	}
	if gotImagePath != imagePath {
		t.Fatalf("expected existing episode image path %q, got %q", imagePath, gotImagePath)
	}

	missingFile := filepath.Join(dataDir, "missing.file")
	if err := DeleteFile(missingFile); !os.IsNotExist(err) {
		t.Fatalf("expected DeleteFile on missing file to return not-exist error, got %v", err)
	}

	statusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("unused"))
	}))
	defer statusServer.Close()

	if _, err := GetFileSizeFromURL(statusServer.URL); err == nil {
		t.Fatalf("expected non-200 HEAD response to fail GetFileSizeFromURL")
	}

	lengthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "not-a-number")
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write([]byte("unused"))
	}))
	defer lengthServer.Close()

	if _, err := GetFileSizeFromURL(lengthServer.URL); err == nil {
		t.Fatalf("expected invalid Content-Length to fail GetFileSizeFromURL")
	}

	if _, err := doRequestWithHostLimit(nil, nil); err == nil {
		t.Fatalf("expected nil request to fail")
	}

	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer okServer.Close()

	req, err := http.NewRequest(http.MethodGet, okServer.URL, nil)
	if err != nil {
		t.Fatalf("failed to build test request: %v", err)
	}
	resp, err := doRequestWithHostLimit(nil, req)
	if err != nil {
		t.Fatalf("expected nil client path to fallback to default client, got %v", err)
	}
	_ = resp.Body.Close()
}

// TestHostLimiterAcquireCancellationAndHelpers handles the corresponding operation.
func TestHostLimiterAcquireCancellationAndHelpers(t *testing.T) {
	if got := hostKey(nil); got != "unknown" {
		t.Fatalf("expected nil host key to be unknown, got %q", got)
	}
	if got := hostKey(&url.URL{}); got != "unknown" {
		t.Fatalf("expected empty host key to be unknown, got %q", got)
	}
	if got := hostKey(&url.URL{Host: "EXAMPLE.COM:443"}); got != "example.com" {
		t.Fatalf("expected normalized host key, got %q", got)
	}

	t.Setenv("PER_HOST_MAX_CONCURRENCY", "0")
	t.Setenv("PER_HOST_RATE_LIMIT_RPS", "999999")
	limiterFromEnv := newHostRequestLimiterFromEnv()
	if limiterFromEnv.maxConcurrency != defaultPerHostMaxConcurrency {
		t.Fatalf("expected max concurrency fallback %d, got %d", defaultPerHostMaxConcurrency, limiterFromEnv.maxConcurrency)
	}
	if limiterFromEnv.minInterval < time.Millisecond {
		t.Fatalf("expected min interval to floor at 1ms, got %s", limiterFromEnv.minInterval)
	}

	delayed := &hostRequestLimiter{
		maxConcurrency: 1,
		minInterval:    time.Second,
		hosts:          map[string]*singleHostLimiter{},
	}
	delayHostLimiter := delayed.getHostLimiter("example.com")
	delayHostLimiter.nextAllowed = time.Now().Add(250 * time.Millisecond)
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := delayed.acquire(cancelledCtx, &url.URL{Host: "example.com"}); err == nil {
		t.Fatalf("expected cancelled context to fail delayed acquire")
	}

	blocked := &hostRequestLimiter{
		maxConcurrency: 1,
		minInterval:    0,
		hosts:          map[string]*singleHostLimiter{},
	}
	blockedHostLimiter := blocked.getHostLimiter("example.com")
	blockedHostLimiter.sem <- struct{}{}
	timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer timeoutCancel()
	if _, err := blocked.acquire(timeoutCtx, &url.URL{Host: "example.com"}); err == nil {
		t.Fatalf("expected semaphore contention to fail acquire once context times out")
	}
	<-blockedHostLimiter.sem
}

// TestChapterParseAdditionalBranches handles the corresponding operation.
func TestChapterParseAdditionalBranches(t *testing.T) {
	if got := parseChapters(`{"title":"not-a-list"}`); got != nil {
		t.Fatalf("expected map payload without chapter list to return nil, got %+v", got)
	}

	raw := `[
		{"caption":{"text":"Intro"},"start":["12"],"end":"15"},
		{"label":["Topic"],"start_time":{"value":"00:30"},"end_time":{"text":"00:45"}},
		{"description":"ignored","start":"bad"}
	]`
	chapters := parseChapters(raw)
	if len(chapters) != 2 {
		t.Fatalf("expected two parsed chapters, got %d", len(chapters))
	}
	if chapters[0].Title != "Intro" || chapters[0].StartSeconds != 12 || chapters[0].EndSeconds != 15 {
		t.Fatalf("unexpected first chapter %+v", chapters[0])
	}
	if chapters[1].Title != "Topic" || chapters[1].StartSeconds != 30 || chapters[1].EndSeconds != 45 {
		t.Fatalf("unexpected second chapter %+v", chapters[1])
	}

	if got := pickString(map[string]interface{}{"title": map[string]interface{}{"value": "Nested"}}, "title"); got != "Nested" {
		t.Fatalf("expected nested map string extraction, got %q", got)
	}
	if got := pickString(map[string]interface{}{"name": []interface{}{"List"}}, "name"); got != "List" {
		t.Fatalf("expected list string extraction, got %q", got)
	}

	if got := readTime(map[string]interface{}{"startTimeMS": "2500"}, true, "startTimeMS"); got != 2.5 {
		t.Fatalf("expected milliseconds to seconds conversion, got %v", got)
	}
	if got := parseTimeValue(float64(1500), true); got != 1.5 {
		t.Fatalf("expected float milliseconds conversion, got %v", got)
	}
	if got := parseTimeValue(map[string]interface{}{"text": "00:10"}, false); got != 10 {
		t.Fatalf("expected map text time parsing, got %v", got)
	}
	if got := parseTimeString("1:2:3:4", false); got != -1 {
		t.Fatalf("expected invalid 4-part time to return -1, got %v", got)
	}
	if got := parseTimeString("1::2", false); got != -1 {
		t.Fatalf("expected malformed time with empty part to return -1, got %v", got)
	}
}

// TestRetentionReferenceTimeFallbacks handles the corresponding operation.
func TestRetentionReferenceTimeFallbacks(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	pub := db.PodcastItem{
		PubDate:      now.Add(-2 * time.Hour),
		DownloadDate: now.Add(-1 * time.Hour),
		Base:         db.Base{CreatedAt: now},
	}
	if got := retentionReferenceTime(pub); !got.Equal(pub.PubDate.UTC()) {
		t.Fatalf("expected pub date precedence, got %s", got)
	}

	download := db.PodcastItem{
		DownloadDate: now.Add(-1 * time.Hour),
		Base:         db.Base{CreatedAt: now},
	}
	if got := retentionReferenceTime(download); !got.Equal(download.DownloadDate.UTC()) {
		t.Fatalf("expected download date fallback, got %s", got)
	}

	created := db.PodcastItem{Base: db.Base{CreatedAt: now}}
	if got := retentionReferenceTime(created); !got.Equal(now) {
		t.Fatalf("expected created-at fallback, got %s", got)
	}
}

// TestGetSearchFromPodcastIndexIncludesCategories handles the corresponding operation.
func TestGetSearchFromPodcastIndexIncludesCategories(t *testing.T) {
	pod := &podcastindex.Podcast{
		URL:         "https://example.com/feed.xml",
		Image:       "https://example.com/image.jpg",
		Title:       "PodcastIndex Title",
		Description: "PodcastIndex Description",
		Categories: map[uint]string{
			10: "Technology",
			20: "News",
		},
	}

	got := GetSearchFromPodcastIndex(pod)
	if got.URL != pod.URL || got.Image != pod.Image || got.Title != pod.Title || got.Description != pod.Description {
		t.Fatalf("unexpected mapped podcastindex search result: %+v", got)
	}
	if len(got.Categories) != 2 {
		t.Fatalf("expected mapped categories, got %+v", got.Categories)
	}
	joined := strings.Join(got.Categories, ",")
	if !strings.Contains(joined, "Technology") || !strings.Contains(joined, "News") {
		t.Fatalf("expected categories to include technology and news, got %+v", got.Categories)
	}
}

// TestCheckMissingFilesAndDeleteEpisodeFileBranches handles the corresponding operation.
func TestCheckMissingFilesAndDeleteEpisodeFileBranches(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	dataDir := filepath.Join(tempDir, "assets")
	podcast := createPodcast(t, "missing-files-branches", false)

	missingDelete := createDownloadedItem(t, podcast, "missing-delete", time.Now().Add(-2*time.Hour), false, dataDir)
	if err := os.Remove(missingDelete.DownloadPath); err != nil {
		t.Fatalf("failed to remove seeded file for missing-delete branch: %v", err)
	}
	setting := db.GetOrCreateSetting()
	setting.DontDownloadDeletedFromDisk = true
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatalf("failed to enable DontDownloadDeletedFromDisk: %v", err)
	}
	if err := CheckMissingFiles(); err != nil {
		t.Fatalf("check missing files (delete branch) failed: %v", err)
	}
	var refreshedDelete db.PodcastItem
	if err := db.GetPodcastItemByID(missingDelete.ID, &refreshedDelete); err != nil {
		t.Fatalf("failed to reload missing-delete item: %v", err)
	}
	if refreshedDelete.DownloadStatus != db.Deleted {
		t.Fatalf("expected missing file item to be marked deleted, got %v", refreshedDelete.DownloadStatus)
	}

	missingRequeue := createDownloadedItem(t, podcast, "missing-requeue", time.Now().Add(-90*time.Minute), false, dataDir)
	if err := os.Remove(missingRequeue.DownloadPath); err != nil {
		t.Fatalf("failed to remove seeded file for missing-requeue branch: %v", err)
	}
	setting = db.GetOrCreateSetting()
	setting.DontDownloadDeletedFromDisk = false
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatalf("failed to disable DontDownloadDeletedFromDisk: %v", err)
	}
	if err := CheckMissingFiles(); err != nil {
		t.Fatalf("check missing files (requeue branch) failed: %v", err)
	}
	var refreshedRequeue db.PodcastItem
	if err := db.GetPodcastItemByID(missingRequeue.ID, &refreshedRequeue); err != nil {
		t.Fatalf("failed to reload missing-requeue item: %v", err)
	}
	if refreshedRequeue.DownloadStatus != db.NotDownloaded {
		t.Fatalf("expected missing file item to be requeued, got %v", refreshedRequeue.DownloadStatus)
	}

	toDelete := createDownloadedItem(t, podcast, "delete-episode-file", time.Now().Add(-45*time.Minute), false, dataDir)
	localImagePath := filepath.Join(dataDir, cleanFileName(podcast.Title), "delete-episode-file.jpg")
	if err := os.MkdirAll(filepath.Dir(localImagePath), 0o755); err != nil {
		t.Fatalf("failed to create local image dir: %v", err)
	}
	if err := os.WriteFile(localImagePath, []byte("image"), 0o644); err != nil {
		t.Fatalf("failed to create local image file: %v", err)
	}
	toDelete.LocalImage = localImagePath
	if err := db.UpdatePodcastItem(&toDelete); err != nil {
		t.Fatalf("failed to persist local image path: %v", err)
	}
	if err := DeleteEpisodeFile(toDelete.ID); err != nil {
		t.Fatalf("delete episode file should succeed, got %v", err)
	}
	var refreshedDeletedEpisode db.PodcastItem
	if err := db.GetPodcastItemByID(toDelete.ID, &refreshedDeletedEpisode); err != nil {
		t.Fatalf("failed to reload deleted episode item: %v", err)
	}
	if refreshedDeletedEpisode.DownloadStatus != db.Deleted {
		t.Fatalf("expected deleted episode status after file deletion, got %v", refreshedDeletedEpisode.DownloadStatus)
	}
	if _, err := os.Stat(toDelete.DownloadPath); !os.IsNotExist(err) {
		t.Fatalf("expected deleted episode media file to be removed")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(localImagePath); os.IsNotExist(err) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(localImagePath); !os.IsNotExist(err) {
		t.Fatalf("expected async local image cleanup to remove file")
	}

	missingPathItem := createServicePodcastItem(t, podcast, "delete-missing-path", db.Downloaded)
	missingPath := filepath.Join(dataDir, assetAudioDir, sanitizeAssetName(podcast.Title), "not-present.mp3")
	missingPathItem.DownloadPath = missingPath
	missingPathItem.DownloadStatus = db.Downloaded
	if err := db.UpdatePodcastItem(&missingPathItem); err != nil {
		t.Fatalf("failed to persist missing path item: %v", err)
	}
	if err := DeleteEpisodeFile(missingPathItem.ID); err != nil {
		t.Fatalf("delete episode file should ignore not-exist media path, got %v", err)
	}

	if err := DeleteEpisodeFile("missing-episode-id"); err == nil {
		t.Fatalf("expected missing episode id delete to fail")
	}
}

// TestDownloadMissingEpisodesEarlyReturnBranches handles the corresponding operation.
func TestDownloadMissingEpisodesEarlyReturnBranches(t *testing.T) {
	setupRetentionTestDB(t)
	ResumeDownloads()
	t.Cleanup(ResumeDownloads)

	if err := DownloadMissingEpisodes(); err != nil {
		t.Fatalf("expected empty queue branch to return nil, got %v", err)
	}

	PauseDownloads()
	if err := DownloadMissingEpisodes(); err != nil {
		t.Fatalf("expected paused downloads branch to return nil, got %v", err)
	}
	ResumeDownloads()

	db.Lock("DownloadMissingEpisodes", 120)
	defer db.Unlock("DownloadMissingEpisodes")
	if err := DownloadMissingEpisodes(); err != nil {
		t.Fatalf("expected lock-held branch to return nil, got %v", err)
	}
}

// TestAddOpmlWithFeedUrlsWorkerBranches handles the corresponding operation.
func TestAddOpmlWithFeedUrlsWorkerBranches(t *testing.T) {
	setupRetentionTestDB(t)

	existing := db.Podcast{
		Title: "opml-existing",
		URL:   "https://example.com/existing-feed.xml",
	}
	if err := db.CreatePodcast(&existing); err != nil {
		t.Fatalf("failed to seed existing podcast for OPML duplicate branch: %v", err)
	}

	// Keep the background refresh goroutine deterministic for this unit test.
	db.Lock("RefreshEpisodes", 120)
	defer db.Unlock("RefreshEpisodes")

	opml := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>Import</title></head>
  <body>
    <outline text="dup" type="rss" xmlURL="https://example.com/existing-feed.xml" />
    <outline text="outer">
      <outline text="bad" type="rss" xmlURL="://bad-url" />
    </outline>
  </body>
</opml>`

	if err := AddOpml(opml); err != nil {
		t.Fatalf("expected OPML import with mixed duplicate/malformed URLs to return nil, got %v", err)
	}
}
