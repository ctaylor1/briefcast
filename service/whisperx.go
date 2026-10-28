package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/logging"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// WhisperXConfig represents a public type.
type WhisperXConfig struct {
	Backend            string
	ServiceURL         string
	ServiceTokenFile   string
	EngineProfile      string
	ServiceResultLimit int64
	Enabled            bool
	Python             string
	Script             string
	HFHome             string
	DisableTelemetry   bool
	ThirdPartyLogLevel string
	Model              string
	Language           string
	Device             string
	ComputeType        string
	BatchSize          int
	ChunkSeconds       int
	BeamSize           int
	Patience           float64
	ConditionOnPrev    bool
	InitialPrompt      string
	VADChunkSize       int
	VADOnset           float64
	VADOffset          float64
	VADMethod          string
	Align              bool
	Diarization        bool
	DiarizationModel   string
	MinSpeakers        int
	MaxSpeakers        int
	HFToken            string
	RetryFailed        bool
	MaxConcurrency     int
	MaxItemsPerRun     int
	RetryDelaySeconds  int
	RetryMaxDelay      int
	RetryMaxErrorChars int
	LockDurationMins   int
	LockRefreshSecs    int
}

const (
	defaultWhisperXScript         = "scripts/whisperx_transcribe.py"
	defaultWhisperXTimeoutSeconds = 21600
	defaultWhisperXPreflightSec   = 30
	defaultWhisperXRetryDelay     = 300
	defaultWhisperXRetryMaxDelay  = 21600
	defaultWhisperXMaxErrorChars  = 1000
	defaultWhisperXLockDuration   = 30
	defaultWhisperXLockRefresh    = 60
	defaultWhisperXLogTrimChars   = 2000
	defaultWhisperXProgressPollMS = 2000
	whisperxPythonEnv             = "WHISPERX_PYTHON"
	whisperxScriptEnv             = "WHISPERX_SCRIPT"
	whisperxTimeoutEnv            = "WHISPERX_TIMEOUT_SECONDS"
	whisperxPreflightEnv          = "WHISPERX_PREFLIGHT_TIMEOUT_SECONDS"
	whisperxProgressPollEnv       = "WHISPERX_PROGRESS_POLL_MILLIS"
	whisperxProgressFileEnv       = "WHISPERX_PROGRESS_FILE"
	whisperxResumeFileEnv         = "WHISPERX_RESUME_FILE"
	whisperxSegmentsFileEnv       = "WHISPERX_SEGMENTS_FILE"
	whisperxEnabledEnv            = "WHISPERX_ENABLED"
	whisperxHFTokenEnv            = "WHISPERX_HF_TOKEN" // #nosec G101 -- env var key name only, not a credential value.
	whisperxLockDurationEnv       = "WHISPERX_LOCK_DURATION_MINUTES"
	whisperxLockRefreshEnv        = "WHISPERX_LOCK_REFRESH_SECONDS"
)

type whisperxQueueSnapshot struct {
	TotalEligible int64
	DueNow        int64
	Pending       int64
	Processing    int64
	Failed        int64
	NextRetryAt   *time.Time
}

