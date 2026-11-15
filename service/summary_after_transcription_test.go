package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
)

// enableTestSummaries points summaries at server and turns them on.
func enableTestSummaries(t *testing.T, server *httptest.Server) {
	t.Helper()
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "synthetic-key")
	t.Setenv("LLM_BASE_URL", server.URL)
	t.Setenv("LLM_TIMEOUT_SECONDS", "10")
	setting := db.GetOrCreateSetting()
	setting.SummarizationEnabled = true
	setting.LLMConcurrency = 1
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteTranscriptionReturnsWithoutWaitingForTheSummary(t *testing.T) {
	f := newRemoteTestFixture(t)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		entered <- struct{}{}
		<-release
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Summary after release."}}]}`))
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	enableTestSummaries(t, server)

	// The LLM holds every request until release. If transcription waited for
	// the summary, run would block until the LLM timeout and the summary
	// would be failed rather than still in progress below.
	if err := f.run(t); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the queued summary never reached the LLM")
	}
	got := loadCharacterizationPodcastItem(t, f.item.ID)
	if got.TranscriptStatus != "available" || got.TranscriptCheckpointJSON != "" || f.acks.Load() != 1 {
		t.Fatalf("transcription did not finish while the summary ran: status=%q acks=%d", got.TranscriptStatus, f.acks.Load())
	}
	if got.LLMSummaryStatus != "processing" {
		t.Fatalf("expected the summary in progress, status %q", got.LLMSummaryStatus)
	}

	releaseOnce.Do(func() { close(release) })
	waitForSummaryJobIdle(t)
	got = loadCharacterizationPodcastItem(t, f.item.ID)
	if got.LLMSummary != "Summary after release." || got.LLMSummaryStatus != "available" {
		t.Fatalf("queued summary not saved: status=%q summary=%q", got.LLMSummaryStatus, got.LLMSummary)
	}
}

func TestQueueSummaryAfterTranscriptDoesNothingWhenSummariesAreOff(t *testing.T) {
	setupRetentionTestDB(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"x"}}]}`))
	}))
	t.Cleanup(server.Close)
	enableTestSummaries(t, server)
	setting := db.GetOrCreateSetting()
	setting.SummarizationEnabled = false
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatal(err)
	}
	item := createWaitingEpisodes(t, "summaries-off", 1)[0]

	queueSummaryAfterTranscript(&item, true)
	waitForSummaryJobIdle(t)
	if calls.Load() != 0 || summaryStatusOf(t, item.ID) != "" {
		t.Fatalf("summaries are off but the LLM was called %d times", calls.Load())
	}
}

func TestSummaryForNewTranscriptReplacesTheOldSummaryButRefreshRequestsDoNot(t *testing.T) {
	setupRetentionTestDB(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Summary of the new transcript."}}]}`))
	}))
	t.Cleanup(server.Close)
	enableTestSummaries(t, server)
	item := createWaitingEpisodes(t, "replace-summary", 1)[0]
	if err := db.UpdatePodcastItemColumns(item.ID, map[string]interface{}{"llm_summary": "Summary of the old transcript.", "llm_summary_status": "available"}); err != nil {
		t.Fatal(err)
	}

	RequestSummaryBackfill([]string{item.ID})
	waitForSummaryJobIdle(t)
	if calls.Load() != 0 {
		t.Fatal("a refresh request replaced an existing summary")
	}

	RequestSummaryForNewTranscript(item.ID)
	waitForSummaryJobIdle(t)
	var got db.PodcastItem
	if err := db.GetPodcastItemByID(item.ID, &got); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || got.LLMSummary != "Summary of the new transcript." || got.LLMSummaryStatus != "available" {
		t.Fatalf("new transcript not summarized: calls=%d status=%q summary=%q", calls.Load(), got.LLMSummaryStatus, got.LLMSummary)
	}
}

func TestSummaryForNewTranscriptQueuedBehindARunningJobStillReplaces(t *testing.T) {
	setupRetentionTestDB(t)
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	var releaseOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		entered <- struct{}{}
		<-release
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Fresh summary."}}]}`))
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	enableTestSummaries(t, server)
	items := createWaitingEpisodes(t, "queued-replace", 2)
	busy, replaced := items[0], items[1]
	if err := db.UpdatePodcastItemColumns(replaced.ID, map[string]interface{}{"llm_summary": "Stale summary.", "llm_summary_status": "available"}); err != nil {
		t.Fatal(err)
	}

	RequestSummaryBackfill([]string{busy.ID})
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first job never reached the LLM")
	}
	RequestSummaryForNewTranscript(replaced.ID)
	pendingSummaryMu.Lock()
	replaceQueued := pendingSummaryIDs[replaced.ID]
	pendingSummaryMu.Unlock()
	if !replaceQueued {
		t.Fatal("the request was not queued as a replacement behind the running job")
	}
	releaseOnce.Do(func() { close(release) })
	waitForSummaryJobIdle(t)
	var got db.PodcastItem
	if err := db.GetPodcastItemByID(replaced.ID, &got); err != nil {
		t.Fatal(err)
	}
	if got.LLMSummary != "Fresh summary." {
		t.Fatalf("queued replacement lost: %q", got.LLMSummary)
	}
}

func TestSummarizeEpisodeDiscardsTheSummaryWhenTheTranscriptChangesMeanwhile(t *testing.T) {
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, _ string) {
		item.LLMSummary, item.LLMSummaryStatus = "", ""
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// A re-transcription lands while the provider works on the old text.
		if err := db.UpdatePodcastItemColumns(item.ID, map[string]interface{}{"canonical_transcript": "A different transcript."}); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Summary of the old transcript."}}]}`))
	}))
	t.Cleanup(server.Close)

	err := SummarizeEpisode(&item, LLMConfig{APIKey: "k", BaseURL: server.URL, Model: "m", TimeoutSecs: 5}, "prompt", "User: ")
	if !errors.Is(err, db.ErrTranscriptChanged) || !strings.Contains(err.Error(), "summary discarded") {
		t.Fatalf("expected the stale summary to be discarded, got %v", err)
	}
	var got db.PodcastItem
	if err := db.GetPodcastItemByID(item.ID, &got); err != nil {
		t.Fatal(err)
	}
	if got.LLMSummary != "" || got.LLMSummaryStatus != "" {
		t.Fatalf("stale summary saved or status left behind: status=%q summary=%q", got.LLMSummaryStatus, got.LLMSummary)
	}
}

func TestRequestedSummaryRunSkipsAnEpisodeAlreadyInProgress(t *testing.T) {
	setupRetentionTestDB(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"x"}}]}`))
	}))
	t.Cleanup(server.Close)
	enableTestSummaries(t, server)
	items := createWaitingEpisodes(t, "in-progress", 2)
	if err := db.UpdatePodcastItemColumns(items[0].ID, map[string]interface{}{"llm_summary_status": "processing"}); err != nil {
		t.Fatal(err)
	}
	// Any other status, including values older releases may have left, does
	// not stop a requested episode.
	if err := db.UpdatePodcastItemColumns(items[1].ID, map[string]interface{}{"llm_summary_status": "pending"}); err != nil {
		t.Fatal(err)
	}

	RequestSummaryBackfill([]string{items[0].ID, items[1].ID})
	waitForSummaryJobIdle(t)
	if calls.Load() != 1 || summaryStatusOf(t, items[0].ID) != "processing" || summaryStatusOf(t, items[1].ID) != "available" {
		t.Fatalf("calls=%d statuses=%q,%q", calls.Load(), summaryStatusOf(t, items[0].ID), summaryStatusOf(t, items[1].ID))
	}
}
