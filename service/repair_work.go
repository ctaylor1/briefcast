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

// Conditions shared by the work-queue queries and the partial indexes that serve
// them (db migrations 2026_11_12_01_*). A query meant to use one of those indexes
// must contain the index's condition word for word. Otherwise SQLite tests it by
// reading every stored transcript, holding the database lock while it does: the
// snapshot took 48 s on a copy of a production database before these indexes.
const (
	noTranscriptJSONCondition = "(transcript_json IS NULL OR transcript_json = '')"
	hasCanonicalCondition     = "canonical_transcript IS NOT NULL AND canonical_transcript <> ''"
	noSummaryCondition        = "(llm_summary IS NULL OR llm_summary = '')"
	hasSummaryCondition       = "llm_summary IS NOT NULL AND llm_summary <> ''"
)

var (
	transcriptWorkStatuses = []string{"pending_whisperx", "processing", "failed"}
	summaryRetryStatuses   = []string{"processing", "failed"}

	// workQueueItemColumns are what queue items show. They leave out the stored
	// transcripts, checkpoint and summary, which can be megabytes per episode.
	workQueueItemColumns = []string{
		"id", "podcast_id", "title", "pub_date", "updated_at", "download_status", "download_path",
		"transcript_status", "transcript_progress_pct", "transcript_progress_stage", "transcript_retry_count",
		"transcript_next_attempt", "transcript_last_error", "transcript_model",
		"llm_summary_status", "llm_summary_error", "llm_summary_model",
	}
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

	SafeGo("repair work", func() { runRepairWork(startedAt) })
	return GetRepairWorkStatus(limit)
}

