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

func TestRequestSummaryBackfillRunsAgainAfterABusyJob(t *testing.T) {
	setupRetentionTestDB(t)
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "test-key")
	if err := reserveSummaryJob(); err != nil {
		t.Fatal(err)
	}
	RequestSummaryBackfill()
	if !summaryBackfillRequested.Load() {
		t.Fatal("a request during a running job must be remembered")
	}
	finishSummaryJob(0, 0, nil)
	runRequestedSummaryBackfill()
	if summaryBackfillRequested.Load() {
		t.Fatal("the remembered request was not consumed")
	}
	waitForSummaryJobIdle(t)
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
