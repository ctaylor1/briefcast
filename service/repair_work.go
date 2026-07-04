package service

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"gorm.io/gorm"
)

const (
	defaultWorkQueueLimit = 50
	maxWorkQueueLimit     = 200
	transcriptJobName     = "TranscribePendingEpisodes"
)

var (
	ErrRepairWorkAlreadyRunning = errors.New("repair work is already running")

	repairWorkMu        sync.Mutex
	repairWorkRunning   bool
	repairWorkStartedAt *time.Time
	repairWorkLastRun   *RepairWorkRun
)

type RepairWorkResponse struct {
	Running   bool              `json:"running"`
	StartedAt *time.Time        `json:"startedAt,omitempty"`
	LastRun   *RepairWorkRun    `json:"lastRun,omitempty"`
	Queue     WorkQueueSnapshot `json:"queue"`
}

type RepairWorkRun struct {
	StartedAt   time.Time              `json:"startedAt"`
	FinishedAt  *time.Time             `json:"finishedAt,omitempty"`
	Summary     RepairSummaryResult    `json:"summary"`
	Transcripts RepairTranscriptResult `json:"transcripts"`
	Error       string                 `json:"error,omitempty"`
}

type RepairSummaryResult struct {
	Eligible  int    `json:"eligible"`
	Started   bool   `json:"started"`
	Succeeded int    `json:"succeeded"`
	Failed    int    `json:"failed"`
	Error     string `json:"error,omitempty"`
}

type RepairTranscriptResult struct {
	ReadyNow       int    `json:"readyNow"`
	ForcedDue      int    `json:"forcedDue"`
	Queued         int    `json:"queued"`
	WorkerStarted  bool   `json:"workerStarted"`
	WorkerLockHeld bool   `json:"workerLockHeld"`
	Error          string `json:"error,omitempty"`
}

type WorkQueueSnapshot struct {
	Summary     WorkQueueSummaryCounts    `json:"summary"`
	Transcripts WorkQueueTranscriptCounts `json:"transcripts"`
	Config      WorkQueueConfig           `json:"config"`
	Items       []WorkQueueItem           `json:"items"`
	Limit       int                       `json:"limit"`
}

type WorkQueueSummaryCounts struct {
	Complete            int `json:"complete"`
	Processing          int `json:"processing"`
	Failed              int `json:"failed"`
	Missing             int `json:"missing"`
	EligibleForBackfill int `json:"eligibleForBackfill"`
	BlockedNoTranscript int `json:"blockedNoTranscript"`
}

type WorkQueueTranscriptCounts struct {
	Complete       int `json:"complete"`
	Queued         int `json:"queued"`
	Processing     int `json:"processing"`
	Failed         int `json:"failed"`
	RetryDue       int `json:"retryDue"`
	RetryScheduled int `json:"retryScheduled"`
	Blocked        int `json:"blocked"`
}

type WorkQueueConfig struct {
	WhisperXEnabled      bool `json:"whisperxEnabled"`
	LLMEnabled           bool `json:"llmEnabled"`
	LLMAPIKeyConfigured  bool `json:"llmApiKeyConfigured"`
	SummarizationEnabled bool `json:"summarizationEnabled"`
}

