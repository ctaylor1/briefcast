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

// BackfillShowNoteLinks extracts and persists show note links for episodes
// that have show notes HTML and either no links yet, or links written by an
// older feedmeta.LinkExtractionVersion.
// Episodes are processed most-recent first. Writes are idempotent, so
// re-running it is safe.
func BackfillShowNoteLinks() (succeeded int, skipped int, err error) {
	if !linkBackfillRunning.CompareAndSwap(false, true) {
		return 0, 0, nil
	}
	defer linkBackfillRunning.Store(false)

	logger := logging.Sugar()

	withLinks, err := db.PodcastItemIDsWithShowNoteLinks()
	if err != nil {
		return 0, 0, err
	}
	needsExtraction, err := db.PodcastItemIDsNeedingLinkExtraction(feedmeta.LinkExtractionVersion)
	if err != nil {
		return 0, 0, err
	}

	// Load only the columns needed; full episode rows include transcripts.
	var items []db.PodcastItem
	result := db.DB.
		Select("id", "podcast_id", "summary_html").
		Where("summary_html IS NOT NULL AND summary_html <> ''").
		Order("pub_date DESC").
		Find(&items)
	if result.Error != nil {
		return 0, 0, result.Error
	}

	logger.Infow("link backfill starting",
		"episodes", len(items),
		"needing_reextraction", len(needsExtraction),
		"extraction_version", feedmeta.LinkExtractionVersion,
	)

	for i := range items {
		if withLinks[items[i].ID] && !needsExtraction[items[i].ID] {
			skipped++
			continue
		}

		plainText := strip.StripTags(items[i].SummaryHTML)
		saved, saveErr := persistShowNoteLinks(items[i].PodcastID, items[i].ID, items[i].SummaryHTML, plainText)
		if saveErr != nil {
			logger.Warnw("link backfill failed for episode", "episode_id", items[i].ID, "error", saveErr)
			continue
		}
		if saved == 0 {
			skipped++
			continue
		}
		succeeded++
	}

	logger.Infow("link backfill finished", "succeeded", succeeded, "skipped", skipped)
	return succeeded, skipped, nil
}
