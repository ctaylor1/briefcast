package service

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/logging"
)

var unsafeFileNameChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

func resolveExportDir() string {
	return strings.TrimSpace(os.Getenv("EXPORT_DIR"))
}

func sanitizeName(name string) string {
	safe := unsafeFileNameChars.ReplaceAllString(name, "")
	safe = strings.TrimSpace(safe)
	safe = strings.ReplaceAll(safe, "  ", " ")
	if safe == "" {
		safe = "_"
	}
	return safe
}

func lookupPodcastTitle(podcastID string) string {
	var podcast db.Podcast
	if err := db.GetPodcastByID(podcastID, &podcast); err != nil {
		return "Unknown Podcast"
	}
	return podcast.Title
}

func podcastTitle(item *db.PodcastItem) string {
	if item.Podcast.Title != "" {
		return item.Podcast.Title
	}
	return lookupPodcastTitle(item.PodcastID)
}

func writeExportFile(dir, content string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dir, []byte(content), 0o644)
}

func ExportTranscript(item *db.PodcastItem) {
	exportDir := resolveExportDir()
	if exportDir == "" {
		return
	}
	transcript := strings.TrimSpace(item.CanonicalTranscript)
	if transcript == "" {
		return
	}

	podcast := sanitizeName(podcastTitle(item))
	episode := sanitizeName(item.Title)
	path := filepath.Join(exportDir, podcast, "transcripts", episode+".txt")

	if err := writeExportFile(path, transcript); err != nil {
		logger := logging.Sugar()
		logger.Warnw("failed to export transcript", "podcast_item_id", item.ID, "path", path, "error", err)
	}
}

func exportCanonicalBackfillBatch(updates []canonicalBackfillUpdate) {
	exportDir := resolveExportDir()
	if exportDir == "" {
		return
	}
	logger := logging.Sugar()
	for _, update := range updates {
		if strings.TrimSpace(update.CanonicalTranscript) == "" {
			continue
		}
		var item db.PodcastItem
		if err := db.GetPodcastItemByID(update.ID, &item); err != nil {
			logger.Warnw("failed to load item for transcript export", "podcast_item_id", update.ID, "error", err)
			continue
		}
		podcast := sanitizeName(podcastTitle(&item))
		episode := sanitizeName(item.Title)
		path := filepath.Join(exportDir, podcast, "transcripts", episode+".txt")
		if err := writeExportFile(path, update.CanonicalTranscript); err != nil {
			logger.Warnw("failed to export backfill transcript", "podcast_item_id", update.ID, "path", path, "error", err)
		}
	}
}

func ExportSummary(item *db.PodcastItem) {
	exportDir := resolveExportDir()
	if exportDir == "" {
		return
	}
	summary := strings.TrimSpace(item.LLMSummary)
	if summary == "" {
		return
	}

	podcast := sanitizeName(podcastTitle(item))
	episode := sanitizeName(item.Title)
	path := filepath.Join(exportDir, podcast, "summaries", episode+".txt")

	if err := writeExportFile(path, summary); err != nil {
		logger := logging.Sugar()
		logger.Warnw("failed to export summary", "podcast_item_id", item.ID, "path", path, "error", err)
	}
}
