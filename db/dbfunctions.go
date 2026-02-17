package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ctaylor1/briefcast/internal/logging"
	"github.com/ctaylor1/briefcast/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func GetPodcastByURL(url string, podcast *Podcast) error {
	result := podcastsWithAssociations(DB).Where(&Podcast{URL: url}).First(&podcast)
	return result.Error
}

func GetPodcastsByURLList(urls []string, podcasts *[]Podcast) error {
	result := podcastsWithAssociations(DB).Where("url in ?", urls).First(&podcasts)
	return result.Error
}
func GetAllPodcasts(podcasts *[]Podcast, sorting string) error {
	if sorting == "" {
		sorting = "created_at"
	}
	result := podcastsWithTags(DB).Order(sorting).Find(&podcasts)
	return result.Error
}
func GetAllPodcastItems(podcasts *[]PodcastItem) error {
	result := podcastItemsWithPodcast(DB).Order("pub_date desc").Find(&podcasts)
	return result.Error
}
func GetAllPodcastItemsWithoutSize() (*[]PodcastItem, error) {
	var podcasts []PodcastItem
	result := DB.Where("file_size<=?", 0).Order("pub_date desc").Find(&podcasts)
	return &podcasts, result.Error
}

func getSortOrder(sorting model.EpisodeSort) string {
	switch sorting {
	case model.RELEASE_ASC:
		return "pub_date asc"
	case model.RELEASE_DESC:
		return "pub_date desc"
	case model.DURATION_ASC:
		return "duration asc"
	case model.DURATION_DESC:
		return "duration desc"
	default:
		return "pub_date desc"
	}
}

func GetPaginatedPodcastItemsNew(queryModel model.EpisodesFilter) (*[]PodcastItem, int64, error) {
	var podcasts []PodcastItem
	var total int64
	query := podcastItemsWithPodcast(DB.Debug())
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

	if queryModel.Q != "" {
		query = query.Where("UPPER(title) like ?", "%"+strings.TrimSpace(strings.ToUpper(queryModel.Q))+"%")
	}

	if len(queryModel.TagIds) > 0 {
		query = query.Where("podcast_id in (select podcast_id from podcast_tags where tag_id in ?)", queryModel.TagIds)
	}

	if len(queryModel.PodcastIds) > 0 {
		query = query.Where("podcast_id in ?", queryModel.PodcastIds)
	}

	totalsQuery := query.Order(getSortOrder(queryModel.Sorting)).Find(&podcasts)
	totalsQuery.Count(&total)

	result := query.Limit(queryModel.Count).Offset((queryModel.Page - 1) * queryModel.Count).Order("pub_date desc").Find(&podcasts)
	return &podcasts, total, result.Error
}

func GetPaginatedPodcastItems(page int, count int, downloadedOnly *bool, playedOnly *bool, fromDate time.Time, podcasts *[]PodcastItem, total *int64) error {
	query := podcastItemsWithPodcast(DB)
	query = applyDownloadStatusFilter(query, downloadedOnly)
	query = applyPlayedStatusFilter(query, playedOnly)
	if (fromDate != time.Time{}) {
		query = query.Where("pub_date>=?", fromDate)
	}

	totalsQuery := query.Order("pub_date desc").Find(&podcasts)
	totalsQuery.Count(total)

	result := query.Limit(count).Offset((page - 1) * count).Order("pub_date desc").Find(&podcasts)
	return result.Error
}
func GetPaginatedTags(page int, count int, tags *[]Tag, total *int64) error {
	query := tagsWithPodcasts(DB)

	result := query.Limit(count).Offset((page - 1) * count).Order("created_at desc").Find(&tags)

	query.Count(total)

	return result.Error
}
func GetPodcastById(id string, podcast *Podcast) error {

	result := DB.Preload("PodcastItems", func(db *gorm.DB) *gorm.DB {
		return db.Order("podcast_items.pub_date DESC")
	}).First(&podcast, "id=?", id)
	return result.Error
}

func GetPodcastItemById(id string, podcastItem *PodcastItem) error {

	result := podcastItemsWithAssociations(DB).First(&podcastItem, "id=?", id)
	return result.Error
}
func DeletePodcastItemById(id string) error {

	result := DB.Where("id=?", id).Delete(&PodcastItem{})
	return result.Error
}
func DeletePodcastById(id string) error {

	result := DB.Where("id=?", id).Delete(&Podcast{})
	return result.Error
}

func DeleteTagById(id string) error {

	result := DB.Where("id=?", id).Delete(&Tag{})
	return result.Error
}

