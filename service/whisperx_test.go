package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
)

func TestLoadWhisperXConfigDefaultsAndNormalization(t *testing.T) {
	t.Setenv(whisperxEnabledEnv, "true")
	t.Setenv("WHISPERX_MIN_SPEAKERS", "0")
	t.Setenv("WHISPERX_MAX_SPEAKERS", "0")
	t.Setenv("WHISPERX_MAX_CONCURRENCY", "0")
	t.Setenv(whisperxScriptEnv, "")
	t.Setenv(whisperxLockDurationEnv, "0")
	t.Setenv(whisperxLockRefreshEnv, "0")

	cfg := LoadWhisperXConfig()
	if !cfg.Enabled {
		t.Fatalf("expected whisperx enabled")
	}
	if cfg.Script != defaultWhisperXScript {
		t.Fatalf("expected default script %q, got %q", defaultWhisperXScript, cfg.Script)
	}
	if cfg.MinSpeakers != 1 {
		t.Fatalf("expected min speakers normalized to 1, got %d", cfg.MinSpeakers)
	}
	if cfg.MaxSpeakers != 1 {
		t.Fatalf("expected max speakers normalized to min speakers, got %d", cfg.MaxSpeakers)
	}
	if cfg.MaxConcurrency != 1 {
		t.Fatalf("expected max concurrency normalized to 1, got %d", cfg.MaxConcurrency)
	}
	if cfg.ChunkSeconds != 120 {
		t.Fatalf("expected chunk seconds default 120, got %d", cfg.ChunkSeconds)
	}
	if !cfg.RetryFailed {
		t.Fatalf("expected failed transcripts to be retried by default")
	}
	if cfg.RetryDelaySeconds != defaultWhisperXRetryDelay {
		t.Fatalf("expected default retry delay %d, got %d", defaultWhisperXRetryDelay, cfg.RetryDelaySeconds)
	}
	if cfg.RetryMaxDelay != defaultWhisperXRetryMaxDelay {
		t.Fatalf("expected default retry max delay %d, got %d", defaultWhisperXRetryMaxDelay, cfg.RetryMaxDelay)
	}
	if cfg.LockDurationMins != defaultWhisperXLockDuration {
		t.Fatalf("expected lock duration default %d, got %d", defaultWhisperXLockDuration, cfg.LockDurationMins)
	}
	if cfg.LockRefreshSecs != defaultWhisperXLockRefresh {
		t.Fatalf("expected lock refresh default %d, got %d", defaultWhisperXLockRefresh, cfg.LockRefreshSecs)
	}
}

func TestLoadWhisperXConfigLockRefreshClamp(t *testing.T) {
	t.Setenv(whisperxEnabledEnv, "true")
	t.Setenv(whisperxLockDurationEnv, "2")
	t.Setenv(whisperxLockRefreshEnv, "500")

	cfg := LoadWhisperXConfig()
	if cfg.LockDurationMins != 2 {
		t.Fatalf("expected lock duration of 2 minutes, got %d", cfg.LockDurationMins)
	}
	if cfg.LockRefreshSecs >= cfg.LockDurationMins*60 {
		t.Fatalf("expected lock refresh to be less than lock duration window, got %d", cfg.LockRefreshSecs)
	}
}

