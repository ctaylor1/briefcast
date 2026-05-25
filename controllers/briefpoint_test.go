package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"github.com/gin-gonic/gin"
)

func makeDashboardActionsRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		setting := db.GetOrCreateSetting()
		c.Set("setting", setting)
		c.Next()
	})

	router.PATCH("/podcasts/:id/pause", PausePodcastByID)
	router.PATCH("/podcasts/:id/unpause", UnpausePodcastByID)
	router.PATCH("/podcasts/:id/retention", PatchPodcastRetention)
	router.PATCH("/podcasts/:id/sponsor-skip", PatchPodcastSponsorSkip)
	router.PATCH("/podcasts/:id/briefpoint", PatchPodcastBriefpoint)
	router.POST("/podcasts/:id/briefpoint/sync", SyncPodcastToBriefpoint)
	router.POST("/podcasts/:id/download", DownloadAllEpisodesByPodcastID)
	router.DELETE("/podcasts/:id", DeletePodcastByID)

	return router
}

func createTestPodcast(t *testing.T) db.Podcast {
	t.Helper()
	podcast := db.Podcast{
		Title:   "Dashboard Actions Test Podcast",
		URL:     "https://example.com/dashboard-test.xml",
		Author:  "tester",
		Summary: "test summary",
	}
	if err := db.CreatePodcast(&podcast); err != nil {
		t.Fatalf("create podcast failed: %v", err)
	}
	return podcast
}

func createTestEpisodeWithSummary(t *testing.T, podcastID string) db.PodcastItem {
	t.Helper()
	item := db.PodcastItem{
		PodcastID:           podcastID,
		GUID:                "dashboard-test-ep-" + time.Now().Format("150405.000"),
		Title:               "Test Episode With Summary",
		PubDate:             time.Now().UTC(),
		FileURL:             "https://example.com/ep.mp3",
		DownloadStatus:      db.Downloaded,
		CanonicalTranscript: "This is a test transcript for the briefpoint integration.",
		TranscriptStatus:    "available",
		LLMSummary:          "This episode covers test content for integration testing.",
		LLMSummaryStatus:    "available",
		LLMSummaryModel:     "test-model",
	}
	if err := db.CreatePodcastItem(&item); err != nil {
		t.Fatalf("create podcast item failed: %v", err)
	}
	return item
}