// LoadWhisperXConfig handles the corresponding operation.
func LoadWhisperXConfig() WhisperXConfig {
	cfg := WhisperXConfig{
		Backend:            getEnvString("WHISPERX_BACKEND", "local"),
		ServiceURL:         getEnvString("WHISPERX_SERVICE_URL", "http://whisperx:8090"),
		ServiceTokenFile:   getEnvString("WHISPERX_SERVICE_TOKEN_FILE", ""),
		EngineProfile:      getEnvString("WHISPERX_ENGINE_PROFILE", ""),
		ServiceResultLimit: int64(getEnvInt("WHISPERX_SERVICE_RESULT_LIMIT_BYTES", 64<<20)),
		Enabled:            getEnvBool(whisperxEnabledEnv, false),
		Python:             strings.TrimSpace(os.Getenv(whisperxPythonEnv)),
		Script:             strings.TrimSpace(os.Getenv(whisperxScriptEnv)),
		HFHome:             getEnvString("WHISPERX_HF_HOME", defaultWhisperXHFHome()),
		DisableTelemetry:   getEnvBool("WHISPERX_DISABLE_TELEMETRY", true),
		ThirdPartyLogLevel: getEnvString("WHISPERX_THIRD_PARTY_LOG_LEVEL", "warning"),
		Model:              getEnvString("WHISPERX_MODEL", "medium.en"),
		Language:           getEnvString("WHISPERX_LANGUAGE", "en"),
		Device:             getEnvString("WHISPERX_DEVICE", "auto"),
		ComputeType:        getEnvString("WHISPERX_COMPUTE_TYPE", "auto"),
		BatchSize:          getEnvInt("WHISPERX_BATCH_SIZE", 0),
		ChunkSeconds:       getEnvInt("WHISPERX_CHUNK_SECONDS", 120),
		BeamSize:           getEnvInt("WHISPERX_BEAM_SIZE", 5),
		Patience:           getEnvFloat("WHISPERX_PATIENCE", 1),
		ConditionOnPrev:    getEnvBool("WHISPERX_CONDITION_ON_PREVIOUS_TEXT", true),
		InitialPrompt:      getEnvString("WHISPERX_INITIAL_PROMPT", "Podcast interview. Speakers are Host and Guest. Use punctuation and capitalization."),
		VADChunkSize:       getEnvInt("WHISPERX_VAD_CHUNK_SIZE", 45),
		VADOnset:           getEnvFloat("WHISPERX_VAD_ONSET", 0.50),
		VADOffset:          getEnvFloat("WHISPERX_VAD_OFFSET", 0.50),
		VADMethod:          getEnvString("WHISPERX_VAD_METHOD", "pyannote"),
		Align:              getEnvBool("WHISPERX_ALIGN", true),
		Diarization:        getEnvBool("WHISPERX_DIARIZATION", true),
		DiarizationModel:   getEnvString("WHISPERX_DIARIZATION_MODEL", "pyannote/speaker-diarization-3.1"),
		MinSpeakers:        getEnvInt("WHISPERX_MIN_SPEAKERS", 2),
		MaxSpeakers:        getEnvInt("WHISPERX_MAX_SPEAKERS", 2),
		HFToken:            strings.TrimSpace(os.Getenv(whisperxHFTokenEnv)),
		RetryFailed:        getEnvBool("WHISPERX_RETRY_FAILED", true),
		MaxConcurrency:     getEnvInt("WHISPERX_MAX_CONCURRENCY", 1),
		MaxItemsPerRun:     getEnvInt("WHISPERX_MAX_ITEMS", 0),
		RetryDelaySeconds:  getEnvInt("WHISPERX_RETRY_DELAY_SECONDS", defaultWhisperXRetryDelay),
		RetryMaxDelay:      getEnvInt("WHISPERX_RETRY_MAX_DELAY_SECONDS", defaultWhisperXRetryMaxDelay),
		RetryMaxErrorChars: getEnvInt("WHISPERX_RETRY_MAX_ERROR_CHARS", defaultWhisperXMaxErrorChars),
		LockDurationMins:   getEnvInt(whisperxLockDurationEnv, defaultWhisperXLockDuration),
		LockRefreshSecs:    getEnvInt(whisperxLockRefreshEnv, defaultWhisperXLockRefresh),
	}
	if cfg.Script == "" {
		cfg.Script = defaultWhisperXScript
	}
	if cfg.MinSpeakers <= 0 {
		cfg.MinSpeakers = 1
	}
	if cfg.MaxSpeakers < cfg.MinSpeakers {
		cfg.MaxSpeakers = cfg.MinSpeakers
	}
	if cfg.MaxConcurrency <= 0 {
		cfg.MaxConcurrency = 1
	}
	if cfg.ChunkSeconds <= 0 {
		cfg.ChunkSeconds = 120
	}
	if cfg.RetryDelaySeconds <= 0 {
		cfg.RetryDelaySeconds = defaultWhisperXRetryDelay
	}
	if cfg.RetryMaxDelay < cfg.RetryDelaySeconds {
		cfg.RetryMaxDelay = cfg.RetryDelaySeconds
	}
	if cfg.RetryMaxErrorChars <= 0 {
		cfg.RetryMaxErrorChars = defaultWhisperXMaxErrorChars
	}
	if cfg.LockDurationMins <= 0 {
		cfg.LockDurationMins = defaultWhisperXLockDuration
	}
	if cfg.LockRefreshSecs <= 0 {
		cfg.LockRefreshSecs = defaultWhisperXLockRefresh
	}
	if cfg.LockRefreshSecs >= cfg.LockDurationMins*60 {
		cfg.LockRefreshSecs = max(1, (cfg.LockDurationMins*60)/2)
	}
	return cfg
}

