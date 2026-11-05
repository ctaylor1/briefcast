package service

import (
	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/feedmeta"
)

// maxShowNoteLinkURLBytes caps stored URLs. Longer URLs are almost always
// tracking-laden or malformed, and Postgres btree index entries (used by the
// unique occurrence index) have a size limit.
const maxShowNoteLinkURLBytes = 2048

// persistShowNoteLinks extracts, normalizes, and stores the show note links
// for one episode. It is safe to call repeatedly for the same episode: writes
// are upserts keyed by (episode, URL). It returns the number of links saved.
func persistShowNoteLinks(podcastID, podcastItemID, showNotesHTML, showNotesText string) (int, error) {
	writes, skippedLong := buildShowNoteLinkWrites(podcastID, podcastItemID, feedmeta.ExtractShowNoteLinks(showNotesHTML, showNotesText))
	if skippedLong > 0 {
		Logger.Warnw("skipped overlong show note links", "podcast_item_id", podcastItemID, "count", skippedLong, "max_bytes", maxShowNoteLinkURLBytes)
	}
	if err := db.SaveShowNoteLinks(writes); err != nil {
		return 0, err
	}
	return len(writes), nil
}

func buildShowNoteLinkWrites(podcastID, podcastItemID string, extracted []feedmeta.ShowNoteLinkData) ([]db.ShowNoteLinkWrite, int) {
	writes := make([]db.ShowNoteLinkWrite, 0, len(extracted))
	skippedLong := 0
	for _, data := range extracted {
		if len(data.URL) > maxShowNoteLinkURLBytes {
			skippedLong++
			continue
		}
		write := db.ShowNoteLinkWrite{Link: db.ShowNoteLink{
			PodcastItemID:     podcastItemID,
			PodcastID:         podcastID,
			URL:               data.URL,
			Title:             data.Title,
			Domain:            data.Domain,
			Position:          data.Position,
			ExtractionVersion: feedmeta.LinkExtractionVersion,
		}}
		if normalized, err := feedmeta.NormalizeLink(data.URL); err == nil {
			write.Target = linkTargetFromNormalized(normalized)
			if parent, ok := feedmeta.ParentProfile(normalized); ok {
				write.Parent = linkTargetFromNormalized(parent)
			}
		}
		writes = append(writes, write)
	}
	return writes, skippedLong
}

func linkTargetFromNormalized(link feedmeta.NormalizedLink) *db.LinkTarget {
	return &db.LinkTarget{
		NormalizedKey:     link.Key,
		CanonicalURL:      link.CanonicalURL,
		Kind:              string(link.Kind),
		Platform:          link.Platform,
		Handle:            link.Handle,
		Domain:            link.Domain,
		ExtractionVersion: feedmeta.LinkExtractionVersion,
	}
}
