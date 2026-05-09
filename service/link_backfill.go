package service

import (
	"sync/atomic"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/feedmeta"
	"github.com/ctaylor1/briefcast/internal/logging"
	strip "github.com/grokify/html-strip-tags-go"
)

var linkBackfillRunning atomic.Bool

// GetLinkBackfillRunning reports whether a link backfill is in progress.
func GetLinkBackfillRunning() bool {
	return linkBackfillRunning.Load()
}

// BackfillShowNoteLinks extracts and persists show note links for all episodes
// that have show notes HTML but no links yet. Processes most-recent first.
func BackfillShowNoteLinks() (succeeded int, skipped int, err error) {
	if !linkBackfillRunning.CompareAndSwap(false, true) {
		return 0, 0, nil
	}
	defer linkBackfillRunning.Store(false)

	logger := logging.Sugar()

	var items []db.PodcastItem
	result := db.DB.
		Where("summary_html IS NOT NULL AND summary_html <> ''").
		Order("pub_date DESC").
		Find(&items)
	if result.Error != nil {
		return 0, 0, result.Error
	}

	logger.Infow("link backfill starting", "episodes", len(items))

	for i := range items {
		existing, _ := db.GetShowNoteLinksByPodcastItemID(items[i].ID)
		if len(existing) > 0 {
			skipped++
			continue
		}

		plainText := strip.StripTags(items[i].SummaryHTML)
		linkData := feedmeta.ExtractShowNoteLinks(items[i].SummaryHTML, plainText)
		if len(linkData) == 0 {
			skipped++
			continue
		}

		links := make([]db.ShowNoteLink, len(linkData))
		for li, ld := range linkData {
			links[li] = db.ShowNoteLink{
				PodcastItemID: items[i].ID,
				PodcastID:     items[i].PodcastID,
				URL:           ld.URL,
				Title:         ld.Title,
				Domain:        ld.Domain,
				Position:      ld.Position,
			}
		}
		if createErr := db.CreateShowNoteLinks(links); createErr != nil {
			logger.Warnw("link backfill failed for episode", "episode_id", items[i].ID, "error", createErr)
			continue
		}
		succeeded++
	}

	logger.Infow("link backfill finished", "succeeded", succeeded, "skipped", skipped)
	return succeeded, skipped, nil
}