// TranscribePendingEpisodes handles the corresponding operation.
func TranscribePendingEpisodes() error {
	cfg := LoadWhisperXConfig()
	if !cfg.Enabled {
		return nil
	}
	backend, err := configuredTranscriptionBackend(cfg)
	if err != nil {
		return err
	}
	return transcribePendingEpisodes(context.Background(), cfg, backend)
}

func transcribePendingEpisodes(ctx context.Context, cfg WhisperXConfig, backend transcriptionBackend) error {
	if !cfg.Enabled {
		return nil
	}

	jobLogger, _ := logging.NewJobSugar("TranscribePendingEpisodes")
	start := time.Now()
	jobLogger.Infow("job_started")
	defer func() {
		jobLogger.Infow("job_finished", "duration_ms", time.Since(start).Milliseconds())
	}()

	jobLock, acquired, lockErr := db.TryLock("TranscribePendingEpisodes", cfg.LockDurationMins)
	if lockErr != nil {
		jobLogger.Errorw("job_lock_acquire_failed", "error", lockErr)
		return lockErr
	}
	if !acquired {
		// Another run is already processing; skip this cron tick.
		jobLogger.Infow("job_skipped_lock_exists", "lock_id", jobLock.ID, "lock_date", jobLock.Date, "lock_duration_mins", jobLock.Duration)
		return nil
	}
	if jobLock == nil || jobLock.ID == "" {
		acquireErr := errors.New("failed to acquire transcription job lock")
		jobLogger.Errorw("job_lock_acquire_failed", "error", acquireErr)
		return acquireErr
	}
	if remote, ok := backend.(*httpTranscriptionBackend); ok {
		return transcribeRemotePending(ctx, cfg, remote, jobLock)
	}
	stopLockHeartbeat := startJobLockHeartbeat(
		jobLock.ID,
		cfg.LockDurationMins,
		cfg.LockRefreshSecs,
		func(lockErr error) {
			jobLogger.Warnw("failed to refresh transcription job lock", "error", lockErr)
		},
	)
	defer stopLockHeartbeat()
	defer db.UnlockByID(jobLock.ID)

	if err := backend.Preflight(ctx); err != nil {
		jobLogger.Errorw("transcription backend preflight failed", "error", err)
		return err
	}

	// Include "processing" so interrupted work can be resumed after restarts.
	statuses := []string{"pending_whisperx", "processing"}
	if cfg.RetryFailed {
		statuses = append(statuses, "failed")
	}
	items, err := db.GetPodcastItemsForWhisperx(statuses, cfg.MaxItemsPerRun)
	if err != nil {
		jobLogger.Errorw("failed to fetch pending transcripts", "error", err)
		return err
	}

	if len(*items) == 0 {
		snapshot, snapshotErr := collectWhisperXQueueSnapshot(statuses, time.Now().UTC())
		if snapshotErr != nil {
			jobLogger.Warnw("no pending transcripts (queue snapshot failed)", "error", snapshotErr)
		} else {
			jobLogger.Infow(
				"no pending transcripts",
				"eligible_total",
				snapshot.TotalEligible,
				"due_now",
				snapshot.DueNow,
				"pending",
				snapshot.Pending,
				"processing",
				snapshot.Processing,
				"failed",
				snapshot.Failed,
				"next_retry_at",
				snapshot.NextRetryAt,
			)
		}
		return nil
	}

	workers := boundedWorkerCount(cfg.MaxConcurrency, 1, len(*items))
	if cfg.Diarization && workers > 1 {
		// Diarization pipelines are heavy and frequently contend on shared HF locks;
		// keeping this single-worker avoids lock thrash on NAS/CPU environments.
		workers = 1
		jobLogger.Infow("whisperx concurrency reduced for diarization stability", "workers", workers)
	}
	jobLogger.Infow("whisperx worker pool started", "count", len(*items), "workers", workers)

	var (
		firstErr error
		errMutex sync.Mutex
	)
	setError := func(processErr error) {
		if processErr == nil {
			return
		}
		errMutex.Lock()
		if firstErr == nil {
			firstErr = processErr
		}
		errMutex.Unlock()
	}

	runWorkerPool(*items, workers, func(item db.PodcastItem) {
		// WhisperX only runs against local downloaded audio files.
		// Validate the audio file is present and usable before attempting transcription.
		if item.DownloadPath == "" {
			jobLogger.Warnw(
				"skipping transcription: no download path set",
				"podcast_item_id", item.ID,
				"episode", item.Title,
				"download_status", item.DownloadStatus,
			)
			resetItemForRedownload(&item, "no download path set", jobLogger)
			return
		}
		if !FileExists(item.DownloadPath) {
			jobLogger.Warnw(
				"skipping transcription: audio file not found on disk",
				"podcast_item_id", item.ID,
				"episode", item.Title,
				"path", item.DownloadPath,
			)
			resetItemForRedownload(&item, "audio file not found on disk", jobLogger)
			return
		}
		fileSize, sizeErr := GetFileSize(item.DownloadPath)
		if sizeErr != nil {
			jobLogger.Warnw(
				"skipping transcription: unable to read audio file",
				"podcast_item_id", item.ID,
				"episode", item.Title,
				"path", item.DownloadPath,
				"error", sizeErr,
			)
			resetItemForRedownload(&item, fmt.Sprintf("unable to read audio file: %v", sizeErr), jobLogger)
			return
		}
		const minAudioFileBytes = 1024
		if fileSize < minAudioFileBytes {
			jobLogger.Warnw(
				"skipping transcription: audio file too small (likely corrupted or incomplete download)",
				"podcast_item_id", item.ID,
				"episode", item.Title,
				"path", item.DownloadPath,
				"file_size_bytes", fileSize,
			)
			if removeErr := os.Remove(item.DownloadPath); removeErr != nil && !os.IsNotExist(removeErr) {
				jobLogger.Warnw("failed to remove corrupted audio file", "path", item.DownloadPath, "error", removeErr)
			}
			resetItemForRedownload(&item, fmt.Sprintf("audio file too small (%d bytes)", fileSize), jobLogger)
			return
		}

		jobLogger.Infow(
			"starting transcription item",
			"podcast_item_id",
			item.ID,
			"status",
			item.TranscriptStatus,
			"retry_count",
			item.TranscriptRetryCount,
			"next_attempt",
			item.TranscriptNextAttempt,
			"download_path",
			item.DownloadPath,
		)

		item.TranscriptStatus = "processing"
		item.TranscriptNextAttempt = nil
		if strings.TrimSpace(item.TranscriptCheckpointJSON) != "" {
			if item.TranscriptProgressPct <= 0 {
				item.TranscriptProgressPct = 1
			}
			item.TranscriptProgressStage = "resuming"
		} else {
			item.TranscriptProgressPct = 1
			item.TranscriptProgressStage = "starting"
		}
		if err := db.UpdatePodcastItem(&item); err != nil {
			jobLogger.Warnw("failed to mark transcript processing", "podcast_item_id", item.ID, "error", err)
		}

		output, err := backend.Transcribe(
			ctx,
			item.DownloadPath,
			item.TranscriptCheckpointJSON,
			func(progress whisperxProgressUpdate) {
				if !applyWhisperXProgressUpdate(&item, progress) {
					return
				}
				if updateErr := db.UpdatePodcastItem(&item); updateErr != nil {
					jobLogger.Warnw(
						"failed to persist transcript progress",
						"podcast_item_id",
						item.ID,
						"stage",
						progress.Stage,
						"percent",
						progress.Percent,
						"error",
						updateErr,
					)
				}
			},
		)
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "Failed to load audio") || strings.Contains(errMsg, "No such file or directory") {
				jobLogger.Errorw(
					"whisperx transcription failed: audio file is unreadable or corrupted, resetting for re-download",
					"podcast_item_id", item.ID,
					"episode", item.Title,
					"path", item.DownloadPath,
					"error", err,
				)
				if removeErr := os.Remove(item.DownloadPath); removeErr != nil && !os.IsNotExist(removeErr) {
					jobLogger.Warnw("failed to remove corrupted audio file", "path", item.DownloadPath, "error", removeErr)
				}
				resetItemForRedownload(&item, fmt.Sprintf("audio file unreadable: %s", trimToLength(errMsg, 200)), jobLogger)
				setError(err)
				return
			}

			scheduleTranscriptRetry(&item, cfg, err)
			item.TranscriptProgressStage = "failed"
			if item.TranscriptProgressPct >= 100 {
				item.TranscriptProgressPct = 99
			}
			jobLogger.Warnw(
				"whisperx transcription failed",
				"podcast_item_id", item.ID,
				"episode", item.Title,
				"error", err,
				"retry_count", item.TranscriptRetryCount,
				"next_attempt", item.TranscriptNextAttempt,
			)
			if updateErr := db.UpdatePodcastItem(&item); updateErr != nil {
				jobLogger.Warnw("failed to mark transcript failure", "podcast_item_id", item.ID, "error", updateErr)
			}
			setError(err)
			return
		}

		item.TranscriptJSON = string(output)
		item.CanonicalTranscript = buildCanonicalTranscriptFromTranscriptJSON(item.TranscriptJSON)
		item.CanonicalTranscriptVersion = canonicalTranscriptVersionCurrent
		canonicalUpdatedAt := time.Now().UTC()
		item.CanonicalUpdatedAt = &canonicalUpdatedAt
		item.TranscriptStatus = "available"
		item.TranscriptRetryCount = 0
		item.TranscriptNextAttempt = nil
		item.TranscriptLastError = ""
		item.TranscriptProgressPct = 100
		item.TranscriptProgressStage = "complete"
		item.TranscriptCheckpointJSON = ""
		// Lineage: record which WhisperX model produced this transcript.
		item.TranscriptModel = transcriptResultModel(output, cfg.Model)
		if err := db.UpdatePodcastItem(&item); err != nil {
			jobLogger.Warnw("failed to save transcript output", "podcast_item_id", item.ID, "error", err)
			setError(err)
		}

		ExportTranscript(&item)

		// Summarize the transcript if LLM summarization is enabled.
		llmCfg := LoadLLMConfig()
		setting := db.GetOrCreateSetting()
		if llmCfg.Enabled && setting.SummarizationEnabled && strings.TrimSpace(llmCfg.APIKey) != "" {
			llmCfg.Model = ResolveSummarizationModel(setting, llmCfg)
			prompt := ResolveSummarizationPrompt(setting, llmCfg)
			userPrompt := ResolveSummarizationUserPrompt(setting, llmCfg)
			if sumErr := SummarizeEpisode(&item, llmCfg, prompt, userPrompt); sumErr != nil {
				jobLogger.Warnw(
					"episode summarization failed",
					"podcast_item_id", item.ID,
					"error", sumErr,
				)
			} else {
				jobLogger.Infow(
					"episode summarized successfully",
					"podcast_item_id", item.ID,
					"summary_length", len(item.LLMSummary),
				)
			}
		}
	})

	return firstErr
}

