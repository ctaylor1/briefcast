package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testOpenRouterModel(id string) openRouterModelRecord {
	var record openRouterModelRecord
	record.ID = id
	record.Name = "Test model"
	record.ContextLength = 200000
	record.TopProvider.MaxCompletionTokens = 8192
	record.Architecture.InputModalities = []string{"text"}
	record.Architecture.OutputModalities = []string{"text"}
	record.SupportedParameters = []string{"temperature", "max_tokens"}
	record.Pricing.Prompt = "0.0000002"
	record.Pricing.Completion = "0.000001"
	return record
}

func TestSelectOpenRouterRecommendationsRequiresSummaryCompatibility(t *testing.T) {
	compatible := testOpenRouterModel("google/gemini-3.8-flash")
	unsupportedParameter := testOpenRouterModel("openai/gpt-4.1-mini")
	unsupportedParameter.SupportedParameters = []string{"max_tokens"}
	shortContext := testOpenRouterModel("anthropic/claude-haiku-4.5")
	shortContext.ContextLength = 8192
	noOutputRoom := testOpenRouterModel("anthropic/claude-sonnet-4.6")
	noOutputRoom.TopProvider.MaxCompletionTokens = 2048
	invalidPrice := testOpenRouterModel("deepseek/deepseek-v3.2")
	invalidPrice.Pricing.Prompt = "-1"
	unknown := testOpenRouterModel("unknown/cheap-model")

	models := selectOpenRouterRecommendations([]openRouterModelRecord{
		compatible, unsupportedParameter, shortContext, noOutputRoom, invalidPrice, unknown,
	})
	if len(models) != 1 || models[0].ID != compatible.ID {
		t.Fatalf("expected only the compatible recommended model, got %+v", models)
	}
	if models[0].PromptPrice != compatible.Pricing.Prompt || models[0].MaxCompletionTokens != 8192 {
		t.Fatalf("catalog metadata was not preserved: %+v", models[0])
	}
}

func TestOpenRouterCatalogRefreshAndStaleFallback(t *testing.T) {
	var requests atomic.Int32
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Error("catalog request sent an API key")
		}
		if fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(struct {
			Data []openRouterModelRecord `json:"data"`
		}{Data: []openRouterModelRecord{testOpenRouterModel("google/gemini-3.8-flash")}})
	}))
	defer server.Close()
	catalog := &openRouterCatalogClient{url: server.URL, client: server.Client()}

	first, err := catalog.get(context.Background(), false)
	if err != nil || first.Stale || len(first.Models) != 1 || first.FetchedAt.IsZero() {
		t.Fatalf("unexpected first catalog: %+v, err=%v", first, err)
	}
	_, err = catalog.get(context.Background(), false)
	if err != nil || requests.Load() != 1 {
		t.Fatalf("expected cached catalog, requests=%d err=%v", requests.Load(), err)
	}
	refreshed, err := catalog.get(context.Background(), true)
	if err != nil || requests.Load() != 2 || refreshed.Stale {
		t.Fatalf("expected fresh fetch, requests=%d catalog=%+v err=%v", requests.Load(), refreshed, err)
	}
	fail.Store(true)
	stale, err := catalog.get(context.Background(), true)
	if err != nil || !stale.Stale || len(stale.Models) != 1 || requests.Load() != 3 {
		t.Fatalf("expected stale fallback, requests=%d catalog=%+v err=%v", requests.Load(), stale, err)
	}
	stillStale, err := catalog.get(context.Background(), false)
	if err != nil || !stillStale.Stale || requests.Load() != 3 {
		t.Fatalf("expected cached stale state, requests=%d catalog=%+v err=%v", requests.Load(), stillStale, err)
	}
}

func TestOpenRouterCatalogRejectsBadResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid JSON", body: "{"},
		{name: "missing data", body: "{}"},
		{name: "empty recommendations", body: `{"data":[]}`},
		{name: "oversized response", body: strings.Repeat("x", openRouterCatalogMaxBytes+1)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			catalog := &openRouterCatalogClient{url: server.URL, client: &http.Client{Timeout: time.Second}}
			if _, err := catalog.get(context.Background(), false); err == nil {
				t.Fatal("expected a catalog error")
			}
		})
	}
}