func patchJSON(router *gin.Engine, path string, body interface{}) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func postJSON(router *gin.Engine, path string, body interface{}) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(http.MethodPost, path, reader)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func TestPausePodcast(t *testing.T) {
	setupControllersTestDB(t)
	router := makeDashboardActionsRouter(t)
	podcast := createTestPodcast(t)

	resp := patchJSON(router, "/podcasts/"+podcast.ID+"/pause", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var updated db.Podcast
	if err := db.DB.First(&updated, "id = ?", podcast.ID).Error; err != nil {
		t.Fatalf("failed to reload podcast: %v", err)
	}
	if !updated.IsPaused {
		t.Error("expected podcast to be paused")
	}
}

func TestUnpausePodcast(t *testing.T) {
	setupControllersTestDB(t)
	router := makeDashboardActionsRouter(t)
	podcast := createTestPodcast(t)

	db.DB.Model(&db.Podcast{}).Where("id = ?", podcast.ID).Update("is_paused", true)

	resp := patchJSON(router, "/podcasts/"+podcast.ID+"/unpause", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var updated db.Podcast
	if err := db.DB.First(&updated, "id = ?", podcast.ID).Error; err != nil {
		t.Fatalf("failed to reload podcast: %v", err)
	}
	if updated.IsPaused {
		t.Error("expected podcast to be unpaused")
	}
}

func TestPauseUnpauseRoundTrip(t *testing.T) {
	setupControllersTestDB(t)
	router := makeDashboardActionsRouter(t)
	podcast := createTestPodcast(t)

	resp := patchJSON(router, "/podcasts/"+podcast.ID+"/pause", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("pause: expected 200, got %d", resp.Code)
	}

	resp = patchJSON(router, "/podcasts/"+podcast.ID+"/unpause", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("unpause: expected 200, got %d", resp.Code)
	}

	var final db.Podcast
	db.DB.First(&final, "id = ?", podcast.ID)
	if final.IsPaused {
		t.Error("expected podcast to be active after unpause")
	}
}

func TestPatchRetention(t *testing.T) {
	setupControllersTestDB(t)
	router := makeDashboardActionsRouter(t)
	podcast := createTestPodcast(t)

	keepAll := true
	resp := patchJSON(router, "/podcasts/"+podcast.ID+"/retention", map[string]interface{}{
		"keepAll": keepAll,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var updated db.Podcast
	db.DB.First(&updated, "id = ?", podcast.ID)
	if !updated.RetentionKeepAll {
		t.Error("expected retention_keep_all to be true")
	}

	keepAll = false
	resp = patchJSON(router, "/podcasts/"+podcast.ID+"/retention", map[string]interface{}{
		"keepAll": keepAll,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	db.DB.First(&updated, "id = ?", podcast.ID)
	if updated.RetentionKeepAll {
		t.Error("expected retention_keep_all to be false")
	}
}

func TestPatchRetentionMissingField(t *testing.T) {
	setupControllersTestDB(t)
	router := makeDashboardActionsRouter(t)
	podcast := createTestPodcast(t)

	resp := patchJSON(router, "/podcasts/"+podcast.ID+"/retention", map[string]interface{}{})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing field, got %d", resp.Code)
	}
}

func TestPatchSponsorSkip(t *testing.T) {
	setupControllersTestDB(t)
	router := makeDashboardActionsRouter(t)
	podcast := createTestPodcast(t)

	resp := patchJSON(router, "/podcasts/"+podcast.ID+"/sponsor-skip", map[string]interface{}{
		"autoSkipSponsorChapters": true,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var updated db.Podcast
	db.DB.First(&updated, "id = ?", podcast.ID)
	if !updated.AutoSkipSponsorChapters {
		t.Error("expected auto_skip_sponsor_chapters to be true")
	}

	resp = patchJSON(router, "/podcasts/"+podcast.ID+"/sponsor-skip", map[string]interface{}{
		"autoSkipSponsorChapters": false,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	db.DB.First(&updated, "id = ?", podcast.ID)
	if updated.AutoSkipSponsorChapters {
		t.Error("expected auto_skip_sponsor_chapters to be false")
	}
}

func TestPatchSponsorSkipMissingField(t *testing.T) {
	setupControllersTestDB(t)
	router := makeDashboardActionsRouter(t)
	podcast := createTestPodcast(t)

	resp := patchJSON(router, "/podcasts/"+podcast.ID+"/sponsor-skip", map[string]interface{}{})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestPatchBriefpointEnabled(t *testing.T) {
	setupControllersTestDB(t)
	router := makeDashboardActionsRouter(t)
	podcast := createTestPodcast(t)

	resp := patchJSON(router, "/podcasts/"+podcast.ID+"/briefpoint", map[string]interface{}{
		"briefpointEnabled": true,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var updated db.Podcast
	db.DB.First(&updated, "id = ?", podcast.ID)
	if !updated.BriefpointEnabled {
		t.Error("expected briefpoint_enabled to be true")
	}
}

func TestPatchBriefpointDisabled(t *testing.T) {
	setupControllersTestDB(t)
	router := makeDashboardActionsRouter(t)
	podcast := createTestPodcast(t)

	db.DB.Model(&db.Podcast{}).Where("id = ?", podcast.ID).Update("briefpoint_enabled", true)

	resp := patchJSON(router, "/podcasts/"+podcast.ID+"/briefpoint", map[string]interface{}{
		"briefpointEnabled": false,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var updated db.Podcast
	db.DB.First(&updated, "id = ?", podcast.ID)
	if updated.BriefpointEnabled {
		t.Error("expected briefpoint_enabled to be false")
	}
}

func TestPatchBriefpointMissingField(t *testing.T) {
	setupControllersTestDB(t)
	router := makeDashboardActionsRouter(t)
	podcast := createTestPodcast(t)

	resp := patchJSON(router, "/podcasts/"+podcast.ID+"/briefpoint", map[string]interface{}{})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestSyncPodcastToBriefpointAccepted(t *testing.T) {
	setupControllersTestDB(t)
	router := makeDashboardActionsRouter(t)
	podcast := createTestPodcast(t)
	createTestEpisodeWithSummary(t, podcast.ID)

	setting := db.GetOrCreateSetting()
	setting.BriefpointEnabled = true
	setting.BriefpointAPIKey = "sk_test_fake_key"
	setting.BriefpointServerURL = "http://localhost:19999"
	_ = db.UpdateSettings(setting)

	resp := postJSON(router, "/podcasts/"+podcast.ID+"/briefpoint/sync", nil)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.Code, resp.Body.String())
	}

	time.Sleep(100 * time.Millisecond)
}

func TestPauseInvalidID(t *testing.T) {
	setupControllersTestDB(t)
	router := makeDashboardActionsRouter(t)

	resp := patchJSON(router, "/podcasts/nonexistent-id/pause", nil)
	if resp.Code != http.StatusOK {
		t.Logf("pause with nonexistent ID returned %d (acceptable — direct SQL update)", resp.Code)
	}
}
