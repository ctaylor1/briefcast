package db

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	glebarezsqlite "github.com/glebarez/sqlite"
	"github.com/ctaylor1/briefcast/model"
	"gorm.io/gorm"
)

func setupDBForTest(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()
	t.Setenv("CONFIG", tempDir)
	t.Setenv("DATA", filepath.Join(tempDir, "assets"))
	t.Setenv("DATABASE_URL", filepath.Join(tempDir, "briefcast.db"))

	var err error
	DB, err = Init()
	if err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") || strings.Contains(err.Error(), "requires cgo") {
			DB, err = gorm.Open(glebarezsqlite.Open(filepath.Join(tempDir, "briefcast.db")), &gorm.Config{})
			if err != nil {
				t.Fatalf("db init fallback failed: %v", err)
			}
			activeDriver = DriverSQLite
		} else {
			t.Fatalf("db init failed: %v", err)
		}
	}
	sqlDB, sqlErr := DB.DB()
	if sqlErr == nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}
	Migrate()
	return tempDir
}

func newPodcast(t *testing.T, title, url string) Podcast {
	t.Helper()
	podcast := Podcast{
		Title:   title,
		URL:     url,
		Author:  "Author " + title,
		Summary: "Summary " + title,
	}
	if err := CreatePodcast(&podcast); err != nil {
		t.Fatalf("create podcast failed: %v", err)
	}
	return podcast
}

func newPodcastItem(t *testing.T, podcastID, guid, title string, status DownloadStatus, pubDate time.Time) PodcastItem {
	t.Helper()
	item := PodcastItem{
		PodcastID:      podcastID,
		GUID:           guid,
		Title:          title,
		Summary:        "Summary " + title,
		SummaryHTML:    "<p>Summary " + title + "</p>",
		FileURL:        "https://example.com/" + guid + ".mp3",
		DownloadStatus: status,
		PubDate:        pubDate,
		DownloadDate:   pubDate,
	}
	if err := CreatePodcastItem(&item); err != nil {
		t.Fatalf("create podcast item failed: %v", err)
	}
	return item
}

