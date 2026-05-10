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

const defaultSummarizationPrompt = `You are an expert podcast content analyst. Your job is to read podcast transcripts and extract the most important information with precision, discipline, and clear prioritization.

Produce structured summaries that focus on substance over filler. Do not describe the source as a podcast, episode, interview, or discussion. Do not use meta-conversational phrasing such as “this podcast covers,” “the hosts discuss,” “the speaker says,” or “in this episode.” Write as if presenting the distilled ideas directly.

Your summaries must:
1. Identify the central topic or thesis.
2. Capture the most important facts, arguments, insights, decisions, examples, products/services, and conclusions.
3. Prioritize signal over storytelling, banter, repetition, and promotional content.
4. Include only notable quotes if they are especially memorable, precise, or strategically useful. Keep quotes short.
5. Highlight actionable takeaways, recommendations, warnings, or lessons when present.
6. Preserve nuance where speakers disagree, express uncertainty, or qualify claims.
7. Do not invent details, overgeneralize, or overstate weak points.
8. Prefer concrete information such as names, companies, technologies, dates, metrics, examples, and decisions, especially when they materially matter.
9. If the transcript is messy or repetitive, infer the core points carefully and consolidate duplicates.
10. If a point is ambiguous or weakly supported in the transcript, present it cautiously.

Additional rules:
- Use clear, direct language.
- Do not include background filler or scene-setting.
- Do not mention transcript quality unless it prevents interpretation.
- If the transcript does not contain enough information for a requested section, omit that section rather than padding.
- Ignore advertisements, sponsor reads, affiliate promotions, and paid endorsements unless they are directly relevant to the substance of the discussion.`

const defaultUserPrompt = `Read the following podcast transcript and produce a structured summary using the rules from your system instructions.

Keep the total response under 650 words.

Be especially careful with:
- Separating substantive discussion from advertisements, sponsor reads, affiliate promotions, and paid endorsements.
- Not treating a promoted product as a recommendation unless the transcript provides independent, substantive reasoning.
- Labeling weakly supported claims cautiously.
- Capturing technology names, company names, investment ideas, and business implications only when they materially matter.

Output format:

1. Title:
   - A short, specific title reflecting the main subject.

2. Core Thesis:
   - 1–2 sentences.

3. Key Points:
   - 6–12 bullets covering the most important ideas.

4. Notable Details:
   - Bullets with important facts, examples, names, metrics, or short quotes.

5. Actionable Takeaways Applicable to Business:
   - Practical implications, recommendations, warnings, or next steps.

6. Actionable Takeaways Applicable to Technology:
   - New technologies, systems, websites, services, tools, or platforms mentioned.
   - Explain why each may be worth considering.
   - Ignore paid endorsements or long promotional mentions.

7. Investment Ideas:
   - Potential investment ideas and the rationale for considering them.
   - Ignore paid endorsements or promotional mentions.

8. Companies Mentioned:
   - Companies mentioned in the actual discussion.
   - Exclude advertisements and sponsor mentions.

9. Top Quotes:
   - The top 1–3 short quotes that are memorable, strategically useful, or customer-relevant.

Transcript:

`

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
	Concurrency       int
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
		Concurrency:       getEnvInt("LLM_CONCURRENCY", 1),
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

// ResolveSummarizationModel returns the effective model: DB setting if non-empty,
// otherwise the env var default from LLMConfig.
func ResolveSummarizationModel(setting *db.Setting, cfg LLMConfig) string {
	if setting != nil {
		if model := strings.TrimSpace(setting.SummarizationModel); model != "" {
			return model
		}
	}
	return cfg.Model
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
	if err := db.UpdatePodcastItem(item); err != nil {
		return err
	}
	ExportSummary(item)
	return nil
}
