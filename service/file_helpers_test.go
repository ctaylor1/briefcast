package service

import (
	"archive/tar"
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseContentRangeTotal(t *testing.T) {
	if got := parseContentRangeTotal("bytes 0-99/1234"); got != 1234 {
		t.Fatalf("expected 1234, got %d", got)
	}
	if got := parseContentRangeTotal("bytes 0-99/*"); got != 0 {
		t.Fatalf("expected 0 for unknown total, got %d", got)
	}
	if got := parseContentRangeTotal("bad"); got != 0 {
		t.Fatalf("expected 0 for invalid range, got %d", got)
	}
}

func TestResolveTotalBytes(t *testing.T) {
	resp := &http.Response{
		StatusCode:    http.StatusPartialContent,
		ContentLength: 100,
		Header:        make(http.Header),
	}
	resp.Header.Set("Content-Range", "bytes 0-99/500")
	if got := resolveTotalBytes(resp, 200); got != 500 {
		t.Fatalf("expected 500 from content-range, got %d", got)
	}

	resp.Header.Del("Content-Range")
	if got := resolveTotalBytes(resp, 200); got != 300 {
		t.Fatalf("expected resume+content-length=300, got %d", got)
	}

	resp.StatusCode = http.StatusOK
	if got := resolveTotalBytes(resp, 200); got != 100 {
		t.Fatalf("expected content-length=100, got %d", got)
	}

	if got := resolveTotalBytes(nil, 0); got != 0 {
		t.Fatalf("expected 0 for nil response, got %d", got)
	}
}

func TestFileHelpers(t *testing.T) {
	if name := getFileName("https://example.com/audio", "My Épisode", ".mp3"); !strings.HasSuffix(name, ".mp3") {
		t.Fatalf("expected default extension .mp3, got %q", name)
	}
	if name := getFileName("https://example.com/audio.m4a", "My Episode", ".mp3"); !strings.HasSuffix(name, ".m4a") {
		t.Fatalf("expected parsed extension .m4a, got %q", name)
	}
	if cleaned := cleanFileName("My.Show/Épisode_1"); cleaned != "My-Show-Episode-1" {
		t.Fatalf("unexpected clean file name %q", cleaned)
	}
	if name := getFileName("://bad-url", "Broken URL Episode", ".mp3"); !strings.HasSuffix(name, ".mp3") {
		t.Fatalf("expected fallback extension for invalid URL, got %q", name)
	}
}

func TestDownloadReturnsErrorForInvalidURL(t *testing.T) {
	if _, err := Download("", "://bad-url", "Episode", "Podcast", ""); err == nil {
		t.Fatalf("expected download to fail for invalid URL")
	}
}

func TestFileExistsDeleteAndGetSize(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "sample.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	if !FileExists(filePath) {
		t.Fatalf("expected file to exist")
	}

	size, err := GetFileSize(filePath)
	if err != nil {
		t.Fatalf("GetFileSize failed: %v", err)
	}
	if size != 5 {
		t.Fatalf("expected size 5, got %d", size)
	}

	if err := DeleteFile(filePath); err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}
	if FileExists(filePath) {
		t.Fatalf("expected file to be deleted")
	}
}

func TestAddFileToTarWriter(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "archive-me.txt")
	if err := os.WriteFile(filePath, []byte("archive"), 0o644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	var buffer bytes.Buffer
	tw := tar.NewWriter(&buffer)
	if err := addFileToTarWriter(filePath, tw); err != nil {
		t.Fatalf("addFileToTarWriter failed: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}
	if buffer.Len() == 0 {
		t.Fatalf("expected tar output")
	}
}
