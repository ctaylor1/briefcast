package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TheHippo/podcastindex"
	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/model"
)

func createServicePodcastItem(t *testing.T, podcast db.Podcast, title string, status db.DownloadStatus) db.PodcastItem {
	t.Helper()
	item := db.PodcastItem{
		PodcastID:        podcast.ID,
		Title:            title,
		GUID:             strings.ToLower(strings.ReplaceAll(title, " ", "-")),
		PubDate:          time.Now().UTC(),
		FileURL:          "https://example.com/" + strings.ToLower(strings.ReplaceAll(title, " ", "-")) + ".mp3",
		DownloadStatus:   status,
		Summary:          "summary " + title,
		SummaryHTML:      "<p>summary " + title + "</p>",
		TranscriptStatus: "missing",
	}
	if err := db.CreatePodcastItem(&item); err != nil {
		t.Fatalf("create podcast item failed: %v", err)
	}
	return item
}

// TestDownloadsServiceStateFlows handles the corresponding operation.
func TestDownloadsServiceStateFlows(t *testing.T) {
	setupRetentionTestDB(t)
	podcast := createPodcast(t, "downloads-service", false)

	queued := createServicePodcastItem(t, podcast, "Queued Episode", db.NotDownloaded)
	downloading := createServicePodcastItem(t, podcast, "Downloading Episode", db.Downloading)
	paused := createServicePodcastItem(t, podcast, "Paused Episode", db.Paused)

	if err := CancelEpisodeDownload(queued.ID); err != nil {
		t.Fatalf("cancel queued episode failed: %v", err)
	}
	var refreshedQueued db.PodcastItem
	if err := db.GetPodcastItemByID(queued.ID, &refreshedQueued); err != nil {
		t.Fatalf("reload queued episode failed: %v", err)
	}
	if refreshedQueued.DownloadStatus != db.Paused {
		t.Fatalf("expected queued item to be paused, got %v", refreshedQueued.DownloadStatus)
	}

	if err := CancelEpisodeDownload(downloading.ID); err != nil {
		t.Fatalf("cancel downloading episode failed: %v", err)
	}
	if !IsDownloadPaused(downloading.ID) {
		t.Fatalf("expected downloading item pause flag to be set")
	}

	if err := CancelEpisodeDownload(paused.ID); err != nil {
		t.Fatalf("cancel paused episode should be no-op: %v", err)
	}

	shouldStart, err := ResumeEpisodeDownload(paused.ID)
	if err != nil {
		t.Fatalf("resume paused episode failed: %v", err)
	}
	if !shouldStart {
		t.Fatalf("expected paused episode to require downloader start")
	}
	var refreshedPaused db.PodcastItem
	if err := db.GetPodcastItemByID(paused.ID, &refreshedPaused); err != nil {
		t.Fatalf("reload paused episode failed: %v", err)
	}
	if refreshedPaused.DownloadStatus != db.NotDownloaded {
		t.Fatalf("expected resumed paused item to be queued, got %v", refreshedPaused.DownloadStatus)
	}

	shouldStart, err = ResumeEpisodeDownload(downloading.ID)
	if err != nil {
		t.Fatalf("resume downloading episode failed: %v", err)
	}
	if shouldStart {
		t.Fatalf("expected downloading episode to not trigger new worker")
	}

	if err := PauseAllDownloads(); err != nil {
		t.Fatalf("pause all downloads failed: %v", err)
	}
	if !DownloadsPaused() {
		t.Fatalf("expected global download pause state")
	}

	if err := ResumeAllDownloads(); err != nil {
		t.Fatalf("resume all downloads failed: %v", err)
	}
	if DownloadsPaused() {
		t.Fatalf("expected global download pause state to clear")
	}

	if err := CancelAllDownloads(); err != nil {
		t.Fatalf("cancel all downloads failed: %v", err)
	}
}

// TestSearchLocalRecordsReturnsPodcastEpisodeChapterAndTranscriptMatches handles the corresponding operation.
func TestSearchLocalRecordsReturnsPodcastEpisodeChapterAndTranscriptMatches(t *testing.T) {
	setupRetentionTestDB(t)
	podcast := createPodcast(t, "search-local", false)
	item := createServicePodcastItem(t, podcast, "Search Episode", db.Downloaded)
	item.ChaptersJSON = `{"chapters":[{"title":"Deep Intro","startTime":12}]}`
	item.TranscriptStatus = "available"
	item.TranscriptJSON = `{"segments":[{"start":12.5,"end":15.0,"text":"hello world transcript"}]}`
	item.Summary = "A summary mentioning search keyword"
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("update podcast item failed: %v", err)
	}

	results, err := SearchLocalRecords("", 10)
	if err != nil {
		t.Fatalf("search empty query failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected empty results for empty query, got %d", len(results))
	}

	results, err = SearchLocalRecords("search", 10)
	if err != nil {
		t.Fatalf("search keyword failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected keyword search results")
	}

	results, err = SearchLocalRecords("deep intro", 10)
	if err != nil {
		t.Fatalf("chapter search failed: %v", err)
	}
	foundChapter := false
	for _, result := range results {
		if result.Type == "chapter" {
			foundChapter = true
			break
		}
	}
	if !foundChapter {
		t.Fatalf("expected chapter result in local search")
	}

	results, err = SearchLocalRecords("hello world", 10)
	if err != nil {
		t.Fatalf("transcript search failed: %v", err)
	}
	foundTranscript := false
	for _, result := range results {
		if result.Type == "transcript" {
			foundTranscript = true
			break
		}
	}
	if !foundTranscript {
		t.Fatalf("expected transcript result in local search")
	}
}

