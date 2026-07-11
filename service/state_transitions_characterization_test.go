package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"go.uber.org/zap"
)

type podcastItemRowSnapshot map[string]string

func snapshotPodcastItemRow(t *testing.T, id string) podcastItemRowSnapshot {
	t.Helper()

	rows, err := db.DB.Table("podcast_items").Where("id = ?", id).Rows()
	if err != nil {
		t.Fatalf("query podcast item snapshot: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("podcast item %s not found for snapshot", id)
	}

	columns, err := rows.Columns()
	if err != nil {
		t.Fatalf("read podcast item columns: %v", err)
	}
	values := make([]any, len(columns))
	destinations := make([]any, len(columns))
	for i := range values {
		destinations[i] = &values[i]
	}
	if err := rows.Scan(destinations...); err != nil {
		t.Fatalf("scan podcast item snapshot: %v", err)
	}

	snapshot := make(podcastItemRowSnapshot, len(columns))
	for i, column := range columns {
		snapshot[column] = normalizeSnapshotValue(values[i])
	}
	return snapshot
}

func normalizeSnapshotValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return "<nil>"
	case []byte:
		return "bytes:" + string(typed)
	case time.Time:
		return "time:" + typed.UTC().Format(time.RFC3339Nano)
	default:
		return fmt.Sprintf("%T:%v", typed, typed)
	}
}

func assertPodcastItemChangedColumns(
	t *testing.T,
	before podcastItemRowSnapshot,
	after podcastItemRowSnapshot,
	expected ...string,
) {
	t.Helper()

	if len(before) != len(after) {
		t.Fatalf("snapshot schema changed: before=%d columns after=%d columns", len(before), len(after))
	}

	changed := make([]string, 0, len(expected))
	for column, beforeValue := range before {
		afterValue, ok := after[column]
		if !ok {
			t.Fatalf("column %q missing from after snapshot", column)
		}
		// GORM maintains UpdatedAt on Save; it is intentionally outside each
		// business transition's column contract.
		if column == "updated_at" {
			continue
		}
		if beforeValue != afterValue {
			changed = append(changed, column)
		}
	}

	sort.Strings(changed)
	expected = append([]string(nil), expected...)
	sort.Strings(expected)
	if strings.Join(changed, "\x00") != strings.Join(expected, "\x00") {
		details := make([]string, 0, len(changed))
		for _, column := range changed {
			details = append(details, fmt.Sprintf("%s: %q -> %q", column, before[column], after[column]))
		}
		t.Fatalf("changed columns mismatch\nexpected: %v\nactual:   %v\ndetails:\n%s", expected, changed, strings.Join(details, "\n"))
	}
}

