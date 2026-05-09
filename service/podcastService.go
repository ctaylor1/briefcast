package service

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TheHippo/podcastindex"
	"github.com/antchfx/xmlquery"
	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/feedmeta"
	"github.com/ctaylor1/briefcast/internal/id3meta"
	"github.com/ctaylor1/briefcast/internal/logging"
	"github.com/ctaylor1/briefcast/model"
	strip "github.com/grokify/html-strip-tags-go"
	"gorm.io/gorm"
)

// Logger is a public variable.
var Logger = logging.Sugar()

const (
	InitialDownloadModeCount  = "count"
	InitialDownloadModeMonths = "months"
	InitialDownloadModeAll    = "all"
)

// NormalizeInitialDownloadMode returns a supported initial back-catalog mode, or
// an empty string when the input is invalid.
func NormalizeInitialDownloadMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case InitialDownloadModeCount:
		return InitialDownloadModeCount
	case InitialDownloadModeMonths:
		return InitialDownloadModeMonths
	case InitialDownloadModeAll:
		return InitialDownloadModeAll
	default:
		return ""
	}
}

func NormalizeInitialDownloadModeWithDefault(mode string) string {
	normalized := NormalizeInitialDownloadMode(mode)
	if normalized == "" {
		return InitialDownloadModeCount
	}
	return normalized
}

// ParseOpml handles the corresponding operation.
func ParseOpml(content string) (model.OpmlModel, error) {
	var response model.OpmlModel
	err := xml.Unmarshal([]byte(content), &response)
	return response, err
}

// GetPodcastByID handles the corresponding operation.
func GetPodcastByID(id string) *db.Podcast {
	var podcast db.Podcast

	if err := db.GetPodcastByID(id, &podcast); err != nil {
		Logger.Warnw("failed to load podcast by id", "podcast_id", id, "error", err)
	}

	return &podcast
}

// GetPodcastItemByID handles the corresponding operation.
func GetPodcastItemByID(id string) *db.PodcastItem {
	var podcastItem db.PodcastItem

	if err := db.GetPodcastItemByID(id, &podcastItem); err != nil {
		Logger.Warnw("failed to load podcast item by id", "podcast_item_id", id, "error", err)
	}

	return &podcastItem
}

// GetAllPodcastItemsByIds handles the corresponding operation.
func GetAllPodcastItemsByIds(podcastItemIds []string) (*[]db.PodcastItem, error) {
	return db.GetAllPodcastItemsByIds(podcastItemIds)
}

// GetAllPodcastItemsByPodcastIds handles the corresponding operation.
func GetAllPodcastItemsByPodcastIds(podcastIds []string) *[]db.PodcastItem {
	var podcastItems []db.PodcastItem

	if err := db.GetAllPodcastItemsByPodcastIds(podcastIds, &podcastItems); err != nil {
		Logger.Warnw("failed to load podcast items by podcast ids", "podcast_ids_count", len(podcastIds), "error", err)
		return &[]db.PodcastItem{}
	}
	return &podcastItems
}

// GetTagsByIds handles the corresponding operation.
func GetTagsByIds(ids []string) *[]db.Tag {

	tags, _ := db.GetTagsByIds(ids)

	return tags
}

// GetAllPodcasts handles the corresponding operation.
func GetAllPodcasts(sorting string) *[]db.Podcast {
	var podcasts []db.Podcast
	if err := db.GetAllPodcasts(&podcasts, sorting); err != nil {
		Logger.Warnw("failed to load podcasts", "sorting", sorting, "error", err)
		return &[]db.Podcast{}
	}

	stats, err := db.GetPodcastEpisodeStats()
	if err != nil {
		Logger.Warnw("failed to load podcast episode stats", "error", err)
		return &podcasts
	}

	type Key struct {
		PodcastID      string
		DownloadStatus db.DownloadStatus
	}
	countMap := make(map[Key]int)
	sizeMap := make(map[Key]int64)
	for _, stat := range *stats {
		countMap[Key{stat.PodcastID, stat.DownloadStatus}] = stat.Count
		sizeMap[Key{stat.PodcastID, stat.DownloadStatus}] = stat.Size

	}
	var toReturn []db.Podcast
	for _, podcast := range podcasts {
		podcast.DownloadedEpisodesCount = countMap[Key{podcast.ID, db.Downloaded}]
		podcast.DownloadingEpisodesCount = countMap[Key{podcast.ID, db.NotDownloaded}]
		podcast.AllEpisodesCount = podcast.DownloadedEpisodesCount + podcast.DownloadingEpisodesCount + countMap[Key{podcast.ID, db.Deleted}]

		podcast.DownloadedEpisodesSize = sizeMap[Key{podcast.ID, db.Downloaded}]
		podcast.DownloadingEpisodesSize = sizeMap[Key{podcast.ID, db.NotDownloaded}]
		podcast.AllEpisodesSize = podcast.DownloadedEpisodesSize + podcast.DownloadingEpisodesSize + sizeMap[Key{podcast.ID, db.Deleted}]

		toReturn = append(toReturn, podcast)
	}
	return &toReturn
}

