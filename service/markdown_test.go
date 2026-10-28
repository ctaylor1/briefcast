package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
	pm "github.com/ctaylor1/briefcast/internal/podcastmarkdown"
)

func markdownTestItem(t *testing.T) db.PodcastItem {
	t.Helper()
	setupRetentionTestDB(t)
	podcast := createPodcast(t, "Test Podcast", false)
	podcast.FeedMetadata = `{"language":"en","author":"Not necessarily a host"}`
	if err := db.DB.Save(&podcast).Error; err != nil {
		t.Fatal(err)
	}
	item := createServicePodcastItem(t, podcast, "Episode: #1", db.Downloaded)
	item.Podcast = podcast
	item.ItemMetadata = `{"link":"https://example.com/episode","itunes_episode":"001","hosts":["Known host"],"guests":[]}`
	item.FileURL = "https://example.com/audio.mp3"
	item.GUID = "publisher-guid"
	item.Duration = 120
	item.PubDate = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	item.TranscriptJSON = `{"segments":[{"start":1.25,"end":2.5,"text":"A qualified claim.","speaker":"SPEAKER_01"}]}`
	item.CanonicalTranscript = "SPEAKER_01: A qualified claim."
	item.CanonicalTranscriptVersion = canonicalTranscriptVersionCurrent
	item.LLMSummary = "## Takeaways\n\nA qualified claim."
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatal(err)
	}
	return item
}

func markdownTestProvenance(t *testing.T, item *db.PodcastItem, kind string, meta pm.Metadata) pm.Metadata {
	t.Helper()
	full, err := readMarkdownProvenance(meta, markdownDirectory(item, kind))
	if err != nil {
		t.Fatal(err)
	}
	return full
}

func TestMarkdownLegacyDoesNotInventProductionHistory(t *testing.T) {
	item := markdownTestItem(t)
	t.Setenv("LLM_MODEL", "today-is-not-history")
	t.Setenv("WHISPERX_MODEL", "today-is-not-history")
	transcript, err := EpisodeMarkdown(&item, "transcript")
	if err != nil {
		t.Fatal(err)
	}
	meta, body, err := pm.Parse(transcript)
	if err != nil {
		t.Fatal(err)
	}
	if err := pm.Validate(meta); err != nil {
		t.Fatal(err)
	}
	if meta["transcription_method"] != "unknown" || meta["transcribed_at"] != nil || meta["transcription_model"] != nil {
		t.Fatalf("invented legacy ASR: %#v", meta)
	}
	if meta["source_url"] != "https://example.com/episode" || meta["audio_url"] != item.FileURL || meta["episode_number"] != "001" {
		t.Fatalf("lost publisher metadata: %#v", meta)
	}
	if body != item.CanonicalTranscript || meta["timestamp_granularity"] != "none" {
		t.Fatalf("transcript body is not the plain canonical text: %s", body)
	}
	if meta["provenance_status"] != "partial" || meta["hosts"] == nil || meta["guests"] == nil {
		t.Fatalf("incorrect metadata: %#v", meta)
	}
	summary, err := EpisodeMarkdown(&item, "summary")
	if err != nil {
		t.Fatal(err)
	}
	sm, _, _ := pm.Parse(summary)
	if sm["episode_id"] != meta["episode_id"] || sm["document_id"] == meta["document_id"] {
		t.Fatal("incorrect shared episode/document identity")
	}
	if sm["summarized_at"] != nil || sm["model_requested"] != nil || len(sm["inputs"].([]any)) != 0 {
		t.Fatalf("invented summary provenance: %#v", sm)
	}
}

func TestMarkdownTranscriptBodyIsPlainText(t *testing.T) {
	for _, tc := range []struct {
		name string
		item db.PodcastItem
		want string
	}{
		{
			name: "current canonical text preserves paragraphs and literal punctuation",
			item: db.PodcastItem{CanonicalTranscript: "SPEAKER_01: Cost is $5 * 2; see [notes], #topic, <example>, C:\\audio.\n\nSecond paragraph.", CanonicalTranscriptVersion: canonicalTranscriptVersionCurrent},
			want: "SPEAKER_01: Cost is $5 * 2; see [notes], #topic, <example>, C:\\audio.\n\nSecond paragraph.",
		},
		{
			name: "raw timed segments have no added markers or timestamps",
			item: db.PodcastItem{TranscriptJSON: `{"segments":[{"start":5.361,"end":6.602,"text":"We got a lot to get to."},{"start":6.802,"end":8.003,"text":"Saks is out this week."}]}`},
			want: "We got a lot to get to.\n\nSaks is out this week.",
		},
		{
			name: "manual corrections take precedence over raw ASR",
			item: db.PodcastItem{CanonicalTranscript: "Corrected text.", CanonicalTranscriptVersion: canonicalTranscriptVersionCurrent, TranscriptJSON: `{"segments":[{"start":1,"text":"Uncorrected text."}]}`},
			want: "Corrected text.",
		},
		{
			name: "legacy canonical fallback stays plain",
			item: db.PodcastItem{CanonicalTranscript: "A legacy paragraph.\n\nAnother paragraph."},
			want: "A legacy paragraph.\n\nAnother paragraph.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, granularity := markdownTranscriptBody(&tc.item)
			if body != tc.want || granularity != "none" {
				t.Fatalf("unexpected transcript body %q (granularity %q)", body, granularity)
			}
		})
	}
}

