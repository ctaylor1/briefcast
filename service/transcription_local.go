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
	"strings"
	"time"

	"github.com/ctaylor1/briefcast/internal/logging"
)

// localTranscriptionBackend preserves the existing Python runner during transition.
// It does not select episodes or persist application state.
type localTranscriptionBackend struct {
	cfg WhisperXConfig
}

var _ transcriptionBackend = localTranscriptionBackend{}

func (b localTranscriptionBackend) Preflight(ctx context.Context) error {
	scriptPath, err := resolveWhisperXScript(b.cfg)
	if err != nil {
		return err
	}
	pythonPath, err := resolveWhisperXPython(b.cfg)
	if err != nil {
		return err
	}
	return runWhisperXPreflightContext(ctx, pythonPath, scriptPath)
}

// RunWhisperX handles the corresponding operation.
func RunWhisperX(audioPath string, cfg WhisperXConfig) ([]byte, error) {
	return RunWhisperXWithProgress(audioPath, cfg, "", nil)
}

// RunWhisperXWithProgress handles the corresponding operation.
func RunWhisperXWithProgress(audioPath string, cfg WhisperXConfig, resumeCheckpoint string, onProgress func(whisperxProgressUpdate)) ([]byte, error) {
	return (localTranscriptionBackend{cfg: cfg}).Transcribe(context.Background(), audioPath, resumeCheckpoint, onProgress)
}