func startJobLockHeartbeat(lockID string, durationMins int, refreshSecs int, onError func(error)) func() {
	if lockID == "" || durationMins <= 0 || refreshSecs <= 0 {
		return func() {}
	}

	stopCh := make(chan struct{})
	var stopOnce sync.Once
	ticker := time.NewTicker(time.Duration(refreshSecs) * time.Second)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				if err := db.RefreshLockByID(lockID, durationMins); err != nil && onError != nil {
					onError(err)
				}
			}
		}
	}()

	return func() {
		stopOnce.Do(func() { close(stopCh) })
	}
}

func applyWhisperXProgressUpdate(item *db.PodcastItem, progress whisperxProgressUpdate) bool {
	if item == nil {
		return false
	}

	changed := false
	stage := strings.TrimSpace(progress.Stage)
	if stage != "" && stage != item.TranscriptProgressStage {
		item.TranscriptProgressStage = stage
		changed = true
	}

	pct := progress.Percent
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	if stage == "complete" {
		pct = 100
	}
	if pct > item.TranscriptProgressPct || stage == "complete" {
		if item.TranscriptProgressPct != pct {
			item.TranscriptProgressPct = pct
			changed = true
		}
	}

	if len(progress.Checkpoint) > 0 && json.Valid(progress.Checkpoint) {
		checkpoint := strings.TrimSpace(string(progress.Checkpoint))
		if checkpoint != "" && checkpoint != item.TranscriptCheckpointJSON {
			item.TranscriptCheckpointJSON = checkpoint
			changed = true
		}
	}

	return changed
}

