package model

import "math"

// Pagination represents a public type.
type Pagination struct {
	Page         int `uri:"page" query:"page" json:"page" form:"page" default:"1"`
	Count        int `uri:"count" query:"count" json:"count" form:"count" default:"20"`
	NextPage     int `uri:"nextPage" query:"nextPage" json:"nextPage" form:"nextPage"`
	PreviousPage int `uri:"previousPage" query:"previousPage" json:"previousPage" form:"previousPage"`
	TotalCount   int `uri:"totalCount" query:"totalCount" json:"totalCount" form:"totalCount"`
	TotalPages   int `uri:"totalPages" query:"totalPages" json:"totalPages" form:"totalPages"`
}

// EpisodeSort represents a public type.
type EpisodeSort string

const (
	// ReleaseAsc is a public constant.
	ReleaseAsc EpisodeSort = "ReleaseAsc"
	// ReleaseDesc is a public constant.
	ReleaseDesc EpisodeSort = "ReleaseDesc"
	// DurationAsc is a public constant.
	DurationAsc EpisodeSort = "DurationAsc"
	// DurationDesc is a public constant.
	DurationDesc EpisodeSort = "DurationDesc"
)

// EpisodesFilter represents a public type.
type EpisodesFilter struct {
	Pagination
	IsDownloaded *string     `uri:"isDownloaded" query:"isDownloaded" json:"isDownloaded" form:"isDownloaded"`
	IsPlayed     *string     `uri:"isPlayed" query:"isPlayed" json:"isPlayed" form:"isPlayed"`
	Sorting      EpisodeSort `uri:"sorting" query:"sorting" json:"sorting" form:"sorting"`
	Q            string      `uri:"q" query:"q" json:"q" form:"q"`
	TagIds       []string    `uri:"tagIds" query:"tagIds[]" json:"tagIds" form:"tagIds[]"`
	PodcastIds   []string    `uri:"podcastIds" query:"podcastIds[]" json:"podcastIds" form:"podcastIds[]"`
}

// VerifyPaginationValues handles the corresponding operation.
func (filter *EpisodesFilter) VerifyPaginationValues() {
	if filter.Count == 0 {
		filter.Count = 20
	}
	if filter.Page == 0 {
		filter.Page = 1
	}
	if filter.Sorting == "" {
		filter.Sorting = ReleaseDesc
	}
}

// SetCounts handles the corresponding operation.
func (filter *EpisodesFilter) SetCounts(totalCount int64) {
	totalPages := int(math.Ceil(float64(totalCount) / float64(filter.Count)))
	nextPage, previousPage := 0, 0
	if filter.Page < totalPages {
		nextPage = filter.Page + 1
	}
	if filter.Page > 1 {
		previousPage = filter.Page - 1
	}
	filter.NextPage = nextPage
	filter.PreviousPage = previousPage
	filter.TotalCount = int(totalCount)
	filter.TotalPages = totalPages
}