func seedCharacterizationPodcastItem(
	t *testing.T,
	mutate func(*db.PodcastItem, string),
) (string, db.PodcastItem) {
	t.Helper()
	tempDir := setupRetentionTestDB(t)
	title := strings.ReplaceAll(t.Name(), "/", "-")
	podcast := createPodcast(t, title+"-podcast", false)

	fixed := time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC)
	canonicalUpdatedAt := fixed.Add(time.Hour)
	transcriptNextAttempt := fixed.Add(2 * time.Hour)
	llmSummaryDate := fixed.Add(3 * time.Hour)
	item := db.PodcastItem{
		PodcastID:                  podcast.ID,
		Title:                      title,
		Summary:                    "seed summary",
		SummaryHTML:                "<p>seed summary</p>",
		EpisodeType:                "full",
		Duration:                   1234,
		PubDate:                    fixed,
		FileURL:                    "https://example.com/seed.mp3",
		GUID:                       "seed-guid",
		Image:                      "https://example.com/seed.jpg",
		ChaptersURL:                "https://example.com/chapters.json",
		ChaptersType:               "podcast-index",
		ChaptersJSON:               `[{"title":"Seed chapter"}]`,
		ID3TagsJSON:                `{"artist":["Seed artist"]}`,
		ID3ChaptersJSON:            `[{"title":"Seed ID3 chapter"}]`,
		DownloadDate:               fixed.Add(4 * time.Hour),
		DownloadPath:               filepath.Join(tempDir, "seed.mp3"),
		DownloadStatus:             db.Downloaded,
		IsPlayed:                   false,
		BookmarkDate:               fixed.Add(5 * time.Hour),
		LocalImage:                 filepath.Join(tempDir, "seed.jpg"),
		FileSize:                   321,
		DownloadedBytes:            111,
		DownloadTotalBytes:         222,
		ItemMetadata:               `{"seed":true}`,
		TranscriptJSON:             `{"segments":[{"text":"seed"}]}`,
		CanonicalTranscript:        "seed canonical transcript",
		CanonicalTranscriptVersion: 7,
		CanonicalUpdatedAt:         &canonicalUpdatedAt,
		TranscriptStatus:           "available",
		TranscriptProgressPct:      73,
		TranscriptProgressStage:    "seed-stage",
		TranscriptCheckpointJSON:   `{"segments":[{"text":"checkpoint"}]}`,
		TranscriptRetryCount:       4,
		TranscriptNextAttempt:      &transcriptNextAttempt,
		TranscriptLastError:        "seed transcript error",
		TranscriptModel:            "seed-whisper-model",
		LLMSummary:                 "seed llm summary",
		LLMSummaryStatus:           "available",
		LLMSummaryError:            "seed llm error",
		LLMSummaryDate:             &llmSummaryDate,
		LLMSummaryModel:            "seed-llm-model",
		LLMSummaryPrompt:           "seed prompt",
		AlternateFileURLs:          `["https://example.com/alternate.mp3"]`,
		IsSummaryFavorited:         false,
	}
	if mutate != nil {
		mutate(&item, tempDir)
	}
	if err := db.CreatePodcastItem(&item); err != nil {
		t.Fatalf("create characterization item: %v", err)
	}
	return tempDir, item
}

func loadCharacterizationPodcastItem(t *testing.T, id string) db.PodcastItem {
	t.Helper()
	var item db.PodcastItem
	if err := db.GetPodcastItemByID(id, &item); err != nil {
		t.Fatalf("reload characterization item: %v", err)
	}
	return item
}

func assertRecentTime(t *testing.T, value time.Time, started time.Time, field string) {
	t.Helper()
	if value.Before(started.Add(-time.Second)) || value.After(time.Now().UTC().Add(time.Second)) {
		t.Fatalf("%s was not set to a recent time: %s", field, value)
	}
}

func TestCharacterize_SetPodcastItemAsQueuedForDownload(t *testing.T) {
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, _ string) {
		item.DownloadStatus = db.Downloading
	})
	before := snapshotPodcastItemRow(t, item.ID)

	if err := SetPodcastItemAsQueuedForDownload(item.ID); err != nil {
		t.Fatalf("queue item: %v", err)
	}
	after := snapshotPodcastItemRow(t, item.ID)
	assertPodcastItemChangedColumns(t, before, after, "download_status", "downloaded_bytes", "download_total_bytes")

	got := loadCharacterizationPodcastItem(t, item.ID)
	if got.DownloadStatus != db.NotDownloaded || got.DownloadedBytes != 0 || got.DownloadTotalBytes != 0 {
		t.Fatalf("unexpected queued state: status=%v bytes=%d total=%d", got.DownloadStatus, got.DownloadedBytes, got.DownloadTotalBytes)
	}
}

func TestCharacterize_SetPodcastItemAsQueuedPreserveProgress(t *testing.T) {
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, _ string) {
		item.DownloadStatus = db.Downloading
	})
	before := snapshotPodcastItemRow(t, item.ID)

	if err := SetPodcastItemAsQueuedPreserveProgress(item.ID); err != nil {
		t.Fatalf("queue item preserving progress: %v", err)
	}
	after := snapshotPodcastItemRow(t, item.ID)
	assertPodcastItemChangedColumns(t, before, after, "download_status")

	got := loadCharacterizationPodcastItem(t, item.ID)
	if got.DownloadStatus != db.NotDownloaded || got.DownloadedBytes != 111 || got.DownloadTotalBytes != 222 {
		t.Fatalf("unexpected preserve-progress state: status=%v bytes=%d total=%d", got.DownloadStatus, got.DownloadedBytes, got.DownloadTotalBytes)
	}
}