func collectWhisperXQueueSnapshot(statuses []string, now time.Time) (whisperxQueueSnapshot, error) {
	snapshot := whisperxQueueSnapshot{}
	baseQuery := func() *gorm.DB {
		return db.DB.Model(&db.PodcastItem{}).
			Where("download_status = ?", db.Downloaded).
			Where("transcript_status IN ?", statuses).
			Where("download_path <> ''").
			Where("(transcript_json IS NULL OR transcript_json = '')")
	}

	if err := baseQuery().Count(&snapshot.TotalEligible).Error; err != nil {
		return snapshot, err
	}
	if err := baseQuery().
		Where("(transcript_next_attempt IS NULL OR transcript_next_attempt <= ?)", now).
		Count(&snapshot.DueNow).
		Error; err != nil {
		return snapshot, err
	}

	type transcriptStatusCount struct {
		TranscriptStatus string
		Count            int64
	}
	var counts []transcriptStatusCount
	if err := baseQuery().
		Select("transcript_status, COUNT(*) AS count").
		Group("transcript_status").
		Scan(&counts).
		Error; err != nil {
		return snapshot, err
	}
	for _, row := range counts {
		switch row.TranscriptStatus {
		case "pending_whisperx":
			snapshot.Pending = row.Count
		case "processing":
			snapshot.Processing = row.Count
		case "failed":
			snapshot.Failed = row.Count
		}
	}

	var nextRetry sql.NullString
	if err := baseQuery().
		Where("transcript_next_attempt > ?", now).
		Select("MIN(transcript_next_attempt)").
		Row().
		Scan(&nextRetry); err != nil {
		return snapshot, err
	}
	if nextRetry.Valid {
		trimmed := strings.TrimSpace(nextRetry.String)
		layouts := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05.999999999-07:00",
			"2006-01-02 15:04:05.999999999",
			"2006-01-02 15:04:05",
		}
		for _, layout := range layouts {
			if parsed, parseErr := time.Parse(layout, trimmed); parseErr == nil {
				next := parsed.UTC()
				snapshot.NextRetryAt = &next
				break
			}
		}
	}
	return snapshot, nil
}

