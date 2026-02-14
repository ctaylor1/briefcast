package service

import "github.com/ctaylor1/briefcast/db"

func CancelEpisodeDownload(id string) error {
	var item db.PodcastItem
	if err := db.GetPodcastItemById(id, &item); err != nil {
		return err
	}

	switch item.DownloadStatus {
	case db.NotDownloaded:
		return SetPodcastItemAsPaused(item.ID)
	case db.Downloading:
		PauseDownload(item.ID)
		return nil
	case db.Paused:
		return nil
	default:
		return nil
	}
}

func CancelAllDownloads() error {
	return PauseAllDownloads()
}

func PauseAllDownloads() error {
	PauseDownloads()
	queued, err := db.GetPodcastItemsByDownloadStatuses([]db.DownloadStatus{db.NotDownloaded}, 0)
	if err != nil {
		return err
	}
	for _, item := range queued {
		_ = SetPodcastItemAsPaused(item.ID)
	}

	downloading, err := db.GetPodcastItemsByDownloadStatuses([]db.DownloadStatus{db.Downloading}, 0)
	if err != nil {
		return err
	}
	for _, item := range downloading {
		PauseDownload(item.ID)
	}
	return nil
}

func ResumeAllDownloads() error {
	ResumeDownloads()
	paused, err := db.GetPodcastItemsByDownloadStatuses([]db.DownloadStatus{db.Paused}, 0)
	if err != nil {
		return err
	}
	for _, item := range paused {
		ClearDownloadPause(item.ID)
		_ = SetPodcastItemAsQueuedPreserveProgress(item.ID)
	}
	return nil
}