// AddOpml handles the corresponding operation.
func AddOpml(content string) error {
	opmlModel, err := ParseOpml(content)
	if err != nil {
		Logger.Warnw("failed to parse OPML payload", "error", err)
		return errors.New("Invalid file format")
	}
	var podcastURLs []string
	for _, outline := range opmlModel.Body.Outline {
		if outline.XMLURL != "" {
			podcastURLs = append(podcastURLs, outline.XMLURL)
		}

		for _, innerOutline := range outline.Outline {
			if innerOutline.XMLURL != "" {
				podcastURLs = append(podcastURLs, innerOutline.XMLURL)
			}
		}
	}

	setting := db.GetOrCreateSetting()
	workers := boundedWorkerCount(setting.MaxDownloadConcurrency, 4, len(podcastURLs))
	runWorkerPool(podcastURLs, workers, func(url string) {
		_, addErr := AddPodcast(url)
		if addErr == nil {
			return
		}
		if _, alreadyExists := addErr.(*model.PodcastAlreadyExistsError); alreadyExists {
			return
		}
		Logger.Warnw("Failed to add podcast from OPML", "url", url, "error", addErr)
	})

	go func() {
		if refreshErr := RefreshEpisodes(); refreshErr != nil {
			Logger.Warnw("failed to refresh episodes after OPML import", "error", refreshErr)
		}
	}()
	return nil

}

// ExportOPML handles the corresponding operation.
func ExportOPML(useBriefcastLink bool, baseURL string) ([]byte, error) {

	podcasts := GetAllPodcasts("")

	var outlines []model.OpmlOutline
	for _, podcast := range *podcasts {

		xmlURL := podcast.URL
		if useBriefcastLink {
			xmlURL = fmt.Sprintf("%s/podcasts/%s/rss", baseURL, podcast.ID)
		}

		toAdd := model.OpmlOutline{
			AttrText: podcast.Summary,
			Type:     "rss",
			XMLURL:   xmlURL,
			Title:    podcast.Title,
		}
		outlines = append(outlines, toAdd)
	}

	toExport := model.OpmlExportModel{
		Head: model.OpmlExportHead{
			Title:       "Briefcast Feed Export",
			DateCreated: time.Now().UTC(),
		},
		Body: model.OpmlBody{
			Outline: outlines,
		},
		Version: "2.0",
	}

	data, err := xml.MarshalIndent(toExport, "", "    ")
	if err != nil {
		return nil, err
	}
	data = []byte(xml.Header + string(data))
	return data, nil
}

func getItunesImageURL(body []byte) string {
	doc, err := xmlquery.Parse(strings.NewReader(string(body)))
	if err != nil {
		return ""
	}
	channel, err := xmlquery.Query(doc, "//channel")
	if err != nil {
		return ""
	}

	iimage := channel.SelectElement("itunes:image")
	if iimage == nil {
		return ""
	}
	for _, attr := range iimage.Attr {
		if attr.Name.Local == "href" {
			return attr.Value
		}

	}
	return ""

}

// AddPodcast handles the corresponding operation.
func AddPodcast(url string) (db.Podcast, error) {
	var podcast db.Podcast
	err := db.GetPodcastByURL(url, &podcast)
	setting := db.GetOrCreateSetting()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		parsed, body, err := FetchFeedWithFeedparser(url)
		if err != nil {
			Logger.Errorw("Error adding podcast", "url", url, "error", err)
			return db.Podcast{}, err
		}

		feed := parsed.Feed
		showNotesHTML := feedmeta.ExtractFeedShowNotesHTML(feed)
		showNotesText := strip.StripTags(showNotesHTML)

		podcast := db.Podcast{
			Title:        feedmeta.PickFirstNonEmpty(feedmeta.GetString(feed, "title"), feedmeta.GetString(feed, "itunes_title"), url),
			Summary:      showNotesText,
			SummaryHTML:  showNotesHTML,
			Author:       feedmeta.PickFirstNonEmpty(feedmeta.GetString(feed, "itunes_author"), feedmeta.GetString(feed, "author")),
			Image:        feedmeta.ExtractImageURL(feed),
			URL:          url,
			FeedMetadata: feedmeta.MarshalMetadata(feed),
		}

		if podcast.Image == "" {
			podcast.Image = getItunesImageURL(body)
		}

		err = db.CreatePodcast(&podcast)
		if err != nil {
			return db.Podcast{}, err
		}

		_, coverErr := DownloadPodcastCoverImage(podcast.Image, podcast.Title)
		if coverErr != nil {
			Logger.Warnw("Failed to download podcast cover", "podcast", podcast.Title, "error", coverErr)
		}

		if setting.GenerateNFOFile {
			if nfoErr := CreateNfoFile(&podcast); nfoErr != nil {
				Logger.Warnw("Failed to create podcast NFO", "podcast", podcast.Title, "error", nfoErr)
			}
		}
		return podcast, err
	}

	return podcast, &model.PodcastAlreadyExistsError{URL: url}

}

