package service

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/logging"
)

var exportAllRunning atomic.Bool

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

func GetExportAllRunning() bool {
	return exportAllRunning.Load()
}

func ExportAll() (transcripts int, summaries int, err error) {
	exportDir := resolveExportDir()
	if exportDir == "" {
		return 0, 0, fmt.Errorf("EXPORT_DIR is not configured")
	}
	if !exportAllRunning.CompareAndSwap(false, true) {
		return 0, 0, fmt.Errorf("export is already running")
	}
	defer exportAllRunning.Store(false)

	logger := logging.Sugar()
	logger.Infow("export_all started", "export_dir", exportDir)

	var podcasts []db.Podcast
	if result := db.DB.Find(&podcasts); result.Error != nil {
		return 0, 0, result.Error
	}

	podcastNames := make(map[string]string, len(podcasts))
	for _, p := range podcasts {
		podcastNames[p.ID] = p.Title
	}

	var items []db.PodcastItem
	result := db.DB.
		Where("(canonical_transcript IS NOT NULL AND canonical_transcript <> '') OR (llm_summary IS NOT NULL AND llm_summary <> '')").
		Find(&items)
	if result.Error != nil {
		return 0, 0, result.Error
	}

	for i := range items {
		item := &items[i]
		name := podcastNames[item.PodcastID]
		if name == "" {
			name = "Unknown Podcast"
		}
		podcast := sanitizeName(name)
		episode := sanitizeName(item.Title)

		if ct := strings.TrimSpace(item.CanonicalTranscript); ct != "" {
			p := filepath.Join(exportDir, podcast, "transcripts", episode+".txt")
			if writeErr := writeExportFile(p, ct); writeErr != nil {
				logger.Warnw("export_all transcript failed", "podcast_item_id", item.ID, "error", writeErr)
			} else {
				transcripts++
			}
		}
		if s := strings.TrimSpace(item.LLMSummary); s != "" {
			p := filepath.Join(exportDir, podcast, "summaries", episode+".txt")
			if writeErr := writeExportFile(p, s); writeErr != nil {
				logger.Warnw("export_all summary failed", "podcast_item_id", item.ID, "error", writeErr)
			} else {
				summaries++
			}
		}
	}

	logger.Infow("export_all finished", "transcripts", transcripts, "summaries", summaries)
	return transcripts, summaries, nil
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
