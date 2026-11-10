package db

import "gorm.io/gorm"

// FindAllPodcastsPlain handles the corresponding operation.
func FindAllPodcastsPlain(podcasts *[]Podcast) error {
	return DB.Find(podcasts).Error
}

// FindDownloadedPodcastItems lists downloaded episodes with the columns the
// retention job reads; transcripts and summaries are not loaded.
func FindDownloadedPodcastItems(items *[]PodcastItem) error {
	return DB.Select("id", "podcast_id", "is_played", "pub_date", "download_date", "created_at").
		Where("download_status = ?", Downloaded).Find(items).Error
}

// SearchPodcastsByLike handles the corresponding operation.
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

// SearchPodcastItemsByLike handles the corresponding operation.
func SearchPodcastItemsByLike(like string, limit int, items *[]PodcastItem) error {
	// Only the columns the search reads; it still scans transcripts to match.
	query := DB.Preload("Podcast", func(tx *gorm.DB) *gorm.DB { return tx.Select("id", "title") }).
		Select("id", "podcast_id", "title", "summary", "summary_html", "chapters_json", "id3_chapters_json", "transcript_json", "llm_summary").
		Where(
			"lower(title) like ? OR lower(summary) like ? OR lower(summary_html) like ? OR lower(chapters_json) like ? OR lower(id3_chapters_json) like ? OR lower(transcript_json) like ? OR lower(llm_summary) like ?",
			like, like, like, like, like, like, like,
		)
	if limit > 0 {
		query = query.Limit(limit)
	}
	return query.Find(items).Error
}