func TestCharacterize_SetPodcastItemAsDownloading(t *testing.T) {
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, _ string) {
		item.DownloadStatus = db.Paused
	})
	before := snapshotPodcastItemRow(t, item.ID)

	if err := SetPodcastItemAsDownloading(item.ID); err != nil {
		t.Fatalf("mark item downloading: %v", err)
	}
	after := snapshotPodcastItemRow(t, item.ID)
	assertPodcastItemChangedColumns(t, before, after, "download_status")
	if got := loadCharacterizationPodcastItem(t, item.ID); got.DownloadStatus != db.Downloading {
		t.Fatalf("expected downloading status, got %v", got.DownloadStatus)
	}
}

func TestCharacterize_SetPodcastItemAsPaused(t *testing.T) {
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, _ string) {
		item.DownloadStatus = db.Downloading
	})
	before := snapshotPodcastItemRow(t, item.ID)

	if err := SetPodcastItemAsPaused(item.ID); err != nil {
		t.Fatalf("mark item paused: %v", err)
	}
	after := snapshotPodcastItemRow(t, item.ID)
	assertPodcastItemChangedColumns(t, before, after, "download_status")
	if got := loadCharacterizationPodcastItem(t, item.ID); got.DownloadStatus != db.Paused {
		t.Fatalf("expected paused status, got %v", got.DownloadStatus)
	}
}

func TestCharacterize_SetPodcastItemAsDownloaded_FileExistsAndSkipsID3(t *testing.T) {
	var audioPath string
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, tempDir string) {
		audioPath = filepath.Join(tempDir, "downloaded.mp3")
		if err := os.WriteFile(audioPath, []byte("audio-bytes"), 0o644); err != nil {
			t.Fatalf("write characterization audio: %v", err)
		}
		item.DownloadStatus = db.Paused
		item.FileSize = 999
		item.TranscriptStatus = ""
		item.TranscriptJSON = ""
	})
	before := snapshotPodcastItemRow(t, item.ID)
	started := time.Now().UTC()

	if err := SetPodcastItemAsDownloaded(item.ID, audioPath); err != nil {
		t.Fatalf("mark item downloaded: %v", err)
	}
	after := snapshotPodcastItemRow(t, item.ID)
	assertPodcastItemChangedColumns(t, before, after,
		"download_date", "download_path", "download_status", "file_size",
		"downloaded_bytes", "download_total_bytes", "transcript_status",
		"transcript_progress_pct", "transcript_progress_stage", "transcript_checkpoint_json",
		"transcript_retry_count", "transcript_next_attempt", "transcript_last_error",
	)

	got := loadCharacterizationPodcastItem(t, item.ID)
	assertRecentTime(t, got.DownloadDate, started, "download_date")
	if got.DownloadPath != audioPath || got.DownloadStatus != db.Downloaded || got.FileSize != int64(len("audio-bytes")) {
		t.Fatalf("unexpected downloaded file state: path=%q status=%v size=%d", got.DownloadPath, got.DownloadStatus, got.FileSize)
	}
	if got.DownloadedBytes != got.FileSize || got.DownloadTotalBytes != got.FileSize {
		t.Fatalf("expected complete byte counters, got bytes=%d total=%d size=%d", got.DownloadedBytes, got.DownloadTotalBytes, got.FileSize)
	}
	if got.TranscriptStatus != "pending_whisperx" || got.TranscriptProgressPct != 0 || got.TranscriptProgressStage != "queued" {
		t.Fatalf("unexpected queued transcript state: status=%q pct=%d stage=%q", got.TranscriptStatus, got.TranscriptProgressPct, got.TranscriptProgressStage)
	}
	if got.ID3TagsJSON != item.ID3TagsJSON || got.ID3ChaptersJSON != item.ID3ChaptersJSON || got.ChaptersJSON != item.ChaptersJSON {
		t.Fatal("expected populated chapter metadata to skip ID3 extraction")
	}
}

