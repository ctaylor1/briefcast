package service

import (
	"strings"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/id3meta"
)

type Chapter struct {
	Title        string  `json:"title"`
	StartSeconds float64 `json:"startSeconds"`
	EndSeconds   float64 `json:"endSeconds,omitempty"`
}

type ChapterResponse struct {
	Source   string    `json:"source"`
	Chapters []Chapter `json:"chapters"`
}

func BuildChapterResponse(item db.PodcastItem) ChapterResponse {
	raw := strings.TrimSpace(item.ChaptersJSON)
	source := strings.TrimSpace(item.ChaptersType)
	if raw == "" && strings.TrimSpace(item.ID3ChaptersJSON) != "" {
		raw = item.ID3ChaptersJSON
		if source == "" {
			source = "id3"
		}
	}
	if raw == "" {
		return ChapterResponse{Source: "none", Chapters: []Chapter{}}
	}
	chapters := parseChapters(raw)
	if source == "" {
		source = "feed"
	}
	return ChapterResponse{Source: source, Chapters: chapters}
}

func RefreshChaptersFromID3(item *db.PodcastItem) bool {
	if item == nil {
		return false
	}
	if item.DownloadPath == "" || !FileExists(item.DownloadPath) {
		return false
	}

	existingRaw := strings.TrimSpace(item.ChaptersJSON)
	if existingRaw == "" {
		existingRaw = strings.TrimSpace(item.ID3ChaptersJSON)
	}

	if existingRaw != "" {
		existingChapters := parseChapters(existingRaw)
		if hasMeaningfulTitles(existingChapters) {
			return false
		}
	}

	raw, err := ExtractID3Metadata(item.DownloadPath)
	if err != nil {
		Logger.Warnw("id3 metadata refresh failed", "podcast_item_id", item.ID, "error", err)
		return false
	}

	tagsJSON, chaptersJSON, hasTags, hasChapters, err := id3meta.SplitRaw(raw)
	if err != nil {
		Logger.Warnw("id3 metadata parse failed", "podcast_item_id", item.ID, "error", err)
		return false
	}

	updated := false
	if hasTags && tagsJSON != "" && tagsJSON != item.ID3TagsJSON {
		item.ID3TagsJSON = tagsJSON
		updated = true
	}
	if hasChapters && chaptersJSON != "" {
		if chaptersJSON != item.ID3ChaptersJSON {
			item.ID3ChaptersJSON = chaptersJSON
			updated = true
		}
		if item.ChaptersJSON == "" || item.ChaptersType == "" || item.ChaptersType == "id3" {
			item.ChaptersJSON = chaptersJSON
			item.ChaptersType = "id3"
			updated = true
		}
	}

	if updated {
		if err := db.UpdatePodcastItem(item); err != nil {
			Logger.Warnw("id3 metadata update failed", "podcast_item_id", item.ID, "error", err)
		}
	}
	return updated
}

func hasMeaningfulTitles(chapters []Chapter) bool {
	for _, chapter := range chapters {
		title := strings.TrimSpace(chapter.Title)
		if title == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(title), "chapter ") {
			continue
		}
		return true
	}
	return false
}
