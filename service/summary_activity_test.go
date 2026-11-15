package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/telemetry"
	"go.uber.org/zap"
)

const summarySentinel = "PRIVATE-TRANSCRIPT-SENTINEL"

func TestSummaryAttemptsRecordOutcomeTokensAndReason(t *testing.T) {
	setupRetentionTestDB(t)
	before := telemetry.Summary().SummaryAttempts
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case strings.Contains(string(body), "transcript 0"):
			_, _ = w.Write([]byte(`{"model":"resolved-model","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":120,"completion_tokens":40,"total_tokens":160}}`))
		case strings.Contains(string(body), "transcript 1"):
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"","reasoning":"` + strings.Repeat("r", 300) + `"},"finish_reason":"length"}],"usage":{"prompt_tokens":5000,"completion_tokens":2048,"total_tokens":7048}}`))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)
	enableTestSummaries(t, server)
	items := createWaitingEpisodes(t, "attempt-outcomes", 3)
	for i := range items {
		if err := db.UpdatePodcastItemColumns(items[i].ID, map[string]interface{}{"canonical_transcript": fmt.Sprintf("transcript %d %s", i, summarySentinel)}); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := StartSummaryBackfill(SummaryBackfillOptions{}); err != nil {
		t.Fatal(err)
	}
	waitForSummaryJobIdle(t)

	snap := summaryActivitySnapshot(10)
	if len(snap.Recent) != 3 || len(snap.InFlight) != 0 {
		t.Fatalf("expected 3 finished attempts, got recent=%d in_flight=%d", len(snap.Recent), len(snap.InFlight))
	}
	byEpisode := map[string]SummaryAttempt{}
	for _, attempt := range snap.Recent {
		byEpisode[attempt.EpisodeID] = attempt
		if attempt.Source != SummarySourceBackfill || attempt.FinishedAt == nil || attempt.Title == "" {
			t.Fatalf("attempt lacks source, finish time or title: %+v", attempt)
		}
	}
	ok := byEpisode[items[0].ID]
	if ok.Outcome != SummaryOutcomeSucceeded || ok.PromptTokens != 120 || ok.CompletionTokens != 40 || ok.Model != "resolved-model" || ok.Error != "" {
		t.Fatalf("success not recorded: %+v", ok)
	}
	limit := byEpisode[items[1].ID]
	if limit.Outcome != SummaryOutcomeTokenLimit || limit.FinishReason != "length" || limit.CompletionTokens != 2048 || limit.ThinkingChars != 300 || !strings.Contains(limit.Error, "output token limit") {
		t.Fatalf("token limit not recorded: %+v", limit)
	}
	failed := byEpisode[items[2].ID]
	if failed.Outcome != SummaryOutcomeFailed || !strings.Contains(failed.Error, "status 500") {
		t.Fatalf("failure not recorded: %+v", failed)
	}
	// Newest first, the oldest episode (the success) ran last.
	if snap.ConsecutiveFailures != 0 || snap.LastSuccessAt == nil {
		t.Fatalf("expected the streak reset by the last success: %+v", snap)
	}
	if snap.Outcomes[SummaryOutcomeSucceeded] != 1 || snap.Outcomes[SummaryOutcomeTokenLimit] != 1 || snap.Outcomes[SummaryOutcomeFailed] != 1 {
		t.Fatalf("unexpected outcome counts: %v", snap.Outcomes)
	}
	after := telemetry.Summary().SummaryAttempts
	for _, outcome := range []string{SummaryOutcomeSucceeded, SummaryOutcomeTokenLimit, SummaryOutcomeFailed} {
		if after[outcome]-before[outcome] != 1 {
			t.Fatalf("metric for %s moved by %v", outcome, after[outcome]-before[outcome])
		}
	}
	raw, _ := json.Marshal(snap)
	if strings.Contains(string(raw), summarySentinel) || strings.Contains(string(raw), "rrrr") {
		t.Fatal("the attempt record exposed transcript or thinking text")
	}
}

func TestSummaryAttemptTimeoutAndStreak(t *testing.T) {
	setupRetentionTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	t.Cleanup(server.Close)
	enableTestSummaries(t, server)
	t.Setenv("LLM_TIMEOUT_SECONDS", "1")
	createWaitingEpisodes(t, "attempt-timeout", 2)

	if _, err := StartSummaryBackfill(SummaryBackfillOptions{}); err != nil {
		t.Fatal(err)
	}
	waitForSummaryJobIdle(t)
	snap := summaryActivitySnapshot(10)
	if len(snap.Recent) != 2 || snap.Recent[0].Outcome != SummaryOutcomeTimedOut || snap.Recent[1].Outcome != SummaryOutcomeTimedOut {
		t.Fatalf("expected two timeouts: %+v", snap.Recent)
	}
	if snap.ConsecutiveFailures != 2 || snap.LastSuccessAt != nil {
		t.Fatalf("expected a streak of 2 failures: %+v", snap)
	}
}

func TestSummaryAttemptIsVisibleWhileInFlight(t *testing.T) {
	setupRetentionTestDB(t)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		entered <- struct{}{}
		<-release
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"done"}}]}`))
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	enableTestSummaries(t, server)
	item := createWaitingEpisodes(t, "in-flight", 1)[0]

	RequestSummaryBackfill([]string{item.ID})
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("summary never reached the LLM")
	}
	snap := buildSummariesSnapshot()
	if len(snap.InFlight) != 1 || snap.InFlight[0].EpisodeID != item.ID || snap.InFlight[0].Source != SummarySourceAutomatic || snap.InFlight[0].StartedAt.IsZero() {
		t.Fatalf("in-flight attempt not visible: %+v", snap.InFlight)
	}
	if !snap.Job.Running || snap.Job.Total != 1 {
		t.Fatalf("summary job not reported running: %+v", snap.Job)
	}
	releaseOnce.Do(func() { close(release) })
	waitForSummaryJobIdle(t)
	if snap := summaryActivitySnapshot(10); len(snap.InFlight) != 0 || len(snap.Recent) != 1 {
		t.Fatalf("attempt did not move to recent: %+v", snap)
	}
}

