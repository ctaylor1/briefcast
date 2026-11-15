package service

import (
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
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
	Stopping  bool   `json:"stopping,omitempty"`
	Stopped   bool   `json:"stopped,omitempty"`
}

// SummaryBackfillOptions narrows a backfill. The zero value summarizes every
// episode that is waiting for a summary.
type SummaryBackfillOptions struct {
	// Limit caps the run at the newest Limit waiting episodes; 0 means no cap.
	Limit int
	// EpisodeIDs restricts the run to these episodes when set.
	EpisodeIDs []string
	// Replace lists requested episodes whose transcript was just committed:
	// their existing summary describes the old transcript, so it is replaced.
	// Other episodes are summarized only when they have no summary.
	Replace []string
}

var summaryBackfillRunning atomic.Bool
var summaryJobStopRequested atomic.Bool
var summaryJobStatusMu sync.RWMutex
var summaryJobStatus SummaryBackfillStatus

var ErrSummaryJobAlreadyRunning = errors.New("a summary job is already running")

var ErrNoSummaryJobRunning = errors.New("no summary job is running")

var errSummaryJobStopped = errors.New("summary job stopped unexpectedly")

// GetSummaryJobStatus returns a snapshot of the latest summary job.
func GetSummaryJobStatus() SummaryBackfillStatus {
	summaryJobStatusMu.RLock()
	status := summaryJobStatus
	summaryJobStatusMu.RUnlock()
	status.Stopping = status.Running && summaryJobStopRequested.Load()
	return status
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
	summaryJobStopRequested.Store(false)
	setSummaryJobStatus(SummaryBackfillStatus{Running: true})
	return nil
}

func finishSummaryJob(succeeded, failed int, err error) {
	status := GetSummaryJobStatus()
	status.Running = false
	status.Stopping = false
	status.Stopped = summaryJobStopRequested.Load()
	status.Completed = succeeded + failed
	status.Failed = failed
	if err != nil {
		status.Error = err.Error()
	}
	setSummaryJobStatus(status)
	summaryJobStopRequested.Store(false)
	summaryBackfillRunning.Store(false)
}

// StopSummaryJob asks the running summary job to stop. Episodes already sent to
// the LLM finish; the rest stay waiting for a later run. Episodes queued by a
// feed refresh during the job are dropped too, so nothing starts right after.
func StopSummaryJob() (SummaryBackfillStatus, error) {
	if !summaryBackfillRunning.Load() {
		return GetSummaryJobStatus(), ErrNoSummaryJobRunning
	}
	summaryJobStopRequested.Store(true)
	pendingSummaryMu.Lock()
	clear(pendingSummaryIDs)
	pendingSummaryMu.Unlock()
	return GetSummaryJobStatus(), nil
}