// TestSearchProviderQueriesUseInjectedBaseURLs handles the corresponding operation.
func TestSearchProviderQueriesUseInjectedBaseURLs(t *testing.T) {
	t.Setenv(outboundAllowPrivateEnv, "true")

	origGpodder := gpodderBaseURL
	origItunes := itunesBaseURL
	t.Cleanup(func() {
		gpodderBaseURL = origGpodder
		itunesBaseURL = origItunes
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/search.json"):
			_, _ = w.Write([]byte(`[{"url":"https://pod.example/feed","title":"GPodcast","description":"Desc","logo_url":"https://pod.example/logo.jpg"}]`))
		case strings.HasPrefix(r.URL.Path, "/api/2/tag/"):
			_, _ = w.Write([]byte(`[{"url":"https://tag.example/feed","title":"TagPodcast","description":"TagDesc","logo_url":"https://tag.example/logo.jpg"}]`))
		case strings.HasPrefix(r.URL.Path, "/toplist/"):
			_, _ = w.Write([]byte(`[{"url":"https://top.example/feed","title":"TopPodcast","description":"TopDesc","logo_url":"https://top.example/logo.jpg"}]`))
		case strings.HasPrefix(r.URL.Path, "/api/2/tags/"):
			_, _ = w.Write([]byte(`[{"tag":"tech","title":"Tech","usage":99}]`))
		case strings.HasPrefix(r.URL.Path, "/search"):
			payload := model.ItunesResponse{
				ResultCount: 1,
				Results: []model.ItunesSingleResult{
					{
						TrackName:     "Itunes Podcast",
						FeedURL:       "https://itunes.example/feed",
						ArtworkURL600: "https://itunes.example/art.jpg",
					},
				},
			}
			encoded, _ := json.Marshal(payload)
			_, _ = w.Write(encoded)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	gpodderBaseURL = server.URL
	itunesBaseURL = server.URL

	if got := Query("briefcast"); len(got) != 1 || got[0].Title != "GPodcast" {
		t.Fatalf("unexpected gpodder query results: %+v", got)
	}
	if got := ByTag("tech", 1); len(got) != 1 || got[0].Title != "TagPodcast" {
		t.Fatalf("unexpected gpodder by-tag results: %+v", got)
	}
	if got := Top(1); len(got) != 1 || got[0].Title != "TopPodcast" {
		t.Fatalf("unexpected gpodder top results: %+v", got)
	}
	if got := Tags(1); len(got) != 1 || got[0].Tag != "tech" {
		t.Fatalf("unexpected gpodder tags results: %+v", got)
	}

	itunesResults := (ItunesService{}).Query("briefcast")
	if len(itunesResults) != 1 || itunesResults[0].Title != "Itunes Podcast" {
		t.Fatalf("unexpected itunes query results: %+v", itunesResults)
	}

	// Keep PodcastIndexService deterministic in CI by asserting credential-gated early return.
	t.Setenv(PodcastIndexKeyEnv, "")
	t.Setenv(PodcastIndexSecretEnv, "")
	if got := (PodcastIndexService{}).Query("anything"); len(got) != 0 {
		t.Fatalf("expected empty podcastindex results without credentials, got %+v", got)
	}
}

// TestFileServiceBackupAndFolderHelpers handles the corresponding operation.
func TestFileServiceBackupAndFolderHelpers(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	dataDir := filepath.Join(tempDir, "assets")

	podcast := createPodcast(t, "backup-podcast", false)
	imagePath := GetPodcastLocalImagePath("https://example.com/image.jpg", podcast.Title)
	if !strings.Contains(imagePath, "backup-podcast") {
		t.Fatalf("expected podcast image path to include podcast folder, got %q", imagePath)
	}
	if !strings.Contains(filepath.ToSlash(imagePath), "/images/backup-podcast/") {
		t.Fatalf("expected podcast image path to use images category, got %q", imagePath)
	}

	if err := CreateNfoFile(&podcast); err != nil {
		t.Fatalf("create NFO failed: %v", err)
	}
	rootPodcastFolder := filepath.Join(dataDir, sanitizeAssetName(podcast.Title))
	if _, err := os.Stat(rootPodcastFolder); !os.IsNotExist(err) {
		t.Fatalf("expected root podcast folder not to be created, got err=%v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "123")
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write([]byte("binary-data"))
	}))
	defer server.Close()

	if _, err := DownloadImage(server.URL, "episode-id", podcast.Title); err != nil {
		t.Fatalf("download image failed: %v", err)
	}
	if _, err := DownloadPodcastCoverImage(server.URL, podcast.Title); err != nil {
		t.Fatalf("download cover image failed: %v", err)
	}

	size, err := GetFileSizeFromURL(server.URL)
	if err != nil {
		t.Fatalf("get file size from url failed: %v", err)
	}
	if size != 123 {
		t.Fatalf("expected content length 123, got %d", size)
	}

	backupName, err := CreateBackup()
	if err != nil {
		t.Fatalf("create backup failed: %v", err)
	}
	backupPath := filepath.Join(tempDir, "backups", backupName)
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("expected backup file to exist: %v", err)
	}

	// Add extra backups to exercise retention pruning.
	for i := 0; i < 8; i++ {
		name := filepath.Join(tempDir, "backups", "briefcast_backup_2026.02.22_12010"+string(rune('0'+i))+".tar.gz")
		if err := os.WriteFile(name, []byte("backup"), 0o644); err != nil {
			t.Fatalf("failed to create extra backup: %v", err)
		}
	}
	deleteOldBackup()
	allBackups, err := GetAllBackupFiles()
	if err != nil {
		t.Fatalf("get all backup files failed: %v", err)
	}
	if len(allBackups) > 5 {
		t.Fatalf("expected backup pruning to keep at most 5 files, got %d", len(allBackups))
	}

	configSubfolder := createConfigFolderIfNotExists("test-config-subfolder")
	if _, err := os.Stat(configSubfolder); err != nil {
		t.Fatalf("expected config subfolder to exist: %v", err)
	}

	podcastFolder := filepath.Join(dataDir, cleanFileName(podcast.Title))
	if err := os.MkdirAll(podcastFolder, 0o755); err != nil {
		t.Fatalf("failed to seed legacy podcast folder: %v", err)
	}
	if _, err := os.Stat(podcastFolder); err != nil {
		t.Fatalf("expected podcast folder to exist before delete: %v", err)
	}
	if err := deletePodcastFolder(podcast.Title); err != nil {
		t.Fatalf("delete podcast folder failed: %v", err)
	}
	if _, err := os.Stat(podcastFolder); !os.IsNotExist(err) {
		t.Fatalf("expected podcast folder to be deleted")
	}
}

