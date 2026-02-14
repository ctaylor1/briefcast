package service

import (
	"sync"
	"sync/atomic"
)

var (
	downloadCancelMu   sync.RWMutex
	downloadCancelKeys = make(map[string]struct{})
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
