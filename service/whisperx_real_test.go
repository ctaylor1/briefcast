package service

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWhisperXRealTranscription handles the corresponding operation.
func TestWhisperXRealTranscription(t *testing.T) {
	if os.Getenv("BRIEFCAST_WHISPERX_REAL") == "" {
		t.Skip("set BRIEFCAST_WHISPERX_REAL=1 to run real WhisperX regression")
	}

	audioPath := strings.TrimSpace(os.Getenv("WHISPERX_TEST_AUDIO"))
	if audioPath != "" {
		if _, err := os.Stat(audioPath); err != nil {
			t.Fatalf("WHISPERX_TEST_AUDIO not found: %v", err)
		}
	} else {
		audioPath = filepath.Join(t.TempDir(), "whisperx-smoke.wav")
		if err := writeTestWav(audioPath, 3, 16000); err != nil {
			t.Fatalf("failed to create whisperx smoke-test audio: %v", err)
		}
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
		Provider string                   `json:"provider"`
		Segments []map[string]interface{} `json:"segments"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		t.Fatalf("failed to parse whisperx output: %v", err)
	}
	if payload.Provider != "whisperx" {
		t.Fatalf("expected whisperx provider payload, got %q", payload.Provider)
	}
	if payload.Segments == nil {
		t.Fatalf("expected segments array in whisperx output")
	}
}

func writeTestWav(path string, durationSeconds int, sampleRate int) error {
	if durationSeconds <= 0 {
		durationSeconds = 1
	}
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	totalSamples := durationSeconds * sampleRate
	dataSize := totalSamples * 2 // 16-bit PCM

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writeString := func(value string) error {
		_, writeErr := file.WriteString(value)
		return writeErr
	}

	if err := writeString("RIFF"); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(36+dataSize)); err != nil {
		return err
	}
	if err := writeString("WAVE"); err != nil {
		return err
	}
	if err := writeString("fmt "); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(16)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(sampleRate)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(sampleRate*2)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(2)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(16)); err != nil {
		return err
	}
	if err := writeString("data"); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(dataSize)); err != nil {
		return err
	}

	for i := 0; i < totalSamples; i++ {
		amplitude := math.Sin(2 * math.Pi * 440 * float64(i) / float64(sampleRate))
		sample := int16(amplitude * 0.3 * 32767.0)
		if err := binary.Write(file, binary.LittleEndian, sample); err != nil {
			return err
		}
	}
	return nil
}