// AddPodcastItems handles the corresponding operation.
func AddPodcastItems(podcast *db.Podcast, newPodcast bool) error {
	parsed, _, err := FetchFeedWithFeedparser(podcast.URL)
	if err != nil {
		return err
	}
	feed := parsed.Feed
	feedImage := feedmeta.ExtractImageURL(feed)
	setting := db.GetOrCreateSetting()
	llmCfg := LoadLLMConfig()
	summarizationEnabled := llmCfg.Enabled && setting.SummarizationEnabled && strings.TrimSpace(llmCfg.APIKey) != ""
	summarizationPrompt := ""
	summarizationUserPrompt := ""
	if summarizationEnabled {
		summarizationPrompt = ResolveSummarizationPrompt(setting, llmCfg)
		summarizationUserPrompt = ResolveSummarizationUserPrompt(setting, llmCfg)
	}
	initialDownloadMode := NormalizeInitialDownloadModeWithDefault(setting.InitialDownloadMode)
	var allGuids []string
	for i := 0; i < len(parsed.Entries); i++ {
		entry := parsed.Entries[i]
		if guid := feedmeta.ExtractEntryGUID(entry); guid != "" {
			allGuids = append(allGuids, guid)
		}
	}

	existingItems, getErr := db.GetPodcastItemsByPodcastIDAndGUIDs(podcast.ID, allGuids)
	if getErr != nil {
		return getErr
	}
	keyMap := make(map[string]int)

	for _, item := range *existingItems {
		keyMap[item.GUID] = 1
	}
	var latestDate = time.Time{}
	var firstItemErr error
	for i := 0; i < len(parsed.Entries); i++ {
		entry := parsed.Entries[i]
		var podcastItem db.PodcastItem
		guid := feedmeta.ExtractEntryGUID(entry)
		if guid == "" {
			continue
		}
		_, keyExists := keyMap[guid]
		if !keyExists {
			duration := feedmeta.ParseDurationSeconds(feedmeta.PickFirstNonEmpty(feedmeta.GetString(entry, "itunes_duration"), feedmeta.GetString(entry, "duration")))
			pubDate := feedmeta.ParseEntryDate(entry)

			if (pubDate == time.Time{}) {
				Logger.Warnw("could not parse podcast episode date", "podcast_id", podcast.ID, "podcast_title", podcast.Title)
			}

			if latestDate.Before(pubDate) {
				latestDate = pubDate
			}

			var downloadStatus db.DownloadStatus
			if setting.AutoDownload {
				if !newPodcast {
					downloadStatus = db.NotDownloaded
				} else {
					if shouldQueueInitialBackCatalogEpisode(setting, initialDownloadMode, i, pubDate, time.Now().UTC()) {
						downloadStatus = db.NotDownloaded
					} else {
						downloadStatus = db.Deleted
					}
				}
			} else {
				downloadStatus = db.Deleted
			}

			if newPodcast && !setting.DownloadOnAdd {
				downloadStatus = db.Deleted
			}

			if podcast.IsPaused {
				downloadStatus = db.Deleted
			}

			showNotesHTML := feedmeta.ExtractEntryShowNotesHTML(entry)
			showNotesText := strip.StripTags(showNotesHTML)
			chaptersURL, chaptersType := feedmeta.ExtractPodcastChapters(entry)
			chaptersJSON := ""
			if chaptersURL != "" {
				if chaptersBody, err := makeQuery(chaptersURL); err == nil {
					chaptersJSON = string(chaptersBody)
				} else {
					Logger.Warnw("failed to fetch podcast chapters", "url", chaptersURL, "podcast_id", podcast.ID, "error", err)
				}
			}

			transcriptAssets := feedmeta.ExtractTranscripts(entry)
			transcriptStatus := "pending_whisperx"
			transcriptJSON := ""
			canonicalTranscript := ""
			canonicalTranscriptVersion := 0
			var canonicalUpdatedAt *time.Time
			transcriptProgressPct := 0
			transcriptProgressStage := "queued"
			if len(transcriptAssets) > 0 {
				for i := range transcriptAssets {
					if transcriptAssets[i].URL == "" {
						continue
					}
					body, err := makeQuery(transcriptAssets[i].URL)
					if err != nil {
						Logger.Warnw("failed to fetch podcast transcript", "url", transcriptAssets[i].URL, "podcast_id", podcast.ID, "error", err)
						continue
					}
					transcriptAssets[i].Content = string(body)
				}
				transcriptJSON = feedmeta.MarshalMetadata(transcriptAssets)
				canonicalTranscript = buildCanonicalTranscriptFromTranscriptJSON(transcriptJSON)
				canonicalTranscriptVersion = canonicalTranscriptVersionCurrent
				updatedAt := time.Now().UTC()
				canonicalUpdatedAt = &updatedAt
				transcriptStatus = "available"
				transcriptProgressPct = 100
				transcriptProgressStage = "complete"
			} else {
				// Keep missing transcripts in a pending state so WhisperX workers can process them later.
				Logger.Infow("podcast transcript missing; queued for WhisperX", "podcast_id", podcast.ID, "episode_guid", guid)
			}

			podcastItem = db.PodcastItem{
				PodcastID:                  podcast.ID,
				Title:                      feedmeta.GetString(entry, "title"),
				Summary:                    showNotesText,
				SummaryHTML:                showNotesHTML,
				EpisodeType:                feedmeta.PickFirstNonEmpty(feedmeta.GetString(entry, "itunes_episodetype"), feedmeta.GetString(entry, "episodetype")),
				Duration:                   duration,
				PubDate:                    pubDate,
				FileURL:                    feedmeta.ExtractEnclosureURL(entry),
				GUID:                       guid,
				Image:                      feedmeta.ExtractEntryImage(entry, feedImage),
				DownloadStatus:             downloadStatus,
				ChaptersURL:                chaptersURL,
				ChaptersType:               chaptersType,
				ChaptersJSON:               chaptersJSON,
				ItemMetadata:               feedmeta.MarshalMetadata(entry),
				TranscriptJSON:             transcriptJSON,
				CanonicalTranscript:        canonicalTranscript,
				CanonicalTranscriptVersion: canonicalTranscriptVersion,
				CanonicalUpdatedAt:         canonicalUpdatedAt,
				TranscriptStatus:           transcriptStatus,
				TranscriptProgressPct:      transcriptProgressPct,
				TranscriptProgressStage:    transcriptProgressStage,
				TranscriptCheckpointJSON:   "",
			}
			if createErr := db.CreatePodcastItem(&podcastItem); createErr != nil {
				Logger.Errorw("failed to persist podcast item", "podcast_id", podcast.ID, "episode_guid", guid, "error", createErr)
				if firstItemErr == nil {
					firstItemErr = createErr
				}
				continue
			}
			if linkData := feedmeta.ExtractShowNoteLinks(showNotesHTML, showNotesText); len(linkData) > 0 {
				links := make([]db.ShowNoteLink, len(linkData))
				for li, ld := range linkData {
					links[li] = db.ShowNoteLink{
						PodcastItemID: podcastItem.ID,
						PodcastID:     podcast.ID,
						URL:           ld.URL,
						Title:         ld.Title,
						Domain:        ld.Domain,
						Position:      ld.Position,
					}
				}
				if linkErr := db.CreateShowNoteLinks(links); linkErr != nil {
					Logger.Warnw("failed to persist show note links", "podcast_item_id", podcastItem.ID, "error", linkErr)
				}
			}
			if transcriptStatus == "available" && strings.TrimSpace(podcastItem.CanonicalTranscript) != "" {
				ExportTranscript(&podcastItem)
			}
			if summarizationEnabled && transcriptStatus == "available" && strings.TrimSpace(podcastItem.CanonicalTranscript) != "" {
				if sumErr := SummarizeEpisode(&podcastItem, llmCfg, summarizationPrompt, summarizationUserPrompt); sumErr != nil {
					Logger.Warnw(
						"episode summarization failed for feed transcript",
						"podcast_item_id", podcastItem.ID,
						"episode_guid", guid,
						"error", sumErr,
					)
				}
			}
		}
	}
	if (latestDate != time.Time{}) {
		if updateErr := db.UpdateLastEpisodeDateForPodcast(podcast.ID, latestDate); updateErr != nil {
			Logger.Warnw("failed to update podcast last episode", "podcast_id", podcast.ID, "error", updateErr)
			if firstItemErr == nil {
				firstItemErr = updateErr
			}
		}
	}
	return firstItemErr
}

