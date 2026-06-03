package db

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/ctaylor1/briefcast/internal/logging"
	"github.com/ctaylor1/briefcast/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GetPodcastByURL handles the corresponding operation.
func GetPodcastByURL(url string, podcast *Podcast) error {
	result := podcastsWithAssociations(DB).Where(&Podcast{URL: url}).First(&podcast)
	return result.Error
}

// GetPodcastsByURLList handles the corresponding operation.
func GetPodcastsByURLList(urls []string, podcasts *[]Podcast) error {
	result := podcastsWithAssociations(DB).Where("url in ?", urls).First(&podcasts)
	return result.Error
}

// GetAllPodcasts handles the corresponding operation.
func GetAllPodcasts(podcasts *[]Podcast, sorting string) error {
	if sorting == "" {
		sorting = "created_at"
	}
	result := podcastsWithTags(DB).Order(sorting).Find(&podcasts)
	return result.Error
}

// GetAllPodcastItems handles the corresponding operation.
func GetAllPodcastItems(podcasts *[]PodcastItem) error {
	result := podcastItemsWithPodcast(DB).Order("pub_date desc").Find(&podcasts)
	return result.Error
}

// GetAllPodcastItemsWithoutSize handles the corresponding operation.
func GetAllPodcastItemsWithoutSize() (*[]PodcastItem, error) {
	var podcasts []PodcastItem
	result := DB.Where("file_size<=?", 0).Order("pub_date desc").Find(&podcasts)
	return &podcasts, result.Error
}

func getSortOrder(sorting model.EpisodeSort) string {
	switch sorting {
	case model.ReleaseAsc:
		return "pub_date asc"
	case model.ReleaseDesc:
		return "pub_date desc"
	case model.DurationAsc:
		return "duration asc"
	case model.DurationDesc:
		return "duration desc"
	default:
		return "pub_date desc"
	}
}

// GetPaginatedPodcastItemsNew handles the corresponding operation.
func GetPaginatedPodcastItemsNew(queryModel model.EpisodesFilter) (*[]PodcastItem, int64, error) {
	var podcasts []PodcastItem
	var total int64
	query := DB.Model(&PodcastItem{})
	if queryModel.IsDownloaded != nil {
		isDownloaded, err := strconv.ParseBool(*queryModel.IsDownloaded)
		if err == nil {
			query = applyDownloadStatusFilter(query, &isDownloaded)
		}
	}
	if queryModel.IsPlayed != nil {
		isPlayed, err := strconv.ParseBool(*queryModel.IsPlayed)
		if err == nil {
			query = applyPlayedStatusFilter(query, &isPlayed)
		}
	}
	if queryModel.IsBookmarked != nil {
		isBookmarked, err := strconv.ParseBool(*queryModel.IsBookmarked)
		if err == nil {
			query = applyBookmarkStatusFilter(query, &isBookmarked)
		}
	}

	if queryModel.Q != "" {
		query = query.Where("UPPER(title) like ?", "%"+strings.TrimSpace(strings.ToUpper(queryModel.Q))+"%")
	}

	if len(queryModel.TagIds) > 0 {
		query = query.Where("podcast_id in (select podcast_id from podcast_tags where tag_id in ?)", queryModel.TagIds)
	}

	if len(queryModel.PodcastIds) > 0 {
		query = query.Where("podcast_id in ?", queryModel.PodcastIds)
	}

	if err := query.Count(&total).Error; err != nil {
		return &podcasts, 0, err
	}

	result := podcastItemsWithPodcast(query).
		Limit(queryModel.Count).
		Offset((queryModel.Page - 1) * queryModel.Count).
		Order(getSortOrder(queryModel.Sorting)).
		Find(&podcasts)
	return &podcasts, total, result.Error
}

