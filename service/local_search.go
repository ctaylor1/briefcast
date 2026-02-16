package service

import (
	"strings"

	"github.com/ctaylor1/briefcast/db"
)

type LocalSearchResult struct {
	Type              string   `json:"type"`
	PodcastID         string   `json:"podcastId,omitempty"`
	PodcastTitle      string   `json:"podcastTitle,omitempty"`
	EpisodeID         string   `json:"episodeId,omitempty"`
	EpisodeTitle      string   `json:"episodeTitle,omitempty"`
	ChapterTitle      string   `json:"chapterTitle,omitempty"`
	TranscriptSnippet string   `json:"transcriptSnippet,omitempty"`
	SummarySnippet    string   `json:"summarySnippet,omitempty"`
	StartSeconds      *float64 `json:"startSeconds,omitempty"`
}

func SearchLocalRecords(query string, limit int) ([]LocalSearchResult, error) {
	term := strings.TrimSpace(query)
	if term == "" {
		return []LocalSearchResult{}, nil
	}
	if limit <= 0 {
		limit = 50
	}

	lowerTerm := strings.ToLower(term)
	like := "%" + lowerTerm + "%"
	results := make([]LocalSearchResult, 0, limit)
	add := func(result LocalSearchResult) bool {
		if len(results) >= limit {
			return true
		}
		results = append(results, result)
		return len(results) >= limit
	}

	var podcasts []db.Podcast
	if err := db.SearchPodcastsByLike(like, limit, &podcasts); err != nil {
		return results, err
	}
	for _, podcast := range podcasts {
		snippet := pickSnippet(podcast.Summary, podcast.SummaryHTML, lowerTerm)
		if add(LocalSearchResult{
			Type:           "podcast",
			PodcastID:      podcast.ID,
			PodcastTitle:   podcast.Title,
			SummarySnippet: snippet,
		}) {
			return results, nil
		}
	}

	var items []db.PodcastItem
	if err := db.SearchPodcastItemsByLike(like, limit*2, &items); err != nil {
		return results, err
	}

	for _, item := range items {
		if len(results) >= limit {
			break
		}

		if containsTerm(item.Title, lowerTerm) || containsTerm(item.Summary, lowerTerm) || containsTerm(item.SummaryHTML, lowerTerm) {
			snippet := pickSnippet(item.Summary, item.SummaryHTML, lowerTerm)
			if add(LocalSearchResult{
				Type:           "episode",
				PodcastID:      item.PodcastID,
				PodcastTitle:   item.Podcast.Title,
				EpisodeID:      item.ID,
				EpisodeTitle:   item.Title,
				SummarySnippet: snippet,
			}) {
				break
			}
		}

		if len(results) >= limit {
			break
		}

		chapterMatches := 0
		if containsTerm(item.ChaptersJSON, lowerTerm) || containsTerm(item.ID3ChaptersJSON, lowerTerm) {
			raw := strings.TrimSpace(item.ChaptersJSON)
			if raw == "" {
				raw = strings.TrimSpace(item.ID3ChaptersJSON)
			}
			if raw != "" {
				for _, chapter := range parseChapters(raw) {
					if !containsTerm(chapter.Title, lowerTerm) {
						continue
					}
					start := chapter.StartSeconds
					if add(LocalSearchResult{
						Type:         "chapter",
						PodcastID:    item.PodcastID,
						PodcastTitle: item.Podcast.Title,
						EpisodeID:    item.ID,
						EpisodeTitle: item.Title,
						ChapterTitle: chapter.Title,
						StartSeconds: &start,
					}) {
						return results, nil
					}
					chapterMatches++
					if chapterMatches >= 3 {
						break
					}
				}
			}
		}

		if len(results) >= limit {
			break
		}

		if item.TranscriptJSON != "" && containsTerm(item.TranscriptJSON, lowerTerm) {
			transcriptMatches := 0
			for _, match := range searchTranscriptMatches(item.TranscriptJSON, lowerTerm, 3) {
				match.PodcastID = item.PodcastID
				match.PodcastTitle = item.Podcast.Title
				match.EpisodeID = item.ID
				match.EpisodeTitle = item.Title
				match.Type = "transcript"
				if add(match) {
					return results, nil
				}
				transcriptMatches++
				if transcriptMatches >= 3 {
					break
				}
			}
		}
	}

	return results, nil
}
