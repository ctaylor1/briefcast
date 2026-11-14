package service

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
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
	}, SummaryBackfillOptions{}, nil)
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

func createWaitingEpisodes(t *testing.T, podcastTitle string, count int) []db.PodcastItem {
	t.Helper()
	podcast := createPodcast(t, podcastTitle, false)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	items := make([]db.PodcastItem, count)
	for i := range items {
		items[i] = db.PodcastItem{
			PodcastID:           podcast.ID,
			Title:               fmt.Sprintf("episode %d", i),
			CanonicalTranscript: fmt.Sprintf("transcript %d", i),
			PubDate:             base.AddDate(0, 0, i),
		}
		if err := db.CreatePodcastItem(&items[i]); err != nil {
			t.Fatalf("create episode: %v", err)
		}
	}
	return items
}

func summaryStatusOf(t *testing.T, id string) string {
	t.Helper()
	var item db.PodcastItem
	if err := db.GetPodcastItemByID(id, &item); err != nil {
		t.Fatalf("reload episode: %v", err)
	}
	return item.LLMSummaryStatus
}

func TestSummaryBackfillLimitTakesTheNewestWaitingEpisodes(t *testing.T) {
	setupRetentionTestDB(t)
	items := createWaitingEpisodes(t, "limited-backfill", 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"a summary"}}]}`))
	}))
	defer server.Close()

	succeeded, failed, err := runSummaryBackfill(LLMConfig{
		Enabled: true, APIKey: "test-key", BaseURL: server.URL, Model: "m", TimeoutSecs: 5,
	}, SummaryBackfillOptions{Limit: 2}, nil)
	if err != nil || succeeded != 2 || failed != 0 {
		t.Fatalf("unexpected backfill result: succeeded=%d failed=%d err=%v", succeeded, failed, err)
	}
	if got := summaryStatusOf(t, items[0].ID); got != "" {
		t.Fatalf("the oldest episode is outside the limit but has status %q", got)
	}
	for _, item := range items[1:] {
		if got := summaryStatusOf(t, item.ID); got != "available" {
			t.Fatalf("%s should have been summarized, status %q", item.Title, got)
		}
	}
}

func TestStopSummaryJobLetsTheEpisodeInProgressFinishAndSkipsTheRest(t *testing.T) {
	setupRetentionTestDB(t)
	items := createWaitingEpisodes(t, "stopped-backfill", 3)
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_TIMEOUT_SECONDS", "5")
	var requests atomic.Int32
	entered := make(chan struct{}, 3)
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		entered <- struct{}{}
		<-release
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"a summary"}}]}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("LLM_BASE_URL", server.URL)
	setting := db.GetOrCreateSetting()
	setting.LLMConcurrency = 1
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatal(err)
	}

	if _, err := StartSummaryBackfill(SummaryBackfillOptions{}); err != nil {
		t.Fatalf("start backfill: %v", err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("backfill never reached the LLM server")
	}
	stopping, err := StopSummaryJob()
	if err != nil || !stopping.Running || !stopping.Stopping {
		t.Fatalf("stop: status=%+v err=%v", stopping, err)
	}
	releaseOnce.Do(func() { close(release) })
	waitForSummaryJobIdle(t)

	status := GetSummaryJobStatus()
	if !status.Stopped || status.Stopping || status.Total != 3 || status.Completed != 1 || status.Failed != 0 || status.Error != "" {
		t.Fatalf("unexpected status after stop: %+v", status)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("expected one LLM request after stopping, got %d", got)
	}
	if got := summaryStatusOf(t, items[2].ID); got != "available" {
		t.Fatalf("the episode in progress should finish, status %q", got)
	}
	for _, item := range items[:2] {
		if got := summaryStatusOf(t, item.ID); got != "" {
			t.Fatalf("%s should be left waiting, status %q", item.Title, got)
		}
	}

	if _, err := StartSummaryBackfill(SummaryBackfillOptions{Limit: 1}); err != nil {
		t.Fatalf("a new job after a stop: %v", err)
	}
	waitForSummaryJobIdle(t)
	if status := GetSummaryJobStatus(); status.Stopped || status.Completed != 1 {
		t.Fatalf("the next job must not inherit the stop: %+v", status)
	}
}

func TestStopSummaryJobWithoutARunningJob(t *testing.T) {
	setupRetentionTestDB(t)
	if _, err := StopSummaryJob(); !errors.Is(err, ErrNoSummaryJobRunning) {
		t.Fatalf("expected ErrNoSummaryJobRunning, got %v", err)
	}
}

func TestStopSummaryJobDropsEpisodesQueuedByARefresh(t *testing.T) {
	setupRetentionTestDB(t)
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "test-key")
	if err := reserveSummaryJob(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if GetSummaryBackfillRunning() {
			finishSummaryJob(0, 0, nil)
		}
	})
	RequestSummaryBackfill([]string{"queued-episode"})
	if _, err := StopSummaryJob(); err != nil {
		t.Fatal(err)
	}
	pendingSummaryMu.Lock()
	pending := len(pendingSummaryIDs)
	pendingSummaryMu.Unlock()
	if pending != 0 {
		t.Fatalf("stop must drop queued episodes, %d left", pending)
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

	started, err := StartSummaryBackfill(SummaryBackfillOptions{})
	if err != nil || !started.Running {
		t.Fatalf("start backfill: status=%+v err=%v", started, err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("backfill never reached LLM server")
	}
	if _, err := StartSummaryBackfill(SummaryBackfillOptions{}); !errors.Is(err, ErrSummaryJobAlreadyRunning) {
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
