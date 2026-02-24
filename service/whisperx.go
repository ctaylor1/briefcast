package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/logging"
)

type WhisperXConfig struct {
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

type whisperxScriptConfig struct {
	Model        string                 `json:"model"`
	Language     string                 `json:"language"`
	Device       string                 `json:"device"`
	ComputeType  string                 `json:"compute_type"`
	BatchSize    int                    `json:"batch_size"`
	ASROptions   map[string]interface{} `json:"asr_options"`
	VADOptions   map[string]interface{} `json:"vad_options"`
	VADMethod    string                 `json:"vad_method"`
	Align        bool                   `json:"align"`
	Diarization  bool                   `json:"diarization"`
	DiarizeModel string                 `json:"diarization_model"`
	MinSpeakers  int                    `json:"min_speakers"`
	MaxSpeakers  int                    `json:"max_speakers"`
}

const (
	defaultWhisperXScript         = "scripts/whisperx_transcribe.py"
	defaultWhisperXTimeoutSeconds = 21600
	defaultWhisperXRetryDelay     = 300
	defaultWhisperXRetryMaxDelay  = 21600
	defaultWhisperXMaxErrorChars  = 1000
	defaultWhisperXLockDuration   = 30
	defaultWhisperXLockRefresh    = 60
	whisperxPythonEnv             = "WHISPERX_PYTHON"
	whisperxScriptEnv             = "WHISPERX_SCRIPT"
	whisperxTimeoutEnv            = "WHISPERX_TIMEOUT_SECONDS"
	whisperxEnabledEnv            = "WHISPERX_ENABLED"
	whisperxHFTokenEnv            = "WHISPERX_HF_TOKEN" // #nosec G101 -- env var key name only, not a credential value.
	whisperxLockDurationEnv       = "WHISPERX_LOCK_DURATION_MINUTES"
	whisperxLockRefreshEnv        = "WHISPERX_LOCK_REFRESH_SECONDS"
)

func LoadWhisperXConfig() WhisperXConfig {
	cfg := WhisperXConfig{
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

func TranscribePendingEpisodes() error {
	cfg := LoadWhisperXConfig()
	if !cfg.Enabled {
		return nil
	}

	jobLogger, _ := logging.NewJobSugar("TranscribePendingEpisodes")
	start := time.Now()
	jobLogger.Infow("job_started")
	defer func() {
		jobLogger.Infow("job_finished", "duration_ms", time.Since(start).Milliseconds())
	}()

	lock := db.GetLock("TranscribePendingEpisodes")
	if lock.IsLocked() {
		// Another run is already processing; skip this cron tick.
		jobLogger.Infow("job_skipped_lock_exists")
		return nil
	}
	jobLock := db.Lock("TranscribePendingEpisodes", cfg.LockDurationMins)
	if jobLock == nil || jobLock.ID == "" {
		acquireErr := errors.New("failed to acquire transcription job lock")
		jobLogger.Errorw("job_lock_acquire_failed", "error", acquireErr)
		return acquireErr
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

	if _, err := resolveWhisperXPython(cfg); err != nil {
		jobLogger.Errorw("whisperx python resolution failed", "error", err)
		return err
	}
	if _, err := resolveWhisperXScript(cfg); err != nil {
		jobLogger.Errorw("whisperx script resolution failed", "error", err)
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
		jobLogger.Infow("no pending transcripts")
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
		if item.DownloadPath == "" || !FileExists(item.DownloadPath) {
			missingErr := fmt.Errorf("audio file missing for transcription at %s", item.DownloadPath)
			scheduleTranscriptRetry(&item, cfg, missingErr)
			jobLogger.Warnw(
				"audio file missing for transcription",
				"podcast_item_id",
				item.ID,
				"path",
				item.DownloadPath,
				"retry_count",
				item.TranscriptRetryCount,
				"next_attempt",
				item.TranscriptNextAttempt,
			)
			if err := db.UpdatePodcastItem(&item); err != nil {
				jobLogger.Warnw("failed to mark transcript failure", "podcast_item_id", item.ID, "error", err)
			}
			return
		}

		item.TranscriptStatus = "processing"
		item.TranscriptNextAttempt = nil
		if err := db.UpdatePodcastItem(&item); err != nil {
			jobLogger.Warnw("failed to mark transcript processing", "podcast_item_id", item.ID, "error", err)
		}

		output, err := RunWhisperX(item.DownloadPath, cfg)
		if err != nil {
			scheduleTranscriptRetry(&item, cfg, err)
			jobLogger.Warnw(
				"whisperx transcription failed",
				"podcast_item_id",
				item.ID,
				"error",
				err,
				"retry_count",
				item.TranscriptRetryCount,
				"next_attempt",
				item.TranscriptNextAttempt,
			)
			if updateErr := db.UpdatePodcastItem(&item); updateErr != nil {
				jobLogger.Warnw("failed to mark transcript failure", "podcast_item_id", item.ID, "error", updateErr)
			}
			setError(err)
			return
		}

		item.TranscriptJSON = string(output)
		item.TranscriptStatus = "available"
		item.TranscriptRetryCount = 0
		item.TranscriptNextAttempt = nil
		item.TranscriptLastError = ""
		if err := db.UpdatePodcastItem(&item); err != nil {
			jobLogger.Warnw("failed to save transcript output", "podcast_item_id", item.ID, "error", err)
			setError(err)
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

func RunWhisperX(audioPath string, cfg WhisperXConfig) ([]byte, error) {
	pythonPath, err := resolveWhisperXPython(cfg)
	if err != nil {
		return nil, err
	}
	scriptPath, err := resolveWhisperXScript(cfg)
	if err != nil {
		return nil, err
	}

	scriptCfg := whisperxScriptConfig{
		Model:       cfg.Model,
		Language:    cfg.Language,
		Device:      cfg.Device,
		ComputeType: cfg.ComputeType,
		BatchSize:   cfg.BatchSize,
		ASROptions: map[string]interface{}{
			"beam_size":                  cfg.BeamSize,
			"patience":                   cfg.Patience,
			"condition_on_previous_text": cfg.ConditionOnPrev,
			"initial_prompt":             cfg.InitialPrompt,
		},
		VADOptions: map[string]interface{}{
			"chunk_size": cfg.VADChunkSize,
			"vad_onset":  cfg.VADOnset,
			"vad_offset": cfg.VADOffset,
		},
		VADMethod:    cfg.VADMethod,
		Align:        cfg.Align,
		Diarization:  cfg.Diarization,
		DiarizeModel: cfg.DiarizationModel,
		MinSpeakers:  cfg.MinSpeakers,
		MaxSpeakers:  cfg.MaxSpeakers,
	}

	payload, err := json.Marshal(scriptCfg)
	if err != nil {
		return nil, fmt.Errorf("whisperx config encoding failed: %w", err)
	}

	timeoutSeconds := getEnvInt(whisperxTimeoutEnv, defaultWhisperXTimeoutSeconds)
	cmdCtx := context.Background()
	cancel := func() {}
	if timeoutSeconds > 0 {
		cmdCtx, cancel = context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	}
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, pythonPath, scriptPath, audioPath)
	cmd.Env = append(os.Environ(), "WHISPERX_CONFIG_JSON="+string(payload))
	if cfg.HFToken != "" {
		cmd.Env = append(cmd.Env, whisperxHFTokenEnv+"="+cfg.HFToken)
	}
	if cfg.HFHome != "" {
		// Persisting HF cache under mounted config avoids repeated model downloads.
		if err := os.MkdirAll(cfg.HFHome, 0o755); err == nil {
			cmd.Env = append(cmd.Env,
				"HF_HOME="+cfg.HFHome,
				"HUGGINGFACE_HUB_CACHE="+filepath.Join(cfg.HFHome, "hub"),
				"TRANSFORMERS_CACHE="+filepath.Join(cfg.HFHome, "transformers"),
			)
		}
	}
	if cfg.DisableTelemetry {
		// Disable external telemetry/analytics calls to avoid long blocked retries.
		cmd.Env = append(cmd.Env,
			"HF_HUB_DISABLE_TELEMETRY=1",
			"DO_NOT_TRACK=1",
			"PYANNOTE_METRICS_ENABLED=0",
		)
	}
	cmd.Env = append(cmd.Env, "WHISPERX_THIRD_PARTY_LOG_LEVEL="+cfg.ThirdPartyLogLevel)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrText := strings.TrimSpace(stderr.String())
		if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf(
				"whisperx timed out after %d seconds: %s",
				timeoutSeconds,
				stderrText,
			)
		}
		return nil, fmt.Errorf("whisperx failed: %w: %s", err, stderrText)
	}

	if !json.Valid(stdout.Bytes()) {
		return nil, fmt.Errorf("whisperx output is not valid JSON: %s", strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func resolveWhisperXPython(cfg WhisperXConfig) (string, error) {
	explicit := strings.TrimSpace(cfg.Python)
	if explicit != "" {
		if path, err := exec.LookPath(explicit); err == nil {
			return path, nil
		}
		return explicit, nil
	}

	if path, err := resolvePython(); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("python interpreter not found; set %s or %s", whisperxPythonEnv, feedparserPythonEnv)
}

func resolveWhisperXScript(cfg WhisperXConfig) (string, error) {
	scriptPath := strings.TrimSpace(cfg.Script)
	if scriptPath == "" {
		scriptPath = defaultWhisperXScript
	}
	if abs, absErr := filepath.Abs(scriptPath); absErr == nil {
		scriptPath = abs
	}
	if _, err := os.Stat(scriptPath); err != nil {
		return "", fmt.Errorf("whisperx script not found at %s", scriptPath)
	}
	return scriptPath, nil
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
