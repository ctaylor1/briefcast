package service

import (
	"errors"
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

// ErrDownloadInProgress reports that the episode is already being downloaded.
var ErrDownloadInProgress = errors.New("download already in progress")

var (
	inFlightMu       sync.Mutex
	inFlightEpisodes = make(map[string]struct{})
	inFlightPaths    = make(map[string]string)
)

// beginEpisodeDownload marks an episode as downloading in this process. It
// returns false when a download of the same episode is already running.
func beginEpisodeDownload(id string) (release func(), ok bool) {
	if id == "" {
		return func() {}, true
	}
	inFlightMu.Lock()
	defer inFlightMu.Unlock()
	if _, busy := inFlightEpisodes[id]; busy {
		return nil, false
	}
	inFlightEpisodes[id] = struct{}{}
	return func() {
		inFlightMu.Lock()
		delete(inFlightEpisodes, id)
		inFlightMu.Unlock()
	}, true
}

// claimDownloadPath reserves a file path for one download. It returns false
// when another download already holds the path.
func claimDownloadPath(filePath, owner string) (release func(), ok bool) {
	inFlightMu.Lock()
	defer inFlightMu.Unlock()
	if holder, held := inFlightPaths[filePath]; held && holder != owner {
		return nil, false
	}
	inFlightPaths[filePath] = owner
	return func() {
		inFlightMu.Lock()
		if inFlightPaths[filePath] == owner {
			delete(inFlightPaths, filePath)
		}
		inFlightMu.Unlock()
	}, true
}
