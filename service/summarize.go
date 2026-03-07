package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/logging"
)

const defaultSummarizationPrompt = "You are a helpful podcast summarization assistant. Given the following podcast transcript, produce a concise summary that captures the key topics, main arguments, notable quotes, and any actionable takeaways. Structure the summary with clear sections. Keep it under 500 words."

const defaultUserPrompt = "Here is the podcast transcript:\n\n"

const maxTranscriptChars = 200000

// LLMConfig holds configuration for the LLM provider, loaded from environment variables.
type LLMConfig struct {
	Enabled           bool
	Provider          string
	APIKey            string
	BaseURL           string
	Model             string
	MaxTokens         int
	Temperature       float64
	TimeoutSecs       int
	DefaultPrompt     string
	DefaultUserPrompt string
}

// LoadLLMConfig reads LLM settings from environment variables.
func LoadLLMConfig() LLMConfig {
	return LLMConfig{
		Enabled:           getEnvBool("LLM_ENABLED", false),
		Provider:          getEnvString("LLM_PROVIDER", "openai"),
		APIKey:            strings.TrimSpace(os.Getenv("LLM_API_KEY")),
		BaseURL:           getEnvString("LLM_BASE_URL", "https://api.openai.com/v1"),
		Model:             getEnvString("LLM_MODEL", "gpt-4o-mini"),
		MaxTokens:         getEnvInt("LLM_MAX_TOKENS", 1024),
		Temperature:       getEnvFloat("LLM_TEMPERATURE", 0.3),
		TimeoutSecs:       getEnvInt("LLM_TIMEOUT_SECONDS", 120),
		DefaultPrompt:     getEnvString("LLM_SUMMARIZATION_PROMPT", defaultSummarizationPrompt),
		DefaultUserPrompt: getEnvString("LLM_SUMMARIZATION_USER_PROMPT", defaultUserPrompt),
	}
}

type openAIChatRequest struct {
	Model       string              `json:"model"`
	Messages    []openAIChatMessage `json:"messages"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
	Temperature float64             `json:"temperature,omitempty"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// ResolveSummarizationPrompt returns the effective system prompt: DB setting if non-empty,
// otherwise the env var default.
func ResolveSummarizationPrompt(setting *db.Setting, cfg LLMConfig) string {
	if setting != nil {
		if prompt := strings.TrimSpace(setting.SummarizationPrompt); prompt != "" {
			return prompt
		}
	}
	return cfg.DefaultPrompt
}

// ResolveSummarizationUserPrompt returns the effective user prompt prefix:
// DB setting if non-empty, otherwise the env var default.
func ResolveSummarizationUserPrompt(setting *db.Setting, cfg LLMConfig) string {
	if setting != nil {
		if prompt := strings.TrimSpace(setting.SummarizationUserPrompt); prompt != "" {
			return prompt
		}
	}
	return cfg.DefaultUserPrompt
}

// SummarizeTranscript sends the canonical transcript to the configured LLM and returns the summary.
func SummarizeTranscript(transcript string, prompt string, userPrompt string, cfg LLMConfig) (string, error) {
	if strings.TrimSpace(transcript) == "" {
		return "", fmt.Errorf("transcript is empty")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return "", fmt.Errorf("LLM API key is not configured")
	}

	if len(transcript) > maxTranscriptChars {
		transcript = transcript[:maxTranscriptChars] + "\n\n[Transcript truncated due to length]"
	}

	reqBody := openAIChatRequest{
		Model: cfg.Model,
		Messages: []openAIChatMessage{
			{Role: "system", Content: prompt},
			{Role: "user", Content: userPrompt + transcript},
		},
		MaxTokens:   cfg.MaxTokens,
		Temperature: cfg.Temperature,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to encode LLM request: %w", err)
	}

	url := strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"

	ctx := context.Background()
	cancel := func() {}
	if cfg.TimeoutSecs > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(cfg.TimeoutSecs)*time.Second)
	}
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("failed to create LLM request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("LLM request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read LLM response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM API returned status %d: %s", resp.StatusCode, truncateForLog(string(body), 500))
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("failed to parse LLM response: %w", err)
	}

	if chatResp.Error != nil && chatResp.Error.Message != "" {
		return "", fmt.Errorf("LLM API error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("LLM returned no choices")
	}

	summary := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	if summary == "" {
		return "", fmt.Errorf("LLM returned empty summary")
	}

	return summary, nil
}

// SummarizeEpisode summarizes a single episode's canonical transcript using the configured LLM.
// It persists the result (including lineage: model and prompt used) to the database.
func SummarizeEpisode(item *db.PodcastItem, cfg LLMConfig, prompt string, userPrompt string) error {
	jobLogger := logging.Sugar()

	transcript := strings.TrimSpace(item.CanonicalTranscript)
	if transcript == "" {
		item.LLMSummaryStatus = "failed"
		item.LLMSummaryError = "no canonical transcript available"
		return db.UpdatePodcastItem(item)
	}

	item.LLMSummaryStatus = "processing"
	if err := db.UpdatePodcastItem(item); err != nil {
		jobLogger.Warnw("failed to mark summary processing", "podcast_item_id", item.ID, "error", err)
	}

	summary, err := SummarizeTranscript(transcript, prompt, userPrompt, cfg)
	if err != nil {
		item.LLMSummaryStatus = "failed"
		item.LLMSummaryError = trimToLength(err.Error(), 1000)
		if updateErr := db.UpdatePodcastItem(item); updateErr != nil {
			jobLogger.Warnw("failed to mark summary failure", "podcast_item_id", item.ID, "error", updateErr)
		}
		return err
	}

	now := time.Now().UTC()
	item.LLMSummary = summary
	item.LLMSummaryStatus = "available"
	item.LLMSummaryError = ""
	item.LLMSummaryDate = &now
	// Lineage: record which model and prompt produced this summary.
	item.LLMSummaryModel = cfg.Model
	item.LLMSummaryPrompt = prompt
	return db.UpdatePodcastItem(item)
}