func GetAllPodcastItemsByPodcastId(podcastId string, podcastItems *[]PodcastItem) error {

	result := podcastItemsWithAssociations(DB).Where(&PodcastItem{PodcastID: podcastId}).Find(&podcastItems)
	return result.Error
}
func GetAllPodcastItemsByPodcastIds(podcastIds []string, podcastItems *[]PodcastItem) error {

	result := podcastItemsWithAssociations(DB).Where("podcast_id in ?", podcastIds).Order("pub_date desc").Find(&podcastItems)
	return result.Error
}
func GetAllPodcastItemsByIds(podcastItemIds []string) (*[]PodcastItem, error) {
	var podcastItems []PodcastItem

	var sb strings.Builder

	sb.WriteString("\n CASE ID \n")

	for i, v := range podcastItemIds {
		sb.WriteString(fmt.Sprintf("WHEN '%v' THEN %v \n", v, i+1))
	}

	sb.WriteString(fmt.Sprintln("END"))

	result := podcastItemsWithAssociations(DB.Debug()).Where("id in ?", podcastItemIds).Order(sb.String()).Find(&podcastItems)
	return &podcastItems, result.Error
}

func SetAllEpisodesToDownload(podcastId string) error {
	result := DB.Model(PodcastItem{}).Where(&PodcastItem{PodcastID: podcastId, DownloadStatus: Deleted}).Update("download_status", NotDownloaded)
	return result.Error
}
func UpdateLastEpisodeDateForPodcast(podcastId string, lastEpisode time.Time) error {
	result := DB.Model(Podcast{}).Where("id=?", podcastId).Update("last_episode", lastEpisode)
	return result.Error
}

func UpdatePodcastItemFileSize(podcastItemId string, size int64) error {
	result := DB.Model(PodcastItem{}).Where("id=?", podcastItemId).Update("file_size", size)
	return result.Error
}

func GetAllPodcastItemsWithoutImage() (*[]PodcastItem, error) {
	var podcastItems []PodcastItem
	result := podcastItemsWithAssociations(DB).Where("local_image is ?", nil).Where("image != ?", "").Where("download_status=?", Downloaded).Order("created_at desc").Find(&podcastItems)
	//fmt.Println("To be downloaded : " + string(len(podcastItems)))
	return &podcastItems, result.Error
}

func GetAllPodcastItemsToBeDownloaded() (*[]PodcastItem, error) {
	var podcastItems []PodcastItem
	result := podcastItemsWithAssociations(DB).Where("download_status=?", NotDownloaded).Find(&podcastItems)
	//fmt.Println("To be downloaded : " + string(len(podcastItems)))
	return &podcastItems, result.Error
}
func GetAllPodcastItemsAlreadyDownloaded() (*[]PodcastItem, error) {
	var podcastItems []PodcastItem
	result := podcastItemsWithAssociations(DB).Where("download_status=?", Downloaded).Find(&podcastItems)
	return &podcastItems, result.Error
}

func GetPodcastItemsForWhisperx(statuses []string, limit int) (*[]PodcastItem, error) {
	var podcastItems []PodcastItem
	query := DB.Where("download_status=?", Downloaded).
		Where("transcript_status IN ?", statuses).
		Where("download_path <> ''").
		Where("(transcript_json IS NULL OR transcript_json = '')").
		Order("download_date asc")
	if limit > 0 {
		query = query.Limit(limit)
	}
	result := query.Find(&podcastItems)
	return &podcastItems, result.Error
}

func GetPodcastItemsByDownloadStatuses(statuses []DownloadStatus, limit int) ([]PodcastItem, error) {
	var podcastItems []PodcastItem
	query := podcastItemsWithAssociations(DB).
		Where("download_status IN ?", statuses).
		Order("download_date desc")
	if limit > 0 {
		query = query.Limit(limit)
	}
	result := query.Find(&podcastItems)
	return podcastItems, result.Error
}

func UpdatePodcastItemDownloadProgress(podcastItemId string, downloadedBytes int64, totalBytes int64) error {
	updates := map[string]interface{}{
		"downloaded_bytes": downloadedBytes,
	}
	if totalBytes > 0 {
		updates["download_total_bytes"] = totalBytes
	}
	return DB.Model(&PodcastItem{}).Where("id=?", podcastItemId).Updates(updates).Error
}

func GetPodcastEpisodeStats() (*[]PodcastItemStatsModel, error) {
	var stats []PodcastItemStatsModel
	result := DB.Model(&PodcastItem{}).Select("download_status,podcast_id, count(1) as count,sum(file_size) as size").Group("podcast_id,download_status").Find(&stats)
	return &stats, result.Error
}

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

