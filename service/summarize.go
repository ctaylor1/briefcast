package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/logging"
	pm "github.com/ctaylor1/briefcast/internal/podcastmarkdown"
	"github.com/google/uuid"
)

const defaultSummarizationPrompt = `You are an expert podcast content analyst. Your job is to read podcast transcripts and extract the most important information with precision, discipline, and clear prioritization.

Produce structured summaries that focus on substance over filler. Use direct language while preserving source attribution, material qualifications, and supporting segment identifiers or timestamps when available.

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

const summaryEvidenceInstructions = "\n\nPreserve material qualifications and attribution. Support quotes and material claims with the supplied segment identifiers or timestamps when available. Never invent timestamps, speaker identities, or evidence locators."

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
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
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
	summary, _, err := summarizeTranscriptWithModel(transcript, prompt, userPrompt, cfg)
	return summary, err
}

func summarizeTranscriptWithModel(transcript string, prompt string, userPrompt string, cfg LLMConfig) (string, string, error) {
	return summarizeTranscriptWithReport(transcript, prompt, userPrompt, cfg, nil)
}

func preparedSummaryTranscript(transcript string) (string, int) {
	length := len(transcript)
	if length > maxTranscriptChars {
		length = maxTranscriptChars
		for length > 0 && !utf8.RuneStart(transcript[length]) {
			length--
		}
		return transcript[:length] + "\n\n[Transcript truncated due to length]", length
	}
	return transcript, length
}

func summarizeTranscriptWithReport(transcript string, prompt string, userPrompt string, cfg LLMConfig, report map[string]any) (string, string, error) {
	if strings.TrimSpace(transcript) == "" {
		return "", "", fmt.Errorf("transcript is empty")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return "", "", fmt.Errorf("LLM API key is not configured")
	}

	transcript, _ = preparedSummaryTranscript(transcript)

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
		return "", "", fmt.Errorf("failed to encode LLM request: %w", err)
	}

	url, err := llmChatCompletionsURL(cfg.BaseURL)
	if err != nil {
		return "", "", err
	}

	ctx := context.Background()
	cancel := func() {}
	if cfg.TimeoutSecs > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(cfg.TimeoutSecs)*time.Second)
	}
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", "", fmt.Errorf("failed to create LLM request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := llmHTTPClient(cfg).Do(req)
	if err != nil {
		return "", "", fmt.Errorf("LLM request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := readBoundedOutboundBody(resp.Body, resp.ContentLength)
	if err != nil {
		return "", "", fmt.Errorf("failed to read LLM response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("LLM API returned status %d", resp.StatusCode)
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", "", fmt.Errorf("failed to parse LLM response: %w", err)
	}

	if chatResp.Error != nil && chatResp.Error.Message != "" {
		return "", "", fmt.Errorf("LLM API returned an error")
	}

	if len(chatResp.Choices) == 0 {
		return "", "", fmt.Errorf("LLM returned no choices")
	}
	if chatResp.Choices[0].FinishReason == "length" {
		return "", "", fmt.Errorf("LLM response reached the output token limit (max_tokens=%d)", cfg.MaxTokens)
	}
	if chatResp.Choices[0].FinishReason == "content_filter" {
		return "", "", fmt.Errorf("LLM response was blocked by the provider content filter")
	}

	summary := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	if summary == "" {
		metadata := make([]string, 0, 3)
		if model := strings.TrimSpace(chatResp.Model); model != "" {
			metadata = append(metadata, "model="+truncateForLog(model, 120))
		}
		if reason := strings.TrimSpace(chatResp.Choices[0].FinishReason); reason != "" {
			metadata = append(metadata, "finish_reason="+truncateForLog(reason, 80))
		}
		if chatResp.Usage != nil {
			metadata = append(metadata, fmt.Sprintf("tokens=%d/%d/%d", chatResp.Usage.PromptTokens, chatResp.Usage.CompletionTokens, chatResp.Usage.TotalTokens))
		}
		if len(metadata) > 0 {
			return "", "", fmt.Errorf("LLM returned empty summary (%s)", strings.Join(metadata, ", "))
		}
		return "", "", fmt.Errorf("LLM returned empty summary")
	}

	model := strings.TrimSpace(chatResp.Model)
	if report != nil {
		report["summarized_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		report["model_requested"] = cfg.Model
		report["model_provider"] = "openai-compatible"
		if cfg.Provider != "" {
			report["model_provider"] = cfg.Provider
		}
		if model != "" {
			report["model_resolved"] = model
		}
		settings := map[string]any{"protocol": "openai-chat-completions"}
		if cfg.MaxTokens != 0 {
			settings["max_tokens"] = cfg.MaxTokens
		}
		if cfg.Temperature != 0 {
			settings["temperature"] = cfg.Temperature
		}
		report["generation_settings"] = settings
	}
	if model == "" {
		model = cfg.Model
	}
	return summary, model, nil
}

func llmChatCompletionsURL(baseURL string) (string, error) {
	parsed, err := validateOutboundURL(baseURL, outboundPurposeLLM)
	if err != nil {
		return "", fmt.Errorf("invalid LLM base URL: %w", err)
	}
	return strings.TrimRight(parsed.String(), "/") + "/chat/completions", nil
}

func llmHTTPClient(cfg LLMConfig) *http.Client {
	client := &http.Client{
		Transport: outboundHTTPTransport(outboundPurposeLLM),
		CheckRedirect: func(r *http.Request, _ []*http.Request) error {
			_, err := validateOutboundURL(r.URL.String(), outboundPurposeLLM)
			return err
		},
	}
	if cfg.TimeoutSecs > 0 {
		client.Timeout = time.Duration(cfg.TimeoutSecs) * time.Second
	}
	return client
}

// SummarizeEpisode summarizes a single episode's canonical transcript using the configured LLM.
// It persists the result (including lineage: model and prompt used) to the database.
func SummarizeEpisode(item *db.PodcastItem, cfg LLMConfig, prompt string, userPrompt string) error {
	jobLogger := logging.Sugar()
	hasExistingSummary := strings.TrimSpace(item.LLMSummary) != ""

	transcript := strings.TrimSpace(item.CanonicalTranscript)
	if transcript == "" {
		missingTranscriptErr := fmt.Errorf("no canonical transcript available")
		if hasExistingSummary {
			item.LLMSummaryStatus = "available"
		} else {
			item.LLMSummaryStatus = "failed"
		}
		item.LLMSummaryError = missingTranscriptErr.Error()
		if err := db.UpdatePodcastItem(item); err != nil {
			return err
		}
		return missingTranscriptErr
	}

	transcript, prompt, report, err := prepareSummarySource(item, prompt, userPrompt)
	if err != nil {
		return err
	}
	// Preserve a prior summary and its known provenance before replacing it.
	if hasExistingSummary {
		if _, err := EpisodeMarkdown(item, "summary"); err != nil {
			return fmt.Errorf("preserve previous summary: %w", err)
		}
		item.LLMSummaryStatus = "available"
	} else {
		item.LLMSummaryStatus = "processing"
	}
	if err := db.UpdatePodcastItem(item); err != nil {
		return err
	}
	summary, model, err := summarizeTranscriptWithReport(transcript, prompt, userPrompt, cfg, report)
	if err != nil {
		if hasExistingSummary {
			item.LLMSummaryStatus = "available"
		} else {
			item.LLMSummaryStatus = "failed"
		}
		item.LLMSummaryError = trimToLength(err.Error(), 1000)
		if updateErr := db.UpdatePodcastItem(item); updateErr != nil {
			jobLogger.Warnw("failed to mark summary failure", "podcast_item_id", item.ID, "error", updateErr)
		}
		return err
	}

	now, err := time.Parse(time.RFC3339Nano, report["summarized_at"].(string))
	if err != nil {
		return err
	}
	item.LLMSummary = summary
	item.LLMSummaryStatus = "available"
	item.LLMSummaryError = ""
	item.LLMSummaryDate = &now
	// Lineage: record which model and prompt produced this summary.
	item.LLMSummaryModel = model
	item.LLMSummaryPrompt = prompt
	provenance, err := finishSummaryProvenance(report, cfg)
	if err != nil {
		return err
	}
	item.SummaryProvenanceJSON = provenance
	if err := db.UpdatePodcastItem(item); err != nil {
		return err
	}
	ExportSummary(item)
	return nil
}

// prepareSummarySource freezes exact model inputs for both local and fenced remote workflows.
func prepareSummarySource(item *db.PodcastItem, prompt, userPrompt string) (string, string, map[string]any, error) {
	// Capture the input BEFORE calling the provider. A subsequent transcript edit
	// cannot silently change what the summary claims to have read.
	transcriptDocument, err := EpisodeMarkdown(item, "transcript")
	if err != nil {
		return "", "", nil, fmt.Errorf("capture summary source: %w", err)
	}
	sourceMetadata, sourceBody, err := pm.Parse(transcriptDocument)
	if err != nil {
		return "", "", nil, err
	}
	transcript := sourceBody
	prepared, coveredBytes := preparedSummaryTranscript(transcript)
	inputHash := pm.Hash([]byte(prepared))
	coverage := "full"
	if coveredBytes < len(transcript) {
		coverage = "partial"
	}
	input := map[string]any{
		"kind": "transcript", "id": pm.ResourceID("summary-input", inputHash), "revision": 1,
		"uri": "inputs/" + inputHash + ".txt", "sha256": inputHash, "coverage": coverage, "text": prepared,
		"source_document": map[string]any{"id": sourceMetadata["document_id"], "revision": sourceMetadata["document_revision"], "sha256": pm.Hash([]byte(transcriptDocument))},
	}
	if coverage == "partial" {
		input["locators"] = []any{map[string]any{"unit": "utf8_byte", "start": 0, "end_exclusive": coveredBytes}}
	}
	// Reading the entire available transcript is not proof of full episode coverage.
	input["episode_coverage"] = "unknown"
	ancestry := map[string]any{}
	for _, key := range []string{"transcription_method", "transcribed_at", "transcription_engine", "transcription_engine_version", "transcription_model", "asr_backend", "asr_backend_version", "provenance_status", "provenance_missing"} {
		if value, ok := sourceMetadata[key]; ok {
			ancestry[key] = value
		}
	}
	input["source_provenance"] = ancestry
	prompt += summaryEvidenceInstructions
	promptRecord := map[string]any{"system": prompt, "user_template": userPrompt}
	promptBytes, err := json.Marshal(promptRecord)
	if err != nil {
		return "", "", nil, err
	}
	promptHash := pm.Hash(promptBytes)
	report := map[string]any{
		"production_origin": "captured", "inputs": []any{input}, "prompt": promptRecord,
		"prompt_id": pm.ResourceID("summary-prompt", promptHash), "prompt_version": promptHash, "prompt_sha256": promptHash,
	}
	sourceHash := pm.Hash([]byte(transcriptDocument))
	report["processing_events"] = []any{map[string]any{
		"event_id": uuid.New().URN(), "operation": "prepare_summary_input", "completed_at": time.Now().UTC().Format(time.RFC3339Nano),
		"tool": pm.Generator, "tool_version": pm.GeneratorVersion,
		"inputs":   []any{map[string]any{"kind": "transcript", "id": sourceMetadata["document_id"], "revision": sourceMetadata["document_revision"], "uri": "inputs/" + sourceHash + ".txt", "sha256": sourceHash, "coverage": "full", "text": transcriptDocument}},
		"output":   map[string]any{"id": input["id"], "revision": 1, "sha256": inputHash},
		"settings": map[string]any{"extract": "Markdown body after YAML frontmatter", "source_body_bytes": len(transcript), "included_body_bytes": coveredBytes, "truncation_marker": "[Transcript truncated due to length]"},
	}}
	return transcript, prompt, report, nil
}

func finishSummaryProvenance(report map[string]any, cfg LLMConfig) (string, error) {
	priorEvents, _ := report["processing_events"].([]any)
	summaryEvent := map[string]any{
		"event_id": uuid.New().URN(), "operation": "summarization", "completed_at": report["summarized_at"],
		"tool": pm.Generator, "tool_version": pm.GeneratorVersion, "model_requested": cfg.Model,
		"model_provider": report["model_provider"],
		"prompt_id":      report["prompt_id"], "prompt_version": report["prompt_version"], "prompt_sha256": report["prompt_sha256"],
		"inputs": report["inputs"], "settings": report["generation_settings"],
	}
	if resolved, ok := report["model_resolved"]; ok {
		summaryEvent["model_resolved"] = resolved
	}
	report["processing_events"] = append(priorEvents, summaryEvent)
	provenance, err := json.Marshal(report)
	if err != nil {
		return "", err
	}
	return string(provenance), nil
}
