package service

import (
	"archive/tar"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// TestParseContentRangeTotal handles the corresponding operation.
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

// TestResolveTotalBytes handles the corresponding operation.
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

// TestFileHelpers handles the corresponding operation.
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

// TestDownloadReturnsErrorForInvalidURL handles the corresponding operation.
func TestDownloadReturnsErrorForInvalidURL(t *testing.T) {
	if _, err := Download("", "://bad-url", "Episode", "Podcast", ""); err == nil {
		t.Fatalf("expected download to fail for invalid URL")
	}
}

// TestDownloadRetriesFromScratchAfterRange416 handles the corresponding operation.
func TestDownloadRetriesFromScratchAfterRange416(t *testing.T) {
	setupRetentionTestDB(t)

	var requests int32
	var sawRangeRequest int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		if strings.TrimSpace(r.Header.Get("Range")) != "" {
			atomic.StoreInt32(&sawRangeRequest, 1)
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		_, _ = w.Write([]byte("fresh-audio"))
	}))
	defer server.Close()

	const (
		podcastName = "Range Retry Podcast"
		episodeName = "Range Retry Episode"
	)
	link := server.URL + "/audio.mp3"
	folder := createAudioFolderIfNotExists(podcastName)
	targetPath := path.Join(folder, getFileName(link, episodeName, ".mp3"))
	if err := os.WriteFile(targetPath, []byte("stale-partial"), 0o644); err != nil {
		t.Fatalf("failed to seed partial file: %v", err)
	}

	downloadPath, err := Download("", link, episodeName, podcastName, "")
	if err != nil {
		t.Fatalf("expected retrying download to succeed, got error: %v", err)
	}
	if downloadPath != targetPath {
		t.Fatalf("expected download path %q, got %q", targetPath, downloadPath)
	}
	if atomic.LoadInt32(&sawRangeRequest) != 1 {
		t.Fatalf("expected initial ranged request to be attempted")
	}
	if atomic.LoadInt32(&requests) != 2 {
		t.Fatalf("expected exactly two requests (range + full), got %d", atomic.LoadInt32(&requests))
	}
	data, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(data) != "fresh-audio" {
		t.Fatalf("expected full retry content, got %q", string(data))
	}
}

// TestFileExistsDeleteAndGetSize handles the corresponding operation.
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

// TestAddFileToTarWriter handles the corresponding operation.
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