func TestCharacterize_SetPodcastItemAsDownloaded_FileMissing(t *testing.T) {
	var missingPath string
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, tempDir string) {
		missingPath = filepath.Join(tempDir, "missing.mp3")
		item.DownloadStatus = db.Paused
		item.FileSize = 321
		item.DownloadedBytes = 1
		item.DownloadTotalBytes = 2
		item.TranscriptStatus = "available"
		item.TranscriptProgressPct = 42
		item.TranscriptProgressStage = "old-stage"
	})
	before := snapshotPodcastItemRow(t, item.ID)

	if err := SetPodcastItemAsDownloaded(item.ID, missingPath); err != nil {
		t.Fatalf("mark missing file downloaded: %v", err)
	}
	after := snapshotPodcastItemRow(t, item.ID)
	assertPodcastItemChangedColumns(t, before, after,
		"download_date", "download_path", "download_status", "downloaded_bytes", "download_total_bytes",
		"transcript_progress_pct", "transcript_progress_stage", "transcript_checkpoint_json",
	)

	got := loadCharacterizationPodcastItem(t, item.ID)
	if got.FileSize != 321 {
		t.Fatalf("expected missing-file branch to preserve file size, got %d", got.FileSize)
	}
	if got.DownloadedBytes != 321 || got.DownloadTotalBytes != 321 {
		t.Fatalf("expected counters to use preserved file size, got bytes=%d total=%d", got.DownloadedBytes, got.DownloadTotalBytes)
	}
	if got.TranscriptProgressPct != 100 || got.TranscriptProgressStage != "complete" || got.TranscriptCheckpointJSON != "" {
		t.Fatalf("unexpected available transcript completion state: pct=%d stage=%q checkpoint=%q", got.TranscriptProgressPct, got.TranscriptProgressStage, got.TranscriptCheckpointJSON)
	}
}

func TestCharacterize_SetPodcastItemAsNotDownloaded(t *testing.T) {
	_, item := seedCharacterizationPodcastItem(t, nil)
	before := snapshotPodcastItemRow(t, item.ID)

	if err := SetPodcastItemAsNotDownloaded(item.ID, db.Deleted); err != nil {
		t.Fatalf("mark item not downloaded: %v", err)
	}
	after := snapshotPodcastItemRow(t, item.ID)
	assertPodcastItemChangedColumns(t, before, after,
		"download_date", "download_path", "download_status", "downloaded_bytes", "download_total_bytes",
	)

	got := loadCharacterizationPodcastItem(t, item.ID)
	if !got.DownloadDate.IsZero() || got.DownloadPath != "" || got.DownloadStatus != db.Deleted || got.DownloadedBytes != 0 || got.DownloadTotalBytes != 0 {
		t.Fatalf("unexpected not-downloaded state: date=%s path=%q status=%v bytes=%d total=%d", got.DownloadDate, got.DownloadPath, got.DownloadStatus, got.DownloadedBytes, got.DownloadTotalBytes)
	}
}

func TestCharacterize_SetPodcastItemPlayedStatus(t *testing.T) {
	_, item := seedCharacterizationPodcastItem(t, nil)
	before := snapshotPodcastItemRow(t, item.ID)

	if err := SetPodcastItemPlayedStatus(item.ID, true); err != nil {
		t.Fatalf("mark item played: %v", err)
	}
	after := snapshotPodcastItemRow(t, item.ID)
	assertPodcastItemChangedColumns(t, before, after, "is_played")
	if got := loadCharacterizationPodcastItem(t, item.ID); !got.IsPlayed {
		t.Fatal("expected item to be played")
	}
}

func TestCharacterize_SetPodcastItemBookmarkStatus(t *testing.T) {
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, _ string) {
		item.BookmarkDate = time.Time{}
		item.IsSummaryFavorited = false
	})
	before := snapshotPodcastItemRow(t, item.ID)
	started := time.Now().UTC()

	if err := SetPodcastItemBookmarkStatus(item.ID, true); err != nil {
		t.Fatalf("bookmark item: %v", err)
	}
	after := snapshotPodcastItemRow(t, item.ID)
	assertPodcastItemChangedColumns(t, before, after, "bookmark_date", "is_summary_favorited")

	got := loadCharacterizationPodcastItem(t, item.ID)
	assertRecentTime(t, got.BookmarkDate, started, "bookmark_date")
	if !got.IsSummaryFavorited {
		t.Fatal("expected bookmark transition to preserve current summary-favorite coupling")
	}
}

