package service

import "testing"

func TestSearchTranscriptMatchesSegments(t *testing.T) {
	raw := `{"segments":[{"start":12.5,"end":13.2,"text":"Hello world from transcript"}]}`
	results := searchTranscriptMatches(raw, "world", 5)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].TranscriptSnippet == "" {
		t.Fatalf("expected transcript snippet")
	}
	if results[0].StartSeconds == nil || *results[0].StartSeconds != 12.5 {
		t.Fatalf("expected start seconds 12.5, got %v", results[0].StartSeconds)
	}
}

func TestSearchTranscriptMatchesAssetContent(t *testing.T) {
	raw := `[{"url":"https://example.com/t1.vtt","content":"This is a transcript body"}]`
	results := searchTranscriptMatches(raw, "body", 5)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].TranscriptSnippet == "" {
		t.Fatalf("expected transcript snippet")
	}
	if results[0].StartSeconds != nil {
		t.Fatalf("expected no start seconds for asset content")
	}
}