func shouldQueueInitialBackCatalogEpisode(setting *db.Setting, mode string, index int, pubDate time.Time, now time.Time) bool {
	switch NormalizeInitialDownloadModeWithDefault(mode) {
	case InitialDownloadModeAll:
		return true
	case InitialDownloadModeMonths:
		months := setting.InitialDownloadMonths
		if months <= 0 {
			return true
		}
		if pubDate.IsZero() {
			return false
		}
		return !pubDate.Before(now.AddDate(0, -months, 0))
	default:
		count := setting.InitialDownloadCount
		if count <= 0 {
			return true
		}
		return index < count
	}
}

// UpdateAllFileSizes handles the corresponding operation.
func UpdateAllFileSizes() {
	items, err := db.GetAllPodcastItemsWithoutSize()
	if err != nil {
		return
	}
	for _, item := range *items {
		size := int64(1)
		if item.DownloadStatus == db.Downloaded {
			if resolvedSize, sizeErr := GetFileSize(item.DownloadPath); sizeErr == nil {
				size = resolvedSize
			}
		} else {
			if resolvedSize, sizeErr := GetFileSizeFromURL(item.FileURL); sizeErr == nil {
				size = resolvedSize
			}
		}
		if updateErr := db.UpdatePodcastItemFileSize(item.ID, size); updateErr != nil {
			Logger.Warnw("failed to update podcast item file size", "podcast_item_id", item.ID, "error", updateErr)
		}
	}
}

// SetPodcastItemAsQueuedForDownload handles the corresponding operation.
func SetPodcastItemAsQueuedForDownload(id string) error {
	var podcastItem db.PodcastItem
	err := db.GetPodcastItemByID(id, &podcastItem)
	if err != nil {
		return err
	}
	podcastItem.DownloadStatus = db.NotDownloaded
	podcastItem.DownloadedBytes = 0
	podcastItem.DownloadTotalBytes = 0

	return db.UpdatePodcastItem(&podcastItem)
}

// SetPodcastItemAsQueuedPreserveProgress handles the corresponding operation.
func SetPodcastItemAsQueuedPreserveProgress(id string) error {
	var podcastItem db.PodcastItem
	err := db.GetPodcastItemByID(id, &podcastItem)
	if err != nil {
		return err
	}
	podcastItem.DownloadStatus = db.NotDownloaded
	return db.UpdatePodcastItem(&podcastItem)
}

// SetPodcastItemAsDownloading handles the corresponding operation.
func SetPodcastItemAsDownloading(id string) error {
	var podcastItem db.PodcastItem
	err := db.GetPodcastItemByID(id, &podcastItem)
	if err != nil {
		return err
	}
	podcastItem.DownloadStatus = db.Downloading
	return db.UpdatePodcastItem(&podcastItem)
}

// SetPodcastItemAsPaused handles the corresponding operation.
func SetPodcastItemAsPaused(id string) error {
	var podcastItem db.PodcastItem
	err := db.GetPodcastItemByID(id, &podcastItem)
	if err != nil {
		return err
	}
	podcastItem.DownloadStatus = db.Paused
	return db.UpdatePodcastItem(&podcastItem)
}

// DownloadMissingImages handles the corresponding operation.
func DownloadMissingImages() error {
	setting := db.GetOrCreateSetting()
	if !setting.DownloadEpisodeImages {
		Logger.Infow("skipping episode image download; setting disabled")
		return nil
	}
	items, err := db.GetAllPodcastItemsWithoutImage()
	if err != nil {
		return err
	}
	for _, item := range *items {
		if err := downloadImageLocally(item.ID); err != nil {
			Logger.Warnw("failed to download episode image", "podcast_item_id", item.ID, "error", err)
		}
	}
	return nil
}

