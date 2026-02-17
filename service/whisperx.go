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
	Enabled          bool
	Python           string
	Script           string
	Model            string
	Language         string
	Device           string
	ComputeType      string
	BatchSize        int
	BeamSize         int
	Patience         float64
	ConditionOnPrev  bool
	InitialPrompt    string
	VADChunkSize     int
	VADOnset         float64
	VADOffset        float64
	VADMethod        string
	Align            bool
	Diarization      bool
	DiarizationModel string
	MinSpeakers      int
	MaxSpeakers      int
	HFToken          string
	RetryFailed      bool
	MaxConcurrency   int
	MaxItemsPerRun   int
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
	defaultWhisperXScript = "scripts/whisperx_transcribe.py"
	defaultWhisperXTimeoutSeconds = 7200
	whisperxPythonEnv     = "WHISPERX_PYTHON"
	whisperxScriptEnv     = "WHISPERX_SCRIPT"
	whisperxTimeoutEnv    = "WHISPERX_TIMEOUT_SECONDS"
	whisperxEnabledEnv    = "WHISPERX_ENABLED"
	whisperxHFTokenEnv    = "WHISPERX_HF_TOKEN"
)

func LoadWhisperXConfig() WhisperXConfig {
	cfg := WhisperXConfig{
		Enabled:          getEnvBool(whisperxEnabledEnv, false),
		Python:           strings.TrimSpace(os.Getenv(whisperxPythonEnv)),
		Script:           strings.TrimSpace(os.Getenv(whisperxScriptEnv)),
		Model:            getEnvString("WHISPERX_MODEL", "medium.en"),
		Language:         getEnvString("WHISPERX_LANGUAGE", "en"),
		Device:           getEnvString("WHISPERX_DEVICE", "auto"),
		ComputeType:      getEnvString("WHISPERX_COMPUTE_TYPE", "auto"),
		BatchSize:        getEnvInt("WHISPERX_BATCH_SIZE", 0),
		BeamSize:         getEnvInt("WHISPERX_BEAM_SIZE", 5),
		Patience:         getEnvFloat("WHISPERX_PATIENCE", 1),
		ConditionOnPrev:  getEnvBool("WHISPERX_CONDITION_ON_PREVIOUS_TEXT", true),
		InitialPrompt:    getEnvString("WHISPERX_INITIAL_PROMPT", "Podcast interview. Speakers are Host and Guest. Use punctuation and capitalization."),
		VADChunkSize:     getEnvInt("WHISPERX_VAD_CHUNK_SIZE", 45),
		VADOnset:         getEnvFloat("WHISPERX_VAD_ONSET", 0.50),
		VADOffset:        getEnvFloat("WHISPERX_VAD_OFFSET", 0.50),
		VADMethod:        getEnvString("WHISPERX_VAD_METHOD", "pyannote"),
		Align:            getEnvBool("WHISPERX_ALIGN", true),
		Diarization:      getEnvBool("WHISPERX_DIARIZATION", true),
		DiarizationModel: getEnvString("WHISPERX_DIARIZATION_MODEL", "pyannote/speaker-diarization-3.1"),
		MinSpeakers:      getEnvInt("WHISPERX_MIN_SPEAKERS", 2),
		MaxSpeakers:      getEnvInt("WHISPERX_MAX_SPEAKERS", 2),
		HFToken:          strings.TrimSpace(os.Getenv(whisperxHFTokenEnv)),
		RetryFailed:      getEnvBool("WHISPERX_RETRY_FAILED", false),
		MaxConcurrency:   getEnvInt("WHISPERX_MAX_CONCURRENCY", 1),
		MaxItemsPerRun:   getEnvInt("WHISPERX_MAX_ITEMS", 0),
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
		jobLogger.Infow("job_skipped_lock_exists")
		return nil
	}
	db.Lock("TranscribePendingEpisodes", 120)
	defer db.Unlock("TranscribePendingEpisodes")

	if _, err := resolveWhisperXPython(cfg); err != nil {
		jobLogger.Errorw("whisperx python resolution failed", "error", err)
		return err
	}
	if _, err := resolveWhisperXScript(cfg); err != nil {
		jobLogger.Errorw("whisperx script resolution failed", "error", err)
		return err
	}

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
		if item.DownloadPath == "" || !FileExists(item.DownloadPath) {
			jobLogger.Warnw("audio file missing for transcription", "podcast_item_id", item.ID, "path", item.DownloadPath)
			item.TranscriptStatus = "failed"
			if err := db.UpdatePodcastItem(&item); err != nil {
				jobLogger.Warnw("failed to mark transcript failure", "podcast_item_id", item.ID, "error", err)
			}
			return
		}

		item.TranscriptStatus = "processing"
		if err := db.UpdatePodcastItem(&item); err != nil {
			jobLogger.Warnw("failed to mark transcript processing", "podcast_item_id", item.ID, "error", err)
		}

		output, err := RunWhisperX(item.DownloadPath, cfg)
		if err != nil {
			jobLogger.Warnw("whisperx transcription failed", "podcast_item_id", item.ID, "error", err)
			item.TranscriptStatus = "failed"
			if updateErr := db.UpdatePodcastItem(&item); updateErr != nil {
				jobLogger.Warnw("failed to mark transcript failure", "podcast_item_id", item.ID, "error", updateErr)
			}
			setError(err)
			return
		}

		item.TranscriptJSON = string(output)
		item.TranscriptStatus = "available"
		if err := db.UpdatePodcastItem(&item); err != nil {
			jobLogger.Warnw("failed to save transcript output", "podcast_item_id", item.ID, "error", err)
			setError(err)
		}
	})

	return firstErr
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

func getEnvString(name string, fallback string) string {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	return raw
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