// TestPodcastServiceUtilityAndStateFlows handles the corresponding operation.
func TestPodcastServiceUtilityAndStateFlows(t *testing.T) {
	tempDir := setupRetentionTestDB(t)
	dataDir := filepath.Join(tempDir, "assets")

	// OPML parsing and invalid payload handling.
	if _, err := ParseOpml(`<?xml version="1.0"?><opml version="2.0"><head><title>x</title></head><body></body></opml>`); err != nil {
		t.Fatalf("parse opml failed: %v", err)
	}
	if err := AddOpml("not-xml"); err == nil {
		t.Fatalf("expected AddOpml invalid payload error")
	}

	podcast := createPodcast(t, "utility-podcast", false)
	item := createDownloadedItem(t, podcast, "utility-episode", time.Now().Add(-2*time.Hour), false, dataDir)
	item.ChaptersJSON = `{"chapters":[{"title":"chapter 1","startTime":1.5}]}`
	item.TranscriptStatus = "available"
	item.TranscriptJSON = `{"segments":[{"start":1.5,"end":2.0,"text":"hello"}]}`
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("update podcast item failed: %v", err)
	}

	if got := GetPodcastByID(podcast.ID); got.ID != podcast.ID {
		t.Fatalf("unexpected podcast by id result: %+v", got)
	}
	if got := GetPodcastItemByID(item.ID); got.ID != item.ID {
		t.Fatalf("unexpected podcast item by id result: %+v", got)
	}
	if got := GetAllPodcasts(""); len(*got) == 0 {
		t.Fatalf("expected podcasts in GetAllPodcasts")
	}
	if got := GetAllPodcastItemsByPodcastIds([]string{podcast.ID}); len(*got) == 0 {
		t.Fatalf("expected podcast items by podcast ids")
	}
	if got, err := GetAllPodcastItemsByIds([]string{item.ID}); err != nil || len(*got) == 0 {
		t.Fatalf("expected podcast items by ids, err=%v got=%+v", err, got)
	}

	opmlData, err := ExportOPML(true, "http://localhost:8080")
	if err != nil {
		t.Fatalf("export opml failed: %v", err)
	}
	if !bytes.Contains(opmlData, []byte("/podcasts/")) {
		t.Fatalf("expected exported OPML to contain briefcast rss links")
	}

	itunesXML := []byte(`<rss xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd"><channel><itunes:image href="https://cdn.example/cover.jpg"></itunes:image></channel></rss>`)
	if got := getItunesImageURL(itunesXML); got != "https://cdn.example/cover.jpg" {
		t.Fatalf("unexpected itunes image url: %q", got)
	}

	if got := GetSearchFromGpodder(model.GPodcast{URL: "u", LogoURL: "i", Title: "t", Description: "d"}); got.Title != "t" {
		t.Fatalf("unexpected gpodder mapping: %+v", got)
	}
	if got := GetSearchFromItunes(model.ItunesSingleResult{FeedURL: "u", ArtworkURL600: "i", TrackName: "t"}); got.Title != "t" {
		t.Fatalf("unexpected itunes mapping: %+v", got)
	}
	if got := GetSearchFromPodcastIndex(&podcastindex.Podcast{URL: "u", Image: "i", Title: "t", Description: "d"}); got.Title != "t" {
		t.Fatalf("unexpected podcastindex mapping: %+v", got)
	}

	if err := UpdateSettings(true, 4, InitialDownloadModeCount, 0, true, true, true, false, false, false, false, "http://localhost", 3, "agent"); err != nil {
		t.Fatalf("update settings failed: %v", err)
	}

	tag, err := AddTag("utility-tag", "tag description")
	if err != nil {
		t.Fatalf("add tag failed: %v", err)
	}
	if _, err := AddTag("utility-tag", "duplicate"); err == nil {
		t.Fatalf("expected duplicate add tag error")
	}
	if tags := GetTagsByIds([]string{tag.ID}); len(*tags) != 1 {
		t.Fatalf("expected one tag by ids, got %+v", tags)
	}

	if err := TogglePodcastPause(podcast.ID, true); err != nil {
		t.Fatalf("toggle podcast pause failed: %v", err)
	}
	if err := TogglePodcastPause(podcast.ID, false); err != nil {
		t.Fatalf("toggle podcast unpause failed: %v", err)
	}
	if err := TogglePodcastPause("missing", true); err == nil {
		t.Fatalf("expected toggle pause to fail for missing podcast")
	}
	if err := TogglePodcastPause("missing", false); err == nil {
		t.Fatalf("expected toggle unpause to fail for missing podcast")
	}

	if err := SetPodcastItemBookmarkStatus(item.ID, true); err != nil {
		t.Fatalf("set bookmark true failed: %v", err)
	}
	var favorited db.PodcastItem
	if err := db.GetPodcastItemByID(item.ID, &favorited); err != nil {
		t.Fatalf("reload favorited item failed: %v", err)
	}
	if favorited.BookmarkDate.IsZero() || !favorited.IsSummaryFavorited {
		t.Fatalf("expected episode favorite to set bookmark date and summary favorite, got bookmark=%v summary=%v", favorited.BookmarkDate, favorited.IsSummaryFavorited)
	}
	if err := SetPodcastItemBookmarkStatus(item.ID, false); err != nil {
		t.Fatalf("set bookmark false failed: %v", err)
	}
	if err := db.GetPodcastItemByID(item.ID, &favorited); err != nil {
		t.Fatalf("reload unfavorited item failed: %v", err)
	}
	if !favorited.BookmarkDate.IsZero() || favorited.IsSummaryFavorited {
		t.Fatalf("expected episode unfavorite to clear bookmark date and summary favorite, got bookmark=%v summary=%v", favorited.BookmarkDate, favorited.IsSummaryFavorited)
	}
	if err := SetPodcastItemPlayedStatus(item.ID, true); err != nil {
		t.Fatalf("set played status failed: %v", err)
	}
	if err := SetPodcastItemAsQueuedForDownload(item.ID); err != nil {
		t.Fatalf("set queued failed: %v", err)
	}
	if err := SetPodcastItemAsQueuedPreserveProgress(item.ID); err != nil {
		t.Fatalf("set queued preserve progress failed: %v", err)
	}
	if err := SetPodcastItemAsDownloading(item.ID); err != nil {
		t.Fatalf("set downloading failed: %v", err)
	}
	if err := SetPodcastItemAsPaused(item.ID); err != nil {
		t.Fatalf("set paused failed: %v", err)
	}
	if err := SetPodcastItemAsNotDownloaded(item.ID, db.Deleted); err != nil {
		t.Fatalf("set not downloaded failed: %v", err)
	}

	setting := db.GetOrCreateSetting()
	setting.AppendDateToFileName = true
	setting.AppendEpisodeNumberToFileName = true
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatalf("update settings for prefix failed: %v", err)
	}
	var updatedItem db.PodcastItem
	if err := db.GetPodcastItemByID(item.ID, &updatedItem); err != nil {
		t.Fatalf("reload item for prefix failed: %v", err)
	}
	if prefix := GetPodcastPrefix(&updatedItem, setting); prefix == "" {
		t.Fatalf("expected podcast prefix to be populated")
	}

	// check missing file handling branch
	updatedItem.DownloadStatus = db.Downloaded
	updatedItem.DownloadPath = filepath.Join(dataDir, "missing-file.mp3")
	if err := db.UpdatePodcastItem(&updatedItem); err != nil {
		t.Fatalf("update item for missing file check failed: %v", err)
	}
	setting.DontDownloadDeletedFromDisk = true
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatalf("update settings for missing file check failed: %v", err)
	}
	if err := CheckMissingFiles(); err != nil {
		t.Fatalf("check missing files failed: %v", err)
	}

	// download branch where downloads are globally paused.
	PauseDownloads()
	t.Cleanup(ResumeDownloads)
	if err := DownloadSingleEpisode(updatedItem.ID); err == nil || !strings.Contains(err.Error(), "paused") {
		t.Fatalf("expected paused download error, got %v", err)
	}

	// avoid network in no-work branches.
	if err := DownloadMissingEpisodes(); err != nil {
		t.Fatalf("download missing episodes should no-op under pause: %v", err)
	}

	if err := RefreshEpisodes(); err != nil {
		t.Logf("refresh episodes returned in test environment: %v", err)
	}
	if err := DownloadMissingImages(); err != nil {
		t.Fatalf("download missing images failed: %v", err)
	}
	if err := SetAllEpisodesToDownload("missing-podcast-id"); err == nil {
		t.Fatalf("expected SetAllEpisodesToDownload to fail for missing podcast")
	}

	UnlockMissedJobs()

	// DeleteTag / DeleteEpisodeFile / DeletePodcastEpisodes / DeletePodcast flows.
	episodeFile := filepath.Join(dataDir, "episode-delete.mp3")
	localImage := filepath.Join(dataDir, "episode-delete.jpg")
	if err := os.WriteFile(episodeFile, []byte("audio"), 0o644); err != nil {
		t.Fatalf("failed to write episode file: %v", err)
	}
	if err := os.WriteFile(localImage, []byte("image"), 0o644); err != nil {
		t.Fatalf("failed to write local image: %v", err)
	}
	deleteItem := createServicePodcastItem(t, podcast, "Delete Me", db.Downloaded)
	deleteItem.DownloadPath = episodeFile
	deleteItem.LocalImage = localImage
	if err := db.UpdatePodcastItem(&deleteItem); err != nil {
		t.Fatalf("failed to update delete item paths: %v", err)
	}
	if err := DeleteEpisodeFile(deleteItem.ID); err != nil {
		t.Fatalf("delete episode file failed: %v", err)
	}

	if err := DeleteTag(tag.ID); err != nil {
		t.Fatalf("delete tag failed: %v", err)
	}

	podcastToDeleteEpisodes := createPodcast(t, "delete-episodes-podcast", false)
	itemForDeleteEpisodes := createDownloadedItem(t, podcastToDeleteEpisodes, "delete-episodes-item", time.Now().Add(-3*time.Hour), false, dataDir)
	if err := DeletePodcastEpisodes(podcastToDeleteEpisodes.ID); err != nil {
		t.Fatalf("delete podcast episodes failed: %v", err)
	}
	var refreshedDeleteEpisodes db.PodcastItem
	if err := db.GetPodcastItemByID(itemForDeleteEpisodes.ID, &refreshedDeleteEpisodes); err != nil {
		t.Fatalf("reload delete episodes item failed: %v", err)
	}
	if refreshedDeleteEpisodes.DownloadStatus != db.Deleted {
		t.Fatalf("expected item to be marked deleted after DeletePodcastEpisodes, got %v", refreshedDeleteEpisodes.DownloadStatus)
	}

	podcastToDelete := createPodcast(t, "delete-podcast", false)
	_ = createServicePodcastItem(t, podcastToDelete, "delete-podcast-item", db.Deleted)
	if err := DeletePodcast(podcastToDelete.ID, false); err != nil {
		t.Fatalf("delete podcast failed: %v", err)
	}
	if _, err := db.GetTagByID(tag.ID); err == nil {
		// tag was deleted above, this keeps path deterministic but avoids strict gorm error matching.
		t.Fatalf("expected deleted tag lookup to fail")
	}
}

