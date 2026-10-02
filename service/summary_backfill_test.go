package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
)

func TestSummaryBackfillRetriesOnlyStaleProcessing(t *testing.T) {
	setupRetentionTestDB(t)
	podcast := createPodcast(t, "stale-summary-processing", false)
	stale := db.PodcastItem{PodcastID: podcast.ID, Title: "stale", CanonicalTranscript: "stale transcript", LLMSummaryStatus: "processing"}
	fresh := db.PodcastItem{PodcastID: podcast.ID, Title: "fresh", CanonicalTranscript: "fresh transcript", LLMSummaryStatus: "processing"}
	for _, item := range []*db.PodcastItem{&stale, &fresh} {
		if err := db.CreatePodcastItem(item); err != nil {
			t.Fatalf("create episode: %v", err)
		}
	}
	if err := db.DB.Model(&stale).UpdateColumn("updated_at", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatalf("age processing state: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"recovered summary"}}]}`))
	}))
	defer server.Close()

	succeeded, failed, err := runSummaryBackfill(LLMConfig{
		Enabled: true, APIKey: "test-key", BaseURL: server.URL, Model: "requested-model", TimeoutSecs: 5,
	}, nil)
	if err != nil || succeeded != 1 || failed != 0 {
		t.Fatalf("unexpected backfill result: succeeded=%d failed=%d err=%v", succeeded, failed, err)
	}
	var recovered, stillProcessing db.PodcastItem
	if err := db.GetPodcastItemByID(stale.ID, &recovered); err != nil {
		t.Fatalf("reload stale episode: %v", err)
	}
	if err := db.GetPodcastItemByID(fresh.ID, &stillProcessing); err != nil {
		t.Fatalf("reload fresh episode: %v", err)
	}
	if recovered.LLMSummaryStatus != "available" || recovered.LLMSummary != "recovered summary" {
		t.Fatalf("stale episode was not recovered: status=%q summary=%q", recovered.LLMSummaryStatus, recovered.LLMSummary)
	}
	if stillProcessing.LLMSummaryStatus != "processing" || stillProcessing.LLMSummary != "" {
		t.Fatalf("fresh processing episode was changed: status=%q summary=%q", stillProcessing.LLMSummaryStatus, stillProcessing.LLMSummary)
	}
}

func TestStartSummaryBackfillReservesJobAndReportsCompletion(t *testing.T) {
	setupRetentionTestDB(t)
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_MODEL", "requested-model")
	t.Setenv("LLM_TIMEOUT_SECONDS", "5")
	podcast := createPodcast(t, "async-summary-backfill", false)
	item := db.PodcastItem{PodcastID: podcast.ID, Title: "episode", CanonicalTranscript: "test transcript"}
	if err := db.CreatePodcastItem(&item); err != nil {
		t.Fatalf("create episode: %v", err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"async summary"}}]}`))
	}))
	defer server.Close()
	t.Setenv("LLM_BASE_URL", server.URL)

	started, err := StartSummaryBackfill()
	if err != nil || !started.Running {
		t.Fatalf("start backfill: status=%+v err=%v", started, err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("backfill never reached LLM server")
	}
	if _, err := StartSummaryBackfill(); !errors.Is(err, ErrSummaryJobAlreadyRunning) {
		t.Fatalf("expected busy error, got %v", err)
	}
	releaseOnce.Do(func() { close(release) })
	deadline := time.After(5 * time.Second)
	for {
		status := GetSummaryJobStatus()
		if !status.Running {
			if status.Total != 1 || status.Completed != 1 || status.Failed != 0 || status.Error != "" {
				t.Fatalf("unexpected completed job status: %+v", status)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("backfill did not finish")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
