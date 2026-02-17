package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveMutagenPythonExplicitPath(t *testing.T) {
	t.Setenv(mutagenPythonEnv, "not-a-real-python")
	path, err := resolveMutagenPython()
	if err != nil {
		t.Fatalf("expected explicit mutagen python path to be accepted, got %v", err)
	}
	if path != "not-a-real-python" {
		t.Fatalf("expected explicit path, got %q", path)
	}
}

func TestExtractID3MetadataWithStubScript(t *testing.T) {
	pythonPath := requireWorkingPython(t)

	tempDir := t.TempDir()
	audioPath := filepath.Join(tempDir, "audio.mp3")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("failed to create audio file: %v", err)
	}

	scriptPath := filepath.Join(tempDir, "mutagen_stub.py")
	body := "#!/usr/bin/env python3\nprint('{\"tags\":{},\"chapters\":[]}')\n"
	if err := os.WriteFile(scriptPath, []byte(body), 0o755); err != nil {
		t.Fatalf("failed to write mutagen stub: %v", err)
	}

	t.Setenv(mutagenPythonEnv, pythonPath)
	t.Setenv(mutagenScriptEnv, scriptPath)

	output, err := ExtractID3Metadata(audioPath)
	if err != nil {
		t.Fatalf("ExtractID3Metadata failed: %v", err)
	}
	if string(output) != "{\"tags\":{},\"chapters\":[]}\n" {
		t.Fatalf("unexpected id3 output: %q", string(output))
	}
}
