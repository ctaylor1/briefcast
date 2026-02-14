package feedmeta

import (
	"strings"
	"testing"
	"time"
)

func TestPickFirstNonEmpty(t *testing.T) {
	if got := PickFirstNonEmpty("", "  ", "value", "next"); got != "value" {
		t.Fatalf("expected value, got %q", got)
	}
	if got := PickFirstNonEmpty("", " "); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestPickLongest(t *testing.T) {
	if got := PickLongest("short", "longer", ""); got != "longer" {
		t.Fatalf("expected longer, got %q", got)
	}
}

func TestExtractEntryShowNotesHTML(t *testing.T) {
	entry := map[string]interface{}{
		"summary": "short",
		"description_detail": map[string]interface{}{
			"value": "<p>this is longer than short</p>",
		},
		"content": []interface{}{
			map[string]interface{}{"value": "<p>rich content is the longest of all show notes</p>"},
		},
	}
	got := ExtractEntryShowNotesHTML(entry)
	if !strings.Contains(got, "longest of all show notes") {
		t.Fatalf("expected longest content, got %q", got)
	}
}

func TestExtractFeedShowNotesHTML(t *testing.T) {
	feed := map[string]interface{}{
		"summary":        "short",
		"itunes_summary": "<p>itunes summary longer</p>",
	}
	got := ExtractFeedShowNotesHTML(feed)
	if got != "<p>itunes summary longer</p>" {
		t.Fatalf("expected itunes summary, got %q", got)
	}
}

func TestExtractPodcastChapters(t *testing.T) {
	entry := map[string]interface{}{
		"podcast_chapters": map[string]interface{}{
			"url":  "https://example.com/chapters.json",
			"type": "application/json",
		},
	}
	url, typ := ExtractPodcastChapters(entry)
	if url != "https://example.com/chapters.json" || typ != "application/json" {
		t.Fatalf("unexpected chapters: %q %q", url, typ)
	}
}

func TestExtractPodcastChaptersFallbacks(t *testing.T) {
	entry := map[string]interface{}{
		"chapters": []interface{}{
			map[string]interface{}{
				"href": "https://example.com/chapters2.json",
				"type": "application/json",
			},
		},
	}
	url, typ := ExtractPodcastChapters(entry)
	if url != "https://example.com/chapters2.json" || typ != "application/json" {
		t.Fatalf("unexpected chapters from list: %q %q", url, typ)
	}
	entry = map[string]interface{}{
		"psc_chapters": "https://example.com/chapters3.json",
	}
	url, typ = ExtractPodcastChapters(entry)
	if url != "https://example.com/chapters3.json" || typ != "" {
		t.Fatalf("unexpected chapters from string: %q %q", url, typ)
	}
}

func TestExtractTranscripts(t *testing.T) {
	entry := map[string]interface{}{
		"podcast_transcript": []interface{}{
			map[string]interface{}{
				"url":      "https://example.com/transcript.vtt",
				"type":     "text/vtt",
				"language": "en",
				"rel":      "captions",
			},
			"https://example.com/transcript2.json",
		},
	}
	assets := ExtractTranscripts(entry)
	if len(assets) != 2 {
		t.Fatalf("expected 2 transcripts, got %d", len(assets))
	}
	if assets[0].URL == "" || assets[1].URL == "" {
		t.Fatalf("expected transcript URLs, got %+v", assets)
	}
}

func TestExtractTranscriptsDedup(t *testing.T) {
	entry := map[string]interface{}{
		"transcripts": []interface{}{
			"https://example.com/t1.vtt",
			map[string]interface{}{"url": "https://example.com/t1.vtt"},
		},
	}
	assets := ExtractTranscripts(entry)
	if len(assets) != 1 {
		t.Fatalf("expected 1 transcript after dedupe, got %d", len(assets))
	}
	if assets[0].URL != "https://example.com/t1.vtt" {
		t.Fatalf("unexpected transcript url %q", assets[0].URL)
	}
}

func TestExtractEnclosureURL(t *testing.T) {
	entry := map[string]interface{}{
		"enclosures": []interface{}{
			map[string]interface{}{"href": "https://cdn.example.com/audio.mp3"},
		},
	}
	if got := ExtractEnclosureURL(entry); got != "https://cdn.example.com/audio.mp3" {
		t.Fatalf("unexpected enclosure url %q", got)
	}

	entry = map[string]interface{}{
		"links": []interface{}{
			map[string]interface{}{
				"rel":  "enclosure",
				"href": "https://cdn.example.com/audio2.mp3",
			},
		},
	}
	if got := ExtractEnclosureURL(entry); got != "https://cdn.example.com/audio2.mp3" {
		t.Fatalf("unexpected enclosure url %q", got)
	}

	entry = map[string]interface{}{
		"link": "https://cdn.example.com/fallback.mp3",
	}
	if got := ExtractEnclosureURL(entry); got != "https://cdn.example.com/fallback.mp3" {
		t.Fatalf("unexpected fallback url %q", got)
	}
}

func TestParseDurationSeconds(t *testing.T) {
	if got := ParseDurationSeconds("90"); got != 90 {
		t.Fatalf("expected 90, got %d", got)
	}
	if got := ParseDurationSeconds("01:02"); got != 62 {
		t.Fatalf("expected 62, got %d", got)
	}
	if got := ParseDurationSeconds("01:02:03"); got != 3723 {
		t.Fatalf("expected 3723, got %d", got)
	}
	if got := ParseDurationSeconds("bad"); got != 0 {
		t.Fatalf("expected 0 on invalid input, got %d", got)
	}
	if got := ParseDurationSeconds("1:2:3:4"); got != 0 {
		t.Fatalf("expected 0 on oversized input, got %d", got)
	}
}

func TestParseFeedTime(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	raw := now.Format(time.RFC3339)
	got := ParseFeedTime(raw)
	if got.IsZero() {
		t.Fatalf("expected parsed time, got zero")
	}
	if parsed := ParseFeedTime("not-a-date"); !parsed.IsZero() {
		t.Fatalf("expected zero time for invalid input")
	}
}

func TestExtractEntryGUID(t *testing.T) {
	entry := map[string]interface{}{
		"id": "entry-id",
	}
	if got := ExtractEntryGUID(entry); got != "entry-id" {
		t.Fatalf("unexpected guid %q", got)
	}
	entry = map[string]interface{}{
		"guid": "entry-guid",
	}
	if got := ExtractEntryGUID(entry); got != "entry-guid" {
		t.Fatalf("unexpected guid %q", got)
	}
}

func TestExtractEntryImage(t *testing.T) {
	entry := map[string]interface{}{
		"image": map[string]interface{}{
			"href": "https://example.com/img.jpg",
		},
	}
	if got := ExtractEntryImage(entry, "fallback"); got != "https://example.com/img.jpg" {
		t.Fatalf("unexpected image %q", got)
	}
	if got := ExtractEntryImage(map[string]interface{}{}, "fallback"); got != "fallback" {
		t.Fatalf("expected fallback, got %q", got)
	}
}

func TestGetStringAndNested(t *testing.T) {
	entry := map[string]interface{}{
		"count": 42.0,
		"image": map[string]interface{}{
			"url": "https://example.com/img.png",
		},
	}
	if got := GetString(entry, "count"); got != "42" && got != "42.0" {
		t.Fatalf("unexpected float conversion %q", got)
	}
	if got := GetNestedString(entry, "image", "url"); got != "https://example.com/img.png" {
		t.Fatalf("unexpected nested string %q", got)
	}
	if got := GetNestedString(entry, "image", "missing"); got != "" {
		t.Fatalf("expected empty for missing nested key, got %q", got)
	}
}

func TestExtractImageURL(t *testing.T) {
	entry := map[string]interface{}{
		"itunes_image": "https://example.com/itunes.png",
	}
	if got := ExtractImageURL(entry); got != "https://example.com/itunes.png" {
		t.Fatalf("unexpected image url %q", got)
	}
}

func TestMarshalMetadata(t *testing.T) {
	raw := map[string]interface{}{"key": "value"}
	if got := MarshalMetadata(raw); !strings.Contains(got, "key") {
		t.Fatalf("expected json output, got %q", got)
	}
}
