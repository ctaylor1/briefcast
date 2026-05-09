package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/logging"
)

var exportAllRunning atomic.Bool

type exportVariant struct {
	dir       string
	extension string
}

var (
	transcriptExportVariants = []exportVariant{
		{dir: assetTranscriptDir, extension: ".txt"},
		{dir: assetMarkdownTranscriptDir, extension: ".md"},
	}
	summaryExportVariants = []exportVariant{
		{dir: assetSummariesDir, extension: ".txt"},
		{dir: assetMarkdownSummariesDir, extension: ".md"},
	}
)

func resolveExportDir() string {
	return resolveAssetsDir()
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

func pubDateSubdir(pubDate time.Time) string {
	if pubDate.IsZero() {
		return ""
	}
	return filepath.Join(pubDate.Format("2006"), pubDate.Format("2006-01"))
}

func writeExportVariantFiles(exportDir string, variants []exportVariant, podcast string, dateSub string, episode string, content string) error {
	var errs []error
	for _, variant := range variants {
		path := filepath.Join(exportDir, variant.dir, podcast, dateSub, episode+variant.extension)
		if err := writeExportFile(path, content); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
		}
	}
	return errors.Join(errs...)
}

func exportEpisodeFileName(item *db.PodcastItem) string {
	parts := make([]string, 0, 3)
	if !item.PubDate.IsZero() {
		parts = append(parts, item.PubDate.Format("2006-01-02"))
	}
	if item.ID != "" && item.PodcastID != "" {
		if seq, err := db.GetEpisodeNumber(item.ID, item.PodcastID); err == nil && seq > 0 {
			parts = append(parts, strconv.Itoa(seq))
		}
	}
	parts = append(parts, item.Title)
	return sanitizeAssetName(strings.Join(parts, "-"))
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

	podcast := sanitizeAssetName(podcastTitle(item))
	episode := exportEpisodeFileName(item)

	if err := writeExportVariantFiles(exportDir, transcriptExportVariants, podcast, pubDateSubdir(item.PubDate), episode, transcript); err != nil {
		logger := logging.Sugar()
		logger.Warnw("failed to export transcript", "podcast_item_id", item.ID, "error", err)
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
		podcast := sanitizeAssetName(podcastTitle(&item))
		episode := exportEpisodeFileName(&item)
		if err := writeExportVariantFiles(exportDir, transcriptExportVariants, podcast, pubDateSubdir(item.PubDate), episode, update.CanonicalTranscript); err != nil {
			logger.Warnw("failed to export backfill transcript", "podcast_item_id", update.ID, "error", err)
		}
	}
}

func GetExportAllRunning() bool {
	return exportAllRunning.Load()
}

func ExportAll() (transcripts int, summaries int, err error) {
	exportDir := resolveExportDir()
	if exportDir == "" {
		return 0, 0, fmt.Errorf("DATA is not configured")
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
		Where("(canonical_transcript IS NOT NULL AND canonical_transcript <> '') OR (transcript_json IS NOT NULL AND transcript_json <> '') OR (llm_summary IS NOT NULL AND llm_summary <> '')").
		Find(&items)
	if result.Error != nil {
		return 0, 0, result.Error
	}

	canonicalBackfilled := 0
	for i := range items {
		item := &items[i]
		name := podcastNames[item.PodcastID]
		if name == "" {
			name = "Unknown Podcast"
		}
		podcast := sanitizeAssetName(name)
		episode := exportEpisodeFileName(item)

		ct := exportableCanonicalTranscript(item)
		dateSub := pubDateSubdir(item.PubDate)
		if ct != "" {
			if writeErr := writeExportVariantFiles(exportDir, transcriptExportVariants, podcast, dateSub, episode, ct); writeErr != nil {
				logger.Warnw("export_all transcript failed", "podcast_item_id", item.ID, "error", writeErr)
			} else {
				transcripts++
			}
			if strings.TrimSpace(item.CanonicalTranscript) != ct || item.CanonicalTranscriptVersion < canonicalTranscriptVersionCurrent {
				if err := persistCanonicalTranscript(item.ID, ct); err != nil {
					logger.Warnw("export_all canonical transcript backfill failed", "podcast_item_id", item.ID, "error", err)
				} else {
					canonicalBackfilled++
				}
			}
		}
		if s := strings.TrimSpace(item.LLMSummary); s != "" {
			if writeErr := writeExportVariantFiles(exportDir, summaryExportVariants, podcast, dateSub, episode, s); writeErr != nil {
				logger.Warnw("export_all summary failed", "podcast_item_id", item.ID, "error", writeErr)
			} else {
				summaries++
			}
		}
	}

	logger.Infow("export_all finished", "transcripts", transcripts, "summaries", summaries, "canonical_transcripts_backfilled", canonicalBackfilled)
	return transcripts, summaries, nil
}

func exportableCanonicalTranscript(item *db.PodcastItem) string {
	canonical := strings.TrimSpace(item.CanonicalTranscript)
	if canonical != "" && item.CanonicalTranscriptVersion >= canonicalTranscriptVersionCurrent {
		return canonical
	}
	generated := strings.TrimSpace(buildCanonicalTranscriptFromTranscriptJSON(item.TranscriptJSON))
	if generated != "" {
		return generated
	}
	return canonical
}

func persistCanonicalTranscript(itemID string, transcript string) error {
	now := time.Now().UTC()
	return db.DB.Model(&db.PodcastItem{}).
		Where("id = ?", itemID).
		Updates(map[string]interface{}{
			"canonical_transcript":         transcript,
			"canonical_transcript_version": canonicalTranscriptVersionCurrent,
			"canonical_updated_at":         now,
		}).Error
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

	podcast := sanitizeAssetName(podcastTitle(item))
	episode := exportEpisodeFileName(item)

	if err := writeExportVariantFiles(exportDir, summaryExportVariants, podcast, pubDateSubdir(item.PubDate), episode, summary); err != nil {
		logger := logging.Sugar()
		logger.Warnw("failed to export summary", "podcast_item_id", item.ID, "error", err)
	}
}