// TestChapterRefreshHelpers handles the corresponding operation.
func TestChapterRefreshHelpers(t *testing.T) {
	if hasMeaningfulTitles([]Chapter{{Title: "chapter 1"}, {Title: "chapter 2"}}) {
		t.Fatalf("expected autogenerated chapter labels to be treated as non-meaningful")
	}
	if !hasMeaningfulTitles([]Chapter{{Title: "Intro"}}) {
		t.Fatalf("expected non-generic chapter title to be meaningful")
	}

	setupRetentionTestDB(t)
	podcast := createPodcast(t, "chapter-refresh", false)
	item := createServicePodcastItem(t, podcast, "chapter-refresh-item", db.Downloaded)

	if RefreshChaptersFromID3(nil) {
		t.Fatalf("expected nil item refresh to return false")
	}
	if RefreshChaptersFromID3(&db.PodcastItem{DownloadPath: ""}) {
		t.Fatalf("expected empty download path refresh to return false")
	}

	pythonPath := requireWorkingPython(t)
	tempDir := t.TempDir()
	audioPath := filepath.Join(tempDir, "audio.mp3")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("failed to create chapter audio file: %v", err)
	}
	scriptPath := filepath.Join(tempDir, "mutagen_chapters.py")
	body := "#!/usr/bin/env python3\nprint('{\"tags\":{},\"chapters\":[{\"title\":\"Intro\",\"startTime\":1.2}]}')\n"
	if err := os.WriteFile(scriptPath, []byte(body), 0o755); err != nil {
		t.Fatalf("failed to write mutagen chapters stub: %v", err)
	}
	t.Setenv(mutagenPythonEnv, pythonPath)
	t.Setenv(mutagenScriptEnv, scriptPath)

	item.DownloadPath = audioPath
	item.ChaptersJSON = `{"chapters":[{"title":"chapter 1","startTime":0.0}]}`
	item.ChaptersType = "id3"
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("failed to persist chapter refresh item: %v", err)
	}

	if !RefreshChaptersFromID3(&item) {
		t.Fatalf("expected chapter refresh to update generic chapters")
	}
	if strings.TrimSpace(item.ID3ChaptersJSON) == "" {
		t.Fatalf("expected id3 chapters json after refresh")
	}
}

