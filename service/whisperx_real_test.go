package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWhisperXRealTranscription(t *testing.T) {
	if os.Getenv("BRIEFCAST_WHISPERX_REAL") == "" {
		t.Skip("set BRIEFCAST_WHISPERX_REAL=1 to run real WhisperX regression")
	}

	audioPath := strings.TrimSpace(os.Getenv("WHISPERX_TEST_AUDIO"))
	if audioPath == "" {
		t.Skip("set WHISPERX_TEST_AUDIO to a local audio file to run WhisperX regression")
	}
	if _, err := os.Stat(audioPath); err != nil {
		t.Fatalf("WHISPERX_TEST_AUDIO not found: %v", err)
	}

	if _, err := resolvePython(); err != nil && strings.TrimSpace(os.Getenv("WHISPERX_PYTHON")) == "" {
		t.Fatalf("python not available: %v", err)
	}

	scriptPath := filepath.Join("scripts", "whisperx_transcribe.py")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("whisperx script not found: %v", err)
	}

	if strings.TrimSpace(os.Getenv("WHISPERX_SCRIPT")) == "" {
		t.Setenv("WHISPERX_SCRIPT", scriptPath)
	}
	if strings.TrimSpace(os.Getenv("WHISPERX_DIARIZATION")) == "" {
		t.Setenv("WHISPERX_DIARIZATION", "false")
	}
	if strings.TrimSpace(os.Getenv("WHISPERX_MODEL")) == "" {
		t.Setenv("WHISPERX_MODEL", "tiny.en")
	}
	if strings.TrimSpace(os.Getenv("WHISPERX_DEVICE")) == "" {
		t.Setenv("WHISPERX_DEVICE", "cpu")
	}
	if strings.TrimSpace(os.Getenv("WHISPERX_COMPUTE_TYPE")) == "" {
		t.Setenv("WHISPERX_COMPUTE_TYPE", "int8")
	}

	cfg := LoadWhisperXConfig()
	output, err := RunWhisperX(audioPath, cfg)
	if err != nil {
		t.Fatalf("whisperx run failed: %v", err)
	}

	if !json.Valid(output) {
		t.Fatalf("whisperx output is not valid JSON")
	}

	var payload struct {
		Segments []map[string]interface{} `json:"segments"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		t.Fatalf("failed to parse whisperx output: %v", err)
	}
	if len(payload.Segments) == 0 {
		t.Fatalf("expected at least one segment in whisperx output")
	}
}
