package db

func FindAllPodcastsPlain(podcasts *[]Podcast) error {
	return DB.Find(podcasts).Error
}

func FindDownloadedPodcastItems(items *[]PodcastItem) error {
	return DB.Where("download_status = ?", Downloaded).Find(items).Error
}

func SearchPodcastsByLike(like string, limit int, podcasts *[]Podcast) error {
	query := DB.Where(
		"lower(title) like ? OR lower(summary) like ? OR lower(summary_html) like ?",
		like, like, like,
	)
	if limit > 0 {
		query = query.Limit(limit)
	}
	return query.Find(podcasts).Error
}

func SearchPodcastItemsByLike(like string, limit int, items *[]PodcastItem) error {
	query := podcastItemsWithPodcast(DB).
		Where(
			"lower(title) like ? OR lower(summary) like ? OR lower(summary_html) like ? OR lower(chapters_json) like ? OR lower(id3_chapters_json) like ? OR lower(transcript_json) like ?",
			like, like, like, like, like, like,
		)
	if limit > 0 {
		query = query.Limit(limit)
	}
	return query.Find(items).Error
}