func (b localTranscriptionBackend) Transcribe(ctx context.Context, audioPath, resumeCheckpoint string, onProgress func(whisperxProgressUpdate)) ([]byte, error) {
	cfg := b.cfg
	pythonPath, err := resolveWhisperXPython(cfg)
	if err != nil {
		return nil, err
	}
	scriptPath, err := resolveWhisperXScript(cfg)
	if err != nil {
		return nil, err
	}

	scriptCfg := whisperxScriptConfig{
		Model:        cfg.Model,
		Language:     cfg.Language,
		Device:       cfg.Device,
		ComputeType:  cfg.ComputeType,
		BatchSize:    cfg.BatchSize,
		ChunkSeconds: cfg.ChunkSeconds,
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
	cmdCtx := ctx
	cancel := func() {}
	if timeoutSeconds > 0 {
		cmdCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	}
	defer cancel()

	progressFile, err := os.CreateTemp("", "briefcast-whisperx-progress-*.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create whisperx progress file: %w", err)
	}
	progressPath := progressFile.Name()
	_ = progressFile.Close()
	defer os.Remove(progressPath)

	resumePath := ""
	resumeRaw := strings.TrimSpace(resumeCheckpoint)
	if resumeRaw != "" {
		if !json.Valid([]byte(resumeRaw)) {
			resumeRaw = ""
		}
	}
	segmentsPath := resolveWhisperXSegmentsFilePath(audioPath, resumeRaw)
	if segmentsPath != "" {
		if abs, absErr := filepath.Abs(segmentsPath); absErr == nil {
			segmentsPath = abs
		}
		if mkErr := os.MkdirAll(filepath.Dir(segmentsPath), 0o755); mkErr != nil {
			return nil, fmt.Errorf("failed to create whisperx resume directory: %w", mkErr)
		}
	}
	if resumeRaw != "" {
		resumeFile, createErr := os.CreateTemp("", "briefcast-whisperx-resume-*.json")
		if createErr != nil {
			return nil, fmt.Errorf("failed to create whisperx resume file: %w", createErr)
		}
		resumePath = resumeFile.Name()
		if _, writeErr := resumeFile.WriteString(resumeRaw); writeErr != nil {
			_ = resumeFile.Close()
			return nil, fmt.Errorf("failed to write whisperx resume file: %w", writeErr)
		}
		if closeErr := resumeFile.Close(); closeErr != nil {
			return nil, fmt.Errorf("failed to close whisperx resume file: %w", closeErr)
		}
		defer os.Remove(resumePath)
	}

	cmd := exec.CommandContext(cmdCtx, pythonPath, scriptPath, audioPath)
	cmd.Env = append(
		os.Environ(),
		"WHISPERX_CONFIG_JSON="+string(payload),
		whisperxProgressFileEnv+"="+progressPath,
	)
	if segmentsPath != "" {
		cmd.Env = append(cmd.Env, whisperxSegmentsFileEnv+"="+segmentsPath)
	}
	if resumePath != "" {
		cmd.Env = append(cmd.Env, whisperxResumeFileEnv+"="+resumePath)
	}
	if cfg.HFToken != "" {
		cmd.Env = append(cmd.Env, whisperxHFTokenEnv+"="+cfg.HFToken)
	}
	if cfg.HFHome != "" {
		// Persisting HF cache under mounted config avoids repeated model downloads.
		if err := os.MkdirAll(cfg.HFHome, 0o755); err == nil {
			cmd.Env = append(cmd.Env,
				"HF_HOME="+cfg.HFHome,
				"HUGGINGFACE_HUB_CACHE="+filepath.Join(cfg.HFHome, "hub"),
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
	// Force Python logging to stderr so stdout contains only the JSON transcript.
	// The parent process LOG_OUTPUT may include "stdout" (e.g. LOG_OUTPUT=stdout,file:...)
	// which would contaminate stdout and break JSON parsing of the transcript output.
	cmd.Env = append(cmd.Env, "LOG_OUTPUT=stderr")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	pollMillis := getEnvInt(whisperxProgressPollEnv, defaultWhisperXProgressPollMS)
	if pollMillis < 250 {
		pollMillis = 250
	}
	ticker := time.NewTicker(time.Duration(pollMillis) * time.Millisecond)
	defer ticker.Stop()

	lastProgressRaw := ""
	readProgress := func(force bool) {
		if onProgress == nil {
			return
		}
		update, raw, found, readErr := readWhisperXProgressUpdate(progressPath)
		if readErr != nil || !found {
			return
		}
		if !force && raw == lastProgressRaw {
			return
		}
		lastProgressRaw = raw
		onProgress(update)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start whisperx process: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	var runErr error
waitLoop:
	for {
		select {
		case runErr = <-waitCh:
			break waitLoop
		case <-ticker.C:
			readProgress(false)
		}
	}
	readProgress(true)

	if runErr != nil {
		stderrText := truncateForLog(stderr.String(), defaultWhisperXLogTrimChars)
		stdoutText := truncateForLog(stdout.String(), defaultWhisperXLogTrimChars)
		details := make([]string, 0, 2)
		if stderrText != "" {
			details = append(details, "stderr="+stderrText)
		}
		if stdoutText != "" {
			details = append(details, "stdout="+stdoutText)
		}
		detailText := strings.TrimSpace(strings.Join(details, " | "))
		if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
			if detailText == "" {
				detailText = "no process output"
			}
			return nil, fmt.Errorf(
				"whisperx timed out after %d seconds: %s",
				timeoutSeconds,
				detailText,
			)
		}
		if detailText == "" {
			detailText = "no process output"
		}
		return nil, fmt.Errorf("whisperx failed: %w: %s", runErr, detailText)
	}

	if !json.Valid(stdout.Bytes()) {
		return nil, fmt.Errorf(
			"whisperx output is not valid JSON: stdout=%s stderr=%s",
			truncateForLog(stdout.String(), defaultWhisperXLogTrimChars),
			truncateForLog(stderr.String(), defaultWhisperXLogTrimChars),
		)
	}
	if segmentsPath != "" {
		if err := os.Remove(segmentsPath); err != nil && !os.IsNotExist(err) {
			logging.Sugar().Warnw("failed to remove whisperx resume segments file", "path", segmentsPath, "error", err)
		}
	}
	return stdout.Bytes(), nil
}

func readWhisperXProgressUpdate(path string) (whisperxProgressUpdate, string, bool, error) {
	var update whisperxProgressUpdate
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return update, "", false, nil
	}
	rawBytes, err := os.ReadFile(trimmedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return update, "", false, nil
		}
		return update, "", false, err
	}
	raw := strings.TrimSpace(string(rawBytes))
	if raw == "" {
		return update, "", false, nil
	}
	if err := json.Unmarshal([]byte(raw), &update); err != nil {
		return update, raw, false, err
	}
	return update, raw, true, nil
}

func resolveWhisperXSegmentsFilePath(audioPath string, resumeCheckpoint string) string {
	if checkpoint, ok := parseWhisperXResumeCheckpoint(resumeCheckpoint); ok {
		if existing := strings.TrimSpace(checkpoint.SegmentsFile); existing != "" {
			return existing
		}
	}
	trimmedAudio := strings.TrimSpace(audioPath)
	if trimmedAudio == "" {
		return ""
	}
	return trimmedAudio + ".briefcast.whisperx.resume.json"
}

func parseWhisperXResumeCheckpoint(raw string) (whisperxResumeCheckpoint, bool) {
	var checkpoint whisperxResumeCheckpoint
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return checkpoint, false
	}
	if !json.Valid([]byte(trimmed)) {
		return checkpoint, false
	}
	if err := json.Unmarshal([]byte(trimmed), &checkpoint); err != nil {
		return checkpoint, false
	}
	return checkpoint, true
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

func runWhisperXPreflight(pythonPath string, scriptPath string) error {
	return runWhisperXPreflightContext(context.Background(), pythonPath, scriptPath)
}

func runWhisperXPreflightContext(ctx context.Context, pythonPath, scriptPath string) error {
	if strings.TrimSpace(pythonPath) == "" {
		return errors.New("python interpreter path is empty")
	}
	if strings.TrimSpace(scriptPath) == "" {
		return errors.New("whisperx script path is empty")
	}

	timeoutSeconds := getEnvInt(whisperxPreflightEnv, defaultWhisperXPreflightSec)
	cmdCtx := ctx
	cancel := func() {}
	if timeoutSeconds > 0 {
		cmdCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	}
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, pythonPath, "-m", "py_compile", scriptPath)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(
			strings.Join(
				[]string{
					"stdout=" + truncateForLog(stdout.String(), defaultWhisperXLogTrimChars),
					"stderr=" + truncateForLog(stderr.String(), defaultWhisperXLogTrimChars),
				},
				" | ",
			),
		)
		if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("whisperx script preflight timed out after %d seconds: %s", timeoutSeconds, detail)
		}
		return fmt.Errorf("whisperx script preflight failed: %w: %s", err, detail)
	}
	return nil
}
