package db

import (
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

// captureQueries records the SQL of every query on podcast_items run by fn.
func captureQueries(t *testing.T, fn func()) []string {
	t.Helper()
	var (
		mu      sync.Mutex
		queries []string
	)
	name := "test:capture_" + strings.ReplaceAll(t.Name(), "/", "_")
	if err := DB.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "podcast_items" {
			mu.Lock()
			queries = append(queries, tx.Statement.SQL.String())
			mu.Unlock()
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = DB.Callback().Query().Remove(name) }()
	fn()
	return queries
}

func requireNoHeavyColumns(t *testing.T, label string, queries []string, heavy ...string) {
	t.Helper()
	if len(queries) == 0 {
		t.Fatalf("%s: no podcast_items query captured", label)
	}
	for _, q := range queries {
		if strings.Contains(q, "SELECT *") {
			t.Fatalf("%s: query loads every column: %s", label, q)
		}
		for _, column := range heavy {
			if strings.Contains(q, column) {
				t.Fatalf("%s: query reads %s: %s", label, column, q)
			}
		}
	}
}

func TestHotPathQueriesDoNotLoadTranscripts(t *testing.T) {
	setupDBForTest(t)
	podcast := newPodcast(t, "Shape podcast", "https://example.com/shape.xml")
	item := newPodcastItem(t, podcast.ID, "shape-guid", "Shape episode", Downloaded, time.Now().UTC())
	transcriptColumns := []string{"transcript_json", "canonical_transcript", "llm_summary", "summary_provenance_json", "transcript_checkpoint_json"}

	media := captureQueries(t, func() {
		if _, err := GetPodcastItemMediaByID(item.ID); err != nil {
			t.Fatal(err)
		}
	})
	requireNoHeavyColumns(t, "media", media, transcriptColumns...)

	retention := captureQueries(t, func() {
		var items []PodcastItem
		if err := FindDownloadedPodcastItems(&items); err != nil || len(items) != 1 {
			t.Fatalf("retention list: %v %d", err, len(items))
		}
	})
	requireNoHeavyColumns(t, "retention", retention, transcriptColumns...)

	sizes := captureQueries(t, func() {
		if _, err := GetPodcastItemsWithoutSize(10); err != nil {
			t.Fatal(err)
		}
	})
	requireNoHeavyColumns(t, "file sizes", sizes, transcriptColumns...)

	// Search must read transcripts to match them, but nothing else heavy.
	search := captureQueries(t, func() {
		var items []PodcastItem
		if err := SearchPodcastItemsByLike("%shape%", 5, &items); err != nil || len(items) != 1 {
			t.Fatalf("search: %v %d", err, len(items))
		}
		if items[0].Podcast.Title != podcast.Title {
			t.Fatalf("search lost the podcast title: %q", items[0].Podcast.Title)
		}
	})
	requireNoHeavyColumns(t, "search", search, "canonical_transcript", "summary_provenance_json", "transcript_checkpoint_json", "llm_summary_prompt")
}

func TestGetPodcastItemsWithoutSizeIsBounded(t *testing.T) {
	setupDBForTest(t)
	podcast := newPodcast(t, "Size podcast", "https://example.com/size.xml")
	for i := 0; i < 5; i++ {
		newPodcastItem(t, podcast.ID, "size-"+string(rune('a'+i)), "Size episode", NotDownloaded, time.Now().UTC().Add(-time.Duration(i)*time.Hour))
	}
	items, err := GetPodcastItemsWithoutSize(3)
	if err != nil {
		t.Fatal(err)
	}
	if len(*items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(*items))
	}
	all, err := GetAllPodcastItemsWithoutSize()
	if err != nil || len(*all) != 5 {
		t.Fatalf("unbounded list: %v %d", err, len(*all))
	}
}