// GetPaginatedPodcastItems handles the corresponding operation.
func GetPaginatedPodcastItems(page int, count int, downloadedOnly *bool, playedOnly *bool, fromDate time.Time, podcasts *[]PodcastItem, total *int64) error {
	query := DB.Model(&PodcastItem{})
	query = applyDownloadStatusFilter(query, downloadedOnly)
	query = applyPlayedStatusFilter(query, playedOnly)
	if (fromDate != time.Time{}) {
		query = query.Where("pub_date>=?", fromDate)
	}

	if err := query.Count(total).Error; err != nil {
		return err
	}

	result := podcastItemsWithPodcast(query).
		Limit(count).
		Offset((page - 1) * count).
		Order("pub_date desc").
		Find(&podcasts)
	return result.Error
}

// GetPaginatedTags handles the corresponding operation.
func GetPaginatedTags(page int, count int, tags *[]Tag, total *int64) error {
	query := tagsWithPodcasts(DB)

	result := query.Limit(count).Offset((page - 1) * count).Order("created_at desc").Find(&tags)

	query.Count(total)

	return result.Error
}

// GetPodcastByID loads a podcast without its episodes.
func GetPodcastByID(id string, podcast *Podcast) error {
	result := DB.Preload("Tags").First(&podcast, "id=?", id)
	return result.Error
}

// GetPodcastByIDWithItems loads a podcast with all episodes (expensive for large feeds).
func GetPodcastByIDWithItems(id string, podcast *Podcast) error {
	result := DB.Preload("PodcastItems", func(db *gorm.DB) *gorm.DB {
		return db.Order("podcast_items.pub_date DESC")
	}).Preload("Tags").First(&podcast, "id=?", id)
	return result.Error
}

// GetPodcastItemByID handles the corresponding operation.
func GetPodcastItemByID(id string, podcastItem *PodcastItem) error {

	result := podcastItemsWithAssociations(DB).First(&podcastItem, "id=?", id)
	return result.Error
}

// DeletePodcastItemByID handles the corresponding operation.
func DeletePodcastItemByID(id string) error {

	result := DB.Where("id=?", id).Delete(&PodcastItem{})
	return result.Error
}

// DeletePodcastByID handles the corresponding operation.
func DeletePodcastByID(id string) error {

	result := DB.Where("id=?", id).Delete(&Podcast{})
	return result.Error
}

// DeleteTagByID handles the corresponding operation.
func DeleteTagByID(id string) error {

	result := DB.Where("id=?", id).Delete(&Tag{})
	return result.Error
}

// GetAllPodcastItemsByPodcastID handles the corresponding operation.
func GetAllPodcastItemsByPodcastID(podcastID string, podcastItems *[]PodcastItem) error {
	result := DB.Where(&PodcastItem{PodcastID: podcastID}).Find(&podcastItems)
	return result.Error
}

// podcastItemListColumns contains the columns needed for list/player views,
// excluding heavy text blobs (transcripts, JSON, summaries) that are never
// serialized in the API response.
// podcastItemListColumns contains the columns needed for list/player views,
// excluding heavy text blobs (transcripts, summaries) that are never
// serialized in the API response. Chapters JSON columns are included because
// they are tiny (empty for most episodes) and needed for the HasChapters flag.
var podcastItemListColumns = []string{
	"id", "created_at", "updated_at", "deleted_at",
	"podcast_id", "title", "summary", "summary_html",
	"episode_type", "duration", "pub_date", "file_url",
	"guid", "image", "chapters_url", "chapters_type",
	"chapters_json", "id3_chapters_json",
	"download_date", "download_path", "download_status",
	"is_played", "bookmark_date", "local_image",
	"file_size", "downloaded_bytes", "download_total_bytes",
	"canonical_transcript_version", "canonical_updated_at",
	"transcript_status", "transcript_progress_pct", "transcript_progress_stage",
	"transcript_retry_count", "transcript_next_attempt", "transcript_model",
	"llm_summary_status", "llm_summary_date", "llm_summary_model",
	"is_summary_favorited",
}

