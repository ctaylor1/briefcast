package id3meta

import "testing"

func TestShouldExtract(t *testing.T) {
	if !ShouldExtract("", "", "") {
		t.Fatalf("expected true when all empty")
	}
	if ShouldExtract("chapters", "", "") {
		t.Fatalf("expected false when chapters present")
	}
	if ShouldExtract("", "tags", "") {
		t.Fatalf("expected false when tags present")
	}
	if ShouldExtract("", "", "chapters") {
		t.Fatalf("expected false when id3 chapters present")
	}
}

func TestSplitRawEmpty(t *testing.T) {
	tags, chapters, hasTags, hasChapters, err := SplitRaw(nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if tags != "" || chapters != "" || hasTags || hasChapters {
		t.Fatalf("expected empty output")
	}
}

func TestSplitRawInvalid(t *testing.T) {
	_, _, _, _, err := SplitRaw([]byte("{bad json"))
	if err == nil {
		t.Fatalf("expected error for invalid json")
	}
}

func TestSplitRawTagsOnly(t *testing.T) {
	raw := []byte(`{"tags":{"TIT2":["Episode"]},"chapters":[]}`)
	tags, chapters, hasTags, hasChapters, err := SplitRaw(raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !hasTags || tags == "" {
		t.Fatalf("expected tags output")
	}
	if hasChapters || chapters != "" {
		t.Fatalf("expected no chapters output")
	}
}

func TestSplitRawChaptersOnly(t *testing.T) {
	raw := []byte(`{"tags":{},"chapters":[{"id":"ch1","start_time_ms":0}]}`)
	tags, chapters, hasTags, hasChapters, err := SplitRaw(raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if hasTags || tags != "" {
		t.Fatalf("expected no tags output")
	}
	if !hasChapters || chapters == "" {
		t.Fatalf("expected chapters output")
	}
}

func TestSplitRawBoth(t *testing.T) {
	raw := []byte(`{"tags":{"TIT2":["Episode"]},"chapters":[{"id":"ch1"}]}`)
	tags, chapters, hasTags, hasChapters, err := SplitRaw(raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !hasTags || tags == "" {
		t.Fatalf("expected tags output")
	}
	if !hasChapters || chapters == "" {
		t.Fatalf("expected chapters output")
	}
}
