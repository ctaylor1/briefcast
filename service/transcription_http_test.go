package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoteDeploymentIdentityIsStableAndRejectsCorruption(t *testing.T) {
	dir := t.TempDir()
	a, err := transcriptionDeploymentID(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := transcriptionDeploymentID(dir)
	if err != nil || a != b {
		t.Fatalf("identity changed: %v", err)
	}
	if err = os.WriteFile(filepath.Join(dir, "whisperx-deployment-id"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = transcriptionDeploymentID(dir); err == nil {
		t.Fatal("corrupt identity was replaced")
	}
}

func TestRemoteConfigurationFailsClosed(t *testing.T) {
	for _, backend := range []string{"HTTP", "unsupported"} {
		if _, err := configuredTranscriptionBackend(WhisperXConfig{Backend: backend}); err == nil {
			t.Fatal("unexpected fallback")
		}
	}
	for _, endpoint := range []string{"file:///tmp/worker", "http://user:password@worker", "http://worker/path", "http://worker?key=value", "http://worker/#secret"} {
		if _, err := newHTTPTranscriptionBackend(WhisperXConfig{ServiceURL: endpoint}); err == nil {
			t.Fatal("invalid endpoint accepted")
		}
	}
}