func downloadImageLocally(podcastItemID string) error {
	var podcastItem db.PodcastItem
	err := db.GetPodcastItemByID(podcastItemID, &podcastItem)
	if err != nil {
		return err
	}

	path, err := DownloadImage(podcastItem.Image, podcastItem.ID, podcastItem.Podcast.Title)
	if err != nil {
		return err
	}

	podcastItem.LocalImage = path

	return db.UpdatePodcastItem(&podcastItem)
}

// SetPodcastItemBookmarkStatus handles the corresponding operation.
func SetPodcastItemBookmarkStatus(id string, bookmark bool) error {
	var podcastItem db.PodcastItem
	err := db.GetPodcastItemByID(id, &podcastItem)
	if err != nil {
		return err
	}
	if bookmark {
		podcastItem.BookmarkDate = time.Now().UTC()
	} else {
		podcastItem.BookmarkDate = time.Time{}
	}
	return db.UpdatePodcastItem(&podcastItem)
}

// SetPodcastItemAsDownloaded handles the corresponding operation.
func SetPodcastItemAsDownloaded(id string, location string) error {
	var podcastItem db.PodcastItem

	err := db.GetPodcastItemByID(id, &podcastItem)
	if err != nil {
		Logger.Errorw("failed to load podcast item for download state update", "podcast_item_id", id, "error", err)
		return err
	}

	size, err := GetFileSize(location)
	if err == nil {
		podcastItem.FileSize = size
	}

	podcastItem.DownloadDate = time.Now().UTC()
	podcastItem.DownloadPath = location
	podcastItem.DownloadStatus = db.Downloaded
	if podcastItem.FileSize > 0 {
		podcastItem.DownloadedBytes = podcastItem.FileSize
		podcastItem.DownloadTotalBytes = podcastItem.FileSize
	}
	if podcastItem.TranscriptStatus == "" && podcastItem.TranscriptJSON == "" {
		podcastItem.TranscriptStatus = "pending_whisperx"
		podcastItem.TranscriptProgressPct = 0
		podcastItem.TranscriptProgressStage = "queued"
		podcastItem.TranscriptCheckpointJSON = ""
		podcastItem.TranscriptRetryCount = 0
		podcastItem.TranscriptNextAttempt = nil
		podcastItem.TranscriptLastError = ""
	} else if podcastItem.TranscriptStatus == "available" {
		podcastItem.TranscriptProgressPct = 100
		podcastItem.TranscriptProgressStage = "complete"
		podcastItem.TranscriptCheckpointJSON = ""
	}

	if id3meta.ShouldExtract(podcastItem.ChaptersJSON, podcastItem.ID3TagsJSON, podcastItem.ID3ChaptersJSON) {
		raw, extractErr := ExtractID3Metadata(location)
		if extractErr != nil {
			Logger.Warnw("id3 metadata extraction failed", "podcast_item_id", id, "error", extractErr)
		} else {
			tagsJSON, chaptersJSON, hasTags, hasChapters, splitErr := id3meta.SplitRaw(raw)
			if splitErr != nil {
				Logger.Warnw("id3 metadata parse failed", "podcast_item_id", id, "error", splitErr)
			} else {
				if hasTags {
					podcastItem.ID3TagsJSON = tagsJSON
				}
				if hasChapters {
					podcastItem.ID3ChaptersJSON = chaptersJSON
					if podcastItem.ChaptersJSON == "" {
						podcastItem.ChaptersJSON = chaptersJSON
						podcastItem.ChaptersType = "id3"
					}
				}
			}
		}
	}

	return db.UpdatePodcastItem(&podcastItem)
}

// SetPodcastItemAsNotDownloaded handles the corresponding operation.
func SetPodcastItemAsNotDownloaded(id string, downloadStatus db.DownloadStatus) error {
	var podcastItem db.PodcastItem
	err := db.GetPodcastItemByID(id, &podcastItem)
	if err != nil {
		return err
	}
	podcastItem.DownloadDate = time.Time{}
	podcastItem.DownloadPath = ""
	podcastItem.DownloadStatus = downloadStatus
	podcastItem.DownloadedBytes = 0
	podcastItem.DownloadTotalBytes = 0

	return db.UpdatePodcastItem(&podcastItem)
}

// SetPodcastItemPlayedStatus handles the corresponding operation.
func SetPodcastItemPlayedStatus(id string, isPlayed bool) error {
	var podcastItem db.PodcastItem
	err := db.GetPodcastItemByID(id, &podcastItem)
	if err != nil {
		return err
	}
	podcastItem.IsPlayed = isPlayed
	return db.UpdatePodcastItem(&podcastItem)
}

// SetAllEpisodesToDownload handles the corresponding operation.
func SetAllEpisodesToDownload(podcastID string) error {
	var podcast db.Podcast
	err := db.GetPodcastByID(podcastID, &podcast)
	if err != nil {
		return err
	}
	if err := AddPodcastItems(&podcast, false); err != nil {
		return err
	}
	return db.SetAllEpisodesToDownload(podcastID)
}

// GetPodcastPrefix handles the corresponding operation.
func GetPodcastPrefix(item *db.PodcastItem, setting *db.Setting) string {
	prefix := ""
	if setting.AppendEpisodeNumberToFileName {
		seq, err := db.GetEpisodeNumber(item.ID, item.PodcastID)
		if err == nil {
			prefix = strconv.Itoa(seq)
		}
	}
	if setting.AppendDateToFileName {
		toAppend := item.PubDate.Format("2006-01-02")
		if prefix == "" {
			prefix = toAppend
		} else {
			prefix = prefix + "-" + toAppend
		}
	}
	return prefix
}

