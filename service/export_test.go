package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
)

func TestExportTranscriptAndSummaryUseAssetsLayout(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	dataDir := filepath.Join(tempDir, "assets")

	podcast := createPodcast(t, "Export: Podcast/One?", false)
	item := createServicePodcastItem(t, podcast, "Episode: One/Two?", db.Downloaded)
	item.PubDate = time.Date(2024, 3, 5, 8, 9, 0, 0, time.UTC)
	item.Podcast = podcast
	item.CanonicalTranscript = "transcript text"
	item.LLMSummary = "summary text"
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("failed to update export item: %v", err)
	}

	ExportTranscript(&item)
	ExportSummary(&item)

	dateSub := filepath.Join("2024", "2024-03")
	transcriptPath := filepath.Join(dataDir, "transcripts", "Export- Podcast-One", dateSub, "2024-03-05-1-Episode- One-Two.txt")
	if got, err := os.ReadFile(transcriptPath); err != nil || string(got) != item.CanonicalTranscript {
		t.Fatalf("expected transcript export at %q, got content=%q err=%v", transcriptPath, string(got), err)
	}
	transcriptMarkdownPath := filepath.Join(dataDir, "markdown", "transcripts", "Export- Podcast-One", dateSub, "2024-03-05-1-Episode- One-Two.md")
	if got, err := os.ReadFile(transcriptMarkdownPath); err != nil || string(got) != item.CanonicalTranscript {
		t.Fatalf("expected transcript markdown export at %q, got content=%q err=%v", transcriptMarkdownPath, string(got), err)
	}

	summaryPath := filepath.Join(dataDir, "summaries", "Export- Podcast-One", dateSub, "2024-03-05-1-Episode- One-Two.txt")
	if got, err := os.ReadFile(summaryPath); err != nil || string(got) != item.LLMSummary {
		t.Fatalf("expected summary export at %q, got content=%q err=%v", summaryPath, string(got), err)
	}
	summaryMarkdownPath := filepath.Join(dataDir, "markdown", "summaries", "Export- Podcast-One", dateSub, "2024-03-05-1-Episode- One-Two.md")
	if got, err := os.ReadFile(summaryMarkdownPath); err != nil || string(got) != item.LLMSummary {
		t.Fatalf("expected summary markdown export at %q, got content=%q err=%v", summaryMarkdownPath, string(got), err)
	}
}

func TestExportEpisodeFileNameSkipsMissingPubDate(t *testing.T) {
	setupRetentionTestDB(t)

	podcast := createPodcast(t, "No Date Podcast", false)
	item := createServicePodcastItem(t, podcast, "No Date Episode", db.Downloaded)
	item.PubDate = time.Time{}
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("failed to update no-date item: %v", err)
	}

	if got := exportEpisodeFileName(&item); got != "1-No Date Episode" {
		t.Fatalf("expected missing date to be skipped, got %q", got)
	}
}

func TestExportAllUsesAssetsLayout(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	dataDir := filepath.Join(tempDir, "assets")

	podcast := createPodcast(t, "Export All Podcast", false)
	item := createServicePodcastItem(t, podcast, "Export All Episode", db.Downloaded)
	item.PubDate = time.Date(2024, 4, 6, 0, 0, 0, 0, time.UTC)
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

	dateSub := filepath.Join("2024", "2024-04")
	transcriptPath := filepath.Join(dataDir, "transcripts", "Export All Podcast", dateSub, "2024-04-06-1-Export All Episode.txt")
	if _, err := os.Stat(transcriptPath); err != nil {
		t.Fatalf("expected transcript export at %q: %v", transcriptPath, err)
	}
	transcriptMarkdownPath := filepath.Join(dataDir, "markdown", "transcripts", "Export All Podcast", dateSub, "2024-04-06-1-Export All Episode.md")
	if _, err := os.Stat(transcriptMarkdownPath); err != nil {
		t.Fatalf("expected transcript markdown export at %q: %v", transcriptMarkdownPath, err)
	}

	summaryPath := filepath.Join(dataDir, "summaries", "Export All Podcast", dateSub, "2024-04-06-1-Export All Episode.txt")
	if _, err := os.Stat(summaryPath); err != nil {
		t.Fatalf("expected summary export at %q: %v", summaryPath, err)
	}
	summaryMarkdownPath := filepath.Join(dataDir, "markdown", "summaries", "Export All Podcast", dateSub, "2024-04-06-1-Export All Episode.md")
	if _, err := os.Stat(summaryMarkdownPath); err != nil {
		t.Fatalf("expected summary markdown export at %q: %v", summaryMarkdownPath, err)
	}
}

func TestExportAllBackfillsCanonicalTranscriptFromTranscriptJSON(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	dataDir := filepath.Join(tempDir, "assets")

	podcast := createPodcast(t, "Legacy Podcast", false)
	item := createServicePodcastItem(t, podcast, "Legacy Episode", db.Downloaded)
	item.PubDate = time.Date(2024, 5, 7, 0, 0, 0, 0, time.UTC)
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

	dateSub := filepath.Join("2024", "2024-05")
	transcriptPath := filepath.Join(dataDir, "transcripts", "Legacy Podcast", dateSub, "2024-05-07-1-Legacy Episode.txt")
	got, err := os.ReadFile(transcriptPath)
	if err != nil {
		t.Fatalf("expected transcript export at %q: %v", transcriptPath, err)
	}
	if string(got) != "HOST: Legacy transcript text." {
		t.Fatalf("unexpected transcript export content %q", string(got))
	}
	transcriptMarkdownPath := filepath.Join(dataDir, "markdown", "transcripts", "Legacy Podcast", dateSub, "2024-05-07-1-Legacy Episode.md")
	got, err = os.ReadFile(transcriptMarkdownPath)
	if err != nil {
		t.Fatalf("expected transcript markdown export at %q: %v", transcriptMarkdownPath, err)
	}
	if string(got) != "HOST: Legacy transcript text." {
		t.Fatalf("unexpected transcript markdown export content %q", string(got))
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
