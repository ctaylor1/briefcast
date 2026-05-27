package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/service"
)

func waitForControllerItemStatus(t *testing.T, id string, expected db.DownloadStatus) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var item db.PodcastItem
		if err := db.GetPodcastItemByID(id, &item); err == nil && item.DownloadStatus == expected {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for podcast item %s status %v", id, expected)
}

func TestEpisodeMediaDoesNotServeLocalPathsOutsideAssets(t *testing.T) {
	setupControllersTestDB(t)
	router := makeExpandedRouter(t)

	_, item := createControllerPodcastAndItem(t)
	outsideDir := t.TempDir()
	outsideImage := filepath.Join(outsideDir, "outside.jpg")
	outsideAudio := filepath.Join(outsideDir, "outside.mp3")
	if err := os.WriteFile(outsideImage, []byte("outside-image"), 0o644); err != nil {
		t.Fatalf("write outside image: %v", err)
	}
	if err := os.WriteFile(outsideAudio, []byte("outside-audio"), 0o644); err != nil {
		t.Fatalf("write outside audio: %v", err)
	}

	item.LocalImage = outsideImage
	item.Image = "https://example.com/fallback.jpg"
	item.DownloadPath = outsideAudio
	item.FileURL = "https://example.com/fallback.mp3"
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("update item with outside paths: %v", err)
	}

	resp := performRequest(router, http.MethodGet, "/podcastitems/"+item.ID+"/image", nil, nil)
	if resp.Code != http.StatusFound {
		t.Fatalf("expected outside image path to redirect, got %d", resp.Code)
	}
	if got := resp.Header().Get("Location"); got != item.Image {
		t.Fatalf("expected image fallback redirect %q, got %q", item.Image, got)
	}
	if strings.Contains(resp.Body.String(), "outside-image") {
		t.Fatalf("outside image content was served")
	}

	resp = performRequest(router, http.MethodGet, "/podcastitems/"+item.ID+"/file", nil, nil)
	if resp.Code != http.StatusFound {
		t.Fatalf("expected outside media path to redirect, got %d", resp.Code)
	}
	if got := resp.Header().Get("Location"); got != item.FileURL {
		t.Fatalf("expected media fallback redirect %q, got %q", item.FileURL, got)
	}
	if strings.Contains(resp.Body.String(), "outside-audio") {
		t.Fatalf("outside audio content was served")
	}
}

func TestEpisodeMediaDoesNotServeAssetDirectories(t *testing.T) {
	setupControllersTestDB(t)
	router := makeExpandedRouter(t)

	dataDir := strings.TrimSpace(os.Getenv("DATA"))
	_, item := createControllerPodcastAndItem(t)
	item.LocalImage = dataDir
	item.Image = "https://example.com/fallback.jpg"
	item.DownloadPath = dataDir
	item.FileURL = "https://example.com/fallback.mp3"
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("update item with directory paths: %v", err)
	}

	resp := performRequest(router, http.MethodGet, "/podcastitems/"+item.ID+"/image", nil, nil)
	if resp.Code != http.StatusFound {
		t.Fatalf("expected asset directory image path to redirect, got %d", resp.Code)
	}
	if got := resp.Header().Get("Location"); got != item.Image {
		t.Fatalf("expected image fallback redirect %q, got %q", item.Image, got)
	}

	resp = performRequest(router, http.MethodGet, "/podcastitems/"+item.ID+"/file", nil, nil)
	if resp.Code != http.StatusFound {
		t.Fatalf("expected asset directory media path to redirect, got %d", resp.Code)
	}
	if got := resp.Header().Get("Location"); got != item.FileURL {
		t.Fatalf("expected media fallback redirect %q, got %q", item.FileURL, got)
	}
}

func TestEpisodeMediaRejectsUnsafeFallbackRedirects(t *testing.T) {
	setupControllersTestDB(t)
	router := makeExpandedRouter(t)

	podcast, item := createControllerPodcastAndItem(t)
	podcast.Image = "javascript:alert(1)"
	if err := db.UpdatePodcast(&podcast); err != nil {
		t.Fatalf("update podcast unsafe image fallback: %v", err)
	}

	item.LocalImage = ""
	item.Image = "javascript:alert(1)"
	item.DownloadPath = ""
	item.FileURL = "file:///etc/passwd"
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("update item with unsafe fallback URLs: %v", err)
	}

	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "podcast image", path: "/podcasts/" + podcast.ID + "/image"},
		{name: "episode image", path: "/podcastitems/" + item.ID + "/image"},
		{name: "episode file", path: "/podcastitems/" + item.ID + "/file"},
	} {
		resp := performRequest(router, http.MethodGet, tc.path, nil, nil)
		if resp.Code != http.StatusNotFound {
			t.Fatalf("%s: expected unsafe fallback URL to return 404, got %d", tc.name, resp.Code)
		}
		if got := resp.Header().Get("Location"); got != "" {
			t.Fatalf("%s: expected no Location header for unsafe fallback URL, got %q", tc.name, got)
		}
	}
}