// runSummaryJob runs a reserved job and always finishes it. The finish is deferred
// so a panic in job reports errSummaryJobStopped instead of leaving the job marked
// as running until the next restart.
func runSummaryJob(job func() (succeeded, failed int, err error)) (int, int, error) {
	succeeded, failed, err := 0, 0, errSummaryJobStopped
	defer func() { finishSummaryJob(succeeded, failed, err) }()
	succeeded, failed, err = job()
	return succeeded, failed, err
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
func StartSummaryBackfill(opts SummaryBackfillOptions) (SummaryBackfillStatus, error) {
	cfg := LoadLLMConfig()
	if err := validateSummaryConfig(cfg); err != nil {
		return SummaryBackfillStatus{}, err
	}
	if err := reserveSummaryJob(); err != nil {
		return SummaryBackfillStatus{}, err
	}
	SafeGo("summary backfill", func() {
		defer runRequestedSummaryBackfill()
		_, _, _ = runSummaryJob(func() (int, int, error) {
			return runSummaryBackfill(cfg, opts, setSummaryJobStatus)
		})
	})
	return GetSummaryJobStatus(), nil
}

// pendingSummaryIDs holds episodes requested while a summary job was running.
// That job chose its episodes before these were requested, so they get their
// own run once it finishes. The value is true when the existing summary is
// to be replaced (see SummaryBackfillOptions.Replace).
var pendingSummaryMu sync.Mutex
var pendingSummaryIDs = map[string]bool{}

// RequestSummaryBackfill summarizes the given episodes, now or as soon as the
// running summary job finishes. Other waiting episodes are left alone, so a feed
// refresh never starts a run over the whole back catalog.
func RequestSummaryBackfill(episodeIDs []string) {
	requestSummaries(episodeIDs, nil)
}

// RequestSummaryForNewTranscript queues the summary of an episode whose
// transcript was just committed, replacing a summary of an older transcript.
// Transcription never waits for the LLM: the summary runs in the summary job,
// where Stop and the LLM concurrency limit apply.
func RequestSummaryForNewTranscript(episodeID string) {
	requestSummaries([]string{episodeID}, []string{episodeID})
}

// queueSummaryAfterTranscript requests the summary of an episode whose
// transcript was just committed, when summarization is on and configured.
// replaceExisting replaces a summary of an older transcript; recovery paths
// pass false so a completed or edited summary is kept.
func queueSummaryAfterTranscript(item *db.PodcastItem, replaceExisting bool) {
	if strings.TrimSpace(item.CanonicalTranscript) == "" {
		return
	}
	cfg := LoadLLMConfig()
	if !cfg.Enabled || strings.TrimSpace(cfg.APIKey) == "" || !db.GetOrCreateSetting().SummarizationEnabled {
		return
	}
	if replaceExisting {
		RequestSummaryForNewTranscript(item.ID)
	} else {
		RequestSummaryBackfill([]string{item.ID})
	}
	logging.Sugar().Infow("summary_queued_after_transcription", "podcast_item_id", item.ID, "replace_existing", replaceExisting)
}

func requestSummaries(episodeIDs, replace []string) {
	if len(episodeIDs) == 0 {
		return
	}
	_, err := StartSummaryBackfill(SummaryBackfillOptions{EpisodeIDs: episodeIDs, Replace: replace})
	switch {
	case err == nil:
	case errors.Is(err, ErrSummaryJobAlreadyRunning):
		pendingSummaryMu.Lock()
		for _, id := range episodeIDs {
			if _, ok := pendingSummaryIDs[id]; !ok {
				pendingSummaryIDs[id] = false
			}
		}
		for _, id := range replace {
			pendingSummaryIDs[id] = true
		}
		pendingSummaryMu.Unlock()
		// The running job may have finished before the IDs were recorded.
		if !summaryBackfillRunning.Load() {
			runRequestedSummaryBackfill()
		}
	default:
		logging.Sugar().Warnw("summary backfill not started", "error", err)
	}
}

func runRequestedSummaryBackfill() {
	pendingSummaryMu.Lock()
	ids := make([]string, 0, len(pendingSummaryIDs))
	var replace []string
	for id, replaceExisting := range pendingSummaryIDs {
		ids = append(ids, id)
		if replaceExisting {
			replace = append(replace, id)
		}
	}
	clear(pendingSummaryIDs)
	pendingSummaryMu.Unlock()
	requestSummaries(ids, replace)
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
	SafeGo("resummarize summaries", func() {
		defer runRequestedSummaryBackfill()
		_, _, _ = runSummaryJob(func() (int, int, error) {
			result, err := runResummarizeSummaries(filter, cfg, setSummaryJobStatus)
			return result.Succeeded, result.Failed, err
		})
	})
	return GetSummaryJobStatus(), nil
}

// GetSummaryBackfillRunning reports whether a backfill is in progress.
func GetSummaryBackfillRunning() bool {
	return summaryBackfillRunning.Load()
}

// summarizeEpisodeContained turns a panic in one episode's summary into that
// episode's error, so the batch worker goes on with the rest instead of
// ending and silently skipping them. SummarizeEpisode has already recorded
// the failure on the episode.
func summarizeEpisodeContained(item *db.PodcastItem, cfg LLMConfig, prompt, userPrompt, source string) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logging.Sugar().Errorw("summary panicked", "podcast_item_id", item.ID, "panic", trimToLength(fmt.Sprint(recovered), 300), "stack", string(debug.Stack()))
			err = fmt.Errorf("summary panicked: %s", trimToLength(fmt.Sprint(recovered), 300))
		}
	}()
	return summarizeEpisode(item, cfg, prompt, userPrompt, source)
}