func TestWhisperXEnvHelpers(t *testing.T) {
	t.Setenv("WX_STR", " value ")
	if got := getEnvString("WX_STR", "fallback"); got != "value" {
		t.Fatalf("expected trimmed env string, got %q", got)
	}
	t.Setenv("WX_STR", "")
	if got := getEnvString("WX_STR", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback string, got %q", got)
	}

	t.Setenv("WX_INT", "12")
	if got := getEnvInt("WX_INT", 3); got != 12 {
		t.Fatalf("expected 12, got %d", got)
	}
	t.Setenv("WX_INT", "bad")
	if got := getEnvInt("WX_INT", 3); got != 3 {
		t.Fatalf("expected fallback 3, got %d", got)
	}

	t.Setenv("WX_FLOAT", "1.25")
	if got := getEnvFloat("WX_FLOAT", 2.0); got != 1.25 {
		t.Fatalf("expected 1.25, got %v", got)
	}
	t.Setenv("WX_FLOAT", "bad")
	if got := getEnvFloat("WX_FLOAT", 2.0); got != 2.0 {
		t.Fatalf("expected fallback 2.0, got %v", got)
	}

	t.Setenv("WX_BOOL", "on")
	if got := getEnvBool("WX_BOOL", false); !got {
		t.Fatalf("expected true")
	}
	t.Setenv("WX_BOOL", "off")
	if got := getEnvBool("WX_BOOL", true); got {
		t.Fatalf("expected false")
	}
	t.Setenv("WX_BOOL", "???")
	if got := getEnvBool("WX_BOOL", true); !got {
		t.Fatalf("expected fallback true")
	}
}

func TestResolveWhisperXScript(t *testing.T) {
	cfg := WhisperXConfig{Script: filepath.Join(t.TempDir(), "missing.py")}
	if _, err := resolveWhisperXScript(cfg); err == nil {
		t.Fatalf("expected missing script error")
	}

	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "whisper.py")
	if err := os.WriteFile(scriptPath, []byte("print('ok')"), 0o644); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}
	cfg.Script = scriptPath
	resolved, err := resolveWhisperXScript(cfg)
	if err != nil {
		t.Fatalf("expected script to resolve: %v", err)
	}
	if !filepath.IsAbs(resolved) {
		t.Fatalf("expected absolute script path, got %q", resolved)
	}
}

func TestResolveWhisperXPythonExplicitPath(t *testing.T) {
	cfg := WhisperXConfig{Python: "definitely-not-real-python"}
	path, err := resolveWhisperXPython(cfg)
	if err != nil {
		t.Fatalf("expected no error for explicit python, got %v", err)
	}
	if path != "definitely-not-real-python" {
		t.Fatalf("expected explicit python path back, got %q", path)
	}
}

func TestRunWhisperXPreflightDetectsSyntaxErrors(t *testing.T) {
	pythonPath := requireWorkingPython(t)
	tempDir := t.TempDir()

	badScript := filepath.Join(tempDir, "bad_whisperx.py")
	if err := os.WriteFile(badScript, []byte("def broken(:\n"), 0o644); err != nil {
		t.Fatalf("failed to write invalid script: %v", err)
	}
	if err := runWhisperXPreflight(pythonPath, badScript); err == nil {
		t.Fatalf("expected preflight to fail for invalid syntax")
	}

	okScript := filepath.Join(tempDir, "ok_whisperx.py")
	okBody := "#!/usr/bin/env python3\nprint('ok')\n"
	if err := os.WriteFile(okScript, []byte(okBody), 0o755); err != nil {
		t.Fatalf("failed to write valid script: %v", err)
	}
	if err := runWhisperXPreflight(pythonPath, okScript); err != nil {
		t.Fatalf("expected preflight to pass for valid script, got %v", err)
	}
}

func TestWhisperXRetryDelayBackoff(t *testing.T) {
	if got := whisperxRetryDelay(1, 60, 600); got != 60*time.Second {
		t.Fatalf("expected first retry delay to be 60s, got %s", got)
	}
	if got := whisperxRetryDelay(2, 60, 600); got != 120*time.Second {
		t.Fatalf("expected second retry delay to be 120s, got %s", got)
	}
	if got := whisperxRetryDelay(5, 60, 600); got != 600*time.Second {
		t.Fatalf("expected capped retry delay to be 600s, got %s", got)
	}
}