func resetItemForRedownload(item *db.PodcastItem, reason string, jobLogger *zap.SugaredLogger) {
	item.DownloadStatus = db.NotDownloaded
	item.DownloadPath = ""
	item.DownloadedBytes = 0
	item.DownloadTotalBytes = 0
	item.TranscriptStatus = "pending_whisperx"
	item.TranscriptProgressPct = 0
	item.TranscriptProgressStage = "waiting_for_download"
	item.TranscriptCheckpointJSON = ""
	item.TranscriptRetryCount = 0
	item.TranscriptNextAttempt = nil
	item.TranscriptLastError = reason
	if err := db.UpdatePodcastItem(item); err != nil {
		jobLogger.Warnw("failed to reset item for re-download", "podcast_item_id", item.ID, "error", err)
	}
}

func scheduleTranscriptRetry(item *db.PodcastItem, cfg WhisperXConfig, transcriptionErr error) {
	item.TranscriptStatus = "failed"
	item.TranscriptRetryCount++
	item.TranscriptLastError = trimToLength(transcriptionErr.Error(), cfg.RetryMaxErrorChars)

	if !cfg.RetryFailed {
		item.TranscriptNextAttempt = nil
		return
	}

	delay := whisperxRetryDelay(item.TranscriptRetryCount, cfg.RetryDelaySeconds, cfg.RetryMaxDelay)
	nextAttempt := time.Now().UTC().Add(delay)
	item.TranscriptNextAttempt = &nextAttempt
}