func TestSummaryActivityKeepsTheNewestResultsUnderConcurrentUse(t *testing.T) {
	resetSummaryActivityForTests()
	t.Cleanup(resetSummaryActivityForTests)
	var wg sync.WaitGroup
	for i := 0; i < summaryRecentLimit+20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			h := beginSummaryAttempt(&db.PodcastItem{Base: db.Base{ID: fmt.Sprintf("episode-%d", i)}, Title: "t"}, SummarySourceAutomatic, "m")
			_ = summaryActivitySnapshot(5)
			h.finish(SummaryOutcomeFailed, llmCallDetails{}, errors.New(strings.Repeat("e", 1000)))
		}(i)
	}
	wg.Wait()
	snap := summaryActivitySnapshot(1000)
	if len(snap.Recent) != summaryRecentLimit || len(snap.InFlight) != 0 {
		t.Fatalf("expected %d recent and none in flight, got %d and %d", summaryRecentLimit, len(snap.Recent), len(snap.InFlight))
	}
	if snap.ConsecutiveFailures != summaryRecentLimit+20 || snap.Outcomes[SummaryOutcomeFailed] != summaryRecentLimit+20 {
		t.Fatalf("counts lost under concurrency: %+v", snap.Outcomes)
	}
	if got := len(snap.Recent[0].Error); got > summaryErrorMaxChars {
		t.Fatalf("error not shortened: %d chars", got)
	}
}