func TestApplyWhisperXProgressUpdate(t *testing.T) {
	item := db.PodcastItem{}
	progress := whisperxProgressUpdate{
		Stage:      "transcribing",
		Percent:    42,
		Checkpoint: json.RawMessage(`{"segments":[{"start":1.0,"text":"hello"}],"completed_seconds":60}`),
	}
	if changed := applyWhisperXProgressUpdate(&item, progress); !changed {
		t.Fatalf("expected progress update to apply")
	}
	if item.TranscriptProgressStage != "transcribing" {
		t.Fatalf("expected stage transcribing, got %q", item.TranscriptProgressStage)
	}
	if item.TranscriptProgressPct != 42 {
		t.Fatalf("expected progress pct 42, got %d", item.TranscriptProgressPct)
	}
	if !strings.Contains(item.TranscriptCheckpointJSON, "completed_seconds") {
		t.Fatalf("expected checkpoint json to be stored, got %q", item.TranscriptCheckpointJSON)
	}

	if changed := applyWhisperXProgressUpdate(&item, progress); changed {
		t.Fatalf("expected duplicate progress update to be ignored")
	}

	lowerPct := whisperxProgressUpdate{Stage: "transcribing", Percent: 10}
	_ = applyWhisperXProgressUpdate(&item, lowerPct)
	if item.TranscriptProgressPct != 42 {
		t.Fatalf("expected progress pct to remain monotonic, got %d", item.TranscriptProgressPct)
	}

	complete := whisperxProgressUpdate{Stage: "complete", Percent: 0}
	if changed := applyWhisperXProgressUpdate(&item, complete); !changed {
		t.Fatalf("expected completion update to apply")
	}
	if item.TranscriptProgressPct != 100 {
		t.Fatalf("expected completion to force 100 pct, got %d", item.TranscriptProgressPct)
	}
}

func TestReadWhisperXProgressUpdate(t *testing.T) {
	progressPath := filepath.Join(t.TempDir(), "progress.json")
	if _, _, ok, err := readWhisperXProgressUpdate(progressPath); err != nil || ok {
		t.Fatalf("expected missing progress file to return no update, got ok=%v err=%v", ok, err)
	}

	body := `{"stage":"transcribing","percent":55,"checkpoint":{"segments":[]}}`
	if err := os.WriteFile(progressPath, []byte(body), 0o644); err != nil {
		t.Fatalf("failed to write progress file: %v", err)
	}
	update, raw, ok, err := readWhisperXProgressUpdate(progressPath)
	if err != nil {
		t.Fatalf("expected progress read success, got %v", err)
	}
	if !ok {
		t.Fatalf("expected progress update to be found")
	}
	if update.Stage != "transcribing" || update.Percent != 55 {
		t.Fatalf("unexpected progress payload: %+v", update)
	}
	if strings.TrimSpace(raw) == "" {
		t.Fatalf("expected raw payload text")
	}
}

func TestScheduleTranscriptRetry(t *testing.T) {
	cfg := WhisperXConfig{
		RetryFailed:        true,
		RetryDelaySeconds:  60,
		RetryMaxDelay:      600,
		RetryMaxErrorChars: 10,
	}
	item := db.PodcastItem{}

	scheduleTranscriptRetry(&item, cfg, errors.New("1234567890ABCD"))
	if item.TranscriptStatus != "failed" {
		t.Fatalf("expected status failed, got %q", item.TranscriptStatus)
	}
	if item.TranscriptRetryCount != 1 {
		t.Fatalf("expected retry count 1, got %d", item.TranscriptRetryCount)
	}
	if item.TranscriptNextAttempt == nil {
		t.Fatalf("expected next attempt to be scheduled")
	}
	if item.TranscriptLastError != "1234567890" {
		t.Fatalf("expected trimmed error text, got %q", item.TranscriptLastError)
	}

	firstAttemptAt := *item.TranscriptNextAttempt
	scheduleTranscriptRetry(&item, cfg, errors.New("boom"))
	if item.TranscriptRetryCount != 2 {
		t.Fatalf("expected retry count 2, got %d", item.TranscriptRetryCount)
	}
	if item.TranscriptNextAttempt == nil {
		t.Fatalf("expected second next attempt to be scheduled")
	}
	if !item.TranscriptNextAttempt.After(firstAttemptAt) {
		t.Fatalf("expected second retry to be scheduled later than first attempt")
	}
}