// GetPodcastItemsByPodcastIDLightweight loads items without heavy text columns.
func GetPodcastItemsByPodcastIDLightweight(podcastID string, podcastItems *[]PodcastItem) error {
	result := DB.Select(podcastItemListColumns).
		Where("podcast_id = ?", podcastID).
		Order("pub_date DESC").
		Find(&podcastItems)
	return result.Error
}

// GetAllPodcastItemsByPodcastIds handles the corresponding operation.
func GetAllPodcastItemsByPodcastIds(podcastIds []string, podcastItems *[]PodcastItem) error {

	result := podcastItemsWithAssociations(DB).Where("podcast_id in ?", podcastIds).Order("pub_date desc").Find(&podcastItems)
	return result.Error
}

// GetAllPodcastItemsByIds handles the corresponding operation.
func GetAllPodcastItemsByIds(podcastItemIds []string) (*[]PodcastItem, error) {
	var podcastItems []PodcastItem
	if len(podcastItemIds) == 0 {
		return &podcastItems, nil
	}

	var sqlBuilder strings.Builder
	sqlBuilder.WriteString("CASE id ")
	for i, id := range podcastItemIds {
		escapedID := strings.ReplaceAll(id, "'", "''")
		sqlBuilder.WriteString("WHEN '")
		sqlBuilder.WriteString(escapedID)
		sqlBuilder.WriteString("' THEN ")
		sqlBuilder.WriteString(strconv.Itoa(i + 1))
		sqlBuilder.WriteString(" ")
	}
	sqlBuilder.WriteString("ELSE ")
	sqlBuilder.WriteString(strconv.Itoa(len(podcastItemIds) + 1))
	sqlBuilder.WriteString(" END")

	result := podcastItemsWithAssociations(DB).
		Where("id in ?", podcastItemIds).
		Order(sqlBuilder.String()).
		Find(&podcastItems)
	return &podcastItems, result.Error
}

// SetAllEpisodesToDownload handles the corresponding operation.
func SetAllEpisodesToDownload(podcastID string) error {
	result := DB.Model(PodcastItem{}).Where(&PodcastItem{PodcastID: podcastID, DownloadStatus: Deleted}).Update("download_status", NotDownloaded)
	return result.Error
}

// UpdateLastEpisodeDateForPodcast handles the corresponding operation.
func UpdateLastEpisodeDateForPodcast(podcastID string, lastEpisode time.Time) error {
	result := DB.Model(Podcast{}).Where("id=?", podcastID).Update("last_episode", lastEpisode)
	return result.Error
}

// UpdatePodcastItemFileSize handles the corresponding operation.
func UpdatePodcastItemFileSize(podcastItemID string, size int64) error {
	result := DB.Model(PodcastItem{}).Where("id=?", podcastItemID).Update("file_size", size)
	return result.Error
}

// GetAllPodcastItemsWithoutImage handles the corresponding operation.
func GetAllPodcastItemsWithoutImage() (*[]PodcastItem, error) {
	var podcastItems []PodcastItem
	result := podcastItemsWithAssociations(DB).
		Where("(local_image IS NULL OR local_image = '')").
		Where("image != ?", "").
		Where("download_status = ?", Downloaded).
		Order("created_at desc").
		Find(&podcastItems)
	return &podcastItems, result.Error
}

// GetAllPodcastItemsToBeDownloaded handles the corresponding operation.
func GetAllPodcastItemsToBeDownloaded() (*[]PodcastItem, error) {
	var podcastItems []PodcastItem
	result := podcastItemsWithAssociations(DB).Where("download_status=?", NotDownloaded).Find(&podcastItems)
	return &podcastItems, result.Error
}

// GetAllPodcastItemsAlreadyDownloaded handles the corresponding operation.
func GetAllPodcastItemsAlreadyDownloaded() (*[]PodcastItem, error) {
	var podcastItems []PodcastItem
	result := podcastItemsWithAssociations(DB).Where("download_status=?", Downloaded).Find(&podcastItems)
	return &podcastItems, result.Error
}

