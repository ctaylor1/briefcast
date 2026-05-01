package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ctaylor1/briefcast/db"
)

func TestExportTranscriptAndSummaryUseAssetsLayout(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	dataDir := filepath.Join(tempDir, "assets")

	podcast := createPodcast(t, "Export: Podcast/One?", false)
	item := createServicePodcastItem(t, podcast, "Episode: One/Two?", db.Downloaded)
	item.Podcast = podcast
	item.CanonicalTranscript = "transcript text"
	item.LLMSummary = "summary text"

	ExportTranscript(&item)
	ExportSummary(&item)

	transcriptPath := filepath.Join(dataDir, "transcripts", "Export- Podcast-One", "Episode- One-Two.txt")
	if got, err := os.ReadFile(transcriptPath); err != nil || string(got) != item.CanonicalTranscript {
		t.Fatalf("expected transcript export at %q, got content=%q err=%v", transcriptPath, string(got), err)
	}

	summaryPath := filepath.Join(dataDir, "summaries", "Export- Podcast-One", "Episode- One-Two.txt")
	if got, err := os.ReadFile(summaryPath); err != nil || string(got) != item.LLMSummary {
		t.Fatalf("expected summary export at %q, got content=%q err=%v", summaryPath, string(got), err)
	}
}

func TestExportAllUsesAssetsLayout(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	dataDir := filepath.Join(tempDir, "assets")

	podcast := createPodcast(t, "Export All Podcast", false)
	item := createServicePodcastItem(t, podcast, "Export All Episode", db.Downloaded)
	item.CanonicalTranscript = "all transcript"
	item.LLMSummary = "all summary"
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("failed to update podcast item: %v", err)
	}

	transcripts, summaries, err := ExportAll()
	if err != nil {
		t.Fatalf("export all failed: %v", err)
	}
	if transcripts != 1 || summaries != 1 {
		t.Fatalf("expected one transcript and summary export, got transcripts=%d summaries=%d", transcripts, summaries)
	}

	transcriptPath := filepath.Join(dataDir, "transcripts", "Export All Podcast", "Export All Episode.txt")
	if _, err := os.Stat(transcriptPath); err != nil {
		t.Fatalf("expected transcript export at %q: %v", transcriptPath, err)
	}

	summaryPath := filepath.Join(dataDir, "summaries", "Export All Podcast", "Export All Episode.txt")
	if _, err := os.Stat(summaryPath); err != nil {
		t.Fatalf("expected summary export at %q: %v", summaryPath, err)
	}
}

func TestExportAllBackfillsCanonicalTranscriptFromTranscriptJSON(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	dataDir := filepath.Join(tempDir, "assets")

	podcast := createPodcast(t, "Legacy Podcast", false)
	item := createServicePodcastItem(t, podcast, "Legacy Episode", db.Downloaded)
	item.TranscriptJSON = `{"segments":[{"speaker":"host","start":0,"end":1,"text":"Legacy transcript text."}]}`
	item.CanonicalTranscript = ""
	item.CanonicalTranscriptVersion = 0
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("failed to update podcast item: %v", err)
	}

	transcripts, summaries, err := ExportAll()
	if err != nil {
		t.Fatalf("export all failed: %v", err)
	}
	if transcripts != 1 || summaries != 0 {
		t.Fatalf("expected one transcript and no summaries, got transcripts=%d summaries=%d", transcripts, summaries)
	}

	transcriptPath := filepath.Join(dataDir, "transcripts", "Legacy Podcast", "Legacy Episode.txt")
	got, err := os.ReadFile(transcriptPath)
	if err != nil {
		t.Fatalf("expected transcript export at %q: %v", transcriptPath, err)
	}
	if string(got) != "HOST: Legacy transcript text." {
		t.Fatalf("unexpected transcript export content %q", string(got))
	}

	var refreshed db.PodcastItem
	if err := db.GetPodcastItemByID(item.ID, &refreshed); err != nil {
		t.Fatalf("failed to reload podcast item: %v", err)
	}
	if refreshed.CanonicalTranscript != "HOST: Legacy transcript text." {
		t.Fatalf("expected canonical transcript to be backfilled, got %q", refreshed.CanonicalTranscript)
	}
	if refreshed.CanonicalTranscriptVersion != canonicalTranscriptVersionCurrent {
		t.Fatalf("expected canonical transcript version %d, got %d", canonicalTranscriptVersionCurrent, refreshed.CanonicalTranscriptVersion)
	}
}