func TestScheduleTranscriptRetryDisabled(t *testing.T) {
	cfg := WhisperXConfig{
		RetryFailed:        false,
		RetryDelaySeconds:  60,
		RetryMaxDelay:      600,
		RetryMaxErrorChars: 100,
	}
	item := db.PodcastItem{}

	scheduleTranscriptRetry(&item, cfg, errors.New("boom"))
	if item.TranscriptStatus != "failed" {
		t.Fatalf("expected status failed, got %q", item.TranscriptStatus)
	}
	if item.TranscriptRetryCount != 1 {
		t.Fatalf("expected retry count 1, got %d", item.TranscriptRetryCount)
	}
	if item.TranscriptNextAttempt != nil {
		t.Fatalf("expected no next attempt when retry is disabled")
	}
}

func TestStartJobLockHeartbeatRefreshesLease(t *testing.T) {
	setupRetentionTestDB(t)

	lock := db.Lock("heartbeat-lock", 1)
	if lock == nil || lock.ID == "" {
		t.Fatalf("expected persisted lock id")
	}

	aged := time.Now().UTC().Add(-5 * time.Minute)
	if err := db.DB.Model(&db.JobLock{}).Where("id = ?", lock.ID).Updates(map[string]interface{}{
		"date":     aged,
		"duration": 1,
	}).Error; err != nil {
		t.Fatalf("failed to age lock: %v", err)
	}

	errCh := make(chan error, 1)
	stop := startJobLockHeartbeat(lock.ID, 3, 1, func(err error) {
		select {
		case errCh <- err:
		default:
		}
	})
	defer stop()

	time.Sleep(1200 * time.Millisecond)

	select {
	case err := <-errCh:
		t.Fatalf("heartbeat returned error: %v", err)
	default:
	}

	updated := db.GetLock("heartbeat-lock")
	if updated.Duration != 3 {
		t.Fatalf("expected heartbeat to refresh duration to 3, got %d", updated.Duration)
	}
	if updated.Date.Before(time.Now().UTC().Add(-30 * time.Second)) {
		t.Fatalf("expected heartbeat to refresh lock date, got %s", updated.Date)
	}
}

func TestRunWhisperXWithStubScript(t *testing.T) {
	pythonPath := requireWorkingPython(t)

	tempDir := t.TempDir()
	audioPath := filepath.Join(tempDir, "audio.mp3")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("failed to create audio file: %v", err)
	}

	successScript := filepath.Join(tempDir, "whisper_ok.py")
	successBody := "#!/usr/bin/env python3\nimport json\nprint(json.dumps({'segments':[{'start':0,'end':1,'text':'ok'}]}))\n"
	if err := os.WriteFile(successScript, []byte(successBody), 0o755); err != nil {
		t.Fatalf("failed to write success script: %v", err)
	}

	cfg := WhisperXConfig{
		Python:          pythonPath,
		Script:          successScript,
		Model:           "tiny.en",
		Language:        "en",
		Device:          "cpu",
		ComputeType:     "int8",
		BatchSize:       1,
		BeamSize:        1,
		Patience:        1,
		ConditionOnPrev: true,
		InitialPrompt:   "prompt",
		VADChunkSize:    10,
		VADOnset:        0.5,
		VADOffset:       0.5,
		VADMethod:       "pyannote",
		Align:           true,
		Diarization:     false,
		MinSpeakers:     1,
		MaxSpeakers:     1,
	}

	output, err := RunWhisperX(audioPath, cfg)
	if err != nil {
		t.Fatalf("RunWhisperX failed: %v", err)
	}
	if !json.Valid(output) {
		t.Fatalf("expected valid json output, got %q", string(output))
	}
	if !strings.Contains(string(output), `"segments"`) {
		t.Fatalf("expected segments in output, got %q", string(output))
	}

	badScript := filepath.Join(tempDir, "whisper_bad.py")
	badBody := "#!/usr/bin/env python3\nprint('not-json')\n"
	if err := os.WriteFile(badScript, []byte(badBody), 0o755); err != nil {
		t.Fatalf("failed to write bad script: %v", err)
	}
	cfg.Script = badScript
	if _, err := RunWhisperX(audioPath, cfg); err == nil {
		t.Fatalf("expected invalid json error")
	}

	slowScript := filepath.Join(tempDir, "whisper_slow.py")
	slowBody := "#!/usr/bin/env python3\nimport json\nimport time\ntime.sleep(2)\nprint(json.dumps({'segments': []}))\n"
	if err := os.WriteFile(slowScript, []byte(slowBody), 0o755); err != nil {
		t.Fatalf("failed to write slow script: %v", err)
	}
	t.Setenv(whisperxTimeoutEnv, "1")
	cfg.Script = slowScript
	if _, err := RunWhisperX(audioPath, cfg); err == nil {
		t.Fatalf("expected timeout error")
	} else if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error text, got %v", err)
	}
}

