package service

import "github.com/ctaylor1/briefcast/db"

func CancelEpisodeDownload(id string) error {
	var item db.PodcastItem
	if err := db.GetPodcastItemById(id, &item); err != nil {
		return err
	}

	switch item.DownloadStatus {
	case db.NotDownloaded:
		return SetPodcastItemAsNotDownloaded(item.ID, db.Deleted)
	case db.Downloading:
		CancelDownload(item.ID)
		return nil
	default:
		return nil
	}
}

func CancelAllDownloads() error {
	queued, err := db.GetPodcastItemsByDownloadStatuses([]db.DownloadStatus{db.NotDownloaded}, 0)
	if err != nil {
		return err
	}
	for _, item := range queued {
		_ = SetPodcastItemAsNotDownloaded(item.ID, db.Deleted)
	}

	downloading, err := db.GetPodcastItemsByDownloadStatuses([]db.DownloadStatus{db.Downloading}, 0)
	if err != nil {
		return err
	}
	for _, item := range downloading {
		CancelDownload(item.ID)
	}
	return nil
}
