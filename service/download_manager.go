package service

import (
	"sync"
	"sync/atomic"
)

var (
	downloadCancelMu   sync.RWMutex
	downloadCancelKeys = make(map[string]struct{})
	downloadPauseMu    sync.RWMutex
	downloadPauseKeys  = make(map[string]struct{})
	downloadsPaused    atomic.Bool
)

// PauseDownloads handles the corresponding operation.
func PauseDownloads() {
	downloadsPaused.Store(true)
}

// ResumeDownloads handles the corresponding operation.
func ResumeDownloads() {
	downloadsPaused.Store(false)
}

// DownloadsPaused handles the corresponding operation.
func DownloadsPaused() bool {
	return downloadsPaused.Load()
}

// CancelDownload handles the corresponding operation.
func CancelDownload(id string) {
	if id == "" {
		return
	}
	downloadCancelMu.Lock()
	downloadCancelKeys[id] = struct{}{}
	downloadCancelMu.Unlock()
}

// ClearDownloadCancellation handles the corresponding operation.
func ClearDownloadCancellation(id string) {
	if id == "" {
		return
	}
	downloadCancelMu.Lock()
	delete(downloadCancelKeys, id)
	downloadCancelMu.Unlock()
}

// IsDownloadCancelled handles the corresponding operation.
func IsDownloadCancelled(id string) bool {
	if id == "" {
		return false
	}
	downloadCancelMu.RLock()
	_, exists := downloadCancelKeys[id]
	downloadCancelMu.RUnlock()
	return exists
}

// PauseDownload handles the corresponding operation.
func PauseDownload(id string) {
	if id == "" {
		return
	}
	downloadPauseMu.Lock()
	downloadPauseKeys[id] = struct{}{}
	downloadPauseMu.Unlock()
}

// ClearDownloadPause handles the corresponding operation.
func ClearDownloadPause(id string) {
	if id == "" {
		return
	}
	downloadPauseMu.Lock()
	delete(downloadPauseKeys, id)
	downloadPauseMu.Unlock()
}

// IsDownloadPaused handles the corresponding operation.
func IsDownloadPaused(id string) bool {
	if id == "" {
		return false
	}
	downloadPauseMu.RLock()
	_, exists := downloadPauseKeys[id]
	downloadPauseMu.RUnlock()
	return exists
}
