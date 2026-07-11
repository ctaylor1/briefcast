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

// TestDownloadRetainsAndResumesInterruptedTransfer verifies read errors preserve resumable bytes.
func TestDownloadRetainsAndResumesInterruptedTransfer(t *testing.T) {
	setupRetentionTestDB(t)

	fullBody := []byte("0123456789")
	var requests atomic.Int32
	var sawExpectedRange atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber := requests.Add(1)
		if requestNumber == 1 {
			w.Header().Set("Content-Length", "10")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(fullBody[:5])
			return
		}

		if r.Header.Get("Range") == "bytes=5-" {
			sawExpectedRange.Store(true)
		}
		w.Header().Set("Content-Length", "5")
		w.Header().Set("Content-Range", "bytes 5-9/10")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(fullBody[5:])
	}))
	t.Cleanup(server.Close)

	const (
		podcastName = "Interrupted Transfer Podcast"
		episodeName = "Interrupted Transfer Episode"
	)
	link := server.URL + "/audio.mp3"
	targetPath := path.Join(createAudioFolderIfNotExists(podcastName), getFileName(link, episodeName, ".mp3"))

	if _, err := Download("", link, episodeName, podcastName, ""); err == nil {
		t.Fatal("expected interrupted transfer to return an error")
	}
	partial, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("expected interrupted transfer to retain partial file: %v", err)
	}
	if !bytes.Equal(partial, fullBody[:5]) {
		t.Fatalf("expected five retained bytes, got %q", string(partial))
	}

	downloadPath, err := Download("", link, episodeName, podcastName, "")
	if err != nil {
		t.Fatalf("expected resumed transfer to succeed: %v", err)
	}
	if downloadPath != targetPath {
		t.Fatalf("expected resumed path %q, got %q", targetPath, downloadPath)
	}
	if !sawExpectedRange.Load() {
		t.Fatal("expected resumed transfer to request bytes from the retained offset")
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("expected two requests, got %d", got)
	}
	completed, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatalf("read completed transfer: %v", err)
	}
	if !bytes.Equal(completed, fullBody) {
		t.Fatalf("expected completed body %q, got %q", string(fullBody), string(completed))
	}
}

// TestDownloadRetainsPartialOnIncompleteResponse verifies clean short responses preserve resumable bytes.
func TestDownloadRetainsPartialOnIncompleteResponse(t *testing.T) {
	setupRetentionTestDB(t)

	fullBody := []byte("abcdefghij")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber := requests.Add(1)
		if requestNumber == 1 {
			w.Header().Set("Content-Range", "bytes 0-4/10")
			w.WriteHeader(http.StatusPartialContent)
			w.(http.Flusher).Flush()
			_, _ = w.Write(fullBody[:5])
			return
		}

		if r.Header.Get("Range") != "bytes=5-" {
			t.Errorf("expected resume range bytes=5-, got %q", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Range", "bytes 5-9/10")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(fullBody[5:])
	}))
	t.Cleanup(server.Close)

	const (
		podcastName = "Incomplete Response Podcast"
		episodeName = "Incomplete Response Episode"
	)
	link := server.URL + "/audio.mp3"
	targetPath := path.Join(createAudioFolderIfNotExists(podcastName), getFileName(link, episodeName, ".mp3"))

	if _, err := Download("", link, episodeName, podcastName, ""); err == nil || !strings.Contains(err.Error(), "download incomplete") {
		t.Fatalf("expected incomplete download error, got %v", err)
	}
	partial, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("expected incomplete response to retain partial file: %v", err)
	}
	if !bytes.Equal(partial, fullBody[:5]) {
		t.Fatalf("expected five retained bytes, got %q", string(partial))
	}

	downloadPath, err := Download("", link, episodeName, podcastName, "")
	if err != nil {
		t.Fatalf("expected incomplete response to resume successfully: %v", err)
	}
	completed, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatalf("read resumed download: %v", err)
	}
	if !bytes.Equal(completed, fullBody) {
		t.Fatalf("expected completed body %q, got %q", string(fullBody), string(completed))
	}
}

// TestDownloadRemovesZeroByteFile preserves the existing empty-download cleanup behavior.
func TestDownloadRemovesZeroByteFile(t *testing.T) {
	setupRetentionTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	const (
		podcastName = "Empty Download Podcast"
		episodeName = "Empty Download Episode"
	)
	link := server.URL + "/audio.mp3"
	targetPath := path.Join(createAudioFolderIfNotExists(podcastName), getFileName(link, episodeName, ".mp3"))

	if _, err := Download("", link, episodeName, podcastName, ""); err == nil || !strings.Contains(err.Error(), "download produced empty file") {
		t.Fatalf("expected empty download error, got %v", err)
	}
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("expected zero-byte download file to be removed, stat error=%v", err)
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
