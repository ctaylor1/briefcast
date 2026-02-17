package model

import "testing"

func TestPodcastAlreadyExistsError(t *testing.T) {
	err := &PodcastAlreadyExistsError{Url: "https://example.com/feed.xml"}
	if err.Error() != "Podcast with this url already exists" {
		t.Fatalf("unexpected error text: %q", err.Error())
	}
}

func TestTagAlreadyExistsError(t *testing.T) {
	err := &TagAlreadyExistsError{Label: "news"}
	expected := "Tag with this label already exists : news"
	if err.Error() != expected {
		t.Fatalf("expected %q, got %q", expected, err.Error())
	}
}