// GetPodcastItemsForWhisperx handles the corresponding operation.
func GetPodcastItemsForWhisperx(statuses []string, limit int) (*[]PodcastItem, error) {
	var podcastItems []PodcastItem
	// Transcript workers consume only downloaded episodes with empty transcript payloads.
	now := time.Now().UTC()
	query := DB.Where("download_status=?", Downloaded).
		Where("transcript_status IN ?", statuses).
		Where("download_path <> ''").
		Where("(transcript_json IS NULL OR transcript_json = '')").
		Where("(transcript_next_attempt IS NULL OR transcript_next_attempt <= ?)", now).
		Order(`CASE
			WHEN transcript_status = 'processing' THEN 0
			WHEN transcript_status = 'pending_whisperx' THEN 1
			WHEN transcript_status = 'failed' THEN 2
			ELSE 3
		END ASC`).
		Order("COALESCE(transcript_next_attempt, download_date) asc").
		Order("download_date asc")
	if limit > 0 {
		query = query.Limit(limit)
	}
	result := query.Find(&podcastItems)
	return &podcastItems, result.Error
}

// GetPodcastItemsByDownloadStatuses handles the corresponding operation.
func GetPodcastItemsByDownloadStatuses(statuses []DownloadStatus, limit int) ([]PodcastItem, error) {
	var podcastItems []PodcastItem
	statusPriority := clause.Expr{
		SQL:  "CASE download_status WHEN ? THEN 0 WHEN ? THEN 1 WHEN ? THEN 2 WHEN ? THEN 3 ELSE 4 END",
		Vars: []interface{}{Downloading, NotDownloaded, Paused, Downloaded},
	}
	query := podcastItemsWithPodcast(DB).
		Where("download_status IN ?", statuses).
		Order(statusPriority).
		Order("download_date desc").
		Order("created_at desc")
	if limit > 0 {
		query = query.Limit(limit)
	}
	result := query.Find(&podcastItems)
	return podcastItems, result.Error
}

// UpdatePodcastItemDownloadProgress handles the corresponding operation.
func UpdatePodcastItemDownloadProgress(podcastItemID string, downloadedBytes int64, totalBytes int64) error {
	updates := map[string]interface{}{
		"downloaded_bytes": downloadedBytes,
	}
	if totalBytes > 0 {
		updates["download_total_bytes"] = totalBytes
	}
	return DB.Model(&PodcastItem{}).Where("id=?", podcastItemID).Updates(updates).Error
}

// GetPaginatedSummaries returns podcast items that have available LLM summaries,
// with optional search, podcast filtering, sorting, and pagination.
func GetPaginatedSummaries(page, count int, q string, podcastIds []string, sorting string, favoritesOnly bool) (*[]PodcastItem, int64, error) {
	var items []PodcastItem
	var total int64

	query := DB.Model(&PodcastItem{}).Where("llm_summary_status = ?", "available")

	if q != "" {
		like := "%" + strings.TrimSpace(strings.ToUpper(q)) + "%"
		query = query.Where("UPPER(title) LIKE ? OR UPPER(llm_summary) LIKE ?", like, like)
	}

	if len(podcastIds) > 0 {
		query = query.Where("podcast_id IN ?", podcastIds)
	}

	if favoritesOnly {
		query = applyEpisodeFavoriteFilter(query)
	}

	var order string
	switch sorting {
	case "oldest":
		order = "pub_date asc"
	case "title_asc":
		order = "title asc"
	case "title_desc":
		order = "title desc"
	case "shortest":
		order = "duration asc"
	case "longest":
		order = "duration desc"
	default:
		order = "pub_date desc"
	}

	if err := query.Count(&total).Error; err != nil {
		return &items, 0, err
	}

	result := podcastItemsWithPodcast(query).
		Limit(count).
		Offset((page - 1) * count).
		Order(order).
		Find(&items)
	return &items, total, result.Error
}

