package controllers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/ctaylor1/briefcast/db"
)

// Each fixed branch used to answer with an empty 200 or carry on after an error.
func TestHandlersAnswerMissingRecordsWithNotFound(t *testing.T) {
	setupControllersTestDB(t)
	router := makeExpandedRouter(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/podcastitems/missing/image"},
		{http.MethodGet, "/podcastitems/missing/file"},
		{http.MethodGet, "/podcasts/missing/image"},
		{http.MethodGet, "/podcasts/missing/rss"},
		{http.MethodGet, "/tags/missing"},
		{http.MethodGet, "/podcastitems/missing/delete"},
	} {
		resp := performRequest(router, tc.method, tc.path, nil, nil)
		if resp.Code != http.StatusNotFound {
			t.Fatalf("%s %s: expected 404, got %d (%s)", tc.method, tc.path, resp.Code, resp.Body.String())
		}
		if !strings.Contains(resp.Body.String(), "not found") {
			t.Fatalf("%s %s: expected an error body, got %q", tc.method, tc.path, resp.Body.String())
		}
	}
}

func TestPatchPodcastItemCanClearValues(t *testing.T) {
	setupControllersTestDB(t)
	router := makeExpandedRouter(t)
	_, item := createControllerPodcastAndItem(t)
	if err := db.UpdatePodcastItemColumns(item.ID, map[string]interface{}{"is_played": true}); err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Content-Type": "application/json"}

	resp := performRequest(router, http.MethodPatch, "/podcastitems/"+item.ID, strings.NewReader(`{"isPlayed":false}`), headers)
	if resp.Code != http.StatusOK {
		t.Fatalf("PATCH isPlayed=false: %d %s", resp.Code, resp.Body.String())
	}
	var got db.PodcastItem
	if err := db.GetPodcastItemByID(item.ID, &got); err != nil {
		t.Fatal(err)
	}
	if got.IsPlayed {
		t.Fatal("isPlayed=false was ignored")
	}
	if got.Title != item.Title {
		t.Fatalf("an absent field changed: title %q", got.Title)
	}

	resp = performRequest(router, http.MethodPatch, "/podcastitems/"+item.ID, strings.NewReader(`{"title":""}`), headers)
	if resp.Code != http.StatusOK {
		t.Fatalf("PATCH title=\"\": %d", resp.Code)
	}
	if err := db.GetPodcastItemByID(item.ID, &got); err != nil {
		t.Fatal(err)
	}
	if got.Title != "" {
		t.Fatalf("title was not cleared: %q", got.Title)
	}
}

// Queuing every episode re-reads the feed; the request must not wait for it.
func TestDownloadAllEpisodesAnswersBeforeFetchingTheFeed(t *testing.T) {
	setupControllersTestDB(t)
	router := makeExpandedRouter(t)
	podcast, _ := createControllerPodcastAndItem(t)
	// An unroutable feed URL: fetching it inside the request would fail or hang.
	if err := db.DB.Model(&db.Podcast{}).Where("id = ?", podcast.ID).Update("url", "http://127.0.0.1:1/feed.xml").Error; err != nil {
		t.Fatal(err)
	}
	resp := performRequest(router, http.MethodGet, "/podcasts/"+podcast.ID+"/download", nil, nil)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d (%s)", resp.Code, resp.Body.String())
	}
}