func TestMarkdownRevisionIsIdempotentAndSurvivesRename(t *testing.T) {
	item := markdownTestItem(t)
	first, err := EpisodeMarkdown(&item, "transcript")
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := EpisodeMarkdown(&item, "transcript")
	if err != nil || repeated != first {
		t.Fatal("unchanged export created a new revision", err)
	}
	item.Title = "Renamed episode"
	second, err := EpisodeMarkdown(&item, "transcript")
	if err != nil {
		t.Fatal(err)
	}
	a, _, _ := pm.Parse(first)
	b, _, _ := pm.Parse(second)
	if a["document_id"] != b["document_id"] || b["document_revision"] != float64(2) || a["created"] != b["created"] || a["generation_id"] == b["generation_id"] {
		t.Fatalf("bad revision identity: %#v", b)
	}
	var count int64
	db.DB.Model(&db.MarkdownRevision{}).Where("episode_id = ?", item.ID).Count(&count)
	if count != 2 {
		t.Fatalf("lost revision history: %d", count)
	}
}

func TestMarkdownNeverOverwritesLegacyOrEditedFiles(t *testing.T) {
	item := markdownTestItem(t)
	dir := filepath.Join(resolveExportDir(), assetMarkdownTranscriptDir, sanitizeAssetName(item.Podcast.Title), pubDateSubdir(item.PubDate))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, exportEpisodeFileName(&item)+".md")
	original := "---\ncustom_field: preserved\n---\n\nOriginal user content"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeEpisodeMarkdown(&item, "transcript"); err != nil {
		t.Fatal(err)
	}
	if err := writeEpisodeMarkdown(&item, "transcript"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != original {
		t.Fatal("overwrote original file")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	if len(files) != 2 {
		t.Fatalf("expected original and one revision, got %v", files)
	}
	latest, err := EpisodeMarkdown(&item, "transcript")
	if err != nil {
		t.Fatal(err)
	}
	meta, _, _ := pm.Parse(latest)
	if meta["custom_field"] != "preserved" || meta["document_revision"] != float64(1) {
		t.Fatal("legacy metadata was lost or re-export was not idempotent")
	}
}

func TestSummaryInputSnapshotMatchesRequestAndCoverage(t *testing.T) {
	// Full provider request is checked in summarize_test.go; this checks the
	// byte-boundary truncation used by both request and provenance capture.
	text := strings.Repeat("a", maxTranscriptChars-1) + "é" + "tail"
	prepared, count := preparedSummaryTranscript(text)
	if !strings.HasSuffix(prepared, "[Transcript truncated due to length]") || count != maxTranscriptChars-1 {
		t.Fatalf("invalid UTF-8 boundary: %d", count)
	}
	encoded, err := json.Marshal(prepared)
	if err != nil || strings.Contains(string(encoded), "�") {
		t.Fatal("invalid UTF-8 snapshot", err)
	}
}

func TestMarkdownRetainsUnfamiliarMetadataFromEditedExport(t *testing.T) {
	item := markdownTestItem(t)
	if err := writeEpisodeMarkdown(&item, "transcript"); err != nil {
		t.Fatal(err)
	}
	var revision db.MarkdownRevision
	if err := db.DB.Where("episode_id = ?", item.ID).First(&revision).Error; err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(resolveExportDir(), filepath.FromSlash(revision.ExportPath))
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(original), "---\n", "---\ncustom_label: \"keep this\"\n", 1)
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeEpisodeMarkdown(&item, "transcript"); err != nil {
		t.Fatal(err)
	}
	latest, err := EpisodeMarkdown(&item, "transcript")
	if err != nil {
		t.Fatal(err)
	}
	meta, _, _ := pm.Parse(latest)
	if meta["custom_label"] != "keep this" || meta["verification_status"] != "unreviewed" || meta["document_revision"] != float64(2) {
		t.Fatalf("lost unfamiliar metadata: %#v", meta)
	}
	untouched, _ := os.ReadFile(path)
	if string(untouched) != edited {
		t.Fatal("modified external input")
	}
}

