package service

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
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

var Logger = logging.Sugar()

func ParseOpml(content string) (model.OpmlModel, error) {
	var response model.OpmlModel
	err := xml.Unmarshal([]byte(content), &response)
	return response, err
}

func GetPodcastById(id string) *db.Podcast {
	var podcast db.Podcast

	db.GetPodcastById(id, &podcast)

	return &podcast
}
func GetPodcastItemById(id string) *db.PodcastItem {
	var podcastItem db.PodcastItem

	db.GetPodcastItemById(id, &podcastItem)

	return &podcastItem
}

func GetAllPodcastItemsByIds(podcastItemIds []string) (*[]db.PodcastItem, error) {
	return db.GetAllPodcastItemsByIds(podcastItemIds)
}
func GetAllPodcastItemsByPodcastIds(podcastIds []string) *[]db.PodcastItem {
	var podcastItems []db.PodcastItem

	db.GetAllPodcastItemsByPodcastIds(podcastIds, &podcastItems)
	return &podcastItems
}

func GetTagsByIds(ids []string) *[]db.Tag {

	tags, _ := db.GetTagsByIds(ids)

	return tags
}
func GetAllPodcasts(sorting string) *[]db.Podcast {
	var podcasts []db.Podcast
	db.GetAllPodcasts(&podcasts, sorting)

	stats, _ := db.GetPodcastEpisodeStats()

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

func AddOpml(content string) error {
	opmlModel, err := ParseOpml(content)
	if err != nil {
		Logger.Warnw("failed to parse OPML payload", "error", err)
		return errors.New("Invalid file format")
	}
	var podcastURLs []string
	for _, outline := range opmlModel.Body.Outline {
		if outline.XmlUrl != "" {
			podcastURLs = append(podcastURLs, outline.XmlUrl)
		}

		for _, innerOutline := range outline.Outline {
			if innerOutline.XmlUrl != "" {
				podcastURLs = append(podcastURLs, innerOutline.XmlUrl)
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

	go RefreshEpisodes()
	return nil

}

func ExportOmpl(useBriefcastLink bool, baseUrl string) ([]byte, error) {

	podcasts := GetAllPodcasts("")

	var outlines []model.OpmlOutline
	for _, podcast := range *podcasts {

		xmlUrl := podcast.URL
		if useBriefcastLink {
			xmlUrl = fmt.Sprintf("%s/podcasts/%s/rss", baseUrl, podcast.ID)
		}

		toAdd := model.OpmlOutline{
			AttrText: podcast.Summary,
			Type:     "rss",
			XmlUrl:   xmlUrl,
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

	if data, err := xml.MarshalIndent(toExport, "", "    "); err == nil {
		//	fmt.Println(xml.Header + string(data))
		data = []byte(xml.Header + string(data))
		return data, err
	} else {
		return nil, err
	}
}

func getItunesImageUrl(body []byte) string {
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
			podcast.Image = getItunesImageUrl(body)
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

	return podcast, &model.PodcastAlreadyExistsError{Url: url}

}

func AddPodcastItems(podcast *db.Podcast, newPodcast bool) error {
	//fmt.Println("Creating: " + podcast.ID)
	parsed, _, err := FetchFeedWithFeedparser(podcast.URL)
	if err != nil {
		//log.Fatal(err)
		return err
	}
	feed := parsed.Feed
	feedImage := feedmeta.ExtractImageURL(feed)
	setting := db.GetOrCreateSetting()
	limit := setting.InitialDownloadCount
	// if len(data.Channel.Item) < limit {
	// 	limit = len(data.Channel.Item)
	// }
	var allGuids []string
	for i := 0; i < len(parsed.Entries); i++ {
		entry := parsed.Entries[i]
		if guid := feedmeta.ExtractEntryGUID(entry); guid != "" {
			allGuids = append(allGuids, guid)
		}
	}

	existingItems, err := db.GetPodcastItemsByPodcastIdAndGUIDs(podcast.ID, allGuids)
	keyMap := make(map[string]int)

	for _, item := range *existingItems {
		keyMap[item.GUID] = 1
	}
	var latestDate = time.Time{}
	var itemsAdded = make(map[string]string)
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
					if i < limit {
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
				transcriptStatus = "available"
			} else {
				// TODO: Queue WhisperX transcription when available.
				Logger.Infow("podcast transcript missing; queued for WhisperX", "podcast_id", podcast.ID, "episode_guid", guid)
			}

			podcastItem = db.PodcastItem{
				PodcastID:        podcast.ID,
				Title:            feedmeta.GetString(entry, "title"),
				Summary:          showNotesText,
				SummaryHTML:      showNotesHTML,
				EpisodeType:      feedmeta.PickFirstNonEmpty(feedmeta.GetString(entry, "itunes_episodetype"), feedmeta.GetString(entry, "episodetype")),
				Duration:         duration,
				PubDate:          pubDate,
				FileURL:          feedmeta.ExtractEnclosureURL(entry),
				GUID:             guid,
				Image:            feedmeta.ExtractEntryImage(entry, feedImage),
				DownloadStatus:   downloadStatus,
				ChaptersURL:      chaptersURL,
				ChaptersType:     chaptersType,
				ChaptersJSON:     chaptersJSON,
				ItemMetadata:     feedmeta.MarshalMetadata(entry),
				TranscriptJSON:   transcriptJSON,
				TranscriptStatus: transcriptStatus,
			}
			db.CreatePodcastItem(&podcastItem)
			itemsAdded[podcastItem.ID] = podcastItem.FileURL
		}
	}
	if (latestDate != time.Time{}) {
		db.UpdateLastEpisodeDateForPodcast(podcast.ID, latestDate)
	}
	//go updateSizeFromUrl(itemsAdded)
	return err
}

func updateSizeFromUrl(itemUrlMap map[string]string) {

	for id, url := range itemUrlMap {
		size, err := GetFileSizeFromUrl(url)
		if err != nil {
			size = 1
		}

		db.UpdatePodcastItemFileSize(id, size)
	}

}

func UpdateAllFileSizes() {
	items, err := db.GetAllPodcastItemsWithoutSize()
	if err != nil {
		return
	}
	for _, item := range *items {
		var size int64 = 1
		if item.DownloadStatus == db.Downloaded {
			size, _ = GetFileSize(item.DownloadPath)
		} else {
			size, _ = GetFileSizeFromUrl(item.FileURL)
		}
		db.UpdatePodcastItemFileSize(item.ID, size)
	}
}

func SetPodcastItemAsQueuedForDownload(id string) error {
	var podcastItem db.PodcastItem
	err := db.GetPodcastItemById(id, &podcastItem)
	if err != nil {
		return err
	}
	podcastItem.DownloadStatus = db.NotDownloaded
	podcastItem.DownloadedBytes = 0
	podcastItem.DownloadTotalBytes = 0

	return db.UpdatePodcastItem(&podcastItem)
}

func SetPodcastItemAsQueuedPreserveProgress(id string) error {
	var podcastItem db.PodcastItem
	err := db.GetPodcastItemById(id, &podcastItem)
	if err != nil {
		return err
	}
	podcastItem.DownloadStatus = db.NotDownloaded
	return db.UpdatePodcastItem(&podcastItem)
}

func SetPodcastItemAsDownloading(id string) error {
	var podcastItem db.PodcastItem
	err := db.GetPodcastItemById(id, &podcastItem)
	if err != nil {
		return err
	}
	podcastItem.DownloadStatus = db.Downloading
	return db.UpdatePodcastItem(&podcastItem)
}

func SetPodcastItemAsPaused(id string) error {
	var podcastItem db.PodcastItem
	err := db.GetPodcastItemById(id, &podcastItem)
	if err != nil {
		return err
	}
	podcastItem.DownloadStatus = db.Paused
	return db.UpdatePodcastItem(&podcastItem)
}

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
		downloadImageLocally(item.ID)
	}
	return nil
}

func downloadImageLocally(podcastItemId string) error {
	var podcastItem db.PodcastItem
	err := db.GetPodcastItemById(podcastItemId, &podcastItem)
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

func SetPodcastItemBookmarkStatus(id string, bookmark bool) error {
	var podcastItem db.PodcastItem
	err := db.GetPodcastItemById(id, &podcastItem)
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

func SetPodcastItemAsDownloaded(id string, location string) error {
	var podcastItem db.PodcastItem

	err := db.GetPodcastItemById(id, &podcastItem)
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
func SetPodcastItemAsNotDownloaded(id string, downloadStatus db.DownloadStatus) error {
	var podcastItem db.PodcastItem
	err := db.GetPodcastItemById(id, &podcastItem)
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

func SetPodcastItemPlayedStatus(id string, isPlayed bool) error {
	var podcastItem db.PodcastItem
	err := db.GetPodcastItemById(id, &podcastItem)
	if err != nil {
		return err
	}
	podcastItem.IsPlayed = isPlayed
	return db.UpdatePodcastItem(&podcastItem)
}
func SetAllEpisodesToDownload(podcastId string) error {
	var podcast db.Podcast
	err := db.GetPodcastById(podcastId, &podcast)
	if err != nil {
		return err
	}
	AddPodcastItems(&podcast, false)
	return db.SetAllEpisodesToDownload(podcastId)
}

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
func DownloadMissingEpisodes() error {
	const JOB_NAME = "DownloadMissingEpisodes"
	jobLogger, _ := logging.NewJobSugar(JOB_NAME)
	start := time.Now()
	jobLogger.Infow("job_started")
	defer func() {
		jobLogger.Infow("job_finished", "duration_ms", time.Since(start).Milliseconds())
	}()

	if DownloadsPaused() {
		jobLogger.Infow("downloads_paused")
		return nil
	}

	lock := db.GetLock(JOB_NAME)
	if lock.IsLocked() {
		jobLogger.Infow("job_skipped_lock_exists")
		return nil
	}
	db.Lock(JOB_NAME, 120)
	defer db.Unlock(JOB_NAME)

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
func CheckMissingFiles() error {
	data, err := db.GetAllPodcastItemsAlreadyDownloaded()
	setting := db.GetOrCreateSetting()

	//fmt.Println("Processing episodes: ", strconv.Itoa(len(*data)))
	if err != nil {
		return err
	}
	for _, item := range *data {
		fileExists := FileExists(item.DownloadPath)
		if !fileExists {
			if setting.DontDownloadDeletedFromDisk {
				SetPodcastItemAsNotDownloaded(item.ID, db.Deleted)
			} else {
				SetPodcastItemAsNotDownloaded(item.ID, db.NotDownloaded)
			}
		}
	}
	return nil
}

func DeleteEpisodeFile(podcastItemId string) error {
	var podcastItem db.PodcastItem
	err := db.GetPodcastItemById(podcastItemId, &podcastItem)

	//fmt.Println("Processing episodes: ", strconv.Itoa(len(*data)))
	if err != nil {
		return err
	}

	err = DeleteFile(podcastItem.DownloadPath)

	if err != nil && !os.IsNotExist(err) {
		Logger.Errorw("failed to delete episode file", "podcast_item_id", podcastItemId, "path", podcastItem.DownloadPath, "error", err)
		return err
	}

	if podcastItem.LocalImage != "" {
		go DeleteFile(podcastItem.LocalImage)
	}

	return SetPodcastItemAsNotDownloaded(podcastItem.ID, db.Deleted)
}
func DownloadSingleEpisode(podcastItemId string) error {
	var podcastItem db.PodcastItem
	err := db.GetPodcastItemById(podcastItemId, &podcastItem)

	//fmt.Println("Processing episodes: ", strconv.Itoa(len(*data)))
	if err != nil {
		return err
	}

	setting := db.GetOrCreateSetting()
	if DownloadsPaused() {
		return errors.New("downloads are paused")
	}
	if err := SetPodcastItemAsDownloading(podcastItemId); err != nil {
		Logger.Warnw("failed to mark episode downloading", "podcast_item_id", podcastItemId, "error", err)
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
		Logger.Errorw("failed to download single episode", "podcast_item_id", podcastItemId, "error", err)
		_ = SetPodcastItemAsNotDownloaded(podcastItem.ID, db.NotDownloaded)
		return err
	}
	err = SetPodcastItemAsDownloaded(podcastItem.ID, url)

	if setting.DownloadEpisodeImages {
		downloadImageLocally(podcastItem.ID)
	}
	return err
}

func RefreshEpisodes() error {
	const JOB_NAME = "RefreshEpisodes"
	jobLogger, _ := logging.NewJobSugar(JOB_NAME)
	start := time.Now()
	jobLogger.Infow("job_started")
	defer func() {
		jobLogger.Infow("job_finished", "duration_ms", time.Since(start).Milliseconds())
	}()

	lock := db.GetLock(JOB_NAME)
	if lock.IsLocked() {
		jobLogger.Infow("job_skipped_lock_exists")
		return nil
	}
	db.Lock(JOB_NAME, 120)
	defer db.Unlock(JOB_NAME)

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

	go DownloadMissingEpisodes()

	if firstErr != nil {
		jobLogger.Errorw("job_completed_with_errors", "error", firstErr)
		return firstErr
	}

	jobLogger.Infow("job_completed_successfully")
	return firstErr
}

func DeletePodcastEpisodes(id string) error {
	var podcast db.Podcast

	err := db.GetPodcastById(id, &podcast)
	if err != nil {
		return err
	}
	var podcastItems []db.PodcastItem

	err = db.GetAllPodcastItemsByPodcastId(id, &podcastItems)
	if err != nil {
		return err
	}
	for _, item := range podcastItems {
		DeleteFile(item.DownloadPath)
		if item.LocalImage != "" {
			DeleteFile(item.LocalImage)
		}
		SetPodcastItemAsNotDownloaded(item.ID, db.Deleted)

	}
	return nil

}
func DeletePodcast(id string, deleteFiles bool) error {
	var podcast db.Podcast

	err := db.GetPodcastById(id, &podcast)
	if err != nil {
		return err
	}
	var podcastItems []db.PodcastItem

	err = db.GetAllPodcastItemsByPodcastId(id, &podcastItems)
	if err != nil {
		return err
	}
	for _, item := range podcastItems {
		if deleteFiles {
			DeleteFile(item.DownloadPath)
			if item.LocalImage != "" {
				DeleteFile(item.LocalImage)
			}

		}
		db.DeletePodcastItemById(item.ID)

	}

	err = deletePodcastFolder(podcast.Title)
	if err != nil {
		return err
	}

	err = db.DeletePodcastById(id)
	if err != nil {
		return err
	}
	return nil

}
func DeleteTag(id string) error {
	db.UntagAllByTagId(id)
	err := db.DeleteTagById(id)
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
	body, err := ioutil.ReadAll(resp.Body)

	return body, nil

}
func GetSearchFromGpodder(pod model.GPodcast) *model.CommonSearchResultModel {
	p := new(model.CommonSearchResultModel)
	p.URL = pod.URL
	p.Image = pod.LogoURL
	p.Title = pod.Title
	p.Description = pod.Description
	return p
}
func GetSearchFromItunes(pod model.ItunesSingleResult) *model.CommonSearchResultModel {
	p := new(model.CommonSearchResultModel)
	p.URL = pod.FeedURL
	p.Image = pod.ArtworkURL600
	p.Title = pod.TrackName

	return p
}
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

func UpdateSettings(downloadOnAdd bool, initialDownloadCount int, autoDownload bool,
	appendDateToFileName bool, appendEpisodeNumberToFileName bool, darkMode bool, downloadEpisodeImages bool,
	generateNFOFile bool, dontDownloadDeletedFromDisk bool, baseUrl string, maxDownloadConcurrency int, userAgent string) error {
	setting := db.GetOrCreateSetting()

	setting.AutoDownload = autoDownload
	setting.DownloadOnAdd = downloadOnAdd
	setting.InitialDownloadCount = initialDownloadCount
	setting.AppendDateToFileName = appendDateToFileName
	setting.AppendEpisodeNumberToFileName = appendEpisodeNumberToFileName
	setting.DarkMode = darkMode
	setting.DownloadEpisodeImages = downloadEpisodeImages
	setting.GenerateNFOFile = generateNFOFile
	setting.DontDownloadDeletedFromDisk = dontDownloadDeletedFromDisk
	setting.BaseUrl = baseUrl
	setting.MaxDownloadConcurrency = maxDownloadConcurrency
	setting.UserAgent = userAgent

	return db.UpdateSettings(setting)
}

func UnlockMissedJobs() {
	db.UnlockMissedJobs()
}

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

func TogglePodcastPause(id string, isPaused bool) error {
	var podcast db.Podcast
	err := db.GetPodcastById(id, &podcast)
	if err != nil {
		return err
	}

	return db.TogglePodcastPauseStatus(id, isPaused)
}