// TestUpdateAllFileSizesCoversDownloadedAndRemotePaths handles the corresponding operation.
func TestUpdateAllFileSizesCoversDownloadedAndRemotePaths(t *testing.T) {
	setupRetentionTestDB(t)
	podcast := createPodcast(t, "filesize-refresh", false)

	downloadedFilePath := filepath.Join(t.TempDir(), "episode-downloaded.mp3")
	if err := os.WriteFile(downloadedFilePath, []byte("1234567"), 0o644); err != nil {
		t.Fatalf("failed to create downloaded media file: %v", err)
	}
	downloadedItem := createServicePodcastItem(t, podcast, "downloaded-size", db.Downloaded)
	downloadedItem.DownloadPath = downloadedFilePath
	if err := db.UpdatePodcastItem(&downloadedItem); err != nil {
		t.Fatalf("failed to persist downloaded item path: %v", err)
	}

	missingFileItem := createServicePodcastItem(t, podcast, "missing-size", db.Downloaded)
	missingFileItem.DownloadPath = filepath.Join(t.TempDir(), "missing-file.mp3")
	if err := db.UpdatePodcastItem(&missingFileItem); err != nil {
		t.Fatalf("failed to persist missing-file item path: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "123")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	remoteItem := createServicePodcastItem(t, podcast, "remote-size", db.NotDownloaded)
	remoteItem.FileURL = server.URL + "/episode.mp3"
	if err := db.UpdatePodcastItem(&remoteItem); err != nil {
		t.Fatalf("failed to persist remote item url: %v", err)
	}

	UpdateAllFileSizes()

	var refreshedDownloaded db.PodcastItem
	if err := db.GetPodcastItemByID(downloadedItem.ID, &refreshedDownloaded); err != nil {
		t.Fatalf("failed to reload downloaded item: %v", err)
	}
	if refreshedDownloaded.FileSize != 7 {
		t.Fatalf("expected downloaded file size 7, got %d", refreshedDownloaded.FileSize)
	}

	var refreshedMissing db.PodcastItem
	if err := db.GetPodcastItemByID(missingFileItem.ID, &refreshedMissing); err != nil {
		t.Fatalf("failed to reload missing-file item: %v", err)
	}
	if refreshedMissing.FileSize != 1 {
		t.Fatalf("expected fallback file size 1 for missing file, got %d", refreshedMissing.FileSize)
	}

	var refreshedRemote db.PodcastItem
	if err := db.GetPodcastItemByID(remoteItem.ID, &refreshedRemote); err != nil {
		t.Fatalf("failed to reload remote item: %v", err)
	}
	if refreshedRemote.FileSize != 123 {
		t.Fatalf("expected remote file size 123, got %d", refreshedRemote.FileSize)
	}
}

// TestDownloadMissingImagesAndLocalDownloadHelper handles the corresponding operation.
func TestDownloadMissingImagesAndLocalDownloadHelper(t *testing.T) {
	setupRetentionTestDB(t)
	podcast := createPodcast(t, "image-downloads", false)
	item := createServicePodcastItem(t, podcast, "image-item", db.Downloaded)

	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fake-image-bytes"))
	}))
	defer imageServer.Close()

	item.Image = imageServer.URL + "/cover.jpg"
	item.LocalImage = ""
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("failed to persist image item url: %v", err)
	}

	setting := db.GetOrCreateSetting()
	setting.DownloadEpisodeImages = false
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatalf("failed to disable image downloads: %v", err)
	}
	if err := DownloadMissingImages(); err != nil {
		t.Fatalf("expected disabled image download path to return nil, got %v", err)
	}

	setting = db.GetOrCreateSetting()
	setting.DownloadEpisodeImages = true
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatalf("failed to enable image downloads: %v", err)
	}
	if err := DownloadMissingImages(); err != nil {
		t.Fatalf("download missing images failed: %v", err)
	}

	var refreshed db.PodcastItem
	if err := db.GetPodcastItemByID(item.ID, &refreshed); err != nil {
		t.Fatalf("failed to reload image item: %v", err)
	}
	if strings.TrimSpace(refreshed.LocalImage) == "" {
		t.Fatalf("expected local image path to be saved")
	}
	if _, err := os.Stat(refreshed.LocalImage); err != nil {
		t.Fatalf("expected downloaded local image file to exist: %v", err)
	}

	if err := downloadImageLocally("missing-id"); err == nil {
		t.Fatalf("expected downloadImageLocally to fail for unknown item id")
	}
}

