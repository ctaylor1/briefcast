package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ctaylor1/briefcast/db"
)

func TestAddPodcastItemsQueuesUnavailableFeedTranscriptsAndUsesSettingsModel(t *testing.T) {
	setupRetentionTestDB(t)
	pythonPath := requireWorkingPython(t)
	models := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed":
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(`<rss><channel><title>Test feed</title></channel></rss>`))
		case "/good.txt":
			_, _ = w.Write([]byte("A useful transcript."))
		case "/empty.txt":
			_, _ = w.Write([]byte("  \n  "))
		case "/missing.txt":
			http.NotFound(w, r)
		case "/chat/completions":
			var request struct {
				Model string `json:"model"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			models <- request.Model
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Summary"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	script := writeTempScript(t, t.TempDir(), "feedparser_summary_pipeline.py", `import json
import os
import sys
base = os.environ["TEST_BASE_URL"]
json.dump({"feed": {"title": "Test feed"}, "entries": [
  {"guid": "good", "title": "Good", "transcripts": [{"url": base + "/good.txt", "type": "text/plain"}]},
  {"guid": "empty", "title": "Empty", "transcripts": [{"url": base + "/empty.txt", "type": "text/plain"}]},
  {"guid": "missing", "title": "Missing", "transcripts": [{"url": base + "/missing.txt", "type": "text/plain"}]}
]}, sys.stdout)
`)
	t.Setenv("TEST_BASE_URL", server.URL)
	t.Setenv(feedparserPythonEnv, pythonPath)
	t.Setenv(feedparserScriptEnv, script)
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_BASE_URL", server.URL)
	t.Setenv("LLM_MODEL", "environment-model")
	setting := db.GetOrCreateSetting()
	setting.SummarizationEnabled = true
	setting.SummarizationModel = "settings-model"
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatalf("update settings: %v", err)
	}

	podcast := createPodcast(t, "summary-feed", false)
	podcast.URL = server.URL + "/feed"
	if err := AddPodcastItems(&podcast, false); err != nil {
		t.Fatalf("add episodes: %v", err)
	}
	items := make([]db.PodcastItem, 0)
	if err := db.GetAllPodcastItemsByPodcastID(podcast.ID, &items); err != nil {
		t.Fatalf("load episodes: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected three episodes, got %d", len(items))
	}
	for _, item := range items {
		switch item.GUID {
		case "good":
			if item.TranscriptStatus != "available" || item.LLMSummaryStatus != "available" || item.LLMSummaryModel != "settings-model" {
				t.Fatalf("good feed transcript not summarized with Settings model: transcript=%q summary=%q model=%q", item.TranscriptStatus, item.LLMSummaryStatus, item.LLMSummaryModel)
			}
		case "empty", "missing":
			if item.TranscriptStatus != "pending_whisperx" || item.TranscriptJSON != "" || item.CanonicalTranscript != "" || item.TranscriptProgressStage != "queued" {
				t.Fatalf("%s transcript should remain queued: status=%q json=%q canonical=%q stage=%q", item.GUID, item.TranscriptStatus, item.TranscriptJSON, item.CanonicalTranscript, item.TranscriptProgressStage)
			}
			if item.LLMSummaryStatus != "" {
				t.Fatalf("%s transcript should not be summarized, got status %q", item.GUID, item.LLMSummaryStatus)
			}
		default:
			t.Fatalf("unexpected episode %q", item.GUID)
		}
	}
	select {
	case got := <-models:
		if got != "settings-model" {
			t.Fatalf("expected Settings model in request, got %q", got)
		}
	default:
		t.Fatal("expected a summarization request")
	}
}

func TestWhisperXSummarizationUsesSettingsModel(t *testing.T) {
	setupRetentionTestDB(t)
	pythonPath := requireWorkingPython(t)
	models := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		models <- request.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"WhisperX summary"}}]}`))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	script := writeTempScript(t, tempDir, "whisperx_summary_pipeline.py", `import json
import sys
json.dump({"segments": [{"start": 0, "end": 1, "text": "Useful transcription."}]}, sys.stdout)
`)
	audioPath := filepath.Join(tempDir, "episode.mp3")
	if err := os.WriteFile(audioPath, []byte(strings.Repeat("a", 2048)), 0o644); err != nil {
		t.Fatalf("create audio fixture: %v", err)
	}
	podcast := createPodcast(t, "summary-whisperx", false)
	item := createServicePodcastItem(t, podcast, "episode", db.Downloaded)
	item.DownloadPath = audioPath
	item.TranscriptStatus = "pending_whisperx"
	item.TranscriptJSON = ""
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("update episode: %v", err)
	}
	t.Setenv(whisperxEnabledEnv, "true")
	t.Setenv(whisperxPythonEnv, pythonPath)
	t.Setenv(whisperxScriptEnv, script)
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_BASE_URL", server.URL)
	t.Setenv("LLM_MODEL", "environment-model")
	setting := db.GetOrCreateSetting()
	setting.SummarizationEnabled = true
	setting.SummarizationModel = "settings-model"
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatalf("update settings: %v", err)
	}

	if err := TranscribePendingEpisodes(); err != nil {
		t.Fatalf("transcribe pending episodes: %v", err)
	}
	var refreshed db.PodcastItem
	if err := db.GetPodcastItemByID(item.ID, &refreshed); err != nil {
		t.Fatalf("reload episode: %v", err)
	}
	if refreshed.TranscriptStatus != "available" || refreshed.LLMSummaryStatus != "available" || refreshed.LLMSummaryModel != "settings-model" {
		t.Fatalf("WhisperX transcript not summarized with Settings model: transcript=%q summary=%q model=%q", refreshed.TranscriptStatus, refreshed.LLMSummaryStatus, refreshed.LLMSummaryModel)
	}
	select {
	case got := <-models:
		if got != "settings-model" {
			t.Fatalf("expected Settings model in request, got %q", got)
		}
	default:
		t.Fatal("expected a summarization request")
	}
}