func TestRunWhisperXWithProgressAndResume(t *testing.T) {
	pythonPath := requireWorkingPython(t)

	tempDir := t.TempDir()
	audioPath := filepath.Join(tempDir, "audio.mp3")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("failed to create audio file: %v", err)
	}

	scriptPath := filepath.Join(tempDir, "whisper_progress.py")
	scriptBody := `#!/usr/bin/env python3
import json
import os

progress_path = os.environ.get("WHISPERX_PROGRESS_FILE", "")
resume_path = os.environ.get("WHISPERX_RESUME_FILE", "")
resume_payload = {}
if resume_path and os.path.exists(resume_path):
    with open(resume_path, "r", encoding="utf-8") as handle:
        resume_payload = json.load(handle)
if progress_path:
    progress_payload = {
        "stage": "transcribing",
        "percent": 55,
        "checkpoint": {
            "segments": [{"start": 1.0, "text": "progress"}],
            "completed_seconds": 12.0
        }
    }
    tmp_path = progress_path + ".tmp"
    with open(tmp_path, "w", encoding="utf-8") as handle:
        json.dump(progress_payload, handle)
    os.replace(tmp_path, progress_path)
print(json.dumps({"segments":[{"start":0,"end":1,"text":"ok"}],"resumed": bool(resume_payload)}))
`
	if err := os.WriteFile(scriptPath, []byte(scriptBody), 0o755); err != nil {
		t.Fatalf("failed to write progress script: %v", err)
	}

	cfg := WhisperXConfig{
		Python:       pythonPath,
		Script:       scriptPath,
		ChunkSeconds: 120,
	}

	var updates []whisperxProgressUpdate
	checkpoint := `{"segments":[{"start":0.0,"text":"seed"}],"completed_seconds":10.0}`
	output, err := RunWhisperXWithProgress(audioPath, cfg, checkpoint, func(progress whisperxProgressUpdate) {
		updates = append(updates, progress)
	})
	if err != nil {
		t.Fatalf("RunWhisperXWithProgress failed: %v", err)
	}
	if len(updates) == 0 {
		t.Fatalf("expected at least one progress callback")
	}
	last := updates[len(updates)-1]
	if last.Stage != "transcribing" || last.Percent != 55 {
		t.Fatalf("unexpected progress callback payload: %+v", last)
	}
	if len(last.Checkpoint) == 0 || !json.Valid(last.Checkpoint) {
		t.Fatalf("expected checkpoint JSON in progress callback, got %q", string(last.Checkpoint))
	}
	var payload struct {
		Resumed bool `json:"resumed"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	if !payload.Resumed {
		t.Fatalf("expected resume checkpoint to be passed to script, output=%s", string(output))
	}
}