func whisperxRetryDelay(attempt int, baseSeconds int, maxSeconds int) time.Duration {
	if baseSeconds <= 0 {
		baseSeconds = defaultWhisperXRetryDelay
	}
	if maxSeconds < baseSeconds {
		maxSeconds = baseSeconds
	}

	delay := time.Duration(baseSeconds) * time.Second
	maxDelay := time.Duration(maxSeconds) * time.Second
	if attempt <= 1 {
		return delay
	}

	for i := 1; i < attempt; i++ {
		if delay >= maxDelay {
			return maxDelay
		}
		next := delay * 2
		if next <= 0 || next > maxDelay {
			return maxDelay
		}
		delay = next
	}
	return delay
}

func trimToLength(value string, maxLen int) string {
	if maxLen <= 0 || len(value) <= maxLen {
		return value
	}
	return value[:maxLen]
}

func truncateForLog(value string, maxLen int) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if maxLen <= 0 || len(trimmed) <= maxLen {
		return trimmed
	}
	return trimmed[:maxLen] + "...(truncated)"
}

func getEnvString(name string, fallback string) string {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	return raw
}

func defaultWhisperXHFHome() string {
	// Prefer mounted config when available so model cache survives container restarts.
	if stat, err := os.Stat("/config"); err == nil && stat.IsDir() {
		return "/config/.cache/huggingface"
	}
	return ""
}

func getEnvInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func getEnvFloat(name string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return value
}

func getEnvBool(name string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

// TranscriptModel describes actual ASR, not the current model in a checkpoint-only execution.
func transcriptResultModel(output []byte, fallback string) string {
	model := fallback
	var resultModel struct {
		Model    string `json:"model"`
		Metadata struct {
			ASRPerformed *bool `json:"asr_performed"`
		} `json:"metadata"`
	}
	if json.Unmarshal(output, &resultModel) == nil {
		if resultModel.Model != "" {
			model = resultModel.Model
		}
		if resultModel.Metadata.ASRPerformed != nil && !*resultModel.Metadata.ASRPerformed {
			model = ""
		}
	}
	return model
}
