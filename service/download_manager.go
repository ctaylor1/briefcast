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

func PauseDownloads() {
	downloadsPaused.Store(true)
}

func ResumeDownloads() {
	downloadsPaused.Store(false)
}

func DownloadsPaused() bool {
	return downloadsPaused.Load()
}

func CancelDownload(id string) {
	if id == "" {
		return
	}
	downloadCancelMu.Lock()
	downloadCancelKeys[id] = struct{}{}
	downloadCancelMu.Unlock()
}

func ClearDownloadCancellation(id string) {
	if id == "" {
		return
	}
	downloadCancelMu.Lock()
	delete(downloadCancelKeys, id)
	downloadCancelMu.Unlock()
}

func IsDownloadCancelled(id string) bool {
	if id == "" {
		return false
	}
	downloadCancelMu.RLock()
	_, exists := downloadCancelKeys[id]
	downloadCancelMu.RUnlock()
	return exists
}

func PauseDownload(id string) {
	if id == "" {
		return
	}
	downloadPauseMu.Lock()
	downloadPauseKeys[id] = struct{}{}
	downloadPauseMu.Unlock()
}

func ClearDownloadPause(id string) {
	if id == "" {
		return
	}
	downloadPauseMu.Lock()
	delete(downloadPauseKeys, id)
	downloadPauseMu.Unlock()
}

func IsDownloadPaused(id string) bool {
	if id == "" {
		return false
	}
	downloadPauseMu.RLock()
	_, exists := downloadPauseKeys[id]
	downloadPauseMu.RUnlock()
	return exists
}
