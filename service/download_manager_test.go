package service

import "testing"

func resetDownloadManagerState() {
	downloadCancelMu.Lock()
	downloadCancelKeys = map[string]struct{}{}
	downloadCancelMu.Unlock()

	downloadPauseMu.Lock()
	downloadPauseKeys = map[string]struct{}{}
	downloadPauseMu.Unlock()

	downloadsPaused.Store(false)
}

func TestDownloadsPauseState(t *testing.T) {
	resetDownloadManagerState()

	if DownloadsPaused() {
		t.Fatalf("expected downloads to start as not paused")
	}

	PauseDownloads()
	if !DownloadsPaused() {
		t.Fatalf("expected downloads to be paused")
	}

	ResumeDownloads()
	if DownloadsPaused() {
		t.Fatalf("expected downloads to be resumed")
	}
}

func TestDownloadCancellationAndPausePerID(t *testing.T) {
	resetDownloadManagerState()

	if IsDownloadCancelled("") {
		t.Fatalf("empty id should never be cancelled")
	}
	if IsDownloadPaused("") {
		t.Fatalf("empty id should never be paused")
	}

	CancelDownload("ep-1")
	if !IsDownloadCancelled("ep-1") {
		t.Fatalf("expected ep-1 to be cancelled")
	}
	ClearDownloadCancellation("ep-1")
	if IsDownloadCancelled("ep-1") {
		t.Fatalf("expected ep-1 cancellation cleared")
	}

	PauseDownload("ep-2")
	if !IsDownloadPaused("ep-2") {
		t.Fatalf("expected ep-2 to be paused")
	}
	ClearDownloadPause("ep-2")
	if IsDownloadPaused("ep-2") {
		t.Fatalf("expected ep-2 pause cleared")
	}

	CancelDownload("")
	ClearDownloadCancellation("")
	PauseDownload("")
	ClearDownloadPause("")
}