// DownloadMissingEpisodes handles the corresponding operation.
func DownloadMissingEpisodes() error {
	const jobName = "DownloadMissingEpisodes"
	jobLogger, _ := logging.NewJobSugar(jobName)
	start := time.Now()
	jobLogger.Infow("job_started")
	defer func() {
		jobLogger.Infow("job_finished", "duration_ms", time.Since(start).Milliseconds())
	}()

	if DownloadsPaused() {
		jobLogger.Infow("downloads_paused")
		return nil
	}

	lock := db.GetLock(jobName)
	if lock.IsLocked() {
		jobLogger.Infow("job_skipped_lock_exists")
		return nil
	}
	jobLock := db.Lock(jobName, 120)
	defer db.UnlockByID(jobLock.ID)

	setting := db.GetOrCreateSetting()

	data, err := db.GetAllPodcastItemsToBeDownloaded()
	if err != nil {
		jobLogger.Errorw("failed to fetch episodes to download", "error", err)
		return err
	}

	items := *data
	jobLogger.Infow("processing episodes", "count", len(items))
	if len(items) == 0 {
		return nil
	}

	settingSnapshot := *setting
	workers := boundedWorkerCount(setting.MaxDownloadConcurrency, 1, len(items))
	jobLogger.Infow("download worker pool started", "worker_count", workers)
	var (
		firstErr error
		errMutex sync.Mutex
	)
	setError := func(downloadErr error) {
		if downloadErr == nil {
			return
		}
		errMutex.Lock()
		if firstErr == nil {
			firstErr = downloadErr
		}
		errMutex.Unlock()
	}

	runWorkerPool(items, workers, func(item db.PodcastItem) {
		if DownloadsPaused() {
			return
		}
		if IsDownloadCancelled(item.ID) {
			ClearDownloadCancellation(item.ID)
			_ = SetPodcastItemAsNotDownloaded(item.ID, db.Deleted)
			return
		}

		if err := SetPodcastItemAsDownloading(item.ID); err != nil {
			jobLogger.Warnw("failed to mark episode downloading", "podcast_item_id", item.ID, "error", err)
		}

		url, downloadErr := Download(item.ID, item.FileURL, item.Title, item.Podcast.Title, GetPodcastPrefix(&item, &settingSnapshot))
		if downloadErr != nil {
			if downloadErr == ErrDownloadCancelled {
				jobLogger.Infow("download cancelled", "podcast_item_id", item.ID)
				_ = SetPodcastItemAsNotDownloaded(item.ID, db.Deleted)
				return
			}
			if downloadErr == ErrDownloadPaused {
				jobLogger.Infow("download paused", "podcast_item_id", item.ID)
				_ = SetPodcastItemAsPaused(item.ID)
				return
			}
			jobLogger.Errorw("failed to download episode", "podcast_item_id", item.ID, "error", downloadErr)
			_ = SetPodcastItemAsNotDownloaded(item.ID, db.NotDownloaded)
			setError(downloadErr)
			return
		}

		if saveErr := SetPodcastItemAsDownloaded(item.ID, url); saveErr != nil {
			jobLogger.Errorw("failed to update downloaded episode", "podcast_item_id", item.ID, "error", saveErr)
			setError(saveErr)
		}
	})

	if firstErr != nil {
		jobLogger.Errorw("job_completed_with_errors", "error", firstErr)
		return firstErr
	}

	jobLogger.Infow("job_completed_successfully")
	return firstErr
}

// CheckMissingFiles handles the corresponding operation.
func CheckMissingFiles() error {
	data, err := db.GetAllPodcastItemsAlreadyDownloaded()
	setting := db.GetOrCreateSetting()

	if err != nil {
		return err
	}
	for _, item := range *data {
		fileExists := FileExists(item.DownloadPath)
		if !fileExists {
			if setting.DontDownloadDeletedFromDisk {
				if setErr := SetPodcastItemAsNotDownloaded(item.ID, db.Deleted); setErr != nil {
					Logger.Warnw("failed to mark missing file episode deleted", "podcast_item_id", item.ID, "error", setErr)
				}
			} else {
				if setErr := SetPodcastItemAsNotDownloaded(item.ID, db.NotDownloaded); setErr != nil {
					Logger.Warnw("failed to requeue missing file episode", "podcast_item_id", item.ID, "error", setErr)
				}
			}
		}
	}
	return nil
}

// DeleteEpisodeFile handles the corresponding operation.
func DeleteEpisodeFile(podcastItemID string) error {
	var podcastItem db.PodcastItem
	err := db.GetPodcastItemByID(podcastItemID, &podcastItem)

	if err != nil {
		return err
	}

	err = DeleteFile(podcastItem.DownloadPath)

	if err != nil && !os.IsNotExist(err) {
		Logger.Errorw("failed to delete episode file", "podcast_item_id", podcastItemID, "path", podcastItem.DownloadPath, "error", err)
		return err
	}

	if podcastItem.LocalImage != "" {
		go func(localImagePath string) {
			if deleteErr := DeleteFile(localImagePath); deleteErr != nil && !os.IsNotExist(deleteErr) {
				Logger.Warnw("failed to delete local image", "path", localImagePath, "error", deleteErr)
			}
		}(podcastItem.LocalImage)
	}

	return SetPodcastItemAsNotDownloaded(podcastItem.ID, db.Deleted)
}

