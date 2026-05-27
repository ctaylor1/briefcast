package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/logging"
)

type BriefpointConfig struct {
	Enabled   bool
	ServerURL string
	APIKey    string
}

func LoadBriefpointConfig() BriefpointConfig {
	setting := db.GetOrCreateSetting()
	serverURL := setting.BriefpointServerURL
	if serverURL == "" {
		serverURL = strings.TrimSpace(os.Getenv("BRIEFPOINT_SERVER_URL"))
	}
	if serverURL == "" {
		serverURL = "http://localhost:12314"
	}
	apiKey := setting.BriefpointAPIKey
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("BRIEFPOINT_API_KEY"))
	}
	return BriefpointConfig{
		Enabled:   setting.BriefpointEnabled && apiKey != "",
		ServerURL: strings.TrimRight(serverURL, "/"),
		APIKey:    apiKey,
	}
}

type BriefpointSource struct {
	Name string `json:"name"`
	Type string `json:"type"`
	URL  string `json:"url,omitempty"`
}

type BriefpointItem struct {
	Title       string `json:"title"`
	URL         string `json:"url,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
	Author      string `json:"author,omitempty"`
}

type BriefpointArtifact struct {
	Kind       string `json:"kind"`
	Content    string `json:"content"`
	TokenCount int    `json:"token_count,omitempty"`
}

type BriefpointSummary struct {
	Kind             string `json:"kind"`
	OneSentence      string `json:"one_sentence,omitempty"`
	ExecutiveSummary string `json:"executive_summary,omitempty"`
	ModelUsed        string `json:"model_used,omitempty"`
}

type BriefpointIngestRequest struct {
	ExternalRef        string               `json:"external_ref"`
	SkipSteps          []string             `json:"skip_steps,omitempty"`
	Source             BriefpointSource     `json:"source"`
	Item               BriefpointItem       `json:"item"`
	ProcessedArtifacts []BriefpointArtifact `json:"processed_artifacts,omitempty"`
	Summaries          []BriefpointSummary  `json:"summaries,omitempty"`
}

type BriefpointIngestResponse struct {
	ItemID        string   `json:"item_id"`
	ExternalRef   string   `json:"external_ref"`
	SourceID      string   `json:"source_id"`
	SourceCreated bool     `json:"source_created"`
	Updated       bool     `json:"updated"`
	Status        string   `json:"status"`
	FinalScore    *float64 `json:"final_score"`
	Promoted      bool     `json:"promoted"`
}

type BriefpointSyncResult struct {
	PodcastID   string `json:"podcastId"`
	Total       int    `json:"total"`
	Sent        int    `json:"sent"`
	Skipped     int    `json:"skipped"`
	Failed      int    `json:"failed"`
	AlreadySent int    `json:"alreadySent"`
}

var briefpointSyncRunning atomic.Bool

func GetBriefpointSyncRunning() bool {
	return briefpointSyncRunning.Load()
}

func SendPodcastToBriefpoint(podcastID string) (*BriefpointSyncResult, error) {
	if !briefpointSyncRunning.CompareAndSwap(false, true) {
		return nil, fmt.Errorf("a briefpoint sync is already running")
	}
	defer briefpointSyncRunning.Store(false)

	logger := logging.Sugar()
	cfg := LoadBriefpointConfig()
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("briefpoint API key is not configured")
	}

	var podcast db.Podcast
	if err := db.DB.First(&podcast, "id = ?", podcastID).Error; err != nil {
		return nil, fmt.Errorf("podcast not found: %w", err)
	}

	var episodes []db.PodcastItem
	db.DB.Where("podcast_id = ? AND (canonical_transcript != '' OR llm_summary != '')", podcastID).
		Find(&episodes)

	result := &BriefpointSyncResult{
		PodcastID: podcastID,
		Total:     len(episodes),
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 3)
	var mu sync.Mutex

	for i := range episodes {
		ep := &episodes[i]
		if ep.CanonicalTranscript == "" && ep.LLMSummary == "" {
			mu.Lock()
			result.Skipped++
			mu.Unlock()
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(episode *db.PodcastItem) {
			defer wg.Done()
			defer func() { <-sem }()

			err := sendEpisodeToBriefpoint(cfg, &podcast, episode)
			mu.Lock()
			if err != nil {
				logger.Warnw("briefpoint ingest failed", "episode_id", episode.ID, "error", err)
				result.Failed++
			} else {
				result.Sent++
			}
			mu.Unlock()
		}(ep)
	}

	wg.Wait()

	logger.Infow("briefpoint sync completed",
		"podcast_id", podcastID,
		"podcast_title", podcast.Title,
		"total", result.Total,
		"sent", result.Sent,
		"skipped", result.Skipped,
		"failed", result.Failed,
	)

	return result, nil
}

func sendEpisodeToBriefpoint(cfg BriefpointConfig, podcast *db.Podcast, episode *db.PodcastItem) error {
	externalRef := fmt.Sprintf("briefcast:%s:%s", podcast.ID, episode.ID)

	req := BriefpointIngestRequest{
		ExternalRef: externalRef,
		SkipSteps:   []string{"scrape", "transcribe", "summarize", "extract"},
		Source: BriefpointSource{
			Name: podcast.Title,
			Type: "podcast",
			URL:  podcast.URL,
		},
		Item: BriefpointItem{
			Title:       episode.Title,
			URL:         episode.FileURL,
			PublishedAt: episode.PubDate.UTC().Format(time.RFC3339),
			Author:      podcast.Author,
		},
	}

	if episode.CanonicalTranscript != "" {
		req.ProcessedArtifacts = append(req.ProcessedArtifacts, BriefpointArtifact{
			Kind:    "transcript",
			Content: episode.CanonicalTranscript,
		})
	}

	if episode.LLMSummary != "" {
		req.Summaries = append(req.Summaries, BriefpointSummary{
			Kind:             "executive",
			ExecutiveSummary: episode.LLMSummary,
			ModelUsed:        episode.LLMSummaryModel,
		})
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url, err := briefpointIngestURL(cfg.ServerURL)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: outboundHTTPTransport(outboundPurposeBriefpoint),
		CheckRedirect: func(r *http.Request, _ []*http.Request) error {
			_, redirectErr := validateOutboundURL(r.URL.String(), outboundPurposeBriefpoint)
			return redirectErr
		},
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("briefpoint returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func briefpointIngestURL(serverURL string) (string, error) {
	parsed, err := validateOutboundURL(serverURL, outboundPurposeBriefpoint)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(parsed.String(), "/") + "/api/ingest/items", nil
}