func TestCharacterize_ResetItemForRedownload(t *testing.T) {
	_, item := seedCharacterizationPodcastItem(t, nil)
	before := snapshotPodcastItemRow(t, item.ID)

	resetItemForRedownload(&item, "characterized redownload reason", zap.NewNop().Sugar())
	after := snapshotPodcastItemRow(t, item.ID)
	assertPodcastItemChangedColumns(t, before, after,
		"download_status", "download_path", "downloaded_bytes", "download_total_bytes",
		"transcript_status", "transcript_progress_pct", "transcript_progress_stage",
		"transcript_checkpoint_json", "transcript_retry_count", "transcript_next_attempt", "transcript_last_error",
	)

	got := loadCharacterizationPodcastItem(t, item.ID)
	if got.DownloadStatus != db.NotDownloaded || got.DownloadPath != "" || got.DownloadedBytes != 0 || got.DownloadTotalBytes != 0 {
		t.Fatalf("unexpected redownload state: status=%v path=%q bytes=%d total=%d", got.DownloadStatus, got.DownloadPath, got.DownloadedBytes, got.DownloadTotalBytes)
	}
	if got.TranscriptStatus != "pending_whisperx" || got.TranscriptProgressStage != "waiting_for_download" || got.TranscriptLastError != "characterized redownload reason" {
		t.Fatalf("unexpected redownload transcript state: status=%q stage=%q error=%q", got.TranscriptStatus, got.TranscriptProgressStage, got.TranscriptLastError)
	}
}

func TestCharacterize_ScheduleTranscriptRetry(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, _ string) {
		item.TranscriptStatus = "processing"
		item.TranscriptRetryCount = 2
		item.TranscriptNextAttempt = &past
	})
	before := snapshotPodcastItemRow(t, item.ID)
	started := time.Now().UTC()

	scheduleTranscriptRetry(&item, WhisperXConfig{
		RetryFailed:        true,
		RetryDelaySeconds:  60,
		RetryMaxDelay:      60,
		RetryMaxErrorChars: 100,
	}, errors.New("characterized transcription failure"))
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("persist scheduled transcript retry: %v", err)
	}
	after := snapshotPodcastItemRow(t, item.ID)
	assertPodcastItemChangedColumns(t, before, after,
		"transcript_status", "transcript_retry_count", "transcript_next_attempt", "transcript_last_error",
	)

	got := loadCharacterizationPodcastItem(t, item.ID)
	if got.TranscriptStatus != "failed" || got.TranscriptRetryCount != 3 || got.TranscriptLastError != "characterized transcription failure" {
		t.Fatalf("unexpected retry state: status=%q count=%d error=%q", got.TranscriptStatus, got.TranscriptRetryCount, got.TranscriptLastError)
	}
	if got.TranscriptNextAttempt == nil || got.TranscriptNextAttempt.Before(started.Add(59*time.Second)) || got.TranscriptNextAttempt.After(started.Add(61*time.Second)) {
		t.Fatalf("unexpected retry schedule: %v", got.TranscriptNextAttempt)
	}
}

func configureCharacterizationWhisperX(t *testing.T, scriptBody string) string {
	t.Helper()
	pythonPath := requireWorkingPython(t)
	scriptPath := writeTempScript(t, t.TempDir(), "whisperx_characterization_stub.py", scriptBody)
	t.Setenv(whisperxEnabledEnv, "true")
	t.Setenv(whisperxPythonEnv, pythonPath)
	t.Setenv(whisperxScriptEnv, scriptPath)
	t.Setenv("WHISPERX_MODEL", "characterization-whisper-model")
	t.Setenv("WHISPERX_RETRY_FAILED", "true")
	t.Setenv("WHISPERX_RETRY_DELAY_SECONDS", "60")
	t.Setenv("WHISPERX_RETRY_MAX_DELAY_SECONDS", "60")
	t.Setenv("WHISPERX_MAX_ITEMS", "1")
	t.Setenv("WHISPERX_MAX_CONCURRENCY", "1")
	t.Setenv("WHISPERX_DIARIZATION", "false")
	t.Setenv("LLM_ENABLED", "false")
	setting := db.GetOrCreateSetting()
	setting.SummarizationEnabled = false
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatalf("disable summarization: %v", err)
	}
	return scriptPath
}