// DownloadSingleEpisode handles the corresponding operation.
func DownloadSingleEpisode(podcastItemID string) error {
	var podcastItem db.PodcastItem
	err := db.GetPodcastItemByID(podcastItemID, &podcastItem)

	if err != nil {
		return err
	}

	setting := db.GetOrCreateSetting()
	if DownloadsPaused() {
		return errors.New("downloads are paused")
	}
	if err := SetPodcastItemAsDownloading(podcastItemID); err != nil {
		Logger.Warnw("failed to mark episode downloading", "podcast_item_id", podcastItemID, "error", err)
	}

	url, err := Download(podcastItem.ID, podcastItem.FileURL, podcastItem.Title, podcastItem.Podcast.Title, GetPodcastPrefix(&podcastItem, setting))

	if err != nil {
		if err == ErrDownloadCancelled {
			_ = SetPodcastItemAsNotDownloaded(podcastItem.ID, db.Deleted)
			return nil
		}
		if err == ErrDownloadPaused {
			_ = SetPodcastItemAsPaused(podcastItem.ID)
			return nil
		}
		Logger.Errorw("failed to download single episode", "podcast_item_id", podcastItemID, "error", err)
		_ = SetPodcastItemAsNotDownloaded(podcastItem.ID, db.NotDownloaded)
		return err
	}
	err = SetPodcastItemAsDownloaded(podcastItem.ID, url)

	if setting.DownloadEpisodeImages {
		if imageErr := downloadImageLocally(podcastItem.ID); imageErr != nil {
			Logger.Warnw("failed to download episode image after single download", "podcast_item_id", podcastItem.ID, "error", imageErr)
		}
	}
	return err
}

// RefreshEpisodes handles the corresponding operation.
func RefreshEpisodes() error {
	const jobName = "RefreshEpisodes"
	jobLogger, _ := logging.NewJobSugar(jobName)
	start := time.Now()
	jobLogger.Infow("job_started")
	defer func() {
		jobLogger.Infow("job_finished", "duration_ms", time.Since(start).Milliseconds())
	}()

	lock := db.GetLock(jobName)
	if lock.IsLocked() {
		jobLogger.Infow("job_skipped_lock_exists")
		return nil
	}
	jobLock := db.Lock(jobName, 120)
	defer db.UnlockByID(jobLock.ID)

	var data []db.Podcast
	err := db.GetAllPodcasts(&data, "")

	if err != nil {
		jobLogger.Errorw("failed to fetch podcasts", "error", err)
		return err
	}

	if len(data) == 0 {
		jobLogger.Infow("no podcasts found to refresh")
		return nil
	}

	setting := db.GetOrCreateSetting()
	workers := boundedWorkerCount(setting.MaxDownloadConcurrency, 4, len(data))
	jobLogger.Infow("refresh worker pool started", "podcast_count", len(data), "worker_count", workers)
	var (
		firstErr error
		errMutex sync.Mutex
	)
	setError := func(refreshErr error) {
		if refreshErr == nil {
			return
		}
		errMutex.Lock()
		if firstErr == nil {
			firstErr = refreshErr
		}
		errMutex.Unlock()
	}

	runWorkerPool(data, workers, func(item db.Podcast) {
		isNewPodcast := item.LastEpisode == nil
		if isNewPodcast {
			jobLogger.Infow("forcing last episode date for new podcast", "podcast_id", item.ID, "title", item.Title)
			db.ForceSetLastEpisodeDate(item.ID)
		}
		if addErr := AddPodcastItems(&item, isNewPodcast); addErr != nil {
			jobLogger.Errorw("failed to refresh podcast feed", "podcast_id", item.ID, "title", item.Title, "error", addErr)
			setError(addErr)
		}
	})

	go func() {
		if downloadErr := DownloadMissingEpisodes(); downloadErr != nil {
			jobLogger.Warnw("background download job failed after refresh", "error", downloadErr)
		}
	}()

	if firstErr != nil {
		jobLogger.Errorw("job_completed_with_errors", "error", firstErr)
		return firstErr
	}

	jobLogger.Infow("job_completed_successfully")
	return firstErr
}

// DeletePodcastEpisodes handles the corresponding operation.
func DeletePodcastEpisodes(id string) error {
	var podcast db.Podcast

	err := db.GetPodcastByID(id, &podcast)
	if err != nil {
		return err
	}
	var podcastItems []db.PodcastItem

	err = db.GetAllPodcastItemsByPodcastID(id, &podcastItems)
	if err != nil {
		return err
	}
	for _, item := range podcastItems {
		if err := DeleteFile(item.DownloadPath); err != nil && !os.IsNotExist(err) {
			Logger.Warnw("failed to delete episode media file", "podcast_item_id", item.ID, "path", item.DownloadPath, "error", err)
		}
		if item.LocalImage != "" {
			if err := DeleteFile(item.LocalImage); err != nil && !os.IsNotExist(err) {
				Logger.Warnw("failed to delete episode local image", "podcast_item_id", item.ID, "path", item.LocalImage, "error", err)
			}
		}
		if err := SetPodcastItemAsNotDownloaded(item.ID, db.Deleted); err != nil {
			Logger.Warnw("failed to mark episode as deleted", "podcast_item_id", item.ID, "error", err)
		}

	}
	return nil

}

