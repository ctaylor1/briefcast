package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadWhisperXConfigDefaultsAndNormalization(t *testing.T) {
	t.Setenv(whisperxEnabledEnv, "true")
	t.Setenv("WHISPERX_MIN_SPEAKERS", "0")
	t.Setenv("WHISPERX_MAX_SPEAKERS", "0")
	t.Setenv("WHISPERX_MAX_CONCURRENCY", "0")
	t.Setenv(whisperxScriptEnv, "")

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
