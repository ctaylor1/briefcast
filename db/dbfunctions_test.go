package db

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/model"
	glebarezsqlite "github.com/glebarez/sqlite"
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

// TestPodcastAndItemQueries handles the corresponding operation.
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
	if err := GetPodcastItemByID(itemA1.ID, &byID); err != nil {
		t.Fatalf("GetPodcastItemByID failed: %v", err)
	}
	if byID.ID != itemA1.ID {
		t.Fatalf("expected itemA1, got %q", byID.ID)
	}

	var byPodcastID []PodcastItem
	if err := GetAllPodcastItemsByPodcastID(podcastA.ID, &byPodcastID); err != nil {
		t.Fatalf("GetAllPodcastItemsByPodcastID failed: %v", err)
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

	byGUIDs, err := GetPodcastItemsByPodcastIDAndGUIDs(podcastA.ID, []string{"a-1", "a-2"})
	if err != nil {
		t.Fatalf("GetPodcastItemsByPodcastIDAndGUIDs failed: %v", err)
	}
	if len(*byGUIDs) != 2 {
		t.Fatalf("expected 2 guid matches, got %d", len(*byGUIDs))
	}

	var byGUID PodcastItem
	if err := GetPodcastItemByPodcastIDAndGUID(podcastA.ID, "a-1", &byGUID); err != nil {
		t.Fatalf("GetPodcastItemByPodcastIDAndGUID failed: %v", err)
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
	if err := GetPodcastItemByID(itemA2.ID, &refreshedA2); err != nil {
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

	orderedStatuses, err := GetPodcastItemsByDownloadStatuses([]DownloadStatus{NotDownloaded, Paused, Downloaded}, 0)
	if err != nil {
		t.Fatalf("GetPodcastItemsByDownloadStatuses ordered query failed: %v", err)
	}
	if len(orderedStatuses) != 3 {
		t.Fatalf("expected 3 items by ordered status query, got %d", len(orderedStatuses))
	}
	if orderedStatuses[0].DownloadStatus != NotDownloaded {
		t.Fatalf("expected first item to be queued, got status %v", orderedStatuses[0].DownloadStatus)
	}
	if orderedStatuses[1].DownloadStatus != Paused {
		t.Fatalf("expected second item to be paused, got status %v", orderedStatuses[1].DownloadStatus)
	}
	if orderedStatuses[2].DownloadStatus != Downloaded {
		t.Fatalf("expected third item to be downloaded, got status %v", orderedStatuses[2].DownloadStatus)
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
	if err := GetPodcastByID(podcastA.ID, &refreshedPodcastA); err != nil {
		t.Fatalf("GetPodcastByID failed: %v", err)
	}
	if refreshedPodcastA.LastEpisode == nil {
		t.Fatalf("expected last episode date to be set")
	}

	if err := TogglePodcastPauseStatus(podcastA.ID, true); err != nil {
		t.Fatalf("TogglePodcastPauseStatus failed: %v", err)
	}
	if err := GetPodcastByID(podcastA.ID, &refreshedPodcastA); err != nil {
		t.Fatalf("reload podcast A failed: %v", err)
	}
	if !refreshedPodcastA.IsPaused {
		t.Fatalf("expected podcast A paused")
	}
	if err := TogglePodcastPauseStatus(podcastA.ID, true); err != nil {
		t.Fatalf("idempotent TogglePodcastPauseStatus pause failed: %v", err)
	}
	if err := TogglePodcastPauseStatus(podcastA.ID, false); err != nil {
		t.Fatalf("TogglePodcastPauseStatus unpause failed: %v", err)
	}
	if err := TogglePodcastPauseStatus(podcastA.ID, false); err != nil {
		t.Fatalf("idempotent TogglePodcastPauseStatus unpause failed: %v", err)
	}

	if err := SetAllEpisodesToDownload(podcastA.ID); err != nil {
		t.Fatalf("SetAllEpisodesToDownload failed: %v", err)
	}
	var refreshedA3 PodcastItem
	if err := GetPodcastItemByID(itemA3.ID, &refreshedA3); err != nil {
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
		Sorting:      model.ReleaseDesc,
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

func TestEpisodeFavoriteFiltersAndSummaryFavoriteSync(t *testing.T) {
	setupDBForTest(t)
	now := time.Now().UTC()
	podcast := newPodcast(t, "Favorite Podcast", "https://example.com/favorites.xml")

	bookmarked := newPodcastItem(t, podcast.ID, "fav-1", "Bookmarked Episode", Downloaded, now.Add(-3*time.Hour))
	bookmarked.BookmarkDate = now
	bookmarked.LLMSummary = "bookmarked summary"
	bookmarked.LLMSummaryStatus = "available"
	if err := UpdatePodcastItem(&bookmarked); err != nil {
		t.Fatalf("update bookmarked item failed: %v", err)
	}

	summaryFavorited := newPodcastItem(t, podcast.ID, "fav-2", "Summary Favorite Episode", Downloaded, now.Add(-2*time.Hour))
	summaryFavorited.IsSummaryFavorited = true
	summaryFavorited.LLMSummary = "summary favorite"
	summaryFavorited.LLMSummaryStatus = "available"
	if err := UpdatePodcastItem(&summaryFavorited); err != nil {
		t.Fatalf("update summary-favorited item failed: %v", err)
	}

	plain := newPodcastItem(t, podcast.ID, "fav-3", "Plain Episode", Downloaded, now.Add(-time.Hour))
	plain.LLMSummary = "plain summary"
	plain.LLMSummaryStatus = "available"
	if err := UpdatePodcastItem(&plain); err != nil {
		t.Fatalf("update plain item failed: %v", err)
	}

	bookmarkedOnly := "true"
	filter := model.EpisodesFilter{
		Pagination: model.Pagination{
			Page:  1,
			Count: 10,
		},
		IsBookmarked: &bookmarkedOnly,
	}
	filter.VerifyPaginationValues()
	favoriteEpisodes, total, err := GetPaginatedPodcastItemsNew(filter)
	if err != nil {
		t.Fatalf("GetPaginatedPodcastItemsNew favorites failed: %v", err)
	}
	if total != 2 || len(*favoriteEpisodes) != 2 {
		t.Fatalf("expected two favorite episodes, got total=%d len=%d", total, len(*favoriteEpisodes))
	}
	favoriteIDs := map[string]bool{}
	for _, item := range *favoriteEpisodes {
		favoriteIDs[item.ID] = true
	}
	if !favoriteIDs[bookmarked.ID] || !favoriteIDs[summaryFavorited.ID] || favoriteIDs[plain.ID] {
		t.Fatalf("unexpected favorite episode ids: %+v", favoriteIDs)
	}

	unbookmarkedOnly := "false"
	filter.IsBookmarked = &unbookmarkedOnly
	nonFavoriteEpisodes, total, err := GetPaginatedPodcastItemsNew(filter)
	if err != nil {
		t.Fatalf("GetPaginatedPodcastItemsNew non-favorites failed: %v", err)
	}
	if total != 1 || len(*nonFavoriteEpisodes) != 1 || (*nonFavoriteEpisodes)[0].ID != plain.ID {
		t.Fatalf("expected only plain episode as non-favorite, got total=%d items=%+v", total, *nonFavoriteEpisodes)
	}

	favoriteSummaries, total, err := GetPaginatedSummaries(1, 10, "", nil, "newest", true)
	if err != nil {
		t.Fatalf("GetPaginatedSummaries favorites failed: %v", err)
	}
	if total != 2 || len(*favoriteSummaries) != 2 {
		t.Fatalf("expected two favorite summaries, got total=%d len=%d", total, len(*favoriteSummaries))
	}

	if err := SetSummaryFavorited(plain.ID, true); err != nil {
		t.Fatalf("SetSummaryFavorited true failed: %v", err)
	}
	var refreshedPlain PodcastItem
	if err := GetPodcastItemByID(plain.ID, &refreshedPlain); err != nil {
		t.Fatalf("reload summary-favorited item failed: %v", err)
	}
	if !refreshedPlain.IsSummaryFavorited || refreshedPlain.BookmarkDate.IsZero() {
		t.Fatalf("expected summary favorite to set both favorite flags, got summary=%v bookmark=%v", refreshedPlain.IsSummaryFavorited, refreshedPlain.BookmarkDate)
	}

	if err := SetSummaryFavorited(plain.ID, false); err != nil {
		t.Fatalf("SetSummaryFavorited false failed: %v", err)
	}
	if err := GetPodcastItemByID(plain.ID, &refreshedPlain); err != nil {
		t.Fatalf("reload unfavorited item failed: %v", err)
	}
	if refreshedPlain.IsSummaryFavorited || !refreshedPlain.BookmarkDate.IsZero() {
		t.Fatalf("expected unfavorite to clear both favorite flags, got summary=%v bookmark=%v", refreshedPlain.IsSummaryFavorited, refreshedPlain.BookmarkDate)
	}
}

// TestSettingsLocksTagsAndSearchHelpers handles the corresponding operation.
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
	tagByID, err := GetTagByID(tag.ID)
	if err != nil {
		t.Fatalf("GetTagByID failed: %v", err)
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
	if err := UntagAllByTagID(tag.ID); err != nil {
		t.Fatalf("UntagAllByTagID failed: %v", err)
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

	processing := newPodcastItem(t, podcast.ID, "s-3", "Processing transcript", Downloaded, time.Now().UTC().Add(-1*time.Minute))
	processing.DownloadPath = filepath.Join(t.TempDir(), "audio-processing.mp3")
	processing.TranscriptStatus = "processing"
	processing.TranscriptJSON = ""
	if err := UpdatePodcastItem(&processing); err != nil {
		t.Fatalf("failed to create processing transcript item: %v", err)
	}

	failedReady := newPodcastItem(t, podcast.ID, "s-4", "Failed ready transcript", Downloaded, time.Now().UTC().Add(-2*time.Minute))
	failedReady.DownloadPath = filepath.Join(t.TempDir(), "audio-failed-ready.mp3")
	failedReady.TranscriptStatus = "failed"
	failedReady.TranscriptJSON = ""
	readyAt := time.Now().UTC().Add(-30 * time.Second)
	failedReady.TranscriptNextAttempt = &readyAt
	if err := UpdatePodcastItem(&failedReady); err != nil {
		t.Fatalf("failed to create failed-ready transcript item: %v", err)
	}

	failedFuture := newPodcastItem(t, podcast.ID, "s-5", "Failed future transcript", Downloaded, time.Now().UTC().Add(-3*time.Minute))
	failedFuture.DownloadPath = filepath.Join(t.TempDir(), "audio-failed-future.mp3")
	failedFuture.TranscriptStatus = "failed"
	failedFuture.TranscriptJSON = ""
	futureAt := time.Now().UTC().Add(10 * time.Minute)
	failedFuture.TranscriptNextAttempt = &futureAt
	if err := UpdatePodcastItem(&failedFuture); err != nil {
		t.Fatalf("failed to create failed-future transcript item: %v", err)
	}

	itemsForWhisperx, err := GetPodcastItemsForWhisperx([]string{"pending_whisperx", "processing", "failed"}, 10)
	if err != nil {
		t.Fatalf("GetPodcastItemsForWhisperx failed: %v", err)
	}
	if len(*itemsForWhisperx) != 3 {
		t.Fatalf("expected three items for whisperx (excluding scheduled future retry), got %d", len(*itemsForWhisperx))
	}

	if (*itemsForWhisperx)[0].ID != processing.ID {
		t.Fatalf("expected processing item first in whisperx queue, got %s", (*itemsForWhisperx)[0].ID)
	}
	if (*itemsForWhisperx)[1].ID != pending.ID {
		t.Fatalf("expected pending item second in whisperx queue, got %s", (*itemsForWhisperx)[1].ID)
	}
	if (*itemsForWhisperx)[2].ID != failedReady.ID {
		t.Fatalf("expected failed-ready item last in whisperx queue, got %s", (*itemsForWhisperx)[2].ID)
	}

	if err := DeletePodcastItemByID(item.ID); err != nil {
		t.Fatalf("DeletePodcastItemByID failed: %v", err)
	}
	if err := DeleteTagByID(tag.ID); err != nil {
		t.Fatalf("DeleteTagByID failed: %v", err)
	}
	if err := DeletePodcastByID(podcast.ID); err != nil {
		t.Fatalf("DeletePodcastByID failed: %v", err)
	}
}

func TestGetPodcastItemsForWhisperxResumesProcessingBeforePending(t *testing.T) {
	setupDBForTest(t)
	podcast := newPodcast(t, "WhisperX Queue", "https://example.com/queue.xml")

	pending := newPodcastItem(t, podcast.ID, "pending-new", "New pending transcript", Downloaded, time.Now().UTC())
	pending.DownloadPath = filepath.Join(t.TempDir(), "pending.mp3")
	pending.TranscriptStatus = "pending_whisperx"
	pending.TranscriptJSON = ""
	if err := UpdatePodcastItem(&pending); err != nil {
		t.Fatalf("failed to create pending transcript item: %v", err)
	}

	processing := newPodcastItem(t, podcast.ID, "processing-stale", "Stale processing transcript", Downloaded, time.Now().UTC().Add(-10*24*time.Hour))
	processing.DownloadPath = filepath.Join(t.TempDir(), "processing.mp3")
	processing.TranscriptStatus = "processing"
	processing.TranscriptJSON = ""
	if err := UpdatePodcastItem(&processing); err != nil {
		t.Fatalf("failed to create processing transcript item: %v", err)
	}

	itemsForWhisperx, err := GetPodcastItemsForWhisperx([]string{"pending_whisperx", "processing"}, 10)
	if err != nil {
		t.Fatalf("GetPodcastItemsForWhisperx failed: %v", err)
	}
	if len(*itemsForWhisperx) != 2 {
		t.Fatalf("expected two items for whisperx, got %d", len(*itemsForWhisperx))
	}
	if (*itemsForWhisperx)[0].ID != processing.ID {
		t.Fatalf("expected stale processing item first in whisperx queue, got %s", (*itemsForWhisperx)[0].ID)
	}
	if (*itemsForWhisperx)[1].ID != pending.ID {
		t.Fatalf("expected pending item second in whisperx queue, got %s", (*itemsForWhisperx)[1].ID)
	}
}

// TestJobLockUpsertAndUnlockByID handles the corresponding operation.
func TestJobLockUpsertAndUnlockByID(t *testing.T) {
	setupDBForTest(t)

	first := Lock("upsert-job", 1)
	if first == nil || first.ID == "" {
		t.Fatalf("expected first lock acquisition to return persisted lock id")
	}

	second := Lock("upsert-job", 2)
	if second == nil || second.ID == "" {
		t.Fatalf("expected second lock acquisition to return persisted lock id")
	}

	var count int64
	if err := DB.Model(&JobLock{}).Where("name = ?", "upsert-job").Count(&count).Error; err != nil {
		t.Fatalf("failed to count lock rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly one lock row after upsert, got %d", count)
	}

	current := GetLock("upsert-job")
	if current.Duration != 2 {
		t.Fatalf("expected upsert to update duration to 2, got %d", current.Duration)
	}
	if !current.IsLocked() {
		t.Fatalf("expected lock to be active")
	}

	UnlockByID(second.ID)

	reloaded := GetLock("upsert-job")
	if reloaded.IsLocked() {
		t.Fatalf("expected lock to be released by id")
	}
	if reloaded.Duration != 0 {
		t.Fatalf("expected released lock duration 0, got %d", reloaded.Duration)
	}
}

// TestTryLockReturnsBusyWhenLeaseIsActive handles the corresponding operation.
func TestTryLockReturnsBusyWhenLeaseIsActive(t *testing.T) {
	setupDBForTest(t)

	first, acquired, err := TryLock("try-lock-active", 2)
	if err != nil {
		t.Fatalf("TryLock first acquire failed: %v", err)
	}
	if !acquired || first == nil || first.ID == "" {
		t.Fatalf("expected first TryLock to acquire lock")
	}

	second, acquired, err := TryLock("try-lock-active", 2)
	if err != nil {
		t.Fatalf("TryLock second acquire failed: %v", err)
	}
	if acquired {
		t.Fatalf("expected second TryLock to report busy lock")
	}
	if second == nil || second.ID == "" {
		t.Fatalf("expected current lock row when busy")
	}
	if second.ID != first.ID {
		t.Fatalf("expected busy lock to reference same lock row, first=%s second=%s", first.ID, second.ID)
	}
}

// TestTryLockReacquiresExpiredLease handles the corresponding operation.
func TestTryLockReacquiresExpiredLease(t *testing.T) {
	setupDBForTest(t)

	first, acquired, err := TryLock("try-lock-expired", 1)
	if err != nil {
		t.Fatalf("TryLock first acquire failed: %v", err)
	}
	if !acquired || first == nil || first.ID == "" {
		t.Fatalf("expected first TryLock to acquire lock")
	}

	staleDate := time.Now().UTC().Add(-10 * time.Minute)
	if err := DB.Model(&JobLock{}).Where("id = ?", first.ID).Updates(map[string]interface{}{
		"date": staleDate,
	}).Error; err != nil {
		t.Fatalf("failed to age lock row: %v", err)
	}

	second, acquired, err := TryLock("try-lock-expired", 2)
	if err != nil {
		t.Fatalf("TryLock reacquire failed: %v", err)
	}
	if !acquired {
		t.Fatalf("expected TryLock to reacquire expired lease")
	}
	if second == nil || second.ID != first.ID {
		t.Fatalf("expected reacquire to reuse same lock row id")
	}
	if second.Duration != 2 {
		t.Fatalf("expected new duration 2, got %d", second.Duration)
	}
}

// TestTryLockIsAtomicUnderConcurrentContenders handles the corresponding operation.
func TestTryLockIsAtomicUnderConcurrentContenders(t *testing.T) {
	setupDBForTest(t)

	const contenders = 12
	var wg sync.WaitGroup
	results := make(chan bool, contenders)
	start := make(chan struct{})

	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, acquired, err := TryLock("try-lock-race", 5)
			if err != nil {
				t.Errorf("TryLock contender failed: %v", err)
				return
			}
			results <- acquired
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	acquiredCount := 0
	for acquired := range results {
		if acquired {
			acquiredCount++
		}
	}
	if acquiredCount != 1 {
		t.Fatalf("expected exactly one contender to acquire lock, got %d", acquiredCount)
	}
}

// TestJobLockIsLockedHandlesUTCZeroTimestamp handles the corresponding operation.
func TestJobLockIsLockedHandlesUTCZeroTimestamp(t *testing.T) {
	lock := &JobLock{
		Date:     time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC),
		Duration: 120,
	}
	if lock.IsLocked() {
		t.Fatalf("expected UTC zero timestamp to be treated as unlocked")
	}
}

// TestJobLockIsLockedUsesLeaseExpiry handles the corresponding operation.
func TestJobLockIsLockedUsesLeaseExpiry(t *testing.T) {
	lock := &JobLock{
		Date:     time.Now().UTC().Add(-3 * time.Minute),
		Duration: 1,
	}
	if lock.IsLocked() {
		t.Fatalf("expected expired lock lease to be treated as unlocked")
	}

	lock.Date = time.Now().UTC()
	if !lock.IsLocked() {
		t.Fatalf("expected fresh lock lease to be treated as locked")
	}
}

// TestGetLockTreatsUnlockedUTCZeroTimestampAsUnlocked handles the corresponding operation.
func TestGetLockTreatsUnlockedUTCZeroTimestampAsUnlocked(t *testing.T) {
	setupDBForTest(t)

	lock := Lock("utc-zero-unlocked", 5)
	if lock == nil || lock.ID == "" {
		t.Fatalf("expected lock row")
	}

	zeroUTC := time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)
	if err := DB.Model(&JobLock{}).Where("id = ?", lock.ID).Updates(map[string]interface{}{
		"duration": 0,
		"date":     zeroUTC,
	}).Error; err != nil {
		t.Fatalf("failed to force UTC zero timestamp: %v", err)
	}

	reloaded := GetLock("utc-zero-unlocked")
	if reloaded.IsLocked() {
		t.Fatalf("expected lock with UTC zero timestamp to be unlocked")
	}
}

// TestDatabaseWrapperCoveragePaths handles the corresponding operation.
func TestDatabaseWrapperCoveragePaths(t *testing.T) {
	setupDBForTest(t)

	podcast := newPodcast(t, "Wrapper Coverage", "https://example.com/wrapper.xml")
	item := newPodcastItem(t, podcast.ID, "wrapper-1", "Wrapper Episode", Downloaded, time.Now().UTC())
	item.Image = "https://example.com/wrapper.jpg"
	item.LocalImage = ""
	if err := UpdatePodcastItem(&item); err != nil {
		t.Fatalf("failed to update wrapper item image fields: %v", err)
	}

	itemsWithoutImage, err := GetAllPodcastItemsWithoutImage()
	if err != nil {
		t.Fatalf("GetAllPodcastItemsWithoutImage failed: %v", err)
	}
	if len(*itemsWithoutImage) == 0 {
		t.Fatalf("expected at least one downloaded item without local image")
	}

	var byTitleAuthor Podcast
	if err := GetPodcastByTitleAndAuthor(podcast.Title, podcast.Author, &byTitleAuthor); err != nil {
		t.Fatalf("GetPodcastByTitleAndAuthor failed: %v", err)
	}
	if byTitleAuthor.ID != podcast.ID {
		t.Fatalf("expected podcast lookup by title+author to return same podcast id")
	}

	podcast.Summary = "updated summary"
	if err := UpdatePodcast(&podcast); err != nil {
		t.Fatalf("UpdatePodcast failed: %v", err)
	}

	updatedLastEpisode := time.Now().UTC().Add(-15 * time.Minute).Truncate(time.Second)
	if err := UpdateLastEpisodeDateForPodcast(podcast.ID, updatedLastEpisode); err != nil {
		t.Fatalf("UpdateLastEpisodeDateForPodcast failed: %v", err)
	}
	var refreshedPodcast Podcast
	if err := GetPodcastByID(podcast.ID, &refreshedPodcast); err != nil {
		t.Fatalf("GetPodcastByID failed: %v", err)
	}
	if refreshedPodcast.LastEpisode == nil || !refreshedPodcast.LastEpisode.UTC().Equal(updatedLastEpisode) {
		t.Fatalf("expected last episode to be updated to %s, got %v", updatedLastEpisode, refreshedPodcast.LastEpisode)
	}

	tag := Tag{Label: "wrapper-tag", Description: "wrapper coverage tag"}
	if err := CreateTag(&tag); err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}
	if err := AddTagToPodcast(podcast.ID, tag.ID); err != nil {
		t.Fatalf("AddTagToPodcast failed: %v", err)
	}
	var pagedTags []Tag
	var total int64
	if err := GetPaginatedTags(1, 10, &pagedTags, &total); err != nil {
		t.Fatalf("GetPaginatedTags failed: %v", err)
	}
	if total == 0 || len(pagedTags) == 0 {
		t.Fatalf("expected paginated tags to include created tag")
	}

	if GetDB() == nil {
		t.Fatalf("expected global DB handle to be initialized")
	}

	lock := Lock("wrapper-refresh-lock", 1)
	if lock == nil || lock.ID == "" {
		t.Fatalf("expected persisted lock from Lock")
	}
	oldDate := lock.Date
	if err := RefreshLockByID(lock.ID, 5); err != nil {
		t.Fatalf("RefreshLockByID failed: %v", err)
	}
	if err := RefreshLockByID("", 3); err != nil {
		t.Fatalf("expected empty-id refresh to be a no-op, got %v", err)
	}

	reloaded := GetLock("wrapper-refresh-lock")
	if reloaded.Duration != 5 {
		t.Fatalf("expected refreshed lock duration 5, got %d", reloaded.Duration)
	}
	if reloaded.Date.Before(oldDate) {
		t.Fatalf("expected refreshed lock date to be updated, old=%s new=%s", oldDate, reloaded.Date)
	}
}