func runRepairWork(startedAt time.Time) {
	run := RepairWorkRun{StartedAt: startedAt}
	var errorsOut []string
	completed := false
	// Deferred so a panic reports a failed run instead of leaving repair work
	// marked as running until the next restart.
	defer func() {
		if !completed {
			errorsOut = append(errorsOut, "repair work stopped unexpectedly")
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
	}()

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

	completed = true
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
	total, err := countPodcastItems(db.DB.Model(&db.PodcastItem{}))
	if err != nil {
		return counts, err
	}
	available, err := countPodcastItems(db.DB.Model(&db.PodcastItem{}).Where("llm_summary_status = ?", "available"))
	if err != nil {
		return counts, err
	}
	withSummary, err := countPodcastItems(hasSummaryQuery())
	if err != nil {
		return counts, err
	}
	withSummaryAvailable, err := countPodcastItems(hasSummaryQuery().Where("COALESCE(llm_summary_status, '') = ?", "available"))
	if err != nil {
		return counts, err
	}
	// Complete is "status available or has a summary": both sets, less the
	// episodes in each, so that no stored summary or transcript is read.
	counts.Complete = available + withSummary - withSummaryAvailable
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
	// No summary and no transcript. Every episode either has a summary or
	// matches noSummaryCondition, so this is the episodes without a summary
	// less those with a transcript.
	noSummaryWithTranscript, err := countPodcastItems(summaryCandidateQuery())
	if err != nil {
		return counts, err
	}
	counts.BlockedNoTranscript = total - withSummary - noSummaryWithTranscript
	return counts, nil
}

func buildTranscriptQueueCounts(now time.Time) (WorkQueueTranscriptCounts, error) {
	var counts WorkQueueTranscriptCounts
	var err error
	baseNoTranscript := func() *gorm.DB {
		return db.DB.Model(&db.PodcastItem{}).Where(noTranscriptJSONCondition)
	}
	total, err := countPodcastItems(db.DB.Model(&db.PodcastItem{}))
	if err != nil {
		return counts, err
	}
	noTranscript, err := countPodcastItems(baseNoTranscript())
	if err != nil {
		return counts, err
	}
	availableWithoutTranscript, err := countPodcastItems(baseNoTranscript().Where("transcript_status = ?", "available"))
	if err != nil {
		return counts, err
	}
	// Complete is "status available or has a transcript", split as in
	// buildSummaryQueueCounts so that no stored transcript is read.
	counts.Complete = total - noTranscript + availableWithoutTranscript
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

type workQueueCandidate struct {
	ID        string
	UpdatedAt time.Time
}

// summaryMissingColumns come from idx_podcast_items_summary_missing or from the
// start of the row, so listing these episodes reads no stored transcript.
var summaryMissingColumns = []string{
	"id", "podcast_id", "title", "pub_date", "updated_at",
	"transcript_status", "llm_summary_status", "llm_summary_error", "llm_summary_model",
}

// buildWorkQueueItems lists the most recently updated episodes that need a
// transcript or a summary. Three index-backed queries pick them; a single OR
// query that loaded whole rows used to read every stored transcript instead.
// The two summary queries split its condition "has a transcript, and either no
// summary and an empty status, or a processing or failed status".
func buildWorkQueueItems(limit int) ([]WorkQueueItem, error) {
	queryLimit := limit * 4
	if queryLimit < 100 {
		queryLimit = 100
	}

	type candidate struct {
		updatedAt    time.Time
		needsSummary bool
		row          *db.PodcastItem // nil until its columns are loaded
	}
	candidates := map[string]*candidate{}
	add := func(id string, updatedAt time.Time, needsSummary bool, row *db.PodcastItem) {
		c := candidates[id]
		if c == nil {
			c = &candidate{updatedAt: updatedAt}
			candidates[id] = c
		}
		c.needsSummary = c.needsSummary || needsSummary
		if row != nil {
			c.row = row
		}
	}
	latest := func(query *gorm.DB) ([]workQueueCandidate, error) {
		var rows []workQueueCandidate
		err := query.Select("id", "updated_at").Order("updated_at DESC").Limit(queryLimit).Scan(&rows).Error
		return rows, err
	}

	waiting, err := latest(transcriptWorkQuery())
	if err != nil {
		return nil, err
	}
	for _, row := range waiting {
		add(row.ID, row.UpdatedAt, false, nil)
	}
	var missing []db.PodcastItem
	if err := summaryMissingQuery().Select(summaryMissingColumns).Order("updated_at DESC").Limit(queryLimit).Find(&missing).Error; err != nil {
		return nil, err
	}
	for i := range missing {
		add(missing[i].ID, missing[i].UpdatedAt, true, &missing[i])
	}
	retrying, err := latest(summaryRetryQuery())
	if err != nil {
		return nil, err
	}
	for _, row := range retrying {
		add(row.ID, row.UpdatedAt, true, nil)
	}

	// Keep the queryLimit most recently updated, as the single query did.
	ids := make([]string, 0, len(candidates))
	for id := range candidates {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := candidates[ids[i]].updatedAt, candidates[ids[j]].updatedAt
		if !a.Equal(b) {
			return a.After(b)
		}
		return ids[i] < ids[j]
	})
	if len(ids) > queryLimit {
		ids = ids[:queryLimit]
	}

	// Load the remaining columns for episodes waiting for a transcript (they
	// have none stored to read past), the few whose summary is processing or
	// failed, and a missing-summary episode whose transcript is queued again.
	var fetch []string
	for _, id := range ids {
		if row := candidates[id].row; row == nil || transcriptNeedsWork(*row) {
			fetch = append(fetch, id)
		}
	}
	if len(fetch) > 0 {
		var full []db.PodcastItem
		if err := db.DB.Select(workQueueItemColumns).Where("id IN ?", fetch).Find(&full).Error; err != nil {
			return nil, err
		}
		for i := range full {
			candidates[full[i].ID].row = &full[i]
		}
	}
	podcastIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		if row := candidates[id].row; row != nil {
			podcastIDs = append(podcastIDs, row.PodcastID)
		}
	}
	podcastTitles, err := podcastTitlesByID(podcastIDs)
	if err != nil {
		return nil, err
	}

	queued := make([]queuedWorkItem, 0, len(ids)*2)
	now := time.Now().UTC()
	for _, id := range ids {
		c := candidates[id]
		if c.row == nil {
			continue // deleted since it was picked
		}
		item := *c.row
		item.Podcast.Title = podcastTitles[item.PodcastID]
		if transcriptNeedsWork(item) {
			queued = append(queued, queuedWorkItem{
				item:     buildTranscriptWorkQueueItem(item, now),
				priority: transcriptWorkPriority(item, now),
			})
		}
		if c.needsSummary {
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

func podcastTitlesByID(podcastIDs []string) (map[string]string, error) {
	titles := map[string]string{}
	unique := make([]string, 0, len(podcastIDs))
	for _, id := range podcastIDs {
		if _, seen := titles[id]; !seen && id != "" {
			titles[id] = ""
			unique = append(unique, id)
		}
	}
	if len(unique) == 0 {
		return titles, nil
	}
	var podcasts []db.Podcast
	if err := db.DB.Select("id", "title").Where("id IN ?", unique).Find(&podcasts).Error; err != nil {
		return nil, err
	}
	for _, podcast := range podcasts {
		titles[podcast.ID] = podcast.Title
	}
	return titles, nil
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

// Status tests on the summary candidates use COALESCE, which means the same as
// "is null or empty" but stops SQLite choosing idx_podcast_items_summary_list
// instead: that index finds every episode by status and then reads each one's
// transcript to test the other conditions.
func countSummaryBackfillEligible() (int, error) {
	return countPodcastItems(summaryCandidateQuery().
		Where("(COALESCE(llm_summary_status, '') IN ('', 'failed') OR (llm_summary_status = 'processing' AND updated_at < ?))", summaryProcessingStaleBefore(LoadLLMConfig(), time.Now())))
}

// hasSummaryQuery uses idx_podcast_items_with_summary. Status tests on it use
// COALESCE for the reason given above countSummaryBackfillEligible.
func hasSummaryQuery() *gorm.DB {
	return db.DB.Model(&db.PodcastItem{}).Where(hasSummaryCondition)
}

// summaryCandidateQuery matches episodes with a transcript and no summary. It
// and the queries built on it use idx_podcast_items_summary_missing.
func summaryCandidateQuery() *gorm.DB {
	return db.DB.Model(&db.PodcastItem{}).Where(hasCanonicalCondition).Where(noSummaryCondition)
}

func summaryMissingQuery() *gorm.DB {
	return summaryCandidateQuery().Where("COALESCE(llm_summary_status, '') = ''")
}

// summaryRetryQuery matches episodes with a transcript whose summary is
// processing or failed. It finds them by status (idx_podcast_items_summary_list)
// and reads the transcript column only for those rows.
func summaryRetryQuery() *gorm.DB {
	return db.DB.Model(&db.PodcastItem{}).Where(hasCanonicalCondition).Where("llm_summary_status IN ?", summaryRetryStatuses)
}

// transcriptWorkQuery uses idx_podcast_items_no_transcript_work.
func transcriptWorkQuery() *gorm.DB {
	return db.DB.Model(&db.PodcastItem{}).Where(noTranscriptJSONCondition).Where("transcript_status IN ?", transcriptWorkStatuses)
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