func TestPodcastAndItemQueries(t *testing.T) {
	setupDBForTest(t)
	now := time.Now().UTC()

	podcastA := newPodcast(t, "Podcast A", "https://example.com/a.xml")
	podcastB := newPodcast(t, "Podcast B", "https://example.com/b.xml")

	itemA1 := newPodcastItem(t, podcastA.ID, "a-1", "Episode A1", Downloaded, now.Add(-2*time.Hour))
	itemA1.FileSize = 111
	itemA1.Image = "https://example.com/a1.jpg"
	itemA1.ChaptersJSON = `{"chapters":[{"title":"Topic match","startTime":5}]}`
	itemA1.TranscriptJSON = `{"segments":[{"start":12,"text":"keyword transcript"}]}`
	if err := UpdatePodcastItem(&itemA1); err != nil {
		t.Fatalf("update itemA1 failed: %v", err)
	}

	itemA2 := newPodcastItem(t, podcastA.ID, "a-2", "Episode A2", NotDownloaded, now.Add(-1*time.Hour))
	itemA3 := newPodcastItem(t, podcastA.ID, "a-3", "Episode A3", Deleted, now.Add(-30*time.Minute))
	itemB1 := newPodcastItem(t, podcastB.ID, "b-1", "Episode B1", Paused, now.Add(-90*time.Minute))
	itemB1.IsPlayed = true
	if err := UpdatePodcastItem(&itemB1); err != nil {
		t.Fatalf("update itemB1 failed: %v", err)
	}

	var byURL Podcast
	if err := GetPodcastByURL(podcastA.URL, &byURL); err != nil {
		t.Fatalf("GetPodcastByURL failed: %v", err)
	}
	if byURL.ID != podcastA.ID {
		t.Fatalf("expected podcast A id, got %q", byURL.ID)
	}

	var byURLList []Podcast
	if err := GetPodcastsByURLList([]string{podcastA.URL, podcastB.URL}, &byURLList); err != nil {
		t.Fatalf("GetPodcastsByURLList failed: %v", err)
	}
	if len(byURLList) == 0 {
		t.Fatalf("expected at least one podcast in URL list query")
	}

	var allPodcasts []Podcast
	if err := GetAllPodcasts(&allPodcasts, "title asc"); err != nil {
		t.Fatalf("GetAllPodcasts failed: %v", err)
	}
	if len(allPodcasts) != 2 {
		t.Fatalf("expected 2 podcasts, got %d", len(allPodcasts))
	}

	var allItems []PodcastItem
	if err := GetAllPodcastItems(&allItems); err != nil {
		t.Fatalf("GetAllPodcastItems failed: %v", err)
	}
	if len(allItems) != 4 {
		t.Fatalf("expected 4 podcast items, got %d", len(allItems))
	}

	withoutSize, err := GetAllPodcastItemsWithoutSize()
	if err != nil {
		t.Fatalf("GetAllPodcastItemsWithoutSize failed: %v", err)
	}
	if len(*withoutSize) == 0 {
		t.Fatalf("expected at least one item without size")
	}

	var byID PodcastItem
	if err := GetPodcastItemById(itemA1.ID, &byID); err != nil {
		t.Fatalf("GetPodcastItemById failed: %v", err)
	}
	if byID.ID != itemA1.ID {
		t.Fatalf("expected itemA1, got %q", byID.ID)
	}

	var byPodcastID []PodcastItem
	if err := GetAllPodcastItemsByPodcastId(podcastA.ID, &byPodcastID); err != nil {
		t.Fatalf("GetAllPodcastItemsByPodcastId failed: %v", err)
	}
	if len(byPodcastID) != 3 {
		t.Fatalf("expected 3 items for podcast A, got %d", len(byPodcastID))
	}

	var byPodcastIDs []PodcastItem
	if err := GetAllPodcastItemsByPodcastIds([]string{podcastA.ID, podcastB.ID}, &byPodcastIDs); err != nil {
		t.Fatalf("GetAllPodcastItemsByPodcastIds failed: %v", err)
	}
	if len(byPodcastIDs) != 4 {
		t.Fatalf("expected 4 items for both podcasts, got %d", len(byPodcastIDs))
	}

	ordered, err := GetAllPodcastItemsByIds([]string{itemA2.ID, itemA1.ID})
	if err != nil {
		t.Fatalf("GetAllPodcastItemsByIds failed: %v", err)
	}
	if len(*ordered) != 2 || (*ordered)[0].ID != itemA2.ID {
		t.Fatalf("expected custom ordering from GetAllPodcastItemsByIds, got %+v", *ordered)
	}

	byGUIDs, err := GetPodcastItemsByPodcastIdAndGUIDs(podcastA.ID, []string{"a-1", "a-2"})
	if err != nil {
		t.Fatalf("GetPodcastItemsByPodcastIdAndGUIDs failed: %v", err)
	}
	if len(*byGUIDs) != 2 {
		t.Fatalf("expected 2 guid matches, got %d", len(*byGUIDs))
	}

	var byGUID PodcastItem
	if err := GetPodcastItemByPodcastIdAndGUID(podcastA.ID, "a-1", &byGUID); err != nil {
		t.Fatalf("GetPodcastItemByPodcastIdAndGUID failed: %v", err)
	}
	if byGUID.ID != itemA1.ID {
		t.Fatalf("expected guid a-1 to map to itemA1")
	}

	if err := UpdatePodcastItemFileSize(itemA2.ID, 222); err != nil {
		t.Fatalf("UpdatePodcastItemFileSize failed: %v", err)
	}
	if err := UpdatePodcastItemDownloadProgress(itemA2.ID, 12, 34); err != nil {
		t.Fatalf("UpdatePodcastItemDownloadProgress failed: %v", err)
	}
	var refreshedA2 PodcastItem
	if err := GetPodcastItemById(itemA2.ID, &refreshedA2); err != nil {
		t.Fatalf("reload itemA2 failed: %v", err)
	}
	if refreshedA2.FileSize != 222 || refreshedA2.DownloadedBytes != 12 || refreshedA2.DownloadTotalBytes != 34 {
		t.Fatalf("unexpected itemA2 progress/size %+v", refreshedA2)
	}

	queued, err := GetAllPodcastItemsToBeDownloaded()
	if err != nil {
		t.Fatalf("GetAllPodcastItemsToBeDownloaded failed: %v", err)
	}
	if len(*queued) != 1 || (*queued)[0].ID != itemA2.ID {
		t.Fatalf("expected only itemA2 queued, got %+v", *queued)
	}

	downloaded, err := GetAllPodcastItemsAlreadyDownloaded()
	if err != nil {
		t.Fatalf("GetAllPodcastItemsAlreadyDownloaded failed: %v", err)
	}
	if len(*downloaded) != 1 || (*downloaded)[0].ID != itemA1.ID {
		t.Fatalf("expected only itemA1 downloaded, got %+v", *downloaded)
	}

	queueByStatus, err := GetPodcastItemsByDownloadStatuses([]DownloadStatus{NotDownloaded, Paused}, 0)
	if err != nil {
		t.Fatalf("GetPodcastItemsByDownloadStatuses failed: %v", err)
	}
	if len(queueByStatus) != 2 {
		t.Fatalf("expected 2 items by status, got %d", len(queueByStatus))
	}

	stats, err := GetPodcastEpisodeStats()
	if err != nil {
		t.Fatalf("GetPodcastEpisodeStats failed: %v", err)
	}
	if len(*stats) == 0 {
		t.Fatalf("expected episode stats entries")
	}

	diskStats, err := GetPodcastEpisodeDiskStats()
	if err != nil {
		t.Fatalf("GetPodcastEpisodeDiskStats failed: %v", err)
	}
	if diskStats.Downloaded != 111 {
		t.Fatalf("expected downloaded size 111, got %d", diskStats.Downloaded)
	}

	seq, err := GetEpisodeNumber(itemA2.ID, podcastA.ID)
	if err != nil {
		t.Fatalf("GetEpisodeNumber failed: %v", err)
	}
	if seq != 2 {
		t.Fatalf("expected episode sequence 2, got %d", seq)
	}

	ForceSetLastEpisodeDate(podcastA.ID)
	var refreshedPodcastA Podcast
	if err := GetPodcastById(podcastA.ID, &refreshedPodcastA); err != nil {
		t.Fatalf("GetPodcastById failed: %v", err)
	}
	if refreshedPodcastA.LastEpisode == nil {
		t.Fatalf("expected last episode date to be set")
	}

	if err := TogglePodcastPauseStatus(podcastA.ID, true); err != nil {
		t.Fatalf("TogglePodcastPauseStatus failed: %v", err)
	}
	if err := GetPodcastById(podcastA.ID, &refreshedPodcastA); err != nil {
		t.Fatalf("reload podcast A failed: %v", err)
	}
	if !refreshedPodcastA.IsPaused {
		t.Fatalf("expected podcast A paused")
	}

	if err := SetAllEpisodesToDownload(podcastA.ID); err != nil {
		t.Fatalf("SetAllEpisodesToDownload failed: %v", err)
	}
	var refreshedA3 PodcastItem
	if err := GetPodcastItemById(itemA3.ID, &refreshedA3); err != nil {
		t.Fatalf("reload itemA3 failed: %v", err)
	}
	if refreshedA3.DownloadStatus != NotDownloaded {
		t.Fatalf("expected deleted item to become queued, got %v", refreshedA3.DownloadStatus)
	}

	downloadedOnly := "true"
	filter := model.EpisodesFilter{
		Pagination: model.Pagination{
			Page:  1,
			Count: 10,
		},
		Sorting:      model.RELEASE_DESC,
		IsDownloaded: &downloadedOnly,
	}
	filter.VerifyPaginationValues()
	filtered, total, err := GetPaginatedPodcastItemsNew(filter)
	if err != nil {
		t.Fatalf("GetPaginatedPodcastItemsNew failed: %v", err)
	}
	if total == 0 || len(*filtered) == 0 {
		t.Fatalf("expected non-empty filtered results")
	}

	var legacy []PodcastItem
	var legacyTotal int64
	if err := GetPaginatedPodcastItems(1, 10, nil, nil, time.Time{}, &legacy, &legacyTotal); err != nil {
		t.Fatalf("GetPaginatedPodcastItems failed: %v", err)
	}
	if legacyTotal == 0 || len(legacy) == 0 {
		t.Fatalf("expected non-empty legacy pagination results")
	}
}

