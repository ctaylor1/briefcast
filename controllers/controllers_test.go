package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/service"
	glebarezsqlite "github.com/glebarez/sqlite"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func setupControllersTestDB(t *testing.T) {
	t.Helper()
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "assets")
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
}

func makeRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/settings", GetSettings)
	router.PATCH("/settings", PatchSettings)
	router.GET("/podcastitems/:id/transcript", GetPodcastItemTranscript)
	router.GET("/podcastitems/:id/chapters", GetPodcastItemChapters)
	router.GET("/downloads/queue", GetDownloadQueue)
	router.POST("/downloads/pause", PauseDownloads)
	router.POST("/downloads/resume", ResumeDownloads)
	router.POST("/downloads/cancel", CancelAllDownloads)
	router.POST("/podcastitems/:id/cancel", CancelPodcastItemDownload)
	router.GET("/search/local", SearchLocalRecords)
	return router
}

func createControllerPodcastAndItem(t *testing.T) (db.Podcast, db.PodcastItem) {
	t.Helper()
	podcast := db.Podcast{
		Title:   "Controller Podcast",
		URL:     "https://example.com/controller.xml",
		Author:  "tester",
		Summary: "controller summary",
	}
	if err := db.CreatePodcast(&podcast); err != nil {
		t.Fatalf("create podcast failed: %v", err)
	}

	item := db.PodcastItem{
		PodcastID:        podcast.ID,
		GUID:             "controller-episode",
		Title:            "Controller Episode",
		Summary:          "episode summary",
		SummaryHTML:      "<p>episode summary</p>",
		PubDate:          time.Now().UTC(),
		FileURL:          "https://example.com/controller.mp3",
		DownloadStatus:   db.NotDownloaded,
		ChaptersJSON:     `{"chapters":[{"title":"Intro","startTime":12}]}`,
		TranscriptStatus: "available",
		TranscriptJSON:   `{"segments":[{"start":12.5,"end":15.0,"text":"hello world"}]}`,
	}
	if err := db.CreatePodcastItem(&item); err != nil {
		t.Fatalf("create podcast item failed: %v", err)
	}
	return podcast, item
}

func TestSettingsEndpoints(t *testing.T) {
	setupControllersTestDB(t)
	router := makeRouter()

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 from GET /settings, got %d", resp.Code)
	}

	invalidPatch := `{"keepLatestEpisodes":-1}`
	req = httptest.NewRequest(http.MethodPatch, "/settings", bytes.NewBufferString(invalidPatch))
	req.Header.Set("Content-Type", "application/json")
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 from invalid PATCH /settings, got %d", resp.Code)
	}

	validPatch := `{"keepAllEpisodes":false,"keepLatestEpisodes":3,"deleteAfterDays":10,"deleteOnlyPlayed":true}`
	req = httptest.NewRequest(http.MethodPatch, "/settings", bytes.NewBufferString(validPatch))
	req.Header.Set("Content-Type", "application/json")
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 from valid PATCH /settings, got %d", resp.Code)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode settings response: %v", err)
	}
	if payload["keepLatestEpisodes"] != float64(3) {
		t.Fatalf("expected keepLatestEpisodes=3, got %+v", payload)
	}
}

func TestEpisodeMediaEndpoints(t *testing.T) {
	setupControllersTestDB(t)
	router := makeRouter()
	_, item := createControllerPodcastAndItem(t)

	req := httptest.NewRequest(http.MethodGet, "/podcastitems/"+item.ID+"/transcript", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 from transcript endpoint, got %d", resp.Code)
	}
	var transcriptPayload map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &transcriptPayload); err != nil {
		t.Fatalf("failed to decode transcript payload: %v", err)
	}
	if transcriptPayload["status"] != "available" {
		t.Fatalf("expected transcript status available, got %+v", transcriptPayload)
	}
	if _, ok := transcriptPayload["transcript"]; !ok {
		t.Fatalf("expected transcript payload to include transcript body")
	}

	req = httptest.NewRequest(http.MethodGet, "/podcastitems/"+item.ID+"/chapters", nil)
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 from chapters endpoint, got %d", resp.Code)
	}
	var chaptersPayload map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &chaptersPayload); err != nil {
		t.Fatalf("failed to decode chapters payload: %v", err)
	}
	if chaptersPayload["source"] != "feed" {
		t.Fatalf("expected chapter source feed, got %+v", chaptersPayload)
	}

	req = httptest.NewRequest(http.MethodGet, "/podcastitems/unknown-id/transcript", nil)
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing transcript item, got %d", resp.Code)
	}
}

func TestDownloadEndpoints(t *testing.T) {
	setupControllersTestDB(t)
	router := makeRouter()
	podcast, _ := createControllerPodcastAndItem(t)

	downloading := db.PodcastItem{
		PodcastID:      podcast.ID,
		GUID:           "controller-downloading",
		Title:          "Downloading Episode",
		PubDate:        time.Now().UTC(),
		FileURL:        "https://example.com/downloading.mp3",
		DownloadStatus: db.Downloading,
	}
	if err := db.CreatePodcastItem(&downloading); err != nil {
		t.Fatalf("failed to create downloading item: %v", err)
	}
	downloaded := db.PodcastItem{
		PodcastID:      podcast.ID,
		GUID:           "controller-downloaded",
		Title:          "Downloaded Episode",
		PubDate:        time.Now().UTC(),
		FileURL:        "https://example.com/downloaded.mp3",
		DownloadStatus: db.Downloaded,
	}
	if err := db.CreatePodcastItem(&downloaded); err != nil {
		t.Fatalf("failed to create downloaded item: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/downloads/queue?limit=10", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 from queue endpoint, got %d", resp.Code)
	}
	var queuePayload map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &queuePayload); err != nil {
		t.Fatalf("failed to decode queue payload: %v", err)
	}
	if _, ok := queuePayload["counts"]; !ok {
		t.Fatalf("expected queue payload to include counts")
	}
	if _, ok := queuePayload["items"]; !ok {
		t.Fatalf("expected queue payload to include items")
	}

	req = httptest.NewRequest(http.MethodPost, "/downloads/pause", nil)
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 from pause endpoint, got %d", resp.Code)
	}
	if !service.DownloadsPaused() {
		t.Fatalf("expected downloads to be paused")
	}

	req = httptest.NewRequest(http.MethodPost, "/downloads/resume", nil)
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 from resume endpoint, got %d", resp.Code)
	}
	if service.DownloadsPaused() {
		t.Fatalf("expected downloads to be resumed")
	}

	req = httptest.NewRequest(http.MethodPost, "/downloads/cancel", nil)
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 from cancel all endpoint, got %d", resp.Code)
	}
}

func TestCancelPodcastItemDownloadAndLocalSearch(t *testing.T) {
	setupControllersTestDB(t)
	router := makeRouter()
	_, item := createControllerPodcastAndItem(t)

	req := httptest.NewRequest(http.MethodPost, "/podcastitems/"+item.ID+"/cancel", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 from cancel episode endpoint, got %d", resp.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/search/local?q=", nil)
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 for empty local search query, got %d", resp.Code)
	}
	var results []interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &results); err != nil {
		t.Fatalf("failed to decode local search response: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no local search results for empty query")
	}
}