// TestAddOpmlValidAndInvalidPayloads handles the corresponding operation.
func TestAddOpmlValidAndInvalidPayloads(t *testing.T) {
	setupRetentionTestDB(t)

	valid := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>Briefcast Export</title></head>
  <body>
    <outline text="Folder">
      <outline text="Nested without URL"/>
    </outline>
  </body>
</opml>`
	if err := AddOpml(valid); err != nil {
		t.Fatalf("expected valid OPML with no feed URLs to parse successfully, got %v", err)
	}

	if err := AddOpml("not-opml"); err == nil {
		t.Fatalf("expected invalid OPML payload to return an error")
	}
}

// TestDownloadSingleEpisodeAndMissingEpisodesBranches handles the corresponding operation.
func TestDownloadSingleEpisodeAndMissingEpisodesBranches(t *testing.T) {
	setupRetentionTestDB(t)
	podcast := createPodcast(t, "download-branches", false)

	setting := db.GetOrCreateSetting()
	setting.DownloadEpisodeImages = false
	setting.MaxDownloadConcurrency = 2
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatalf("failed to update settings for download branch test: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok.mp3":
			w.Header().Set("Content-Type", "audio/mpeg")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("audio-bytes-for-tests"))
		case "/fail.mp3":
			http.Error(w, "boom", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	okItem := createServicePodcastItem(t, podcast, "single-success", db.NotDownloaded)
	okItem.FileURL = server.URL + "/ok.mp3"
	if err := db.UpdatePodcastItem(&okItem); err != nil {
		t.Fatalf("failed to update ok item file url: %v", err)
	}

	if err := DownloadSingleEpisode(okItem.ID); err != nil {
		t.Fatalf("expected DownloadSingleEpisode success, got %v", err)
	}

	var refreshedOK db.PodcastItem
	if err := db.GetPodcastItemByID(okItem.ID, &refreshedOK); err != nil {
		t.Fatalf("failed to reload ok item: %v", err)
	}
	if refreshedOK.DownloadStatus != db.Downloaded {
		t.Fatalf("expected downloaded status after success, got %v", refreshedOK.DownloadStatus)
	}
	if strings.TrimSpace(refreshedOK.DownloadPath) == "" {
		t.Fatalf("expected download path to be populated on success")
	}
	if _, err := os.Stat(refreshedOK.DownloadPath); err != nil {
		t.Fatalf("expected downloaded file to exist: %v", err)
	}

	failItem := createServicePodcastItem(t, podcast, "single-failure", db.NotDownloaded)
	failItem.FileURL = server.URL + "/fail.mp3"
	if err := db.UpdatePodcastItem(&failItem); err != nil {
		t.Fatalf("failed to update fail item file url: %v", err)
	}

	if err := DownloadSingleEpisode(failItem.ID); err == nil {
		t.Fatalf("expected DownloadSingleEpisode failure for 500 response")
	}
	var refreshedFail db.PodcastItem
	if err := db.GetPodcastItemByID(failItem.ID, &refreshedFail); err != nil {
		t.Fatalf("failed to reload failed item: %v", err)
	}
	if refreshedFail.DownloadStatus != db.NotDownloaded {
		t.Fatalf("expected failed item to be reset to not_downloaded, got %v", refreshedFail.DownloadStatus)
	}

	pausedItem := createServicePodcastItem(t, podcast, "single-paused", db.NotDownloaded)
	pausedItem.FileURL = server.URL + "/ok.mp3"
	if err := db.UpdatePodcastItem(&pausedItem); err != nil {
		t.Fatalf("failed to update paused item file url: %v", err)
	}
	PauseDownload(pausedItem.ID)
	if err := DownloadSingleEpisode(pausedItem.ID); err != nil {
		t.Fatalf("expected paused download branch to return nil, got %v", err)
	}
	var refreshedPaused db.PodcastItem
	if err := db.GetPodcastItemByID(pausedItem.ID, &refreshedPaused); err != nil {
		t.Fatalf("failed to reload paused item: %v", err)
	}
	if refreshedPaused.DownloadStatus != db.Paused {
		t.Fatalf("expected paused status after paused branch, got %v", refreshedPaused.DownloadStatus)
	}
	if IsDownloadPaused(pausedItem.ID) {
		t.Fatalf("expected paused flag to be cleared after paused branch")
	}

	cancelledItem := createServicePodcastItem(t, podcast, "single-cancelled", db.NotDownloaded)
	cancelledItem.FileURL = server.URL + "/ok.mp3"
	if err := db.UpdatePodcastItem(&cancelledItem); err != nil {
		t.Fatalf("failed to update cancelled item file url: %v", err)
	}
	CancelDownload(cancelledItem.ID)
	if err := DownloadSingleEpisode(cancelledItem.ID); err != nil {
		t.Fatalf("expected cancelled download branch to return nil, got %v", err)
	}
	var refreshedCancelled db.PodcastItem
	if err := db.GetPodcastItemByID(cancelledItem.ID, &refreshedCancelled); err != nil {
		t.Fatalf("failed to reload cancelled item: %v", err)
	}
	if refreshedCancelled.DownloadStatus != db.Deleted {
		t.Fatalf("expected cancelled item to be marked deleted, got %v", refreshedCancelled.DownloadStatus)
	}

	// Keep this scenario focused on queue worker branches.
	if err := SetPodcastItemAsNotDownloaded(failItem.ID, db.Deleted); err != nil {
		t.Fatalf("failed to move single-failure item out of queue: %v", err)
	}

	queueOK := createServicePodcastItem(t, podcast, "queue-success", db.NotDownloaded)
	queueOK.FileURL = server.URL + "/ok.mp3"
	if err := db.UpdatePodcastItem(&queueOK); err != nil {
		t.Fatalf("failed to update queue success item file url: %v", err)
	}

	queueFail := createServicePodcastItem(t, podcast, "queue-failure", db.NotDownloaded)
	queueFail.FileURL = server.URL + "/fail.mp3"
	if err := db.UpdatePodcastItem(&queueFail); err != nil {
		t.Fatalf("failed to update queue fail item file url: %v", err)
	}

	queueCancelled := createServicePodcastItem(t, podcast, "queue-cancelled", db.NotDownloaded)
	queueCancelled.FileURL = server.URL + "/ok.mp3"
	if err := db.UpdatePodcastItem(&queueCancelled); err != nil {
		t.Fatalf("failed to update queue cancelled item file url: %v", err)
	}
	CancelDownload(queueCancelled.ID)

	ResumeDownloads()
	err := DownloadMissingEpisodes()
	if err == nil {
		t.Fatalf("expected DownloadMissingEpisodes to return first error when one item fails")
	}

	var refreshedQueueOK db.PodcastItem
	if err := db.GetPodcastItemByID(queueOK.ID, &refreshedQueueOK); err != nil {
		t.Fatalf("failed to reload queue success item: %v", err)
	}
	if refreshedQueueOK.DownloadStatus != db.Downloaded {
		t.Fatalf("expected queue success item to be downloaded, got %v", refreshedQueueOK.DownloadStatus)
	}

	var refreshedQueueFail db.PodcastItem
	if err := db.GetPodcastItemByID(queueFail.ID, &refreshedQueueFail); err != nil {
		t.Fatalf("failed to reload queue fail item: %v", err)
	}
	if refreshedQueueFail.DownloadStatus != db.NotDownloaded {
		t.Fatalf("expected queue fail item to remain not_downloaded, got %v", refreshedQueueFail.DownloadStatus)
	}

	var refreshedQueueCancelled db.PodcastItem
	if err := db.GetPodcastItemByID(queueCancelled.ID, &refreshedQueueCancelled); err != nil {
		t.Fatalf("failed to reload queue cancelled item: %v", err)
	}
	if refreshedQueueCancelled.DownloadStatus != db.Deleted {
		t.Fatalf("expected queue cancelled item to be marked deleted, got %v", refreshedQueueCancelled.DownloadStatus)
	}
}

// TestSetAllEpisodesToDownloadSuccess handles the corresponding operation.
func TestSetAllEpisodesToDownloadSuccess(t *testing.T) {
	setupRetentionTestDB(t)
	pythonPath := requireWorkingPython(t)

	server := httptest.NewServer(testFeedServer())
	t.Cleanup(server.Close)

	scriptDir := t.TempDir()
	feedScript := writeTempScript(t, scriptDir, "feedparser_stub.py", feedparserStub)
	t.Setenv("FEEDPARSER_PYTHON", pythonPath)
	t.Setenv("FEEDPARSER_SCRIPT", feedScript)
	t.Setenv("TEST_BASE_URL", server.URL)

	podcast := db.Podcast{
		Title: "set-all-episodes",
		URL:   server.URL + "/feed",
	}
	if err := db.CreatePodcast(&podcast); err != nil {
		t.Fatalf("failed to create podcast: %v", err)
	}
	item := createServicePodcastItem(t, podcast, "set-all-item", db.Deleted)

	if err := SetAllEpisodesToDownload(podcast.ID); err != nil {
		t.Fatalf("expected SetAllEpisodesToDownload success, got %v", err)
	}

	var refreshed db.PodcastItem
	if err := db.GetPodcastItemByID(item.ID, &refreshed); err != nil {
		t.Fatalf("failed to reload set-all item: %v", err)
	}
	if refreshed.DownloadStatus != db.NotDownloaded {
		t.Fatalf("expected deleted item to be re-queued as not_downloaded, got %v", refreshed.DownloadStatus)
	}
}