// DeletePodcast handles the corresponding operation.
func DeletePodcast(id string, deleteFiles bool) error {
	var podcast db.Podcast

	err := db.GetPodcastByID(id, &podcast)
	if err != nil {
		return err
	}
	var podcastItems []db.PodcastItem

	err = db.GetAllPodcastItemsByPodcastID(id, &podcastItems)
	if err != nil {
		return err
	}
	for _, item := range podcastItems {
		if deleteFiles {
			if err := DeleteFile(item.DownloadPath); err != nil && !os.IsNotExist(err) {
				Logger.Warnw("failed to delete episode media file", "podcast_item_id", item.ID, "path", item.DownloadPath, "error", err)
			}
			if item.LocalImage != "" {
				if err := DeleteFile(item.LocalImage); err != nil && !os.IsNotExist(err) {
					Logger.Warnw("failed to delete episode local image", "podcast_item_id", item.ID, "path", item.LocalImage, "error", err)
				}
			}

		}
		if deleteErr := db.DeletePodcastItemByID(item.ID); deleteErr != nil {
			return deleteErr
		}

	}

	err = deletePodcastFolder(podcast.Title)
	if err != nil {
		return err
	}

	err = db.DeletePodcastByID(id)
	if err != nil {
		return err
	}
	return nil

}

// DeleteTag handles the corresponding operation.
func DeleteTag(id string) error {
	if err := db.UntagAllByTagID(id); err != nil {
		return err
	}
	err := db.DeleteTagByID(id)
	if err != nil {
		return err
	}
	return nil

}

func makeQuery(url string) ([]byte, error) {
	//link := "https://www.goodreads.com/search/index.xml?q=Good%27s+Omens&key=" + "jCmNlIXjz29GoB8wYsrd0w"
	//link := "https://www.goodreads.com/search/index.xml?key=jCmNlIXjz29GoB8wYsrd0w&q=Ender%27s+Game"
	Logger.Debugw("executing outbound query", "url", url)
	req, err := getRequest(url)
	if err != nil {
		return nil, err
	}

	resp, err := doRequestWithHostLimit(httpClient(), req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	Logger.Debugw("received outbound query response", "url", url, "status", resp.Status)
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, readErr
	}

	return body, nil

}

// GetSearchFromGpodder handles the corresponding operation.
func GetSearchFromGpodder(pod model.GPodcast) *model.CommonSearchResultModel {
	p := new(model.CommonSearchResultModel)
	p.URL = pod.URL
	p.Image = pod.LogoURL
	p.Title = pod.Title
	p.Description = pod.Description
	return p
}

// GetSearchFromItunes handles the corresponding operation.
func GetSearchFromItunes(pod model.ItunesSingleResult) *model.CommonSearchResultModel {
	p := new(model.CommonSearchResultModel)
	p.URL = pod.FeedURL
	p.Image = pod.ArtworkURL600
	p.Title = pod.TrackName

	return p
}

// GetSearchFromPodcastIndex handles the corresponding operation.
func GetSearchFromPodcastIndex(pod *podcastindex.Podcast) *model.CommonSearchResultModel {
	p := new(model.CommonSearchResultModel)
	p.URL = pod.URL
	p.Image = pod.Image
	p.Title = pod.Title
	p.Description = pod.Description

	if pod.Categories != nil {
		values := make([]string, 0, len(pod.Categories))
		for _, val := range pod.Categories {
			values = append(values, val)
		}
		p.Categories = values
	}

	return p
}

// UpdateSettings handles the corresponding operation.
func UpdateSettings(downloadOnAdd bool, initialDownloadCount int, initialDownloadMode string, initialDownloadMonths int, autoDownload bool,
	appendDateToFileName bool, appendEpisodeNumberToFileName bool, darkMode bool, downloadEpisodeImages bool,
	generateNFOFile bool, dontDownloadDeletedFromDisk bool, baseURL string, maxDownloadConcurrency int, userAgent string) error {
	setting := db.GetOrCreateSetting()

	setting.AutoDownload = autoDownload
	setting.DownloadOnAdd = downloadOnAdd
	setting.InitialDownloadCount = initialDownloadCount
	setting.InitialDownloadMode = NormalizeInitialDownloadModeWithDefault(initialDownloadMode)
	if initialDownloadMonths < 0 {
		initialDownloadMonths = 0
	}
	setting.InitialDownloadMonths = initialDownloadMonths
	setting.AppendDateToFileName = appendDateToFileName
	setting.AppendEpisodeNumberToFileName = appendEpisodeNumberToFileName
	setting.DarkMode = darkMode
	setting.DownloadEpisodeImages = downloadEpisodeImages
	setting.GenerateNFOFile = generateNFOFile
	setting.DontDownloadDeletedFromDisk = dontDownloadDeletedFromDisk
	setting.BaseURL = baseURL
	setting.MaxDownloadConcurrency = maxDownloadConcurrency
	setting.UserAgent = userAgent

	return db.UpdateSettings(setting)
}

// UnlockMissedJobs handles the corresponding operation.
func UnlockMissedJobs() {
	db.UnlockMissedJobs()
}

// AddTag handles the corresponding operation.
func AddTag(label, description string) (db.Tag, error) {

	tag, err := db.GetTagByLabel(label)

	if errors.Is(err, gorm.ErrRecordNotFound) {

		tag := db.Tag{
			Label:       label,
			Description: description,
		}

		err = db.CreateTag(&tag)
		return tag, err
	}

	return *tag, &model.TagAlreadyExistsError{Label: label}

}

// TogglePodcastPause handles the corresponding operation.
func TogglePodcastPause(id string, isPaused bool) error {
	var podcast db.Podcast
	err := db.GetPodcastByID(id, &podcast)
	if err != nil {
		return err
	}

	return db.TogglePodcastPauseStatus(id, isPaused)
}