type WorkQueueItem struct {
	ID            string     `json:"id"`
	Kind          string     `json:"kind"`
	Status        string     `json:"status"`
	StatusLabel   string     `json:"statusLabel"`
	Category      string     `json:"category"`
	Title         string     `json:"title"`
	PodcastTitle  string     `json:"podcastTitle,omitempty"`
	PubDate       time.Time  `json:"pubDate"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	ProgressPct   int        `json:"progressPct,omitempty"`
	ProgressStage string     `json:"progressStage,omitempty"`
	RetryCount    int        `json:"retryCount,omitempty"`
	NextAttempt   *time.Time `json:"nextAttempt,omitempty"`
	LastError     string     `json:"lastError,omitempty"`
	Model         string     `json:"model,omitempty"`
}

type queuedWorkItem struct {
	item     WorkQueueItem
	priority int
}

func ParseWorkQueueLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultWorkQueueLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("limit must be an integer")
	}
	if limit <= 0 || limit > maxWorkQueueLimit {
		return 0, fmt.Errorf("limit must be between 1 and %d", maxWorkQueueLimit)
	}
	return limit, nil
}

func GetRepairWorkStatus(limit int) (RepairWorkResponse, error) {
	limit = normalizeWorkQueueLimit(limit)
	queue, err := BuildWorkQueueSnapshot(limit)
	if err != nil {
		return RepairWorkResponse{}, err
	}

	repairWorkMu.Lock()
	defer repairWorkMu.Unlock()
	return RepairWorkResponse{
		Running:   repairWorkRunning,
		StartedAt: cloneTimePointer(repairWorkStartedAt),
		LastRun:   cloneRepairWorkRun(repairWorkLastRun),
		Queue:     queue,
	}, nil
}

func StartRepairWork(limit int) (RepairWorkResponse, error) {
	startedAt := time.Now().UTC()

	repairWorkMu.Lock()
	if repairWorkRunning {
		repairWorkMu.Unlock()
		return RepairWorkResponse{}, ErrRepairWorkAlreadyRunning
	}
	repairWorkRunning = true
	repairWorkStartedAt = &startedAt
	repairWorkLastRun = &RepairWorkRun{StartedAt: startedAt}
	repairWorkMu.Unlock()

	go runRepairWork(startedAt)
	return GetRepairWorkStatus(limit)
}

func runRepairWork(startedAt time.Time) {
	run := RepairWorkRun{StartedAt: startedAt}
	var errorsOut []string

	summaryEligible, err := countSummaryBackfillEligible()
	if err != nil {
		run.Summary.Error = err.Error()
		errorsOut = append(errorsOut, "summary count: "+err.Error())
	} else {
		run.Summary.Eligible = summaryEligible
	}
	if run.Summary.Eligible > 0 {
		run.Summary.Started = true
		succeeded, failed, backfillErr := BackfillSummaries(nil)
		run.Summary.Succeeded = succeeded
		run.Summary.Failed = failed
		if backfillErr != nil {
			run.Summary.Error = backfillErr.Error()
			errorsOut = append(errorsOut, "summary backfill: "+backfillErr.Error())
		}
	}

	now := time.Now().UTC()
	readyBefore, err := countTranscriptRepairReady(now)
	if err != nil {
		run.Transcripts.Error = err.Error()
		errorsOut = append(errorsOut, "transcript count: "+err.Error())
	}
	run.Transcripts.ReadyNow = readyBefore

	forcedDue, err := ForceFailedTranscriptRetriesDue(now)
	if err != nil {
		run.Transcripts.Error = err.Error()
		errorsOut = append(errorsOut, "force transcript retries: "+err.Error())
	} else {
		run.Transcripts.ForcedDue = forcedDue
	}
	if queued, countErr := countTranscriptRepairReady(time.Now().UTC()); countErr == nil {
		run.Transcripts.Queued = queued
	} else if run.Transcripts.Error == "" {
		run.Transcripts.Error = countErr.Error()
		errorsOut = append(errorsOut, "transcript queue count: "+countErr.Error())
	}

	if run.Transcripts.Queued > 0 {
		cfg := LoadWhisperXConfig()
		if !cfg.Enabled {
			run.Transcripts.Error = "WhisperX is disabled"
			errorsOut = append(errorsOut, run.Transcripts.Error)
		} else if lock := db.GetLock(transcriptJobName); lock != nil && lock.IsLocked() {
			run.Transcripts.WorkerLockHeld = true
		} else {
			run.Transcripts.WorkerStarted = true
			if transcribeErr := TranscribePendingEpisodes(); transcribeErr != nil {
				run.Transcripts.Error = transcribeErr.Error()
				errorsOut = append(errorsOut, "transcript worker: "+transcribeErr.Error())
			}
		}
	}

	if len(errorsOut) > 0 {
		run.Error = strings.Join(errorsOut, "; ")
	}
	finishedAt := time.Now().UTC()
	run.FinishedAt = &finishedAt

	repairWorkMu.Lock()
	repairWorkRunning = false
	repairWorkStartedAt = nil
	repairWorkLastRun = &run
	repairWorkMu.Unlock()
}

func BuildWorkQueueSnapshot(limit int) (WorkQueueSnapshot, error) {
	limit = normalizeWorkQueueLimit(limit)
	snapshot := WorkQueueSnapshot{
		Limit:  limit,
		Config: currentWorkQueueConfig(),
		Items:  []WorkQueueItem{},
	}

	var err error
	if snapshot.Summary, err = buildSummaryQueueCounts(); err != nil {
		return snapshot, err
	}
	if snapshot.Transcripts, err = buildTranscriptQueueCounts(time.Now().UTC()); err != nil {
		return snapshot, err
	}
	if snapshot.Items, err = buildWorkQueueItems(limit); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func ForceFailedTranscriptRetriesDue(now time.Time) (int, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	result := db.DB.Model(&db.PodcastItem{}).
		Where("download_status = ?", db.Downloaded).
		Where("download_path <> ''").
		Where("(transcript_json IS NULL OR transcript_json = '')").
		Where("transcript_status = ?", "failed").
		Updates(map[string]interface{}{
			"transcript_status":         "pending_whisperx",
			"transcript_next_attempt":   now.UTC(),
			"transcript_progress_pct":   0,
			"transcript_progress_stage": "queued_for_retry",
		})
	return int(result.RowsAffected), result.Error
}

func buildSummaryQueueCounts() (WorkQueueSummaryCounts, error) {
	var counts WorkQueueSummaryCounts
	var err error
	if counts.Complete, err = countPodcastItems(db.DB.Model(&db.PodcastItem{}).
		Where("llm_summary_status = ? OR (llm_summary IS NOT NULL AND llm_summary <> '')", "available")); err != nil {
		return counts, err
	}
	if counts.Processing, err = countPodcastItems(db.DB.Model(&db.PodcastItem{}).
		Where("llm_summary_status = ?", "processing")); err != nil {
		return counts, err
	}
	if counts.Failed, err = countPodcastItems(db.DB.Model(&db.PodcastItem{}).
		Where("llm_summary_status = ?", "failed")); err != nil {
		return counts, err
	}
	if counts.Missing, err = countPodcastItems(summaryMissingQuery()); err != nil {
		return counts, err
	}
	if counts.EligibleForBackfill, err = countSummaryBackfillEligible(); err != nil {
		return counts, err
	}
	if counts.BlockedNoTranscript, err = countPodcastItems(db.DB.Model(&db.PodcastItem{}).
		Where("(llm_summary IS NULL OR llm_summary = '')").
		Where("(canonical_transcript IS NULL OR canonical_transcript = '')")); err != nil {
		return counts, err
	}
	return counts, nil
}

func buildTranscriptQueueCounts(now time.Time) (WorkQueueTranscriptCounts, error) {
	var counts WorkQueueTranscriptCounts
	var err error
	baseNoTranscript := func() *gorm.DB {
		return db.DB.Model(&db.PodcastItem{}).Where("(transcript_json IS NULL OR transcript_json = '')")
	}
	if counts.Complete, err = countPodcastItems(db.DB.Model(&db.PodcastItem{}).
		Where("transcript_status = ? OR (transcript_json IS NOT NULL AND transcript_json <> '')", "available")); err != nil {
		return counts, err
	}
	if counts.Queued, err = countPodcastItems(baseNoTranscript().Where("transcript_status = ?", "pending_whisperx")); err != nil {
		return counts, err
	}
	if counts.Processing, err = countPodcastItems(baseNoTranscript().Where("transcript_status = ?", "processing")); err != nil {
		return counts, err
	}
	if counts.Failed, err = countPodcastItems(baseNoTranscript().Where("transcript_status = ?", "failed")); err != nil {
		return counts, err
	}
	if counts.RetryDue, err = countPodcastItems(transcriptRetryDueQuery(now)); err != nil {
		return counts, err
	}
	if counts.RetryScheduled, err = countPodcastItems(baseNoTranscript().
		Where("transcript_status = ?", "failed").
		Where("transcript_next_attempt > ?", now.UTC())); err != nil {
		return counts, err
	}
	if counts.Blocked, err = countPodcastItems(baseNoTranscript().
		Where("transcript_status IN ?", []string{"pending_whisperx", "processing", "failed"}).
		Where("(download_status <> ? OR download_path IS NULL OR download_path = '')", db.Downloaded)); err != nil {
		return counts, err
	}
	return counts, nil
}

func buildWorkQueueItems(limit int) ([]WorkQueueItem, error) {
	queryLimit := limit * 4
	if queryLimit < 100 {
		queryLimit = 100
	}

	var items []db.PodcastItem
	result := db.DB.Preload("Podcast").
		Where(
			db.DB.Where("transcript_status IN ?", []string{"pending_whisperx", "processing", "failed"}).
				Where("(transcript_json IS NULL OR transcript_json = '')"),
		).
		Or(
			db.DB.Where("canonical_transcript IS NOT NULL AND canonical_transcript <> ''").
				Where("(llm_summary IS NULL OR llm_summary = '' OR llm_summary_status IN ?)", []string{"processing", "failed"}).
				Where("(llm_summary_status IS NULL OR llm_summary_status = '' OR llm_summary_status IN ?)", []string{"processing", "failed"}),
		).
		Order("updated_at DESC").
		Limit(queryLimit).
		Find(&items)
	if result.Error != nil {
		return nil, result.Error
	}

	queued := make([]queuedWorkItem, 0, len(items)*2)
	now := time.Now().UTC()
	for _, item := range items {
		if transcriptNeedsWork(item) {
			queued = append(queued, queuedWorkItem{
				item:     buildTranscriptWorkQueueItem(item, now),
				priority: transcriptWorkPriority(item, now),
			})
		}
		if summaryNeedsWork(item) {
			queued = append(queued, queuedWorkItem{
				item:     buildSummaryWorkQueueItem(item),
				priority: summaryWorkPriority(item),
			})
		}
	}

	sort.SliceStable(queued, func(i, j int) bool {
		if queued[i].priority != queued[j].priority {
			return queued[i].priority < queued[j].priority
		}
		if !queued[i].item.UpdatedAt.Equal(queued[j].item.UpdatedAt) {
			return queued[i].item.UpdatedAt.After(queued[j].item.UpdatedAt)
		}
		return queued[i].item.Title < queued[j].item.Title
	})

	out := make([]WorkQueueItem, 0, min(len(queued), limit))
	for i, entry := range queued {
		if i >= limit {
			break
		}
		out = append(out, entry.item)
	}
	return out, nil
}

func buildTranscriptWorkQueueItem(item db.PodcastItem, now time.Time) WorkQueueItem {
	status := normalizedStatus(item.TranscriptStatus, "pending_whisperx")
	category := "queued"
	label := "Transcription queued"
	if item.DownloadStatus != db.Downloaded || strings.TrimSpace(item.DownloadPath) == "" {
		category = "blocked"
		label = "Blocked: download unavailable"
	} else {
		switch status {
		case "processing":
			category = "active"
			label = "Transcription in progress"
		case "failed":
			if item.TranscriptNextAttempt != nil && item.TranscriptNextAttempt.After(now) {
				category = "retry"
				label = "Retry scheduled"
			} else {
				category = "failed"
				label = "Retry due"
			}
		case "pending_whisperx":
			label = "Queued for transcription"
		}
	}
	return WorkQueueItem{
		ID:            item.ID,
		Kind:          "transcript",
		Status:        status,
		StatusLabel:   label,
		Category:      category,
		Title:         item.Title,
		PodcastTitle:  item.Podcast.Title,
		PubDate:       item.PubDate,
		UpdatedAt:     item.UpdatedAt,
		ProgressPct:   item.TranscriptProgressPct,
		ProgressStage: item.TranscriptProgressStage,
		RetryCount:    item.TranscriptRetryCount,
		NextAttempt:   item.TranscriptNextAttempt,
		LastError:     item.TranscriptLastError,
		Model:         item.TranscriptModel,
	}
}

func buildSummaryWorkQueueItem(item db.PodcastItem) WorkQueueItem {
	status := normalizedStatus(item.LLMSummaryStatus, "missing")
	category := "queued"
	label := "Summary missing"
	switch status {
	case "processing":
		category = "active"
		label = "Summary in progress"
	case "failed":
		category = "failed"
		label = "Summary failed"
	}
	return WorkQueueItem{
		ID:           item.ID,
		Kind:         "summary",
		Status:       status,
		StatusLabel:  label,
		Category:     category,
		Title:        item.Title,
		PodcastTitle: item.Podcast.Title,
		PubDate:      item.PubDate,
		UpdatedAt:    item.UpdatedAt,
		LastError:    item.LLMSummaryError,
		Model:        item.LLMSummaryModel,
	}
}

func countPodcastItems(query *gorm.DB) (int, error) {
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func countSummaryBackfillEligible() (int, error) {
	return countPodcastItems(db.DB.Model(&db.PodcastItem{}).
		Where("canonical_transcript IS NOT NULL AND canonical_transcript <> ''").
		Where("(llm_summary IS NULL OR llm_summary = '')").
		Where("(llm_summary_status IS NULL OR llm_summary_status = '' OR llm_summary_status = 'failed')"))
}

func summaryMissingQuery() *gorm.DB {
	return db.DB.Model(&db.PodcastItem{}).
		Where("canonical_transcript IS NOT NULL AND canonical_transcript <> ''").
		Where("(llm_summary IS NULL OR llm_summary = '')").
		Where("(llm_summary_status IS NULL OR llm_summary_status = '')")
}

func countTranscriptRepairReady(now time.Time) (int, error) {
	return countPodcastItems(db.DB.Model(&db.PodcastItem{}).
		Where("download_status = ?", db.Downloaded).
		Where("download_path <> ''").
		Where("(transcript_json IS NULL OR transcript_json = '')").
		Where("transcript_status IN ?", []string{"pending_whisperx", "processing"}).
		Where("(transcript_next_attempt IS NULL OR transcript_next_attempt <= ?)", now.UTC()))
}

func transcriptRetryDueQuery(now time.Time) *gorm.DB {
	return db.DB.Model(&db.PodcastItem{}).
		Where("download_status = ?", db.Downloaded).
		Where("download_path <> ''").
		Where("(transcript_json IS NULL OR transcript_json = '')").
		Where("transcript_status = ?", "failed").
		Where("(transcript_next_attempt IS NULL OR transcript_next_attempt <= ?)", now.UTC())
}

func currentWorkQueueConfig() WorkQueueConfig {
	llmCfg := LoadLLMConfig()
	whisperCfg := LoadWhisperXConfig()
	setting := db.GetOrCreateSetting()
	return WorkQueueConfig{
		WhisperXEnabled:      whisperCfg.Enabled,
		LLMEnabled:           llmCfg.Enabled,
		LLMAPIKeyConfigured:  strings.TrimSpace(llmCfg.APIKey) != "",
		SummarizationEnabled: setting.SummarizationEnabled,
	}
}

func transcriptNeedsWork(item db.PodcastItem) bool {
	status := normalizedStatus(item.TranscriptStatus, "")
	return status == "pending_whisperx" || status == "processing" || status == "failed"
}

func summaryNeedsWork(item db.PodcastItem) bool {
	if strings.TrimSpace(item.CanonicalTranscript) == "" {
		return false
	}
	status := normalizedStatus(item.LLMSummaryStatus, "")
	if status == "processing" || status == "failed" {
		return true
	}
	return strings.TrimSpace(item.LLMSummary) == "" && status == ""
}

func transcriptWorkPriority(item db.PodcastItem, now time.Time) int {
	if normalizedStatus(item.TranscriptStatus, "") == "processing" {
		return 0
	}
	if item.DownloadStatus != db.Downloaded || strings.TrimSpace(item.DownloadPath) == "" {
		return 4
	}
	if normalizedStatus(item.TranscriptStatus, "") == "pending_whisperx" {
		return 1
	}
	if item.TranscriptNextAttempt != nil && item.TranscriptNextAttempt.After(now) {
		return 3
	}
	return 2
}

func summaryWorkPriority(item db.PodcastItem) int {
	switch normalizedStatus(item.LLMSummaryStatus, "") {
	case "processing":
		return 0
	case "failed":
		return 2
	default:
		return 1
	}
}

func normalizedStatus(status string, fallback string) string {
	status = strings.TrimSpace(strings.ToLower(status))
	if status == "" {
		return fallback
	}
	return status
}

func normalizeWorkQueueLimit(limit int) int {
	if limit <= 0 {
		return defaultWorkQueueLimit
	}
	if limit > maxWorkQueueLimit {
		return maxWorkQueueLimit
	}
	return limit
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneRepairWorkRun(run *RepairWorkRun) *RepairWorkRun {
	if run == nil {
		return nil
	}
	cloned := *run
	cloned.FinishedAt = cloneTimePointer(run.FinishedAt)
	return &cloned
}

func resetRepairWorkState() {
	repairWorkMu.Lock()
	defer repairWorkMu.Unlock()
	repairWorkRunning = false
	repairWorkStartedAt = nil
	repairWorkLastRun = nil
}
