package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestIsPathWithinAssetsDir(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("DATA", dataDir)

	inside := filepath.Join(dataDir, "audio", "episode.mp3")
	if err := os.MkdirAll(filepath.Dir(inside), 0o755); err != nil {
		t.Fatalf("mkdir inside path: %v", err)
	}
	if err := os.WriteFile(inside, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write inside path: %v", err)
	}

	if !IsPathWithinAssetsDir(inside) {
		t.Fatalf("expected path inside DATA to be allowed")
	}
	if IsPathWithinAssetsDir(filepath.Join(t.TempDir(), "episode.mp3")) {
		t.Fatalf("expected path outside DATA to be rejected")
	}
}

func TestDeleteAssetFileRequiresRegularFileWithinAssetsDir(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("DATA", dataDir)

	inside := filepath.Join(dataDir, "audio", "episode.mp3")
	if err := os.MkdirAll(filepath.Dir(inside), 0o755); err != nil {
		t.Fatalf("mkdir inside path: %v", err)
	}
	if err := os.WriteFile(inside, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write inside path: %v", err)
	}
	if err := DeleteAssetFile(inside); err != nil {
		t.Fatalf("expected inside asset file delete to succeed, got %v", err)
	}
	if _, err := os.Stat(inside); !os.IsNotExist(err) {
		t.Fatalf("expected inside asset file to be deleted, stat err=%v", err)
	}

	outside := filepath.Join(t.TempDir(), "outside.mp3")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatalf("write outside path: %v", err)
	}
	if err := DeleteAssetFile(outside); !errors.Is(err, ErrPathOutsideAssetsDir) {
		t.Fatalf("expected outside delete to return ErrPathOutsideAssetsDir, got %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("expected outside file to remain, got %v", err)
	}

	if err := DeleteAssetFile(dataDir); !errors.Is(err, ErrPathNotRegularAssetFile) {
		t.Fatalf("expected directory delete to return ErrPathNotRegularAssetFile, got %v", err)
	}
	if _, err := os.Stat(dataDir); err != nil {
		t.Fatalf("expected asset directory to remain, got %v", err)
	}

	missing := filepath.Join(dataDir, "missing.mp3")
	if err := DeleteAssetFile(missing); !os.IsNotExist(err) {
		t.Fatalf("expected missing asset file to return not-exist error, got %v", err)
	}
}
