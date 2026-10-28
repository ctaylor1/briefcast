package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/logging"
	"gorm.io/gorm"
)

var exportAllRunning atomic.Bool

const exportAllBatchSize = 25

type exportVariant struct {
	dir       string
	extension string
}

var (
	transcriptExportVariants = []exportVariant{
		{dir: assetTranscriptDir, extension: ".txt"},
	}
	summaryExportVariants = []exportVariant{
		{dir: assetSummariesDir, extension: ".txt"},
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
		path := filepath.Join(exportDir, variant.dir, podcast, dateSub, exportFileComponent(episode, variant.extension))
		if err := writeExportFile(path, content); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
		}
	}
	return errors.Join(errs...)
}

// Reserve room for the extension/revision suffix without exceeding the common
// 255-byte filename limit. The full original name contributes a stable digest,
// preventing different long titles with the same prefix from colliding.
func exportFileComponent(base, suffix string) string {
	const maxBytes = 255
	if len(base)+len(suffix) <= maxBytes {
		return base + suffix
	}
	digest := sha256.Sum256([]byte(base))
	tail := "." + hex.EncodeToString(digest[:])[:12] + suffix
	end := maxBytes - len(tail)
	for end > 0 && !utf8.RuneStart(base[end]) {
		end--
	}
	return strings.TrimRight(base[:end], " .") + tail
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
	if err := writeEpisodeMarkdown(item, "transcript"); err != nil {
		logging.Sugar().Warnw("failed to export transcript Markdown", "podcast_item_id", item.ID, "error", err)
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
		if err := writeEpisodeMarkdown(&item, "transcript"); err != nil {
			logger.Warnw("failed to export backfill transcript Markdown", "podcast_item_id", item.ID, "error", err)
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
	canonicalBackfilled := 0
	var exportErrors []error
	queryErr := db.DB.
		Where("(canonical_transcript IS NOT NULL AND canonical_transcript <> '') OR (transcript_json IS NOT NULL AND transcript_json <> '') OR (llm_summary IS NOT NULL AND llm_summary <> '')").
		FindInBatches(&items, exportAllBatchSize, func(_ *gorm.DB, batch int) error {
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
					writeErr := errors.Join(writeExportVariantFiles(exportDir, transcriptExportVariants, podcast, dateSub, episode, ct), writeEpisodeMarkdown(item, "transcript"))
					if writeErr != nil {
						logger.Warnw("export_all transcript failed", "podcast_item_id", item.ID, "error", writeErr)
						exportErrors = append(exportErrors, fmt.Errorf("transcript %s: %w", item.ID, writeErr))
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
				if s := buildExportSummaryContent(item); s != "" {
					writeErr := errors.Join(writeExportVariantFiles(exportDir, summaryExportVariants, podcast, dateSub, episode, s), writeEpisodeMarkdown(item, "summary"))
					if writeErr != nil {
						logger.Warnw("export_all summary failed", "podcast_item_id", item.ID, "error", writeErr)
						exportErrors = append(exportErrors, fmt.Errorf("summary %s: %w", item.ID, writeErr))
					} else {
						summaries++
					}
				}
			}
			logger.Infow("export_all progress", "batches", batch, "transcripts", transcripts, "summaries", summaries)
			return nil
		}).Error

	logger.Infow("export_all finished", "transcripts", transcripts, "summaries", summaries, "canonical_transcripts_backfilled", canonicalBackfilled)
	return transcripts, summaries, errors.Join(append(exportErrors, queryErr)...)
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

func buildExportSummaryContent(item *db.PodcastItem) string {
	summary := strings.TrimSpace(item.LLMSummary)
	if summary == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString(summary)

	showNotes := strings.TrimSpace(item.Summary)
	if showNotes != "" {
		b.WriteString("\n\n---\n\n## Show Notes\n\n")
		b.WriteString(showNotes)
	}

	links, err := db.GetShowNoteLinksByPodcastItemID(item.ID)
	if err == nil && len(links) > 0 {
		b.WriteString("\n\n## Links\n\n")
		for _, link := range links {
			if link.Title != "" {
				fmt.Fprintf(&b, "- [%s](%s) — %s\n", link.Title, link.URL, link.Domain)
			} else {
				b.WriteString("- ")
				b.WriteString(link.URL)
				b.WriteString("\n")
			}
		}
	}

	return b.String()
}

func ExportSummary(item *db.PodcastItem) {
	exportDir := resolveExportDir()
	if exportDir == "" {
		return
	}
	content := buildExportSummaryContent(item)
	if content == "" {
		return
	}

	podcast := sanitizeAssetName(podcastTitle(item))
	episode := exportEpisodeFileName(item)

	if err := writeExportVariantFiles(exportDir, summaryExportVariants, podcast, pubDateSubdir(item.PubDate), episode, content); err != nil {
		logger := logging.Sugar()
		logger.Warnw("failed to export summary", "podcast_item_id", item.ID, "error", err)
	}
	if err := writeEpisodeMarkdown(item, "summary"); err != nil {
		logging.Sugar().Warnw("failed to export summary Markdown", "podcast_item_id", item.ID, "error", err)
	}
}