func GetEpisodeNumber(podcastItemId, podcastId string) (int, error) {
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
	`, podcastId, podcastItemId).Row()
	error := row.Scan(&id, &sequence)
	return sequence, error
}

func ForceSetLastEpisodeDate(podcastId string) {
	DB.Exec("update podcasts set last_episode = (select max(pi.pub_date) from podcast_items pi where pi.podcast_id = @id) where id = @id", sql.Named("id", podcastId))
}

func TogglePodcastPauseStatus(podcastId string, isPaused bool) error {

	tx := DB.Debug().Exec("update podcasts set is_paused = @isPaused where id = @id", sql.Named("id", podcastId), sql.Named("isPaused", isPaused))
	return tx.Error
}

func GetPodcastItemsByPodcastIdAndGUIDs(podcastId string, guids []string) (*[]PodcastItem, error) {
	var podcastItems []PodcastItem
	result := podcastItemsWithAssociations(DB).Where(&PodcastItem{PodcastID: podcastId}).Where("guid IN ?", guids).Find(&podcastItems)
	return &podcastItems, result.Error
}
func GetPodcastItemByPodcastIdAndGUID(podcastId string, guid string, podcastItem *PodcastItem) error {

	result := podcastItemsWithAssociations(DB).Where(&PodcastItem{PodcastID: podcastId, GUID: guid}).First(&podcastItem)
	return result.Error
}
func GetPodcastByTitleAndAuthor(title string, author string, podcast *Podcast) error {

	result := podcastsWithAssociations(DB).Where(&Podcast{Title: title, Author: author}).First(&podcast)
	return result.Error
}

func CreatePodcast(podcast *Podcast) error {
	tx := DB.Create(&podcast)
	return tx.Error
}

func CreatePodcastItem(podcastItem *PodcastItem) error {
	tx := DB.Omit("Podcast").Create(&podcastItem)
	return tx.Error
}

func UpdatePodcast(podcast *Podcast) error {
	tx := DB.Save(&podcast)
	return tx.Error
}
func UpdatePodcastItem(podcastItem *PodcastItem) error {
	tx := DB.Omit("Podcast").Save(&podcastItem)
	return tx.Error
}
func UpdateSettings(setting *Setting) error {
	tx := DB.Save(&setting)
	return tx.Error
}
func GetOrCreateSetting() *Setting {
	var setting Setting
	result := DB.First(&setting)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		setting = Setting{
			RetentionKeepAll:          true,
			RetentionDeleteOnlyPlayed: true,
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
	return &setting
}

func GetLock(name string) *JobLock {
	var jobLock JobLock
	result := DB.Where("name = ?", name).First(&jobLock)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return &JobLock{
			Name: name,
		}
	}
	return &jobLock
}
func Lock(name string, duration int) {
	jobLock := GetLock(name)
	if jobLock == nil {
		jobLock = &JobLock{
			Name: name,
		}
	}
	jobLock.Duration = duration
	jobLock.Date = time.Now().UTC()
	if jobLock.ID == "" {
		DB.Create(&jobLock)
	} else {
		DB.Save(&jobLock)
	}
}
func Unlock(name string) {
	jobLock := GetLock(name)
	if jobLock == nil {
		return
	}
	jobLock.Duration = 0
	jobLock.Date = time.Time{}
	DB.Save(&jobLock)
}

func UnlockMissedJobs() {
	var jobLocks []JobLock

	result := DB.Find(&jobLocks)
	if result.Error != nil {
		return
	}
	for _, job := range jobLocks {
		if (job.Date == time.Time{}) {
			continue
		}
		var duration time.Duration
		duration = time.Duration(job.Duration)
		d := job.Date.Add(time.Minute * duration)
		if d.Before(time.Now().UTC()) {
			logging.Sugar().Infow("unlocking stale job lock", "job_name", job.Name)
			Unlock(job.Name)
		}
	}
}

func GetAllTags(sorting string) (*[]Tag, error) {
	var tags []Tag
	if sorting == "" {
		sorting = "created_at"
	}
	result := tagsWithAssociations(DB).Order(sorting).Find(&tags)
	return &tags, result.Error
}

func GetTagById(id string) (*Tag, error) {
	var tag Tag
	result := tagsWithAssociations(DB).
		First(&tag, "id=?", id)

	return &tag, result.Error
}
func GetTagsByIds(ids []string) (*[]Tag, error) {
	var tag []Tag
	result := tagsWithAssociations(DB).Where("id in ?", ids).Find(&tag)

	return &tag, result.Error
}
func GetTagByLabel(label string) (*Tag, error) {
	var tag Tag
	result := tagsWithAssociations(DB).
		First(&tag, "label=?", label)

	return &tag, result.Error
}

func CreateTag(tag *Tag) error {
	tx := DB.Omit("Podcasts").Create(&tag)
	return tx.Error
}
func UpdateTag(tag *Tag) error {
	tx := DB.Omit("Podcast").Save(&tag)
	return tx.Error
}
func AddTagToPodcast(id, tagId string) error {
	tx := DB.Table("podcast_tags").Clauses(clause.OnConflict{
		DoNothing: true,
	}).Create(map[string]interface{}{
		"podcast_id": id,
		"tag_id":     tagId,
	})
	return tx.Error
}
func RemoveTagFromPodcast(id, tagId string) error {
	tx := DB.Exec("DELETE FROM podcast_tags WHERE podcast_id=? AND tag_id=?", id, tagId)
	return tx.Error
}

func UntagAllByTagId(tagId string) error {
	tx := DB.Exec("DELETE FROM podcast_tags WHERE tag_id=?", tagId)
	return tx.Error
}
