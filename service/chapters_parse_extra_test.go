package service

import "testing"

func TestParseChaptersInvalidAndObjectForms(t *testing.T) {
	if got := parseChapters("not-json"); got != nil {
		t.Fatalf("expected nil for invalid json")
	}

	raw := `{"chapters":[{"name":"Topic A","start_time_ms":5000},{"start":"00:10"}]}`
	chapters := parseChapters(raw)
	if len(chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(chapters))
	}
	if chapters[0].Title != "Topic A" || chapters[0].StartSeconds != 5 {
		t.Fatalf("unexpected first chapter: %+v", chapters[0])
	}
	if chapters[1].Title != "Chapter 2" || chapters[1].StartSeconds != 10 {
		t.Fatalf("unexpected second chapter: %+v", chapters[1])
	}
}

func TestParseChapterListSortsByStart(t *testing.T) {
	input := []interface{}{
		map[string]interface{}{"title": "B", "start": float64(20)},
		map[string]interface{}{"title": "A", "start": float64(10)},
	}

	chapters := parseChapterList(input)
	if len(chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(chapters))
	}
	if chapters[0].Title != "A" || chapters[1].Title != "B" {
		t.Fatalf("expected sorted chapters, got %+v", chapters)
	}
}

func TestParseTimeHelpers(t *testing.T) {
	if got := parseTimeString("01:02:03", false); got != 3723 {
		t.Fatalf("expected 3723, got %v", got)
	}
	if got := parseTimeString("2000", true); got != 2 {
		t.Fatalf("expected 2 seconds from milliseconds, got %v", got)
	}
	if got := parseTimeString("bad", false); got != -1 {
		t.Fatalf("expected -1 for invalid time string, got %v", got)
	}

	if got := parseTimeValue(map[string]interface{}{"value": "00:30"}, false); got != 30 {
		t.Fatalf("expected 30 from map value, got %v", got)
	}
	if got := parseTimeValue([]interface{}{"00:45"}, false); got != 45 {
		t.Fatalf("expected 45 from list first value, got %v", got)
	}
	if got := parseTimeValue([]interface{}{}, false); got != -1 {
		t.Fatalf("expected -1 for empty list, got %v", got)
	}
}