func TestCharacterize_TranscriptFailure(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, tempDir string) {
		item.DownloadPath = filepath.Join(tempDir, "transcript-failure.mp3")
		if err := os.WriteFile(item.DownloadPath, []byte(strings.Repeat("a", 2048)), 0o644); err != nil {
			t.Fatalf("write failure-path audio: %v", err)
		}
		item.DownloadStatus = db.Downloaded
		item.TranscriptJSON = ""
		item.TranscriptStatus = "pending_whisperx"
		item.TranscriptProgressPct = 73
		item.TranscriptProgressStage = "queued"
		item.TranscriptCheckpointJSON = ""
		item.TranscriptRetryCount = 2
		item.TranscriptNextAttempt = &past
		item.TranscriptLastError = "old failure"
	})
	configureCharacterizationWhisperX(t, `#!/usr/bin/env python3
import sys

print("forced characterization failure", file=sys.stderr)
sys.exit(2)
`)
	before := snapshotPodcastItemRow(t, item.ID)

	if err := TranscribePendingEpisodes(); err == nil {
		t.Fatal("expected characterization WhisperX failure")
	}
	after := snapshotPodcastItemRow(t, item.ID)
	assertPodcastItemChangedColumns(t, before, after,
		"transcript_status", "transcript_progress_pct", "transcript_progress_stage",
		"transcript_retry_count", "transcript_next_attempt", "transcript_last_error",
	)

	got := loadCharacterizationPodcastItem(t, item.ID)
	if got.TranscriptStatus != "failed" || got.TranscriptProgressPct != 1 || got.TranscriptProgressStage != "failed" || got.TranscriptRetryCount != 3 {
		t.Fatalf("unexpected transcript failure state: status=%q pct=%d stage=%q retry=%d", got.TranscriptStatus, got.TranscriptProgressPct, got.TranscriptProgressStage, got.TranscriptRetryCount)
	}
	if got.TranscriptNextAttempt == nil || !strings.Contains(got.TranscriptLastError, "forced characterization failure") {
		t.Fatalf("expected retry time and captured failure, next=%v error=%q", got.TranscriptNextAttempt, got.TranscriptLastError)
	}
}

func TestCharacterize_TranscriptSuccess(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, tempDir string) {
		item.DownloadPath = filepath.Join(tempDir, "transcript-success.mp3")
		if err := os.WriteFile(item.DownloadPath, []byte(strings.Repeat("a", 2048)), 0o644); err != nil {
			t.Fatalf("write success-path audio: %v", err)
		}
		item.DownloadStatus = db.Downloaded
		item.TranscriptJSON = ""
		item.TranscriptStatus = "pending_whisperx"
		item.TranscriptProgressPct = 73
		item.TranscriptProgressStage = "queued"
		item.TranscriptRetryCount = 2
		item.TranscriptNextAttempt = &past
		item.TranscriptLastError = "old failure"
		item.TranscriptModel = "old-model"
	})
	configureCharacterizationWhisperX(t, `#!/usr/bin/env python3
import json
import sys

json.dump({"segments":[{"start":0.0,"end":1.0,"text":"hello characterized world"}]}, sys.stdout)
`)
	before := snapshotPodcastItemRow(t, item.ID)

	if err := TranscribePendingEpisodes(); err != nil {
		t.Fatalf("expected characterization WhisperX success: %v", err)
	}
	after := snapshotPodcastItemRow(t, item.ID)
	assertPodcastItemChangedColumns(t, before, after,
		"transcript_json", "canonical_transcript", "canonical_transcript_version", "canonical_updated_at",
		"transcript_status", "transcript_progress_pct", "transcript_progress_stage", "transcript_checkpoint_json",
		"transcript_retry_count", "transcript_next_attempt", "transcript_last_error", "transcript_model",
	)

	got := loadCharacterizationPodcastItem(t, item.ID)
	if !json.Valid([]byte(got.TranscriptJSON)) || got.CanonicalTranscript != "hello characterized world" {
		t.Fatalf("unexpected transcript output: json=%q canonical=%q", got.TranscriptJSON, got.CanonicalTranscript)
	}
	if got.CanonicalTranscriptVersion != canonicalTranscriptVersionCurrent || got.CanonicalUpdatedAt == nil {
		t.Fatalf("unexpected canonical metadata: version=%d updated=%v", got.CanonicalTranscriptVersion, got.CanonicalUpdatedAt)
	}
	if got.TranscriptStatus != "available" || got.TranscriptProgressPct != 100 || got.TranscriptProgressStage != "complete" || got.TranscriptCheckpointJSON != "" {
		t.Fatalf("unexpected transcript success state: status=%q pct=%d stage=%q checkpoint=%q", got.TranscriptStatus, got.TranscriptProgressPct, got.TranscriptProgressStage, got.TranscriptCheckpointJSON)
	}
	if got.TranscriptRetryCount != 0 || got.TranscriptNextAttempt != nil || got.TranscriptLastError != "" || got.TranscriptModel != "characterization-whisper-model" {
		t.Fatalf("unexpected transcript lineage/retry state: retry=%d next=%v error=%q model=%q", got.TranscriptRetryCount, got.TranscriptNextAttempt, got.TranscriptLastError, got.TranscriptModel)
	}
}

