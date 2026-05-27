package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