// GetDistinctSummaryModels returns the unique LLM model names used across all summaries.
func GetDistinctSummaryModels() ([]string, error) {
	var models []string
	result := DB.Model(&PodcastItem{}).
		Where("llm_summary_model IS NOT NULL AND llm_summary_model <> ''").
		Distinct("llm_summary_model").
		Order("llm_summary_model").
		Pluck("llm_summary_model", &models)
	return models, result.Error
}

// SetSummaryFavorited toggles the is_summary_favorited flag on a podcast item.
func SetSummaryFavorited(id string, favorited bool) error {
	updates := map[string]interface{}{
		"is_summary_favorited": favorited,
	}
	if favorited {
		updates["bookmark_date"] = time.Now().UTC()
	} else {
		updates["bookmark_date"] = time.Time{}
	}
	return DB.Model(&PodcastItem{}).Where("id = ?", id).Updates(updates).Error
}

// GetPodcastEpisodeStats handles the corresponding operation.
func GetPodcastEpisodeStats() (*[]PodcastItemStatsModel, error) {
	var stats []PodcastItemStatsModel
	result := DB.Model(&PodcastItem{}).Select("download_status,podcast_id, count(1) as count,sum(file_size) as size").Group("podcast_id,download_status").Find(&stats)
	return &stats, result.Error
}

// GetPodcastItemStatusCounts returns download-state counts across the whole library.
func GetPodcastItemStatusCounts() (*[]PodcastItemStatusCountModel, error) {
	var stats []PodcastItemStatusCountModel
	result := DB.Model(&PodcastItem{}).
		Select("download_status, count(1) as count").
		Group("download_status").
		Find(&stats)
	return &stats, result.Error
}

// GetPodcastEpisodeDiskStats handles the corresponding operation.
func GetPodcastEpisodeDiskStats() (PodcastItemConsolidateDiskStatsModel, error) {
	var stats []PodcastItemDiskStatsModel
	result := DB.Model(&PodcastItem{}).Select("download_status,count(1) as count,sum(file_size) as size").Group("download_status").Find(&stats)
	dict := make(map[DownloadStatus]int64)
	for _, stat := range stats {
		dict[stat.DownloadStatus] = stat.Size
	}

	toReturn := PodcastItemConsolidateDiskStatsModel{
		Downloaded:      dict[Downloaded],
		Downloading:     dict[Downloading],
		Deleted:         dict[Deleted],
		NotDownloaded:   dict[NotDownloaded],
		PendingDownload: dict[NotDownloaded] + dict[Downloading],
	}

	return toReturn, result.Error
}

// GetEpisodeNumber handles the corresponding operation.
func GetEpisodeNumber(podcastItemID, podcastID string) (int, error) {
	var id string
	var sequence int
	row := DB.Raw(`;With cte as(
		SELECT 
			id, 
			RANK() OVER (ORDER BY pub_date) as sequence 
		FROM 
			podcast_items
		WHERE
			podcast_id=?
	)
	select * 
	from cte 
	where id = ?
	`, podcastID, podcastItemID).Row()
	error := row.Scan(&id, &sequence)
	return sequence, error
}

// ForceSetLastEpisodeDate handles the corresponding operation.
func ForceSetLastEpisodeDate(podcastID string) {
	DB.Exec("update podcasts set last_episode = (select max(pi.pub_date) from podcast_items pi where pi.podcast_id = @id) where id = @id", sql.Named("id", podcastID))
}

// TogglePodcastPauseStatus handles the corresponding operation.
func TogglePodcastPauseStatus(podcastID string, isPaused bool) error {

	tx := DB.Exec("update podcasts set is_paused = @isPaused where id = @id", sql.Named("id", podcastID), sql.Named("isPaused", isPaused))
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		var podcast Podcast
		if err := DB.Select("id").First(&podcast, "id = ?", podcastID).Error; err != nil {
			return err
		}
	}
	return nil
}

