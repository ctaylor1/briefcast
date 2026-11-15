package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ctaylor1/briefcast/db"
)

// panicInSummaryCall makes SummarizeEpisode's provider call panic for the
// calls where shouldPanic returns true, and run normally otherwise.
func panicInSummaryCall(t *testing.T, shouldPanic func(call int32) bool) {
	t.Helper()
	original := summaryLLMCall
	var calls atomic.Int32
	summaryLLMCall = func(transcript, prompt, userPrompt string, cfg LLMConfig, report map[string]any, details *llmCallDetails) (string, string, error) {
		if shouldPanic(calls.Add(1)) {
			panic("synthetic provider client bug")
		}
		return original(transcript, prompt, userPrompt, cfg, report, details)
	}
	t.Cleanup(func() { summaryLLMCall = original })
}

func TestSummarizeEpisodePanicMarksTheEpisodeFailedAndPropagates(t *testing.T) {
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, _ string) {
		item.LLMSummary, item.LLMSummaryStatus = "", ""
	})
	panicInSummaryCall(t, func(int32) bool { return true })

	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("expected the panic to propagate")
			}
		}()
		_ = SummarizeEpisode(&item, LLMConfig{APIKey: "k", BaseURL: "http://127.0.0.1:1", Model: "m", TimeoutSecs: 1}, "prompt", "User: ")
	}()

	var got db.PodcastItem
	if err := db.GetPodcastItemByID(item.ID, &got); err != nil {
		t.Fatal(err)
	}
	if got.LLMSummaryStatus != "failed" || !strings.Contains(got.LLMSummaryError, "summary panicked: synthetic provider client bug") {
		t.Fatalf("panic left status=%q error=%q", got.LLMSummaryStatus, got.LLMSummaryError)
	}
}

func TestSummaryBatchGoesOnAfterOneEpisodePanics(t *testing.T) {
	setupRetentionTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"a summary"}}]}`))
	}))
	t.Cleanup(server.Close)
	enableTestSummaries(t, server)
	items := createWaitingEpisodes(t, "panic-batch", 3)
	panicInSummaryCall(t, func(call int32) bool { return call == 1 })

	if _, err := StartSummaryBackfill(SummaryBackfillOptions{}); err != nil {
		t.Fatal(err)
	}
	waitForSummaryJobIdle(t)

	status := GetSummaryJobStatus()
	if status.Completed != 3 || status.Failed != 1 {
		t.Fatalf("expected 3 processed with 1 failed, got %+v", status)
	}
	// Newest first: the newest episode hit the panic, the other two finished.
	if got := summaryStatusOf(t, items[2].ID); got != "failed" {
		t.Fatalf("panicked episode status %q", got)
	}
	for _, item := range items[:2] {
		if got := summaryStatusOf(t, item.ID); got != "available" {
			t.Fatalf("%s was skipped after the panic, status %q", item.Title, got)
		}
	}
}
