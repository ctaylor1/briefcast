package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	openRouterModelsURL       = "https://openrouter.ai/api/v1/models"
	openRouterCatalogTTL      = time.Hour
	openRouterCatalogTimeout  = 10 * time.Second
	openRouterCatalogMaxBytes = 4 * 1024 * 1024
)

// OpenRouterRecommendedModel contains current catalog metadata for a model
// compatible with Briefcast's transcript summarization request.
type OpenRouterRecommendedModel struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	ContextLength       int    `json:"contextLength"`
	MaxCompletionTokens int    `json:"maxCompletionTokens"`
	PromptPrice         string `json:"promptPrice"`
	CompletionPrice     string `json:"completionPrice"`
	Recommendation      string `json:"recommendation"`
}

type OpenRouterModelCatalog struct {
	Models    []OpenRouterRecommendedModel `json:"models"`
	FetchedAt time.Time                    `json:"fetchedAt"`
	Stale     bool                         `json:"stale"`
}

type openRouterCatalogClient struct {
	mu     sync.Mutex
	url    string
	client *http.Client
	cached OpenRouterModelCatalog
}

// The catalog is shared across Settings requests to avoid repeated upstream
// calls. It contains public model metadata only, never API credentials.
var openRouterCatalog = &openRouterCatalogClient{
	url: openRouterModelsURL,
	client: &http.Client{
		Timeout:   openRouterCatalogTimeout,
		Transport: outboundHTTPTransport(outboundPurposeHTTP),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	},
}

// GetOpenRouterRecommendedModels returns the current recommended model list.
// refresh bypasses the one-hour cache; a prior catalog is returned as stale if
// OpenRouter is temporarily unavailable.
func GetOpenRouterRecommendedModels(ctx context.Context, refresh bool) (OpenRouterModelCatalog, error) {
	return openRouterCatalog.get(ctx, refresh)
}

func (catalog *openRouterCatalogClient) get(ctx context.Context, refresh bool) (OpenRouterModelCatalog, error) {
	catalog.mu.Lock()
	cached := catalog.cached
	catalog.mu.Unlock()
	if !refresh && !cached.FetchedAt.IsZero() && time.Since(cached.FetchedAt) < openRouterCatalogTTL {
		return cached, nil
	}

	fresh, err := catalog.fetch(ctx)
	if err != nil {
		catalog.mu.Lock()
		latest := catalog.cached
		if !latest.FetchedAt.IsZero() {
			if !latest.FetchedAt.After(cached.FetchedAt) {
				latest.Stale = true
				catalog.cached.Stale = true
			}
			catalog.mu.Unlock()
			return latest, nil
		}
		catalog.mu.Unlock()
		return OpenRouterModelCatalog{}, err
	}
	catalog.mu.Lock()
	catalog.cached = fresh
	catalog.mu.Unlock()
	return fresh, nil
}

type openRouterModelRecord struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextLength int    `json:"context_length"`
	Architecture  struct {
		InputModalities  []string `json:"input_modalities"`
		OutputModalities []string `json:"output_modalities"`
	} `json:"architecture"`
	TopProvider struct {
		MaxCompletionTokens int `json:"max_completion_tokens"`
	} `json:"top_provider"`
	Pricing struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
	SupportedParameters []string `json:"supported_parameters"`
}

// The list is intentionally short. Current metadata comes from OpenRouter on
// each refresh, and unavailable or incompatible models disappear from results.
var recommendedOpenRouterModels = []struct {
	id     string
	reason string
}{
	{"google/gemini-3.8-flash", "Large context and moderate token pricing."},
	{"google/gemini-3.5-flash-lite", "Large context with lower token pricing."},
	{"openai/gpt-4.1-mini", "Large context with moderate token pricing."},
	{"anthropic/claude-haiku-4.5", "200K context with moderate token pricing."},
	{"anthropic/claude-sonnet-4.6", "Large context at a higher token price."},
	{"deepseek/deepseek-v3.2", "163K context with lower token pricing."},
}

func (catalog *openRouterCatalogClient) fetch(ctx context.Context) (OpenRouterModelCatalog, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, catalog.url, nil)
	if err != nil {
		return OpenRouterModelCatalog{}, fmt.Errorf("create OpenRouter catalog request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := catalog.client.Do(request)
	if err != nil {
		return OpenRouterModelCatalog{}, fmt.Errorf("fetch OpenRouter model catalog: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return OpenRouterModelCatalog{}, fmt.Errorf("OpenRouter model catalog returned status %d", response.StatusCode)
	}
	if response.ContentLength > openRouterCatalogMaxBytes {
		return OpenRouterModelCatalog{}, fmt.Errorf("OpenRouter model catalog response is too large")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, openRouterCatalogMaxBytes+1))
	if err != nil {
		return OpenRouterModelCatalog{}, fmt.Errorf("read OpenRouter model catalog: %w", err)
	}
	if len(body) > openRouterCatalogMaxBytes {
		return OpenRouterModelCatalog{}, fmt.Errorf("OpenRouter model catalog response is too large")
	}
	var payload struct {
		Data []openRouterModelRecord `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return OpenRouterModelCatalog{}, fmt.Errorf("decode OpenRouter model catalog: %w", err)
	}
	if payload.Data == nil {
		return OpenRouterModelCatalog{}, fmt.Errorf("OpenRouter model catalog has no model list")
	}
	models := selectOpenRouterRecommendations(payload.Data)
	if len(models) == 0 {
		return OpenRouterModelCatalog{}, fmt.Errorf("OpenRouter model catalog has no compatible recommended models")
	}
	return OpenRouterModelCatalog{
		Models:    models,
		FetchedAt: time.Now().UTC(),
	}, nil
}

func selectOpenRouterRecommendations(records []openRouterModelRecord) []OpenRouterRecommendedModel {
	byID := make(map[string]openRouterModelRecord, len(records))
	for _, record := range records {
		byID[record.ID] = record
	}
	models := make([]OpenRouterRecommendedModel, 0, len(recommendedOpenRouterModels))
	for _, candidate := range recommendedOpenRouterModels {
		record, ok := byID[candidate.id]
		if !ok || record.ContextLength < 128000 || record.TopProvider.MaxCompletionTokens < 4096 ||
			!containsCatalogValue(record.Architecture.InputModalities, "text") ||
			!containsCatalogValue(record.Architecture.OutputModalities, "text") ||
			!containsCatalogValue(record.SupportedParameters, "temperature") ||
			!containsCatalogValue(record.SupportedParameters, "max_tokens") ||
			!validOpenRouterPrice(record.Pricing.Prompt) || !validOpenRouterPrice(record.Pricing.Completion) {
			continue
		}
		models = append(models, OpenRouterRecommendedModel{
			ID:                  record.ID,
			Name:                strings.TrimSpace(record.Name),
			ContextLength:       record.ContextLength,
			MaxCompletionTokens: record.TopProvider.MaxCompletionTokens,
			PromptPrice:         record.Pricing.Prompt,
			CompletionPrice:     record.Pricing.Completion,
			Recommendation:      candidate.reason,
		})
	}
	return models
}

func containsCatalogValue(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func validOpenRouterPrice(value string) bool {
	price, err := strconv.ParseFloat(value, 64)
	return err == nil && price >= 0 && price < 1
}