func TestMarkdownCompleteCheckpointDoesNotInventNewASR(t *testing.T) {
	item := markdownTestItem(t)
	item.TranscriptModel = "current-but-not-performed"
	item.TranscriptJSON = `{"provider":"whisperx","model":"current-but-not-performed","segments":[{"text":"Earlier checkpoint."}],"metadata":{"asr_performed":false,"transcribed_at":null,"whisperx_version":"current-runtime"},"provenance":{"production_origin":"captured_postprocessing","inputs":[],"inherited_provenance_missing":["resumed_asr_history"]}}`
	content, err := EpisodeMarkdown(&item, "transcript")
	if err != nil {
		t.Fatal(err)
	}
	meta, _, _ := pm.Parse(content)
	for _, key := range []string{"transcribed_at", "transcription_model", "transcription_engine_version"} {
		if meta[key] != nil {
			t.Fatalf("invented checkpoint history %s", key)
		}
	}
	if meta["provenance_status"] != "partial" {
		t.Fatal("unknown checkpoint history was not declared")
	}
}

func TestMarkdownLongTitlePreservesOriginalAndCreatesValidRevision(t *testing.T) {
	item := markdownTestItem(t)
	item.Title = strings.Repeat("Long title ", 21)
	dir := filepath.Join(resolveExportDir(), assetMarkdownTranscriptDir, sanitizeAssetName(item.Podcast.Title), pubDateSubdir(item.PubDate))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	originalPath := filepath.Join(dir, exportEpisodeFileName(&item)+".md")
	if err := os.WriteFile(originalPath, []byte("Original legacy note"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeEpisodeMarkdown(&item, "transcript"); err != nil {
		t.Fatal(err)
	}
	var revision db.MarkdownRevision
	if err := db.DB.Where("episode_id = ?", item.ID).First(&revision).Error; err != nil {
		t.Fatal(err)
	}
	if len(filepath.Base(revision.ExportPath)) > 255 {
		t.Fatal("revision filename exceeds filesystem limit")
	}
	meta, _, err := pm.Parse(revision.Content)
	if err != nil || meta["episode_title"] != item.Title {
		t.Fatal("lost full episode title", err)
	}
	original, _ := os.ReadFile(originalPath)
	if string(original) != "Original legacy note" {
		t.Fatal("overwrote original note")
	}
	if _, err := os.Stat(filepath.Join(resolveExportDir(), filepath.FromSlash(revision.ExportPath))); err != nil {
		t.Fatal(err)
	}
}

func TestMarkdownPayloadsStayOutOfFrontmatter(t *testing.T) {
	for _, kind := range []string{"transcript", "summary"} {
		t.Run(kind, func(t *testing.T) {
			item := markdownTestItem(t)
			raw := map[string]any{"segments": []any{map[string]any{"start": 1.25, "end": 2.5, "text": "A qualified claim.", "speaker": "SPEAKER_01"}}, "raw_words": strings.Repeat("RAW_ASR_PAYLOAD ", 140000)}
			bytes, _ := json.Marshal(raw)
			item.TranscriptJSON = string(bytes)
			content, err := EpisodeMarkdown(&item, kind)
			if err != nil {
				t.Fatal(err)
			}
			end := strings.Index(content[4:], "\n---\n") + 4
			if end < 4 || end > 20000 || strings.Contains(content[:end], "RAW_ASR_PAYLOAD") || strings.Contains(content[:end], "A qualified claim.") {
				t.Fatal("document content leaked into oversized YAML frontmatter")
			}
			meta, body, err := pm.Parse(content)
			if err != nil || pm.Validate(meta) != nil || !strings.Contains(body, "A qualified claim.") {
				t.Fatal("invalid metadata or missing readable Markdown body", err)
			}
			if meta["input_snapshots"] != nil || meta["processing_events"] != nil {
				t.Fatal("detailed provenance remains in frontmatter")
			}
			full := markdownTestProvenance(t, &item, kind, meta)
			if len(full["processing_events"].([]any)) == 0 {
				t.Fatal("lost processing history")
			}
			if _, err := os.Stat(filepath.Join(markdownDirectory(&item, kind), "inputs")); !os.IsNotExist(err) {
				t.Fatal("created companions in Markdown directory")
			}
			for hash, value := range full["input_snapshots"].(map[string]any) {
				ref, ok := value.(map[string]any)
				if !ok || ref["sha256"] != hash {
					t.Fatal("snapshot payload was embedded instead of referenced")
				}
				payload, err := os.ReadFile(filepath.Join(markdownProvenanceDir(), filepath.FromSlash(ref["uri"].(string))))
				if err != nil || pm.Hash(payload) != hash {
					t.Fatal("missing exact input companion", err)
				}
			}
		})
	}
}

func TestMarkdownUpgradesEmbeddedSnapshotsWithoutChangingBodyOrOriginal(t *testing.T) {
	item := markdownTestItem(t)
	if err := writeEpisodeMarkdown(&item, "transcript"); err != nil {
		t.Fatal(err)
	}
	var previous db.MarkdownRevision
	if err := db.DB.Where("episode_id = ?", item.ID).First(&previous).Error; err != nil {
		t.Fatal(err)
	}
	old, body, _ := pm.Parse(previous.Content)
	old = markdownTestProvenance(t, &item, "transcript", old)
	old["generator_version"] = "1.2.0"
	for hash := range old["input_snapshots"].(map[string]any) {
		payload, err := os.ReadFile(filepath.Join(markdownProvenanceDir(), "inputs", hash+".txt"))
		if err != nil {
			t.Fatal(err)
		}
		old["input_snapshots"].(map[string]any)[hash] = string(payload)
	}
	oldContent, err := pm.Encode(old, body)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(resolveExportDir(), filepath.FromSlash(previous.ExportPath))
	if err := os.WriteFile(path, []byte(oldContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Model(&previous).Updates(map[string]any{"content": oldContent, "fingerprint": "previous-renderer"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := writeEpisodeMarkdown(&item, "transcript"); err != nil {
		t.Fatal(err)
	}
	latest, err := EpisodeMarkdown(&item, "transcript")
	if err != nil {
		t.Fatal(err)
	}
	meta, newBody, _ := pm.Parse(latest)
	untouched, _ := os.ReadFile(path)
	if string(untouched) != oldContent || newBody != body || meta["document_id"] != old["document_id"] || meta["document_revision"] != float64(2) {
		t.Fatal("upgrade overwrote the original or changed its transcript/identity")
	}
}

func TestMarkdownHistoricalCompanionsSurviveDirectoryChange(t *testing.T) {
	item := markdownTestItem(t)
	// Exercise UI-only preparation: no .md ExportPath has been recorded yet.
	first, err := EpisodeMarkdown(&item, "summary")
	if err != nil {
		t.Fatal(err)
	}
	old, _, _ := pm.Parse(first)
	old = markdownTestProvenance(t, &item, "summary", old)
	item.Podcast.Title = "Renamed podcast"
	item.LLMSummary = "New summary body."
	item.PubDate = item.PubDate.AddDate(0, 1, 0)
	second, err := EpisodeMarkdown(&item, "summary")
	if err != nil {
		t.Fatal(err)
	}
	current, _, _ := pm.Parse(second)
	current = markdownTestProvenance(t, &item, "summary", current)
	for hash := range old["input_snapshots"].(map[string]any) {
		if current["input_snapshots"].(map[string]any)[hash] == nil {
			t.Fatal("lost historical input reference")
		}
		payload, err := os.ReadFile(filepath.Join(markdownProvenanceDir(), "inputs", hash+".txt"))
		if err != nil || pm.Hash(payload) != hash {
			t.Fatal("historical input was not copied to renamed output location", err)
		}
	}
}

func TestMarkdownDoesNotCommitRevisionWithAlteredHistoricalCompanion(t *testing.T) {
	item := markdownTestItem(t)
	if _, err := EpisodeMarkdown(&item, "summary"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(markdownProvenanceDir(), "inputs", pm.Hash([]byte(item.LLMSummary))+".txt")
	if err := os.WriteFile(path, []byte("Changed outside Briefcast"), 0o644); err != nil {
		t.Fatal(err)
	}
	item.LLMSummary = "New summary."
	if _, err := EpisodeMarkdown(&item, "summary"); err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatal("export ignored an altered historical input", err)
	}
	var count int64
	if err := db.DB.Model(&db.MarkdownRevision{}).Where("episode_id = ?", item.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("committed a revision with invalid input companions", err)
	}
}
