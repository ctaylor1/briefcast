package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"gorm.io/gorm"
)

// blockingTranscriptionScript signals when it starts and waits for a release
// file, so a test can change the episode while transcription is running.
const blockingTranscriptionScript = `import json, os, sys, time
audio = sys.argv[1]
open(audio + ".started", "w").close()
for _ in range(600):
    if os.path.exists(audio + ".release"):
        break
    time.sleep(0.05)
print(json.dumps({"segments": [{"text": "Concurrent transcript."}]}))
`

func runTranscriptionWithConcurrentChange(t *testing.T, change func(item db.PodcastItem)) db.PodcastItem {
	t.Helper()
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, dir string) {
		item.DownloadPath = filepath.Join(dir, "episode.mp3")
		if err := os.WriteFile(item.DownloadPath, []byte(strings.Repeat("a", 2048)), 0o644); err != nil {
			t.Fatal(err)
		}
		item.TranscriptJSON, item.CanonicalTranscript, item.TranscriptCheckpointJSON = "", "", ""
		item.TranscriptStatus = "pending_whisperx"
		item.TranscriptNextAttempt = nil
		item.IsPlayed = false
		item.BookmarkDate = time.Time{}
	})
	configureCharacterizationWhisperX(t, blockingTranscriptionScript)

	done := make(chan error, 1)
	go func() { done <- TranscribePendingEpisodes() }()
	deadline := time.Now().Add(20 * time.Second)
	for !FileExists(item.DownloadPath + ".started") {
		if time.Now().After(deadline) {
			t.Fatal("transcription did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	change(item)
	if err := os.WriteFile(item.DownloadPath+".release", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("TranscribePendingEpisodes: %v", err)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("transcription did not finish")
	}
	return item
}

func TestTranscriptionKeepsChangesMadeWhileItRuns(t *testing.T) {
	bookmarked := time.Now().UTC().Truncate(time.Second)
	item := runTranscriptionWithConcurrentChange(t, func(item db.PodcastItem) {
		if err := db.UpdatePodcastItemColumns(item.ID, map[string]interface{}{"is_played": true, "bookmark_date": bookmarked}); err != nil {
			t.Fatal(err)
		}
	})
	got := loadCharacterizationPodcastItem(t, item.ID)
	if got.TranscriptStatus != "available" || !strings.Contains(got.TranscriptJSON, "Concurrent transcript.") {
		t.Fatalf("transcript not saved: status=%q", got.TranscriptStatus)
	}
	if !got.IsPlayed || got.BookmarkDate.IsZero() {
		t.Fatalf("transcription overwrote changes made while it ran: played=%v bookmark=%v", got.IsPlayed, got.BookmarkDate)
	}
}

func TestTranscriptionDoesNotRecreateEpisodeDeletedWhileItRuns(t *testing.T) {
	item := runTranscriptionWithConcurrentChange(t, func(item db.PodcastItem) {
		if err := db.DeletePodcastItemByID(item.ID); err != nil {
			t.Fatal(err)
		}
	})
	var got db.PodcastItem
	if err := db.GetPodcastItemByID(item.ID, &got); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("deleted episode came back: err=%v status=%q", err, got.TranscriptStatus)
	}
}

func TestSummarizeEpisodeKeepsChangesMadeWhileItRuns(t *testing.T) {
	item := markdownTestItem(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The user acts while the provider is working on the stale snapshot.
		if err := db.UpdatePodcastItemColumns(item.ID, map[string]interface{}{"is_played": true}); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"model":"fixture","choices":[{"message":{"content":"## Takeaway\n\n- A claim."}}]}`))
	}))
	defer server.Close()
	snapshot := item
	snapshot.IsPlayed = false
	if err := SummarizeEpisode(&snapshot, LLMConfig{APIKey: "test-key", BaseURL: server.URL, Provider: "fixture", Model: "fixture", MaxTokens: 64, TimeoutSecs: 5}, "System\n", "User\n"); err != nil {
		t.Fatal(err)
	}
	var got db.PodcastItem
	if err := db.GetPodcastItemByID(item.ID, &got); err != nil {
		t.Fatal(err)
	}
	if got.LLMSummaryStatus != "available" || !strings.Contains(got.LLMSummary, "A claim.") {
		t.Fatalf("summary not saved: %q", got.LLMSummaryStatus)
	}
	if !got.IsPlayed {
		t.Fatal("summary save overwrote a change made while the provider ran")
	}
}

func TestSummarizeEpisodeDoesNotRecreateDeletedEpisode(t *testing.T) {
	item := markdownTestItem(t)
	if err := db.DeletePodcastItemByID(item.ID); err != nil {
		t.Fatal(err)
	}
	err := SummarizeEpisode(&item, LLMConfig{APIKey: "test-key", BaseURL: "http://127.0.0.1:1", TimeoutSecs: 1}, "System\n", "User\n")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
	var got db.PodcastItem
	if err := db.GetPodcastItemByID(item.ID, &got); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("deleted episode came back: %v", err)
	}
}
