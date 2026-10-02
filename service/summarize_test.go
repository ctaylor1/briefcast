package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ctaylor1/briefcast/db"
)

func TestSummarizeTranscriptUsesBoundedLLMClient(t *testing.T) {
	var sawRequest bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawRequest = true
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("expected chat completions path, got %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("expected bearer auth header, got %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("expected json content type, got %q", got)
		}

		var payload openAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request payload: %v", err)
		}
		if payload.Model != "test-model" {
			t.Fatalf("expected model test-model, got %q", payload.Model)
		}
		if len(payload.Messages) != 2 || payload.Messages[1].Content != "User: transcript text" {
			t.Fatalf("unexpected messages: %+v", payload.Messages)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":" concise summary "}}]}`))
	}))
	defer server.Close()

	summary, err := SummarizeTranscript("transcript text", "System", "User: ", LLMConfig{
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "test-model",
		MaxTokens:   128,
		Temperature: 0.2,
		TimeoutSecs: 5,
	})
	if err != nil {
		t.Fatalf("SummarizeTranscript failed: %v", err)
	}
	if !sawRequest {
		t.Fatalf("expected LLM test server to receive request")
	}
	if summary != "concise summary" {
		t.Fatalf("expected trimmed summary, got %q", summary)
	}
}

func TestSummarizeTranscriptRejectsInvalidLLMBaseURL(t *testing.T) {
	_, err := SummarizeTranscript("transcript text", "System", "User: ", LLMConfig{
		APIKey:  "test-key",
		BaseURL: "file:///tmp/llm",
		Model:   "test-model",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid LLM base URL") {
		t.Fatalf("expected invalid LLM base URL error, got %v", err)
	}
}

func TestSummarizeTranscriptRejectsOversizedLLMResponse(t *testing.T) {
	t.Setenv(outboundMaxResponseBytesEnv, "64")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 65)))
	}))
	defer server.Close()

	_, err := SummarizeTranscript("transcript text", "System", "User: ", LLMConfig{
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "test-model",
		TimeoutSecs: 5,
	})
	if err == nil || !strings.Contains(err.Error(), "outbound response exceeds 64 bytes") {
		t.Fatalf("expected bounded response error, got %v", err)
	}
}

func TestSummarizeEpisodeRecordsResponseModel(t *testing.T) {
	_, item := seedCharacterizationPodcastItem(t, nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"model":"google/gemini-3.8-flash","choices":[{"message":{"content":"new summary"}}]}`))
	}))
	defer server.Close()

	if err := SummarizeEpisode(&item, LLMConfig{
		APIKey: "test-key", BaseURL: server.URL, Model: "openrouter/auto", TimeoutSecs: 5,
	}, "prompt", "User: "); err != nil {
		t.Fatalf("summarize episode: %v", err)
	}
	var saved db.PodcastItem
	if err := db.GetPodcastItemByID(item.ID, &saved); err != nil {
		t.Fatalf("reload episode: %v", err)
	}
	if saved.LLMSummaryModel != "google/gemini-3.8-flash" {
		t.Fatalf("expected routed model, got %q", saved.LLMSummaryModel)
	}
}

func TestSummarizeTranscriptEmptyResponseIncludesSafeMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"model":"routed-model","choices":[{"message":{"content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":8,"total_tokens":108}}`))
	}))
	defer server.Close()

	_, err := SummarizeTranscript("private transcript", "prompt", "User: ", LLMConfig{
		APIKey: "test-key", BaseURL: server.URL, Model: "openrouter/auto", TimeoutSecs: 5,
	})
	if err == nil {
		t.Fatal("expected empty summary error")
	}
	for _, part := range []string{"model=routed-model", "finish_reason=stop", "tokens=100/8/108"} {
		if !strings.Contains(err.Error(), part) {
			t.Fatalf("expected %q in error: %v", part, err)
		}
	}
	if strings.Contains(err.Error(), "private transcript") {
		t.Fatalf("error exposed transcript: %v", err)
	}
}

func TestSummarizeTranscriptRejectsPartialResponseAtTokenLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"model":"routed-model","choices":[{"message":{"content":"incomplete fragment"},"finish_reason":"length"}]}`))
	}))
	defer server.Close()

	summary, err := SummarizeTranscript("transcript", "prompt", "User: ", LLMConfig{
		APIKey: "test-key", BaseURL: server.URL, Model: "openrouter/auto", MaxTokens: 1600, TimeoutSecs: 5,
	})
	if summary != "" || err == nil || !strings.Contains(err.Error(), "max_tokens=1600") {
		t.Fatalf("expected partial response to be rejected, got summary=%q err=%v", summary, err)
	}
}

func TestSummarizeEpisodeMissingTranscriptPreservesExistingSummary(t *testing.T) {
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, _ string) {
		item.CanonicalTranscript = ""
	})
	before := snapshotPodcastItemRow(t, item.ID)
	err := SummarizeEpisode(&item, LLMConfig{}, "prompt", "User: ")
	if err == nil || !strings.Contains(err.Error(), "no canonical transcript") {
		t.Fatalf("expected missing transcript error, got %v", err)
	}
	after := snapshotPodcastItemRow(t, item.ID)
	assertPodcastItemChangedColumns(t, before, after, "llm_summary_error")
	if item.LLMSummaryStatus != "available" || item.LLMSummary != "seed llm summary" || item.LLMSummaryModel != "seed-llm-model" {
		t.Fatalf("existing summary changed: status=%q summary=%q model=%q", item.LLMSummaryStatus, item.LLMSummary, item.LLMSummaryModel)
	}
}

func TestSummarizeTranscriptDoesNotExposeProviderErrorBody(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "HTTP error", status: http.StatusUnauthorized, body: `{"error":{"message":"api_key=private-token"}}`},
		{name: "JSON error", status: http.StatusOK, body: `{"error":{"message":"api_key=private-token"}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			_, err := SummarizeTranscript("transcript", "prompt", "User: ", LLMConfig{
				APIKey: "test-key", BaseURL: server.URL, Model: "requested-model", TimeoutSecs: 5,
			})
			if err == nil || strings.Contains(err.Error(), "private-token") {
				t.Fatalf("expected redacted provider error, got %v", err)
			}
		})
	}
}
