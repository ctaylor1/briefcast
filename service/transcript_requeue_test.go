package service

import (
	"errors"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
)

func TestRequeueTranscriptionResetsOnlyAcknowledgedTranscripts(t *testing.T) {
	setupRetentionTestDB(t)
	podcast := createPodcast(t, "requeue", false)
	now := time.Now().UTC()

	create := func(title, status, checkpoint string, download db.DownloadStatus) db.PodcastItem {
		item := db.PodcastItem{
			PodcastID: podcast.ID, Title: title, DownloadStatus: download, DownloadPath: "/assets/" + title + ".mp3",
			TranscriptStatus: status, TranscriptJSON: `[{"start":0,"end":1,"text":"Use punctuation and capitalization."}]`,
			CanonicalTranscript: "old", CanonicalTranscriptVersion: 2, CanonicalUpdatedAt: &now,
			TranscriptCheckpointJSON: checkpoint, TranscriptProgressPct: 100, TranscriptProgressStage: "complete",
			TranscriptModel: "medium.en", TranscriptRetryCount: 3,
		}
		if err := db.CreatePodcastItem(&item); err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
		return item
	}
	done := create("done", "available", "", db.Downloaded)
	unacked := create("unacked", "available", `{"backend":"http","receipt_result_sha256":"abc"}`, db.Downloaded)
	running := create("running", "processing", `{"backend":"http"}`, db.Downloaded)
	notDownloaded := create("not-downloaded", "available", "", db.NotDownloaded)

	if err := RequeueTranscription(done.ID); err != nil {
		t.Fatalf("RequeueTranscription: %v", err)
	}
	var got db.PodcastItem
	if err := db.GetPodcastItemByID(done.ID, &got); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.TranscriptStatus != "pending_whisperx" || got.TranscriptJSON != "" || got.CanonicalTranscript != "" ||
		got.CanonicalUpdatedAt != nil || got.TranscriptProgressPct != 0 || got.TranscriptProgressStage != "" ||
		got.TranscriptModel != "" || got.TranscriptRetryCount != 0 || got.DownloadStatus != db.Downloaded {
		t.Fatalf("transcript not reset: %+v", got)
	}

	queued, err := db.GetPodcastItemsForWhisperx([]string{"pending_whisperx", "processing", "failed"}, 10)
	if err != nil {
		t.Fatalf("queue query: %v", err)
	}
	if len(*queued) != 1 || (*queued)[0].ID != done.ID {
		t.Fatalf("requeued episode should be the only one waiting for WhisperX, got %d", len(*queued))
	}

	for _, item := range []db.PodcastItem{unacked, running, notDownloaded, done} {
		if err := RequeueTranscription(item.ID); !errors.Is(err, ErrTranscriptNotRequeueable) {
			t.Fatalf("%s: err = %v, want ErrTranscriptNotRequeueable", item.Title, err)
		}
	}
	var kept db.PodcastItem
	if err := db.GetPodcastItemByID(unacked.ID, &kept); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if kept.TranscriptStatus != "available" || kept.TranscriptCheckpointJSON == "" || kept.TranscriptJSON == "" {
		t.Fatalf("unacknowledged receipt must be left alone: %+v", kept)
	}
}

func TestWhisperXInitialPromptIsOffByDefault(t *testing.T) {
	for raw, want := range map[string]string{
		"":                       "",
		"none":                   "",
		"OFF":                    "",
		"  Lex Fridman, FFmpeg ": "Lex Fridman, FFmpeg",
	} {
		t.Setenv("WHISPERX_INITIAL_PROMPT", raw)
		if got := whisperxInitialPrompt(); got != want {
			t.Fatalf("whisperxInitialPrompt(%q) = %q, want %q", raw, got, want)
		}
	}
	t.Setenv("WHISPERX_INITIAL_PROMPT", "")
	if got := LoadWhisperXConfig().InitialPrompt; got != "" {
		t.Fatalf("default config prompt = %q, want empty", got)
	}
}
