package service

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
)

func TestBuildWorkQueueSnapshotCountsAndItems(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	t.Setenv("WHISPERX_ENABLED", "true")
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "test-key")

	setting := db.GetOrCreateSetting()
	setting.SummarizationEnabled = true
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatalf("update settings failed: %v", err)
	}

	podcast := createPodcast(t, "repair-work", false)
	audioPath := filepath.Join(tempDir, "audio.mp3")
	now := time.Now().UTC()

	completeTranscript := createServicePodcastItem(t, podcast, "complete transcript", db.Downloaded)
	completeTranscript.TranscriptStatus = "available"
	completeTranscript.TranscriptJSON = `{"segments":[{"text":"done"}]}`
	if err := db.UpdatePodcastItem(&completeTranscript); err != nil {
		t.Fatalf("update complete transcript failed: %v", err)
	}

	processingTranscript := createServicePodcastItem(t, podcast, "processing transcript", db.Downloaded)
	processingTranscript.DownloadPath = audioPath
	processingTranscript.TranscriptStatus = "processing"
	processingTranscript.TranscriptProgressPct = 42
	processingTranscript.TranscriptProgressStage = "transcribing"
	if err := db.UpdatePodcastItem(&processingTranscript); err != nil {
		t.Fatalf("update processing transcript failed: %v", err)
	}

	failedTranscript := createServicePodcastItem(t, podcast, "failed transcript", db.Downloaded)
	failedTranscript.DownloadPath = audioPath
	failedTranscript.TranscriptStatus = "failed"
	failedTranscript.TranscriptRetryCount = 2
	futureAttempt := now.Add(time.Hour)
	failedTranscript.TranscriptNextAttempt = &futureAttempt
	failedTranscript.TranscriptLastError = "timeout"
	if err := db.UpdatePodcastItem(&failedTranscript); err != nil {
		t.Fatalf("update failed transcript failed: %v", err)
	}

	blockedTranscript := createServicePodcastItem(t, podcast, "blocked transcript", db.NotDownloaded)
	blockedTranscript.TranscriptStatus = "pending_whisperx"
	if err := db.UpdatePodcastItem(&blockedTranscript); err != nil {
		t.Fatalf("update blocked transcript failed: %v", err)
	}

	missingSummary := createServicePodcastItem(t, podcast, "missing summary", db.Downloaded)
	missingSummary.CanonicalTranscript = "ready transcript"
	missingSummary.LLMSummaryStatus = ""
	if err := db.UpdatePodcastItem(&missingSummary); err != nil {
		t.Fatalf("update missing summary failed: %v", err)
	}

	failedSummary := createServicePodcastItem(t, podcast, "failed summary", db.Downloaded)
	failedSummary.CanonicalTranscript = "ready transcript"
	failedSummary.LLMSummaryStatus = "failed"
	failedSummary.LLMSummaryError = "rate limited"
	if err := db.UpdatePodcastItem(&failedSummary); err != nil {
		t.Fatalf("update failed summary failed: %v", err)
	}

	snapshot, err := BuildWorkQueueSnapshot(20)
	if err != nil {
		t.Fatalf("BuildWorkQueueSnapshot failed: %v", err)
	}
	if snapshot.Config.WhisperXEnabled != true || snapshot.Config.LLMEnabled != true || snapshot.Config.LLMAPIKeyConfigured != true || snapshot.Config.SummarizationEnabled != true {
		t.Fatalf("unexpected config snapshot: %+v", snapshot.Config)
	}
	if snapshot.Transcripts.Complete != 1 || snapshot.Transcripts.Processing != 1 || snapshot.Transcripts.Failed != 1 || snapshot.Transcripts.RetryScheduled != 1 || snapshot.Transcripts.Blocked != 1 {
		t.Fatalf("unexpected transcript counts: %+v", snapshot.Transcripts)
	}
	if snapshot.Summary.Missing != 1 || snapshot.Summary.Failed != 1 || snapshot.Summary.EligibleForBackfill != 2 {
		t.Fatalf("unexpected summary counts: %+v", snapshot.Summary)
	}

	var sawProcessing, sawFailedSummary bool
	for _, item := range snapshot.Items {
		if item.Kind == "transcript" && item.ID == processingTranscript.ID && item.ProgressPct == 42 && item.Category == "active" {
			sawProcessing = true
		}
		if item.Kind == "summary" && item.ID == failedSummary.ID && item.LastError == "rate limited" {
			sawFailedSummary = true
		}
	}
	if !sawProcessing || !sawFailedSummary {
		t.Fatalf("expected processing transcript and failed summary in queue items, got %+v", snapshot.Items)
	}
}

func TestStartRepairWorkForcesFailedTranscriptRetries(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	t.Setenv("WHISPERX_ENABLED", "false")
	t.Setenv("LLM_ENABLED", "false")

	podcast := createPodcast(t, "repair-start", false)
	failedTranscript := createServicePodcastItem(t, podcast, "future failed transcript", db.Downloaded)
	failedTranscript.DownloadPath = filepath.Join(tempDir, "future.mp3")
	failedTranscript.TranscriptStatus = "failed"
	failedTranscript.TranscriptRetryCount = 3
	futureAttempt := time.Now().UTC().Add(2 * time.Hour)
	failedTranscript.TranscriptNextAttempt = &futureAttempt
	failedTranscript.TranscriptLastError = "temporary failure"
	if err := db.UpdatePodcastItem(&failedTranscript); err != nil {
		t.Fatalf("update failed transcript failed: %v", err)
	}

	response, err := StartRepairWork(10)
	if err != nil {
		t.Fatalf("StartRepairWork failed: %v", err)
	}
	if response.Queue.Limit != 10 {
		t.Fatalf("expected queue limit 10 in response, got %d", response.Queue.Limit)
	}

	var final RepairWorkResponse
	for i := 0; i < 50; i++ {
		final, err = GetRepairWorkStatus(10)
		if err != nil {
			t.Fatalf("GetRepairWorkStatus failed: %v", err)
		}
		if !final.Running {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if final.Running {
		t.Fatalf("repair work did not finish")
	}
	if final.LastRun == nil {
		t.Fatalf("expected last repair run")
	}
	if final.LastRun.Transcripts.ForcedDue != 1 || final.LastRun.Transcripts.Queued != 1 {
		t.Fatalf("unexpected transcript repair result: %+v", final.LastRun.Transcripts)
	}
	if !strings.Contains(final.LastRun.Transcripts.Error, "WhisperX is disabled") {
		t.Fatalf("expected disabled WhisperX to be reported, got %+v", final.LastRun.Transcripts)
	}

	var refreshed db.PodcastItem
	if err := db.GetPodcastItemByID(failedTranscript.ID, &refreshed); err != nil {
		t.Fatalf("reload failed transcript: %v", err)
	}
	if refreshed.TranscriptStatus != "pending_whisperx" {
		t.Fatalf("expected failed transcript to be queued, got %q", refreshed.TranscriptStatus)
	}
	if refreshed.TranscriptRetryCount != 3 {
		t.Fatalf("expected retry count to be preserved, got %d", refreshed.TranscriptRetryCount)
	}
	if refreshed.TranscriptNextAttempt == nil || refreshed.TranscriptNextAttempt.After(time.Now().UTC().Add(time.Minute)) {
		t.Fatalf("expected retry to be due now, got %v", refreshed.TranscriptNextAttempt)
	}
}
