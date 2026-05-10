package service

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

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

func summarizeBatch(items []db.PodcastItem, cfg LLMConfig, prompt, userPrompt string, progressFn func(SummaryBackfillStatus)) (succeeded, failed int) {
	logger := logging.Sugar()
	total := len(items)
	if total == 0 {
		return 0, 0
	}

	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > total {
		concurrency = total
	}

	var succeededCount, failedCount atomic.Int32

	work := make(chan int, total)
	for i := range items {
		work <- i
	}
	close(work)

	var wg sync.WaitGroup
	wg.Add(concurrency)
	for w := 0; w < concurrency; w++ {
		go func() {
			defer wg.Done()
			for i := range work {
				if sumErr := SummarizeEpisode(&items[i], cfg, prompt, userPrompt); sumErr != nil {
					logger.Warnw("summary failed", "episode_id", items[i].ID, "title", items[i].Title, "error", sumErr)
					failedCount.Add(1)
				} else {
					logger.Infow("summary complete", "episode_id", items[i].ID, "title", items[i].Title)
					succeededCount.Add(1)
				}
				if progressFn != nil {
					s := int(succeededCount.Load())
					f := int(failedCount.Load())
					progressFn(SummaryBackfillStatus{
						Running:   true,
						Total:     total,
						Completed: s + f,
						Failed:    f,
					})
				}
			}
		}()
	}
	wg.Wait()

	return int(succeededCount.Load()), int(failedCount.Load())
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
	cfg.Model = ResolveSummarizationModel(setting, cfg)
	if setting.LLMConcurrency > 0 {
		cfg.Concurrency = setting.LLMConcurrency
	}
	prompt := ResolveSummarizationPrompt(setting, cfg)
	userPrompt := ResolveSummarizationUserPrompt(setting, cfg)

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

	logger.Infow("summary backfill starting", "episodes", len(items), "concurrency", cfg.Concurrency)
	succeeded, failed = summarizeBatch(items, cfg, prompt, userPrompt, progressFn)
	logger.Infow("summary backfill finished", "succeeded", succeeded, "failed", failed, "total", len(items))
	return succeeded, failed, nil
}

// ResummarizeFilter controls which existing summaries are regenerated.
type ResummarizeFilter struct {
	Model     string     `json:"model"`
	Before    *time.Time `json:"before"`
	PodcastID string     `json:"podcastId"`
	DryRun    bool       `json:"dryRun"`
}

// ResummarizeResult contains the outcome of a re-summarize operation.
type ResummarizeResult struct {
	Total     int `json:"total"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

// ResummarizeSummaries regenerates summaries for episodes matching the given
// filter criteria. Shares the backfill mutex so only one batch job runs at a time.
func ResummarizeSummaries(filter ResummarizeFilter, progressFn func(status SummaryBackfillStatus)) (ResummarizeResult, error) {
	if !summaryBackfillRunning.CompareAndSwap(false, true) {
		return ResummarizeResult{}, fmt.Errorf("a summary job is already running")
	}
	defer summaryBackfillRunning.Store(false)

	logger := logging.Sugar()

	cfg := LoadLLMConfig()
	if !cfg.Enabled {
		return ResummarizeResult{}, fmt.Errorf("LLM summarization is not enabled (set LLM_ENABLED=true)")
	}
	if cfg.APIKey == "" {
		return ResummarizeResult{}, fmt.Errorf("LLM_API_KEY is not configured")
	}

	setting := db.GetOrCreateSetting()
	cfg.Model = ResolveSummarizationModel(setting, cfg)
	if setting.LLMConcurrency > 0 {
		cfg.Concurrency = setting.LLMConcurrency
	}
	prompt := ResolveSummarizationPrompt(setting, cfg)
	userPrompt := ResolveSummarizationUserPrompt(setting, cfg)

	query := db.DB.
		Where("canonical_transcript IS NOT NULL AND canonical_transcript <> ''").
		Where("llm_summary_status = ?", "available")

	if filter.Model != "" {
		query = query.Where("llm_summary_model = ?", filter.Model)
	}
	if filter.Before != nil {
		query = query.Where("llm_summary_date < ?", *filter.Before)
	}
	if filter.PodcastID != "" {
		query = query.Where("podcast_id = ?", filter.PodcastID)
	}

	var items []db.PodcastItem
	result := query.Order("pub_date DESC").Find(&items)
	if result.Error != nil {
		return ResummarizeResult{}, fmt.Errorf("query failed: %w", result.Error)
	}

	total := len(items)

	if filter.DryRun {
		return ResummarizeResult{Total: total}, nil
	}

	logger.Infow("re-summarize starting", "episodes", total, "filter_model", filter.Model, "target_model", cfg.Model, "concurrency", cfg.Concurrency)
	succeeded, failed := summarizeBatch(items, cfg, prompt, userPrompt, progressFn)
	logger.Infow("re-summarize finished", "succeeded", succeeded, "failed", failed, "total", total)
	return ResummarizeResult{Total: total, Succeeded: succeeded, Failed: failed}, nil
}
