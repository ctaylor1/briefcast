package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
)

// A stalled LLM provider must not hold up adding episodes from a feed.
func TestAddPodcastItemsDoesNotWaitForTheLLM(t *testing.T) {
	setupRetentionTestDB(t)
	pythonPath := requireWorkingPython(t)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/good.txt":
			_, _ = w.Write([]byte("A feed transcript."))
		case "/chat/completions":
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Summary"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() {
		close(release)
		waitForSummaryJobIdle(t)
	})

	script := writeTempScript(t, t.TempDir(), "feedparser_stalled_llm.py", `import json, sys
base = open(__file__ + ".base").read().strip()
json.dump({"feed": {"title": "Stalled"}, "entries": [
  {"guid": "good", "title": "Good", "transcripts": [{"url": base + "/good.txt", "type": "text/plain"}]}
]}, sys.stdout)
`)
	setStubBaseURL(t, script, server.URL)
	t.Setenv(feedparserPythonEnv, pythonPath)
	t.Setenv(feedparserScriptEnv, script)
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_BASE_URL", server.URL)
	setting := db.GetOrCreateSetting()
	setting.SummarizationEnabled = true
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatal(err)
	}

	podcast := createPodcast(t, "stalled-llm", false)
	podcast.URL = server.URL + "/feed"
	done := make(chan error, 1)
	go func() { done <- AddPodcastItems(&podcast, false) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("AddPodcastItems: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("adding episodes waited on the LLM provider")
	}
	if !GetSummaryBackfillRunning() {
		t.Fatal("expected the summary backfill to have been started")
	}
}

func pendingSummaryCount() int {
	pendingSummaryMu.Lock()
	defer pendingSummaryMu.Unlock()
	return len(pendingSummaryIDs)
}

func TestRequestSummaryBackfillRunsAgainAfterABusyJob(t *testing.T) {
	setupRetentionTestDB(t)
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "test-key")
	if err := reserveSummaryJob(); err != nil {
		t.Fatal(err)
	}
	RequestSummaryBackfill([]string{"episode-a", "episode-b"})
	if pendingSummaryCount() != 2 {
		t.Fatal("a request during a running job must be remembered")
	}
	finishSummaryJob(0, 0, nil)
	runRequestedSummaryBackfill()
	if pendingSummaryCount() != 0 {
		t.Fatal("the remembered request was not consumed")
	}
	waitForSummaryJobIdle(t)
}

// A feed refresh asks for its new episodes only, never the whole back catalog.
func TestRequestSummaryBackfillSummarizesOnlyTheRequestedEpisodes(t *testing.T) {
	setupRetentionTestDB(t)
	podcast := createPodcast(t, "requested-only", false)
	added := db.PodcastItem{PodcastID: podcast.ID, Title: "added", CanonicalTranscript: "new transcript"}
	older := db.PodcastItem{PodcastID: podcast.ID, Title: "older", CanonicalTranscript: "old transcript"}
	for _, item := range []*db.PodcastItem{&added, &older} {
		if err := db.CreatePodcastItem(item); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"a summary"}}]}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_BASE_URL", server.URL)

	RequestSummaryBackfill([]string{added.ID})
	waitForSummaryJobIdle(t)

	var gotAdded, gotOlder db.PodcastItem
	if err := db.GetPodcastItemByID(added.ID, &gotAdded); err != nil {
		t.Fatal(err)
	}
	if err := db.GetPodcastItemByID(older.ID, &gotOlder); err != nil {
		t.Fatal(err)
	}
	if gotAdded.LLMSummaryStatus != "available" {
		t.Fatalf("requested episode was not summarized: status=%q", gotAdded.LLMSummaryStatus)
	}
	if gotOlder.LLMSummaryStatus != "" || gotOlder.LLMSummary != "" {
		t.Fatalf("an episode that was not requested was summarized: status=%q", gotOlder.LLMSummaryStatus)
	}
	if status := GetSummaryJobStatus(); status.Total != 1 {
		t.Fatalf("expected a run over one episode, got %+v", status)
	}
}

func TestRequestSummaryBackfillWithNoEpisodesStartsNothing(t *testing.T) {
	setupRetentionTestDB(t)
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "test-key")
	RequestSummaryBackfill(nil)
	if GetSummaryBackfillRunning() {
		t.Fatal("an empty request must not start a summary job")
	}
}

func TestJobLockHeartbeatStopIsFinalAndCannotReviveARelease(t *testing.T) {
	setupRetentionTestDB(t)
	lock, acquired, err := db.TryLock("HeartbeatTest", 120)
	if err != nil || !acquired {
		t.Fatalf("TryLock: %v %v", acquired, err)
	}
	stop := startJobLockHeartbeat(lock.ID, 120, 1, nil)
	stop()
	db.UnlockByID(lock.ID)
	if err := db.RefreshLockByID(lock.ID, 120); err != nil {
		t.Fatal(err)
	}
	_, acquiredAgain, err := db.TryLock("HeartbeatTest", 120)
	if err != nil || !acquiredAgain {
		t.Fatalf("a late renewal revived a released lock: acquired=%v err=%v", acquiredAgain, err)
	}
}
