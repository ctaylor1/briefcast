package controllers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ctaylor1/briefcast/db"
	"github.com/gin-gonic/gin"
)

func patchPosition(t *testing.T, router *gin.Engine, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/podcastitems/"+id+"/position", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func TestPatchPodcastItemPositionSavesOnlyThePosition(t *testing.T) {
	setupControllersTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PATCH("/podcastitems/:id/position", PatchPodcastItemPosition)

	podcast := db.Podcast{Title: "Resume", URL: "https://example.com/resume.xml"}
	if err := db.CreatePodcast(&podcast); err != nil {
		t.Fatal(err)
	}
	item := db.PodcastItem{PodcastID: podcast.ID, GUID: "resume-1", Title: "Episode", IsPlayed: true}
	if err := db.CreatePodcastItem(&item); err != nil {
		t.Fatal(err)
	}

	if resp := patchPosition(t, router, item.ID, `{"positionSeconds": 754.9}`); resp.Code != http.StatusNoContent {
		t.Fatalf("PATCH = %d %s", resp.Code, resp.Body.String())
	}
	var saved db.PodcastItem
	if err := db.DB.First(&saved, "id = ?", item.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.PlaybackPositionSeconds == nil || *saved.PlaybackPositionSeconds != 754 {
		t.Fatalf("position = %v, want 754", saved.PlaybackPositionSeconds)
	}
	if !saved.IsPlayed || saved.Title != "Episode" {
		t.Fatalf("other columns changed: played=%v title=%q", saved.IsPlayed, saved.Title)
	}

	if resp := patchPosition(t, router, item.ID, `{"positionSeconds": 0}`); resp.Code != http.StatusNoContent {
		t.Fatalf("reset PATCH = %d", resp.Code)
	}
	if err := db.DB.First(&saved, "id = ?", item.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.PlaybackPositionSeconds == nil || *saved.PlaybackPositionSeconds != 0 {
		t.Fatalf("position after reset = %v, want 0", saved.PlaybackPositionSeconds)
	}
}

func TestPatchPodcastItemPositionRejectsBadInput(t *testing.T) {
	setupControllersTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PATCH("/podcastitems/:id/position", PatchPodcastItemPosition)

	podcast := db.Podcast{Title: "Resume", URL: "https://example.com/resume2.xml"}
	if err := db.CreatePodcast(&podcast); err != nil {
		t.Fatal(err)
	}
	item := db.PodcastItem{PodcastID: podcast.ID, GUID: "resume-2", Title: "Episode"}
	if err := db.CreatePodcastItem(&item); err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"missing field": `{}`,
		"negative":      `{"positionSeconds": -1}`,
		"too large":     `{"positionSeconds": 604801}`,
		"not a number":  `{"positionSeconds": "12"}`,
		"not JSON":      `twelve`,
	}
	for name, body := range cases {
		if resp := patchPosition(t, router, item.ID, body); resp.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, resp.Code)
		}
	}
	if resp := patchPosition(t, router, "00000000-0000-4000-8000-000000000000", `{"positionSeconds": 5}`); resp.Code != http.StatusNotFound {
		t.Errorf("unknown episode: status %d, want 404", resp.Code)
	}
	var saved db.PodcastItem
	if err := db.DB.First(&saved, "id = ?", item.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.PlaybackPositionSeconds != nil {
		t.Fatalf("rejected requests wrote a position: %v", *saved.PlaybackPositionSeconds)
	}
}