// GetPodcastItemsByPodcastIDAndGUIDs handles the corresponding operation.
func GetPodcastItemsByPodcastIDAndGUIDs(podcastID string, guids []string) (*[]PodcastItem, error) {
	var podcastItems []PodcastItem
	result := podcastItemsWithAssociations(DB).Where(&PodcastItem{PodcastID: podcastID}).Where("guid IN ?", guids).Find(&podcastItems)
	return &podcastItems, result.Error
}

// GetPodcastItemByPodcastIDAndGUID handles the corresponding operation.
func GetPodcastItemByPodcastIDAndGUID(podcastID string, guid string, podcastItem *PodcastItem) error {

	result := podcastItemsWithAssociations(DB).Where(&PodcastItem{PodcastID: podcastID, GUID: guid}).First(&podcastItem)
	return result.Error
}

// GetPodcastByTitleAndAuthor handles the corresponding operation.
func GetPodcastByTitleAndAuthor(title string, author string, podcast *Podcast) error {

	result := podcastsWithAssociations(DB).Where(&Podcast{Title: title, Author: author}).First(&podcast)
	return result.Error
}

// CreatePodcast handles the corresponding operation.
func CreatePodcast(podcast *Podcast) error {
	tx := DB.Create(&podcast)
	return tx.Error
}

// CreatePodcastItem handles the corresponding operation.
func CreatePodcastItem(podcastItem *PodcastItem) error {
	tx := DB.Omit("Podcast").Create(&podcastItem)
	return tx.Error
}

// UpdatePodcast handles the corresponding operation.
func UpdatePodcast(podcast *Podcast) error {
	tx := DB.Save(&podcast)
	return tx.Error
}

// UpdatePodcastItem handles the corresponding operation.
func UpdatePodcastItem(podcastItem *PodcastItem) error {
	tx := DB.Omit("Podcast").Save(&podcastItem)
	return tx.Error
}

// CreateShowNoteLinks batch-inserts show note links.
func CreateShowNoteLinks(links []ShowNoteLink) error {
	if len(links) == 0 {
		return nil
	}
	tx := DB.Create(&links)
	return tx.Error
}

// GetShowNoteLinksByPodcastItemID returns all links for an episode.
func GetShowNoteLinksByPodcastItemID(podcastItemID string) ([]ShowNoteLink, error) {
	var links []ShowNoteLink
	result := DB.Where("podcast_item_id = ?", podcastItemID).Order("position asc").Find(&links)
	return links, result.Error
}

// GetShowNoteLinksByPodcastID returns all links for a podcast.
func GetShowNoteLinksByPodcastID(podcastID string) ([]ShowNoteLink, error) {
	var links []ShowNoteLink
	result := DB.Where("podcast_id = ?", podcastID).Order("position asc").Find(&links)
	return links, result.Error
}

// DeleteShowNoteLinksByPodcastItemID removes all links for an episode.
func DeleteShowNoteLinksByPodcastItemID(podcastItemID string) error {
	result := DB.Where("podcast_item_id = ?", podcastItemID).Delete(&ShowNoteLink{})
	return result.Error
}

// UpdateSettings handles the corresponding operation.
func UpdateSettings(setting *Setting) error {
	tx := DB.Save(&setting)
	return tx.Error
}

// GetOrCreateSetting handles the corresponding operation.
func GetOrCreateSetting() *Setting {
	var setting Setting
	result := DB.First(&setting)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		setting = Setting{
			RetentionKeepAll:          true,
			RetentionDeleteOnlyPlayed: true,
			InitialDownloadMode:       "count",
			ObsidianFolder:            DefaultObsidianFolder,
		}
		DB.Save(&setting)
		DB.First(&setting)
		return &setting
	}
	if !setting.RetentionKeepAll && setting.RetentionKeepLatest == 0 && setting.RetentionDeleteAfterDays == 0 && !setting.RetentionDeleteOnlyPlayed {
		setting.RetentionKeepAll = true
		setting.RetentionDeleteOnlyPlayed = true
		DB.Save(&setting)
	}
	if strings.TrimSpace(setting.InitialDownloadMode) == "" {
		setting.InitialDownloadMode = "count"
		DB.Save(&setting)
	}
	if strings.TrimSpace(setting.ObsidianFolder) == "" {
		setting.ObsidianFolder = DefaultObsidianFolder
		DB.Save(&setting)
	}
	return &setting
}

