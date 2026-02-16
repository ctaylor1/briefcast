package db

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func podcastsWithAssociations(query *gorm.DB) *gorm.DB {
	return query.Preload(clause.Associations)
}

func podcastsWithTags(query *gorm.DB) *gorm.DB {
	return query.Preload("Tags")
}

func podcastItemsWithAssociations(query *gorm.DB) *gorm.DB {
	return query.Preload(clause.Associations)
}

func podcastItemsWithPodcast(query *gorm.DB) *gorm.DB {
	return query.Preload("Podcast")
}

func tagsWithAssociations(query *gorm.DB) *gorm.DB {
	return query.Preload(clause.Associations)
}

func tagsWithPodcasts(query *gorm.DB) *gorm.DB {
	return query.Preload("Podcasts")
}

func applyDownloadStatusFilter(query *gorm.DB, downloadedOnly *bool) *gorm.DB {
	if downloadedOnly == nil {
		return query
	}
	if *downloadedOnly {
		return query.Where("download_status=?", Downloaded)
	}
	return query.Where("download_status!=?", Downloaded)
}

func applyPlayedStatusFilter(query *gorm.DB, playedOnly *bool) *gorm.DB {
	if playedOnly == nil {
		return query
	}
	if *playedOnly {
		return query.Where("is_played=?", true)
	}
	return query.Where("is_played=?", false)
}
