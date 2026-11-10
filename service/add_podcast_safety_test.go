package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/model"
	"gorm.io/gorm"
)

func TestAddPodcastReportsLookupFailuresAsStorageErrors(t *testing.T) {
	setupRetentionTestDB(t)
	errLookup := errors.New("injected lookup failure")
	if err := db.DB.Callback().Query().Before("gorm:query").Register("test:fail_podcast_lookup", func(tx *gorm.DB) {
		if tx.Statement.Table == "podcasts" {
			_ = tx.AddError(errLookup)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.DB.Callback().Query().Remove("test:fail_podcast_lookup") })

	_, err := AddPodcast("https://example.com/feed.xml")
	var exists *model.PodcastAlreadyExistsError
	if errors.As(err, &exists) {
		t.Fatal("a failed lookup was reported as 'already exists'")
	}
	if !errors.Is(err, ErrPodcastStore) || !errors.Is(err, errLookup) {
		t.Fatalf("expected a storage error wrapping the cause, got %v", err)
	}
}

func TestConcurrentAddPodcastItemsStoreEachEpisodeOnce(t *testing.T) {
	setupRetentionTestDB(t)
	pythonPath := requireWorkingPython(t)
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	script := writeTempScript(t, t.TempDir(), "feedparser_concurrent.py", `import json, sys
json.dump({"feed": {"title": "Concurrent"}, "entries": [
  {"guid": "a", "title": "A", "published": "Mon, 01 Jan 2024 00:00:00 GMT"},
  {"guid": "b", "title": "B", "published": "Tue, 02 Jan 2024 00:00:00 GMT"}
]}, sys.stdout)
`)
	t.Setenv(feedparserPythonEnv, pythonPath)
	t.Setenv(feedparserScriptEnv, script)

	podcast := createPodcast(t, "concurrent-add", false)
	podcast.URL = server.URL + "/feed"
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := podcast
			errs <- AddPodcastItems(&p, false)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("AddPodcastItems: %v", err)
		}
	}
	var items []db.PodcastItem
	if err := db.GetAllPodcastItemsByPodcastID(podcast.ID, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 episodes, got %d", len(items))
	}
}
