package controllers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ctaylor1/briefcast/db"
	"github.com/gin-gonic/gin"
)

func TestRetranscribePodcastItem(t *testing.T) {
	setupControllersTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/podcastitems/:id/retranscribe", RetranscribePodcastItem)

	podcast := db.Podcast{Title: "Retranscribe", URL: "https://feeds.example.com/retranscribe.xml"}
	if err := db.CreatePodcast(&podcast); err != nil {
		t.Fatalf("create podcast: %v", err)
	}
	item := db.PodcastItem{PodcastID: podcast.ID, Title: "Episode", DownloadStatus: db.Downloaded, DownloadPath: "/assets/e.mp3", TranscriptStatus: "available", TranscriptJSON: `[{"text":"x"}]`}
	if err := db.CreatePodcastItem(&item); err != nil {
		t.Fatalf("create item: %v", err)
	}

	post := func(id string) int {
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/podcastitems/"+id+"/retranscribe", nil))
		return resp.Code
	}
	if code := post(item.ID); code != http.StatusAccepted {
		t.Fatalf("first retranscribe = %d, want 202", code)
	}
	if code := post(item.ID); code != http.StatusConflict {
		t.Fatalf("already queued = %d, want 409", code)
	}
	if code := post("00000000-0000-0000-0000-000000000000"); code != http.StatusNotFound {
		t.Fatalf("missing episode = %d, want 404", code)
	}
}