func TestJobHistoryTracksStreakFailingSinceAndPanics(t *testing.T) {
	resetJobHistoryForTests()
	t.Cleanup(resetJobHistoryForTests)
	logger := zap.NewNop().Sugar()
	fail := func(*zap.SugaredLogger) error { return errors.New("Worker token file unavailable") }

	_ = runJobWith("TranscribePendingEpisodes", logger, fail)
	first := JobHistorySnapshot()[0]
	time.Sleep(5 * time.Millisecond)
	_ = runJobWith("TranscribePendingEpisodes", logger, fail)
	func() {
		defer func() { _ = recover() }()
		_ = runJobWith("TranscribePendingEpisodes", logger, func(*zap.SugaredLogger) error { panic("boom") })
	}()
	got := JobHistorySnapshot()[0]
	if got.ConsecutiveFailures != 3 || got.Failures != 3 || got.Runs != 3 || got.LastOutcome != JobOutcomePanicked || got.Running {
		t.Fatalf("unexpected history after failures: %+v", got)
	}
	if got.FailingSince == nil || first.FailingSince == nil || !got.FailingSince.Equal(*first.FailingSince) {
		t.Fatalf("failing since must stay at the first failure: first=%v now=%v", first.FailingSince, got.FailingSince)
	}
	if !strings.Contains(got.LastError, "panic: boom") {
		t.Fatalf("panic not recorded as the last error: %q", got.LastError)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runJobWith("TranscribePendingEpisodes", logger, func(*zap.SugaredLogger) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	if running := JobHistorySnapshot()[0]; !running.Running {
		t.Fatalf("a job in progress must show as running: %+v", running)
	}
	close(release)
	<-done
	got = JobHistorySnapshot()[0]
	if got.ConsecutiveFailures != 0 || got.FailingSince != nil || got.LastError != "" || got.LastSuccessAt == nil || got.LastOutcome != JobOutcomeSucceeded {
		t.Fatalf("success did not reset the streak: %+v", got)
	}
}

func TestMonitoringSnapshotIncludesSummariesJobsAndCounts(t *testing.T) {
	setupRetentionTestDB(t)
	resetPipelineStatsCacheForTests()
	t.Cleanup(resetPipelineStatsCacheForTests)
	t.Setenv("WHISPERX_ENABLED", "false")
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "secret-key-value")
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("LLM_BASE_URL", "http://192.168.1.50:11434/v1")
	t.Setenv("LLM_MODEL", "qwen3.6:27b")
	t.Setenv("LLM_REASONING_EFFORT", "none")
	t.Setenv("LLM_MAX_TOKENS", "2048")
	t.Setenv("LLM_TIMEOUT_SECONDS", "900")
	setting := db.GetOrCreateSetting()
	setting.SummarizationEnabled = true
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatal(err)
	}
	podcast := createPodcast(t, "snapshot-counts", false)
	now := time.Now().UTC()
	recent, old := now.Add(-time.Hour), now.Add(-10*24*time.Hour)
	for i, item := range []db.PodcastItem{
		{TranscriptStatus: "available", LLMSummaryStatus: "available", LLMSummaryDate: &recent},
		{TranscriptStatus: "available", LLMSummaryStatus: "available", LLMSummaryDate: &old},
		{TranscriptStatus: "available", LLMSummaryStatus: "failed"},
		{TranscriptStatus: "available", LLMSummaryStatus: "processing"},
		{TranscriptStatus: "available"},
		{TranscriptStatus: "pending_whisperx"},
	} {
		item.PodcastID, item.Title, item.GUID = podcast.ID, fmt.Sprintf("episode %d", i), fmt.Sprintf("snapshot-%d", i)
		if err := db.DB.Create(&item).Error; err != nil {
			t.Fatal(err)
		}
	}
	_ = runJobWith("RefreshEpisodes", zap.NewNop().Sugar(), func(*zap.SugaredLogger) error { return nil })

	snap, err := BuildMonitoringSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"available": 2, "failed": 1, "processing": 1, "waiting": 1}
	for state, n := range want {
		if snap.Queue.SummariesByState[state] != n {
			t.Fatalf("summaries[%s]=%d want %d (all %v)", state, snap.Queue.SummariesByState[state], n, snap.Queue.SummariesByState)
		}
	}
	if snap.Windows[0].Summaries != 1 || snap.Windows[1].Summaries != 1 {
		t.Fatalf("summary windows: %+v", snap.Windows)
	}
	s := snap.Summaries
	if !s.Enabled || !s.LLMEnabled || !s.APIKeyConfigured || s.Provider != "ollama" || s.Model != "qwen3.6:27b" || s.Host != "192.168.1.50:11434" || s.ReasoningEffort != "none" || s.MaxTokens != 2048 || s.TimeoutSeconds != 900 {
		t.Fatalf("unexpected summaries config: %+v", s)
	}
	if len(snap.Jobs) != 1 || snap.Jobs[0].Name != "RefreshEpisodes" || snap.Jobs[0].LastOutcome != JobOutcomeSucceeded {
		t.Fatalf("job history missing: %+v", snap.Jobs)
	}
	raw, _ := json.Marshal(snap)
	for _, secret := range []string{"secret-key-value", "/v1"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("snapshot exposed %q", secret)
		}
	}
	for _, key := range []string{`"summaries":{`, `"in_flight":[]`, `"recent":[]`, `"jobs":[`, `"repair":{`, `"summaries_by_state":{`} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("snapshot JSON lacks %s: %s", key, raw)
		}
	}
}