func TestSettingsLocksTagsAndSearchHelpers(t *testing.T) {
	setupDBForTest(t)

	podcast := newPodcast(t, "Podcast Search", "https://example.com/search.xml")
	item := newPodcastItem(t, podcast.ID, "s-1", "Episode Search", Downloaded, time.Now().UTC())
	item.ChaptersJSON = `{"chapters":[{"title":"Keyword chapter","startTime":3}]}`
	item.TranscriptJSON = `{"segments":[{"start":1,"text":"keyword transcript"}]}`
	item.FileSize = 321
	if err := UpdatePodcastItem(&item); err != nil {
		t.Fatalf("update item failed: %v", err)
	}

	setting := GetOrCreateSetting()
	if !setting.RetentionKeepAll || !setting.RetentionDeleteOnlyPlayed {
		t.Fatalf("expected default retention settings to be initialized")
	}
	setting.RetentionKeepLatest = 2
	if err := UpdateSettings(setting); err != nil {
		t.Fatalf("UpdateSettings failed: %v", err)
	}
	updatedSetting := GetOrCreateSetting()
	if updatedSetting.RetentionKeepLatest != 2 {
		t.Fatalf("expected retention keep latest 2, got %d", updatedSetting.RetentionKeepLatest)
	}

	lock := GetLock("job")
	if lock.IsLocked() {
		t.Fatalf("expected new lock to be unlocked")
	}
	Lock("job", 1)
	lock = GetLock("job")
	if !lock.IsLocked() {
		t.Fatalf("expected lock to be set")
	}
	lock.Date = time.Now().Add(-5 * time.Minute)
	if err := DB.Save(lock).Error; err != nil {
		t.Fatalf("failed to age lock: %v", err)
	}
	UnlockMissedJobs()
	lock = GetLock("job")
	if lock.IsLocked() {
		t.Fatalf("expected stale lock to be unlocked")
	}
	Unlock("job")

	tag := Tag{Label: "tech", Description: "technology"}
	if err := CreateTag(&tag); err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}
	foundTag, err := GetTagByLabel("tech")
	if err != nil {
		t.Fatalf("GetTagByLabel failed: %v", err)
	}
	if foundTag.ID != tag.ID {
		t.Fatalf("expected same tag id")
	}
	if err := AddTagToPodcast(podcast.ID, tag.ID); err != nil {
		t.Fatalf("AddTagToPodcast failed: %v", err)
	}
	tagByID, err := GetTagById(tag.ID)
	if err != nil {
		t.Fatalf("GetTagById failed: %v", err)
	}
	if len(tagByID.Podcasts) != 1 {
		t.Fatalf("expected one linked podcast, got %d", len(tagByID.Podcasts))
	}
	allTags, err := GetAllTags("created_at asc")
	if err != nil {
		t.Fatalf("GetAllTags failed: %v", err)
	}
	if len(*allTags) != 1 {
		t.Fatalf("expected one tag, got %d", len(*allTags))
	}
	tagsByIDs, err := GetTagsByIds([]string{tag.ID})
	if err != nil {
		t.Fatalf("GetTagsByIds failed: %v", err)
	}
	if len(*tagsByIDs) != 1 {
		t.Fatalf("expected one tag by ids, got %d", len(*tagsByIDs))
	}
	tag.Description = "updated"
	if err := UpdateTag(&tag); err != nil {
		t.Fatalf("UpdateTag failed: %v", err)
	}
	if err := RemoveTagFromPodcast(podcast.ID, tag.ID); err != nil {
		t.Fatalf("RemoveTagFromPodcast failed: %v", err)
	}
	if err := UntagAllByTagId(tag.ID); err != nil {
		t.Fatalf("UntagAllByTagId failed: %v", err)
	}

	var podcasts []Podcast
	if err := FindAllPodcastsPlain(&podcasts); err != nil {
		t.Fatalf("FindAllPodcastsPlain failed: %v", err)
	}
	if len(podcasts) != 1 {
		t.Fatalf("expected one podcast, got %d", len(podcasts))
	}

	var downloadedItems []PodcastItem
	if err := FindDownloadedPodcastItems(&downloadedItems); err != nil {
		t.Fatalf("FindDownloadedPodcastItems failed: %v", err)
	}
	if len(downloadedItems) != 1 {
		t.Fatalf("expected one downloaded item, got %d", len(downloadedItems))
	}

	var podcastSearch []Podcast
	if err := SearchPodcastsByLike("%search%", 10, &podcastSearch); err != nil {
		t.Fatalf("SearchPodcastsByLike failed: %v", err)
	}
	if len(podcastSearch) != 1 {
		t.Fatalf("expected one podcast search hit, got %d", len(podcastSearch))
	}

	var itemSearch []PodcastItem
	if err := SearchPodcastItemsByLike("%keyword%", 10, &itemSearch); err != nil {
		t.Fatalf("SearchPodcastItemsByLike failed: %v", err)
	}
	if len(itemSearch) != 1 {
		t.Fatalf("expected one item search hit, got %d", len(itemSearch))
	}

	pending := newPodcastItem(t, podcast.ID, "s-2", "Pending transcript", Downloaded, time.Now().UTC())
	pending.DownloadPath = filepath.Join(t.TempDir(), "audio.mp3")
	pending.TranscriptStatus = "pending_whisperx"
	pending.TranscriptJSON = ""
	if err := UpdatePodcastItem(&pending); err != nil {
		t.Fatalf("failed to create pending transcript item: %v", err)
	}

	itemsForWhisperx, err := GetPodcastItemsForWhisperx([]string{"pending_whisperx"}, 10)
	if err != nil {
		t.Fatalf("GetPodcastItemsForWhisperx failed: %v", err)
	}
	if len(*itemsForWhisperx) != 1 {
		t.Fatalf("expected one item for whisperx, got %d", len(*itemsForWhisperx))
	}

	if err := DeletePodcastItemById(item.ID); err != nil {
		t.Fatalf("DeletePodcastItemById failed: %v", err)
	}
	if err := DeleteTagById(tag.ID); err != nil {
		t.Fatalf("DeleteTagById failed: %v", err)
	}
	if err := DeletePodcastById(podcast.ID); err != nil {
		t.Fatalf("DeletePodcastById failed: %v", err)
	}
}