// TestEpisodeMediaAndPodcastItemActionBranches handles the corresponding operation.
func TestEpisodeMediaAndPodcastItemActionBranches(t *testing.T) {
	setupControllersTestDB(t)
	service.ResumeDownloads()
	t.Cleanup(service.ResumeDownloads)
	actionRouter := makeExpandedRouter(t)
	mediaRouter := makeRouter()

	podcast, item := createControllerPodcastAndItem(t)
	item.TranscriptStatus = "queued"
	item.TranscriptJSON = "not-json"

	audioServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("controller-audio"))
	}))
	defer audioServer.Close()

	item.FileURL = audioServer.URL + "/audio.mp3"
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("failed to persist controller test item update: %v", err)
	}

	resp := performRequest(mediaRouter, http.MethodGet, "/podcastitems/"+item.ID+"/transcript", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 from transcript endpoint, got %d", resp.Code)
	}
	var transcriptPayload map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &transcriptPayload); err != nil {
		t.Fatalf("failed to decode transcript payload: %v", err)
	}
	if transcriptPayload["status"] != "queued" {
		t.Fatalf("expected queued transcript status, got %+v", transcriptPayload)
	}
	if transcriptPayload["transcript"] != "not-json" {
		t.Fatalf("expected raw transcript payload for invalid json, got %+v", transcriptPayload)
	}

	resp = performRequest(mediaRouter, http.MethodGet, "/podcastitems/missing/chapters", nil, nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing chapter item, got %d", resp.Code)
	}

	resp = performRequest(actionRouter, http.MethodGet, "/podcastitems/"+item.ID+"/download", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 from download item endpoint, got %d", resp.Code)
	}
	waitForControllerItemStatus(t, item.ID, db.Downloaded)

	resp = performRequest(actionRouter, http.MethodGet, "/podcastitems/"+item.ID+"/delete", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 from delete item endpoint, got %d", resp.Code)
	}
	waitForControllerItemStatus(t, item.ID, db.Deleted)

	_ = podcast
}

// TestDownloadControllerResumeAndCancelBranches handles the corresponding operation.
func TestDownloadControllerResumeAndCancelBranches(t *testing.T) {
	setupControllersTestDB(t)
	service.ResumeDownloads()
	t.Cleanup(service.ResumeDownloads)
	router := makeRouter()

	podcast, item := createControllerPodcastAndItem(t)
	audioServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("controller-audio"))
	}))
	defer audioServer.Close()

	item.FileURL = audioServer.URL + "/audio.mp3"
	item.DownloadStatus = db.NotDownloaded
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("failed to update queue item: %v", err)
	}

	resp := performRequest(router, http.MethodPost, "/podcastitems/"+item.ID+"/cancel", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 from cancel endpoint, got %d", resp.Code)
	}
	waitForControllerItemStatus(t, item.ID, db.Paused)

	resp = performRequest(router, http.MethodPost, "/podcastitems/"+item.ID+"/resume", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 from resume paused endpoint, got %d", resp.Code)
	}
	waitForControllerItemStatus(t, item.ID, db.Downloaded)

	downloading := db.PodcastItem{
		PodcastID:      podcast.ID,
		GUID:           "resume-downloading-branch",
		Title:          "Resume downloading branch",
		PubDate:        time.Now().UTC(),
		FileURL:        audioServer.URL + "/audio.mp3",
		DownloadStatus: db.Downloading,
	}
	if err := db.CreatePodcastItem(&downloading); err != nil {
		t.Fatalf("failed to create downloading item: %v", err)
	}
	resp = performRequest(router, http.MethodPost, "/podcastitems/"+downloading.ID+"/resume", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 from resume downloading endpoint, got %d", resp.Code)
	}

	resp = performRequest(router, http.MethodPost, "/podcastitems/missing/cancel", nil, nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for cancel missing item, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodPost, "/podcastitems/missing/resume", nil, nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for resume missing item, got %d", resp.Code)
	}
}

// TestBackupsAndRssErrorBranches handles the corresponding operation.
func TestBackupsAndRssErrorBranches(t *testing.T) {
	setupControllersTestDB(t)
	router := makeExpandedRouter(t)

	configDir := strings.TrimSpace(os.Getenv("CONFIG"))
	backupDir := filepath.Join(configDir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("failed to create backups dir: %v", err)
	}
	validBackup := filepath.Join(backupDir, "briefcast_backup_2026.02.22_120000.tar.gz")
	if err := os.WriteFile(validBackup, []byte("backup"), 0o644); err != nil {
		t.Fatalf("failed to create valid backup file: %v", err)
	}

	resp := performRequest(router, http.MethodGet, "/backups", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 from backups endpoint, got %d", resp.Code)
	}

	sqlDB, err := db.DB.DB()
	if err != nil {
		t.Fatalf("failed to get sql db handle: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("failed to close sql db handle: %v", err)
	}

	resp = performRequest(router, http.MethodGet, "/rss", nil, nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 from rss endpoint when db is closed, got %d", resp.Code)
	}
}

// TestAddPodcastGenericErrorBranches handles the corresponding operation.
func TestAddPodcastGenericErrorBranches(t *testing.T) {
	setupControllersTestDB(t)
	router := makeExpandedRouter(t)

	resp := performRequest(router, http.MethodPost, "/podcasts", strings.NewReader(`{"url":"http://127.0.0.1:1/feed.xml"}`), map[string]string{
		"Content-Type": "application/json",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 from add podcast generic error branch, got %d", resp.Code)
	}

	resp = performRequest(router, http.MethodPost, "/add-new", strings.NewReader("url=http://127.0.0.1:1/feed.xml"), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 from add-new generic error branch, got %d", resp.Code)
	}
}