func summarizeBatch(items []db.PodcastItem, cfg LLMConfig, prompt, userPrompt, source string, progressFn func(SummaryBackfillStatus)) (succeeded, failed int) {
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
		SafeGo("summary batch worker", func() {
			defer wg.Done()
			for i := range work {
				if summaryJobStopRequested.Load() {
					return
				}
				if sumErr := summarizeEpisodeContained(&items[i], cfg, prompt, userPrompt, source); sumErr != nil {
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
		})
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
	return runSummaryJob(func() (int, int, error) {
		return runSummaryBackfill(cfg, SummaryBackfillOptions{}, func(status SummaryBackfillStatus) {
			setSummaryJobStatus(status)
			if progressFn != nil {
				progressFn(status)
			}
		})
	})
}

func runSummaryBackfill(cfg LLMConfig, opts SummaryBackfillOptions, progressFn func(SummaryBackfillStatus)) (succeeded int, failed int, err error) {
	logger := logging.Sugar()

	setting := db.GetOrCreateSetting()
	cfg.Model = ResolveSummarizationModel(setting, cfg)
	if setting.LLMConcurrency > 0 {
		cfg.Concurrency = setting.LLMConcurrency
	}
	prompt := ResolveSummarizationPrompt(setting, cfg)
	userPrompt := ResolveSummarizationUserPrompt(setting, cfg)

	var items []db.PodcastItem
	staleBefore := summaryProcessingStaleBefore(cfg, time.Now())
	query := db.DB.Where("canonical_transcript IS NOT NULL AND canonical_transcript <> ''")
	if len(opts.EpisodeIDs) > 0 {
		// Requested episodes are summarized unless they already have a summary
		// (kept unless listed in Replace) or one is in progress.
		query = query.Where("id IN ?", opts.EpisodeIDs)
		if len(opts.Replace) > 0 {
			query = query.Where("(llm_summary IS NULL OR llm_summary = '' OR id IN ?)", opts.Replace)
		} else {
			query = query.Where("(llm_summary IS NULL OR llm_summary = '')")
		}
		query = query.Where("NOT (COALESCE(llm_summary_status, '') = 'processing' AND updated_at >= ?)", staleBefore)
	} else {
		query = query.
			Where("(llm_summary IS NULL OR llm_summary = '')").
			Where("(llm_summary_status IS NULL OR llm_summary_status = '' OR llm_summary_status = 'failed' OR (llm_summary_status = 'processing' AND updated_at < ?))", staleBefore)
	}
	query = query.Order("pub_date DESC")
	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}
	if result := query.Find(&items); result.Error != nil {
		return 0, 0, fmt.Errorf("query failed: %w", result.Error)
	}

	logger.Infow("summary backfill starting", "episodes", len(items), "limit", opts.Limit, "requested_episodes", len(opts.EpisodeIDs), "replacing", len(opts.Replace), "concurrency", cfg.Concurrency)
	if progressFn != nil {
		progressFn(SummaryBackfillStatus{Running: true, Total: len(items)})
	}
	source := SummarySourceBackfill
	if len(opts.EpisodeIDs) > 0 {
		source = SummarySourceAutomatic
	}
	succeeded, failed = summarizeBatch(items, cfg, prompt, userPrompt, source, progressFn)
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
	var result ResummarizeResult
	_, _, err := runSummaryJob(func() (int, int, error) {
		var jobErr error
		result, jobErr = runResummarizeSummaries(filter, cfg, func(status SummaryBackfillStatus) {
			setSummaryJobStatus(status)
			if progressFn != nil {
				progressFn(status)
			}
		})
		return result.Succeeded, result.Failed, jobErr
	})
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
	succeeded, failed := summarizeBatch(items, cfg, prompt, userPrompt, SummarySourceResummarize, progressFn)
	logger.Infow("re-summarize finished", "succeeded", succeeded, "failed", failed, "total", total)
	return ResummarizeResult{Total: total, Succeeded: succeeded, Failed: failed}, nil
}