func TestCharacterize_SummarizeEpisodeFailure(t *testing.T) {
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, _ string) {
		item.LLMSummaryStatus = "available"
		item.LLMSummaryError = "old summary error"
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "forced summary failure", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	before := snapshotPodcastItemRow(t, item.ID)

	err := SummarizeEpisode(&item, LLMConfig{
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "characterization-llm-model",
		TimeoutSecs: 5,
	}, "characterization prompt", "User: ")
	if err == nil {
		t.Fatal("expected summary failure")
	}
	after := snapshotPodcastItemRow(t, item.ID)
	assertPodcastItemChangedColumns(t, before, after, "llm_summary_status", "llm_summary_error")

	got := loadCharacterizationPodcastItem(t, item.ID)
	if got.LLMSummaryStatus != "failed" || !strings.Contains(got.LLMSummaryError, "status 500") {
		t.Fatalf("unexpected summary failure state: status=%q error=%q", got.LLMSummaryStatus, got.LLMSummaryError)
	}
}

func TestCharacterize_SummarizeEpisodeSuccess(t *testing.T) {
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, _ string) {
		item.LLMSummary = "old summary"
		item.LLMSummaryStatus = "failed"
		item.LLMSummaryError = "old summary error"
		item.LLMSummaryModel = "old-model"
		item.LLMSummaryPrompt = "old-prompt"
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected summary request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"characterized summary"}}]}`))
	}))
	t.Cleanup(server.Close)
	before := snapshotPodcastItemRow(t, item.ID)
	started := time.Now().UTC()

	if err := SummarizeEpisode(&item, LLMConfig{
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "characterization-llm-model",
		TimeoutSecs: 5,
	}, "characterization prompt", "User: "); err != nil {
		t.Fatalf("summarize episode: %v", err)
	}
	after := snapshotPodcastItemRow(t, item.ID)
	assertPodcastItemChangedColumns(t, before, after,
		"llm_summary", "llm_summary_status", "llm_summary_error", "llm_summary_date", "llm_summary_model", "llm_summary_prompt",
	)

	got := loadCharacterizationPodcastItem(t, item.ID)
	if got.LLMSummary != "characterized summary" || got.LLMSummaryStatus != "available" || got.LLMSummaryError != "" {
		t.Fatalf("unexpected summary success state: summary=%q status=%q error=%q", got.LLMSummary, got.LLMSummaryStatus, got.LLMSummaryError)
	}
	if got.LLMSummaryDate == nil {
		t.Fatal("expected summary date")
	}
	assertRecentTime(t, *got.LLMSummaryDate, started, "llm_summary_date")
	if got.LLMSummaryModel != "characterization-llm-model" || got.LLMSummaryPrompt != "characterization prompt" {
		t.Fatalf("unexpected summary lineage: model=%q prompt=%q", got.LLMSummaryModel, got.LLMSummaryPrompt)
	}
}