// GetLock returns the latest persisted lock row for a job name.
// If no row exists yet, it returns an unlocked in-memory lock value.
func GetLock(name string) *JobLock {
	var jobLock JobLock
	result := DB.Where("name = ?", name).Order("date desc").First(&jobLock)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return &JobLock{
			Name: name,
		}
	}
	return &jobLock
}

// Lock acquires or refreshes a named job lock atomically and returns
// the persisted canonical row so callers can safely unlock by ID.
func Lock(name string, duration int) *JobLock {
	now := time.Now().UTC()
	jobLock := &JobLock{
		Name:     name,
		Duration: duration,
		Date:     now,
	}
	DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "name"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"duration":   duration,
			"date":       now,
			"deleted_at": nil,
			"updated_at": now,
		}),
	}).Create(jobLock)

	// Reload canonical lock row so callers can safely unlock by ID.
	return GetLock(name)
}

// TryLock atomically acquires a named job lock when no active lease exists.
// It returns (lock, true, nil) when acquired, (currentLock, false, nil) when busy.
func TryLock(name string, duration int) (*JobLock, bool, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return nil, false, errors.New("lock name is required")
	}
	if duration <= 0 {
		return nil, false, errors.New("lock duration must be positive")
	}

	// Small retry window handles insert/update races between competing workers.
	for attempt := 0; attempt < 5; attempt++ {
		now := time.Now().UTC()

		var existing JobLock
		err := DB.Where("name = ?", trimmedName).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			candidate := &JobLock{
				Name:     trimmedName,
				Duration: duration,
				Date:     now,
			}
			createTx := DB.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "name"}},
				DoNothing: true,
			}).Create(candidate)
			if createTx.Error != nil {
				return nil, false, createTx.Error
			}
			if createTx.RowsAffected == 1 {
				return candidate, true, nil
			}
			continue
		}
		if err != nil {
			return nil, false, err
		}

		if existing.IsLockedAt(now) {
			return &existing, false, nil
		}

		result := DB.Model(&JobLock{}).
			Where("id = ? AND duration = ? AND date = ?", existing.ID, existing.Duration, existing.Date).
			Updates(map[string]interface{}{
				"duration":   duration,
				"date":       now,
				"deleted_at": nil,
				"updated_at": now,
			})
		if result.Error != nil {
			return nil, false, result.Error
		}
		if result.RowsAffected == 1 {
			existing.Duration = duration
			existing.Date = now
			existing.DeletedAt = nil
			return &existing, true, nil
		}
	}

	current := GetLock(trimmedName)
	return current, current != nil && current.IsLocked(), nil
}

// UnlockByID releases a specific lock row.
func UnlockByID(id string) {
	if id == "" {
		return
	}
	DB.Model(&JobLock{}).Where("id = ?", id).Updates(map[string]interface{}{
		"duration": 0,
		"date":     time.Time{},
	})
}

// RefreshLockByID renews a specific lock row lease.
func RefreshLockByID(id string, duration int) error {
	if id == "" {
		return nil
	}
	updates := map[string]interface{}{
		"date": time.Now().UTC(),
	}
	if duration > 0 {
		updates["duration"] = duration
	}
	return DB.Model(&JobLock{}).Where("id = ?", id).Updates(updates).Error
}

// Unlock releases all rows for a named lock.
func Unlock(name string) {
	DB.Model(&JobLock{}).Where("name = ?", name).Updates(map[string]interface{}{
		"duration": 0,
		"date":     time.Time{},
	})
}

