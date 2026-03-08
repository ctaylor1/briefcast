package service

import (
	"fmt"
	"sync/atomic"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/logging"
)

// SummaryBackfillStatus tracks progress of a running backfill job.
type SummaryBackfillStatus struct {
	Running   bool   `json:"running"`
	Total     int    `json:"total"`
	Completed int    `json:"completed"`
	Failed    int    `json:"failed"`
	Error     string `json:"error,omitempty"`
}

var summaryBackfillRunning atomic.Bool

// GetSummaryBackfillRunning reports whether a backfill is in progress.
func GetSummaryBackfillRunning() bool {
	return summaryBackfillRunning.Load()
}

// BackfillSummaries summarizes all episodes that have a canonical transcript
// but no summary yet, processing most-recent first. It calls progressFn after
// each episode (may be nil). Returns total succeeded and failed counts.
func BackfillSummaries(progressFn func(status SummaryBackfillStatus)) (succeeded int, failed int, err error) {
	if !summaryBackfillRunning.CompareAndSwap(false, true) {
		return 0, 0, fmt.Errorf("summary backfill is already running")
	}
	defer summaryBackfillRunning.Store(false)

	logger := logging.Sugar()

	cfg := LoadLLMConfig()
	if !cfg.Enabled {
		return 0, 0, fmt.Errorf("LLM summarization is not enabled (set LLM_ENABLED=true)")
	}
	if cfg.APIKey == "" {
		return 0, 0, fmt.Errorf("LLM_API_KEY is not configured")
	}

	setting := db.GetOrCreateSetting()
	prompt := ResolveSummarizationPrompt(setting, cfg)
	userPrompt := ResolveSummarizationUserPrompt(setting, cfg)

	// Find all episodes with a transcript but no summary, most recent first.
	var items []db.PodcastItem
	result := db.DB.
		Where("canonical_transcript IS NOT NULL AND canonical_transcript <> ''").
		Where("(llm_summary IS NULL OR llm_summary = '')").
		Where("(llm_summary_status IS NULL OR llm_summary_status = '' OR llm_summary_status = 'failed')").
		Order("pub_date DESC").
		Find(&items)
	if result.Error != nil {
		return 0, 0, fmt.Errorf("query failed: %w", result.Error)
	}

	total := len(items)
	logger.Infow("summary backfill starting", "episodes", total)

	for i := range items {
		if progressFn != nil {
			progressFn(SummaryBackfillStatus{
				Running:   true,
				Total:     total,
				Completed: succeeded + failed,
				Failed:    failed,
			})
		}

		if sumErr := SummarizeEpisode(&items[i], cfg, prompt, userPrompt); sumErr != nil {
			logger.Warnw("backfill summary failed", "episode_id", items[i].ID, "title", items[i].Title, "error", sumErr)
			failed++
		} else {
			logger.Infow("backfill summary complete", "episode_id", items[i].ID, "title", items[i].Title)
			succeeded++
		}
	}

	logger.Infow("summary backfill finished", "succeeded", succeeded, "failed", failed, "total", total)
	return succeeded, failed, nil
}
