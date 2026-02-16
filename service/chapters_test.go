package service

import (
	"testing"

	"github.com/ctaylor1/briefcast/db"
)

func TestBuildChapterResponseFromFeed(t *testing.T) {
	item := db.PodcastItem{
		ChaptersJSON: `{"chapters":[{"title":"Intro","startTime":10,"endTime":20},{"name":"Topic","start_time":"01:30"}]}`,
	}

	response := BuildChapterResponse(item)
	if response.Source != "feed" {
		t.Fatalf("expected source feed, got %q", response.Source)
	}
	if len(response.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(response.Chapters))
	}
	if response.Chapters[0].Title != "Intro" || response.Chapters[0].StartSeconds != 10 {
		t.Fatalf("unexpected first chapter: %+v", response.Chapters[0])
	}
	if response.Chapters[1].Title != "Topic" || response.Chapters[1].StartSeconds != 90 {
		t.Fatalf("unexpected second chapter: %+v", response.Chapters[1])
	}
}

func TestBuildChapterResponseFromID3Fallback(t *testing.T) {
	item := db.PodcastItem{
		ID3ChaptersJSON: `[{"title":"From ID3","start_time_ms":5000}]`,
	}

	response := BuildChapterResponse(item)
	if response.Source != "id3" {
		t.Fatalf("expected source id3, got %q", response.Source)
	}
	if len(response.Chapters) != 1 {
		t.Fatalf("expected 1 chapter, got %d", len(response.Chapters))
	}
	if response.Chapters[0].Title != "From ID3" || response.Chapters[0].StartSeconds != 5 {
		t.Fatalf("unexpected chapter: %+v", response.Chapters[0])
	}
}