// UnlockAllJobs unconditionally releases every lock. Call at process startup:
// no previous work can still be running in this process, so any held lock is
// stale (e.g. the container was restarted mid-transcription).
func UnlockAllJobs() {
	result := DB.Model(&JobLock{}).
		Where("duration > 0").
		Updates(map[string]interface{}{
			"duration": 0,
			"date":     time.Time{},
		})
	if result.Error != nil {
		logging.Sugar().Warnw("failed to clear job locks at startup", "error", result.Error)
		return
	}
	if result.RowsAffected > 0 {
		logging.Sugar().Infow("cleared stale job locks at startup", "count", result.RowsAffected)
	}
}

// UnlockMissedJobs clears stale lock rows whose lease has expired.
func UnlockMissedJobs() {
	var jobLocks []JobLock

	result := DB.Find(&jobLocks)
	if result.Error != nil {
		return
	}
	now := time.Now().UTC()
	for _, job := range jobLocks {
		if job.Date.IsZero() || job.Duration <= 0 {
			continue
		}
		duration := time.Duration(job.Duration)
		d := job.Date.Add(time.Minute * duration)
		if d.Before(now) {
			logging.Sugar().Infow("unlocking stale job lock", "job_name", job.Name)
			UnlockByID(job.ID)
		}
	}
}

// GetAllTags handles the corresponding operation.
func GetAllTags(sorting string) (*[]Tag, error) {
	var tags []Tag
	if sorting == "" {
		sorting = "created_at"
	}
	result := tagsWithAssociations(DB).Order(sorting).Find(&tags)
	return &tags, result.Error
}

// GetTagByID handles the corresponding operation.
func GetTagByID(id string) (*Tag, error) {
	var tag Tag
	result := tagsWithAssociations(DB).
		First(&tag, "id=?", id)

	return &tag, result.Error
}

// GetTagsByIds handles the corresponding operation.
func GetTagsByIds(ids []string) (*[]Tag, error) {
	var tag []Tag
	result := tagsWithAssociations(DB).Where("id in ?", ids).Find(&tag)

	return &tag, result.Error
}

// GetTagByLabel handles the corresponding operation.
func GetTagByLabel(label string) (*Tag, error) {
	var tag Tag
	result := tagsWithAssociations(DB).
		First(&tag, "label=?", label)

	return &tag, result.Error
}

// CreateTag handles the corresponding operation.
func CreateTag(tag *Tag) error {
	tx := DB.Omit("Podcasts").Create(&tag)
	return tx.Error
}

// UpdateTag handles the corresponding operation.
func UpdateTag(tag *Tag) error {
	tx := DB.Omit("Podcast").Save(&tag)
	return tx.Error
}

// AddTagToPodcast handles the corresponding operation.
func AddTagToPodcast(id, tagID string) error {
	tx := DB.Table("podcast_tags").Clauses(clause.OnConflict{
		DoNothing: true,
	}).Create(map[string]interface{}{
		"podcast_id": id,
		"tag_id":     tagID,
	})
	return tx.Error
}

// RemoveTagFromPodcast handles the corresponding operation.
func RemoveTagFromPodcast(id, tagID string) error {
	tx := DB.Exec("DELETE FROM podcast_tags WHERE podcast_id=? AND tag_id=?", id, tagID)
	return tx.Error
}

// UntagAllByTagID handles the corresponding operation.
func UntagAllByTagID(tagID string) error {
	tx := DB.Exec("DELETE FROM podcast_tags WHERE tag_id=?", tagID)
	return tx.Error
}

func CreatePromptVersion(promptType, content string) error {
	v := PromptVersion{PromptType: promptType, Content: content}
	return DB.Create(&v).Error
}

func GetPromptVersions(promptType string) ([]PromptVersion, error) {
	var versions []PromptVersion
	result := DB.Where("prompt_type = ?", promptType).Order("created_at DESC").Find(&versions)
	return versions, result.Error
}

func GetPromptVersionByID(id string) (*PromptVersion, error) {
	var v PromptVersion
	result := DB.First(&v, "id = ?", id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &v, nil
}
