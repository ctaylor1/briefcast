package service

import (
	"errors"
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
var summaryJobStatusMu sync.RWMutex
var summaryJobStatus SummaryBackfillStatus

var ErrSummaryJobAlreadyRunning = errors.New("a summary job is already running")

// GetSummaryJobStatus returns a snapshot of the latest summary job.
func GetSummaryJobStatus() SummaryBackfillStatus {
	summaryJobStatusMu.RLock()
	defer summaryJobStatusMu.RUnlock()
	return summaryJobStatus
}

func setSummaryJobStatus(status SummaryBackfillStatus) {
	summaryJobStatusMu.Lock()
	// Workers report progress concurrently, so an older callback must not
	// move the visible counters backwards.
	if status.Running && summaryJobStatus.Running {
		if status.Total < summaryJobStatus.Total {
			status.Total = summaryJobStatus.Total
		}
		if status.Completed < summaryJobStatus.Completed {
			status.Completed = summaryJobStatus.Completed
		}
		if status.Failed < summaryJobStatus.Failed {
			status.Failed = summaryJobStatus.Failed
		}
	}
	summaryJobStatus = status
	summaryJobStatusMu.Unlock()
}

func reserveSummaryJob() error {
	if !summaryBackfillRunning.CompareAndSwap(false, true) {
		return ErrSummaryJobAlreadyRunning
	}
	setSummaryJobStatus(SummaryBackfillStatus{Running: true})
	return nil
}

func finishSummaryJob(succeeded, failed int, err error) {
	status := GetSummaryJobStatus()
	status.Running = false
	status.Completed = succeeded + failed
	status.Failed = failed
	if err != nil {
		status.Error = err.Error()
	}
	setSummaryJobStatus(status)
	summaryBackfillRunning.Store(false)
}

func validateSummaryConfig(cfg LLMConfig) error {
	if !cfg.Enabled {
		return fmt.Errorf("LLM summarization is not enabled (set LLM_ENABLED=true)")
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("LLM_API_KEY is not configured")
	}
	return nil
}

// StartSummaryBackfill reserves the job before returning and reports failures
// through GetSummaryJobStatus.
func StartSummaryBackfill() (SummaryBackfillStatus, error) {
	cfg := LoadLLMConfig()
	if err := validateSummaryConfig(cfg); err != nil {
		return SummaryBackfillStatus{}, err
	}
	if err := reserveSummaryJob(); err != nil {
		return SummaryBackfillStatus{}, err
	}
	go func() {
		succeeded, failed, err := runSummaryBackfill(cfg, setSummaryJobStatus)
		finishSummaryJob(succeeded, failed, err)
	}()
	return GetSummaryJobStatus(), nil
}

// StartResummarizeSummaries reserves the job before returning and reports
// completion through GetSummaryJobStatus.
func StartResummarizeSummaries(filter ResummarizeFilter) (SummaryBackfillStatus, error) {
	cfg := LoadLLMConfig()
	if err := validateSummaryConfig(cfg); err != nil {
		return SummaryBackfillStatus{}, err
	}
	if err := reserveSummaryJob(); err != nil {
		return SummaryBackfillStatus{}, err
	}
	go func() {
		result, err := runResummarizeSummaries(filter, cfg, setSummaryJobStatus)
		finishSummaryJob(result.Succeeded, result.Failed, err)
	}()
	return GetSummaryJobStatus(), nil
}

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
	cfg := LoadLLMConfig()
	if err := validateSummaryConfig(cfg); err != nil {
		return 0, 0, err
	}
	if err := reserveSummaryJob(); err != nil {
		return 0, 0, err
	}
	succeeded, failed, err = runSummaryBackfill(cfg, func(status SummaryBackfillStatus) {
		setSummaryJobStatus(status)
		if progressFn != nil {
			progressFn(status)
		}
	})
	finishSummaryJob(succeeded, failed, err)
	return succeeded, failed, err
}

func runSummaryBackfill(cfg LLMConfig, progressFn func(SummaryBackfillStatus)) (succeeded int, failed int, err error) {
	logger := logging.Sugar()

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
		Where("(llm_summary_status IS NULL OR llm_summary_status = '' OR llm_summary_status = 'failed' OR (llm_summary_status = 'processing' AND updated_at < ?))", summaryProcessingStaleBefore(cfg, time.Now())).
		Order("pub_date DESC").
		Find(&items)
	if result.Error != nil {
		return 0, 0, fmt.Errorf("query failed: %w", result.Error)
	}

	logger.Infow("summary backfill starting", "episodes", len(items), "concurrency", cfg.Concurrency)
	if progressFn != nil {
		progressFn(SummaryBackfillStatus{Running: true, Total: len(items)})
	}
	succeeded, failed = summarizeBatch(items, cfg, prompt, userPrompt, progressFn)
	logger.Infow("summary backfill finished", "succeeded", succeeded, "failed", failed, "total", len(items))
	return succeeded, failed, nil
}

func summaryProcessingStaleBefore(cfg LLMConfig, now time.Time) time.Time {
	// A request with a configured timeout cannot still be active after this
	// interval. The fallback is conservative when no timeout is configured.
	staleAfter := 10 * time.Minute
	if cfg.TimeoutSecs > 0 {
		staleAfter = time.Duration(cfg.TimeoutSecs)*time.Second + time.Minute
	}
	return now.Add(-staleAfter)
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
	cfg := LoadLLMConfig()
	if err := validateSummaryConfig(cfg); err != nil {
		return ResummarizeResult{}, err
	}
	if err := reserveSummaryJob(); err != nil {
		return ResummarizeResult{}, err
	}
	result, err := runResummarizeSummaries(filter, cfg, func(status SummaryBackfillStatus) {
		setSummaryJobStatus(status)
		if progressFn != nil {
			progressFn(status)
		}
	})
	finishSummaryJob(result.Succeeded, result.Failed, err)
	return result, err
}

func runResummarizeSummaries(filter ResummarizeFilter, cfg LLMConfig, progressFn func(SummaryBackfillStatus)) (ResummarizeResult, error) {
	logger := logging.Sugar()

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
		if progressFn != nil {
			progressFn(SummaryBackfillStatus{Running: true, Total: total})
		}
		return ResummarizeResult{Total: total}, nil
	}

	logger.Infow("re-summarize starting", "episodes", total, "filter_model", filter.Model, "target_model", cfg.Model, "concurrency", cfg.Concurrency)
	if progressFn != nil {
		progressFn(SummaryBackfillStatus{Running: true, Total: total})
	}
	succeeded, failed := summarizeBatch(items, cfg, prompt, userPrompt, progressFn)
	logger.Infow("re-summarize finished", "succeeded", succeeded, "failed", failed, "total", total)
	return ResummarizeResult{Total: total, Succeeded: succeeded, Failed: failed}, nil
}
