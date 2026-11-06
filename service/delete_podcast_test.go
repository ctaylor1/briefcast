package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ctaylor1/briefcast/db"
	"gorm.io/gorm"
)

// deleteTestCategories is deliberately independent of the production list, so
// dropping a category there fails these tests.
var deleteTestCategories = []string{
	assetAudioDir,
	assetImagesDir,
	assetTranscriptDir,
	assetSummariesDir,
	assetMarkdownTranscriptDir,
	assetMarkdownSummariesDir,
}

func writeAssetFile(t *testing.T, dir string, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create asset folder: %v", err)
	}
	filePath := filepath.Join(dir, name)
	if err := os.WriteFile(filePath, []byte("asset"), 0o644); err != nil {
		t.Fatalf("write asset file: %v", err)
	}
	return filePath
}

// seedCategoryFolders writes one file into each DATA/<category>/<folder>.
func seedCategoryFolders(t *testing.T, dataDir string, folder string, name string) []string {
	t.Helper()
	files := make([]string, 0, len(deleteTestCategories))
	for _, category := range deleteTestCategories {
		dir := filepath.Join(dataDir, filepath.FromSlash(category), folder)
		files = append(files, writeAssetFile(t, dir, name))
	}
	return files
}

func seedDownloadedEpisode(t *testing.T, dataDir string, podcast db.Podcast, title string) db.PodcastItem {
	t.Helper()
	item := createServicePodcastItem(t, podcast, title, db.Downloaded)
	dir := filepath.Join(dataDir, assetAudioDir, sanitizeAssetName(podcast.Title))
	item.DownloadPath = writeAssetFile(t, dir, title+".mp3")
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("persist episode path: %v", err)
	}
	return item
}

func createPodcastWithFeed(t *testing.T, title string, feed string) db.Podcast {
	t.Helper()
	podcast := db.Podcast{Title: title, URL: "https://example.com/" + feed}
	if err := db.CreatePodcast(&podcast); err != nil {
		t.Fatalf("create podcast: %v", err)
	}
	return podcast
}

func requirePathsExist(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected %q to remain: %v", path, err)
		}
	}
}

func requirePathsRemoved(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("expected %q to be removed, stat err=%v", path, err)
		}
	}
}

func requirePodcastRows(t *testing.T, podcast db.Podcast, item db.PodcastItem, wantRows bool) {
	t.Helper()
	var storedPodcast db.Podcast
	if err := db.GetPodcastByID(podcast.ID, &storedPodcast); (err == nil) != wantRows {
		t.Errorf("podcast %s: want row=%v, lookup err=%v", podcast.ID, wantRows, err)
	}
	var storedItem db.PodcastItem
	if err := db.GetPodcastItemByID(item.ID, &storedItem); (err == nil) != wantRows {
		t.Errorf("episode %s: want row=%v, lookup err=%v", item.ID, wantRows, err)
	}
}

func TestDeletePodcastFolderKeepsCategoryDirectories(t *testing.T) {
	titles := []string{
		"audio", "images", "transcripts", "summaries", "markdown",
		// These sanitize to a reserved name, some only on a case-insensitive
		// filesystem. Guarding the sanitized name covers all of them.
		"AUDIO", "Images", "audio!", " transcripts. ",
	}
	for _, title := range titles {
		t.Run(title, func(t *testing.T) {
			dataDir := t.TempDir()
			t.Setenv("DATA", dataDir)
			folder := sanitizeAssetName(title)

			otherFiles := seedCategoryFolders(t, dataDir, "Other Show", "other.txt")
			ownFiles := seedCategoryFolders(t, dataDir, folder, "own.txt")
			legacyPathFile := writeAssetFile(t, filepath.Join(dataDir, folder), "must-stay.txt")

			if err := deletePodcastFolder(title); err != nil {
				t.Fatalf("delete podcast folder: %v", err)
			}

			requirePathsExist(t, otherFiles...)
			requirePathsExist(t, legacyPathFile)
			requirePathsRemoved(t, ownFiles...)
			for _, category := range deleteTestCategories {
				requirePathsExist(t, filepath.Join(dataDir, filepath.FromSlash(category)))
			}
		})
	}
}

func TestDeletePodcastTitledLikeCategoryKeepsOtherPodcastFiles(t *testing.T) {
	dataDir := filepath.Join(setupRetentionTestDB(t), "assets")

	podcast := createPodcast(t, "audio", false)
	item := seedDownloadedEpisode(t, dataDir, podcast, "audio-episode")
	otherFiles := seedCategoryFolders(t, dataDir, "Other Show", "other.txt")

	if err := DeletePodcast(podcast.ID, true); err != nil {
		t.Fatalf("delete podcast: %v", err)
	}

	requirePathsExist(t, otherFiles...)
	requirePathsRemoved(t, item.DownloadPath)
	requirePodcastRows(t, podcast, item, false)
}

func TestDeletePodcastKeepsFolderSharedWithAnotherPodcast(t *testing.T) {
	cases := []struct {
		name          string
		deletedTitle  string
		survivorTitle string
	}{
		{name: "identical titles", deletedTitle: "Shared Show", survivorTitle: "Shared Show"},
		{name: "titles that sanitize alike", deletedTitle: "Shared Show", survivorTitle: "Shared Show!"},
		{name: "titles that differ by case", deletedTitle: "Shared Show", survivorTitle: "SHARED SHOW"},
		{name: "titles with no Latin characters", deletedTitle: "日本語の番組", survivorTitle: "中文播客"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := filepath.Join(setupRetentionTestDB(t), "assets")
			survivorFolder := sanitizeAssetName(tc.survivorTitle)
			if !strings.EqualFold(sanitizeAssetName(tc.deletedTitle), survivorFolder) {
				t.Fatalf("titles must collide: %q and %q", sanitizeAssetName(tc.deletedTitle), survivorFolder)
			}

			deleted := createPodcastWithFeed(t, tc.deletedTitle, "deleted-feed")
			survivor := createPodcastWithFeed(t, tc.survivorTitle, "survivor-feed")
			deletedItem := seedDownloadedEpisode(t, dataDir, deleted, "deleted-episode")
			survivorItem := seedDownloadedEpisode(t, dataDir, survivor, "survivor-episode")
			survivorFiles := seedCategoryFolders(t, dataDir, survivorFolder, "survivor.txt")
			survivorFiles = append(survivorFiles, survivorItem.DownloadPath,
				writeAssetFile(t, filepath.Join(dataDir, survivorFolder), "survivor-legacy.txt"))

			if err := DeletePodcast(deleted.ID, true); err != nil {
				t.Fatalf("delete podcast: %v", err)
			}

			requirePathsExist(t, survivorFiles...)
			requirePathsRemoved(t, deletedItem.DownloadPath)
			requirePodcastRows(t, deleted, deletedItem, false)
			requirePodcastRows(t, survivor, survivorItem, true)

			// The guard must not outlive the collision: the last podcast cleans up.
			if err := DeletePodcast(survivor.ID, true); err != nil {
				t.Fatalf("delete surviving podcast: %v", err)
			}
			requirePathsRemoved(t, survivorFiles...)
		})
	}
}

func TestDeletePodcastKeepsSharedFolderWhenFilesAreKept(t *testing.T) {
	dataDir := filepath.Join(setupRetentionTestDB(t), "assets")

	deleted := createPodcastWithFeed(t, "Shared Show", "deleted-feed")
	survivor := createPodcastWithFeed(t, "Shared Show", "survivor-feed")
	deletedItem := seedDownloadedEpisode(t, dataDir, deleted, "deleted-episode")
	survivorItem := seedDownloadedEpisode(t, dataDir, survivor, "survivor-episode")

	if err := DeletePodcast(deleted.ID, false); err != nil {
		t.Fatalf("delete podcast: %v", err)
	}

	requirePathsExist(t, survivorItem.DownloadPath, deletedItem.DownloadPath)
	requirePodcastRows(t, deleted, deletedItem, false)
	requirePodcastRows(t, survivor, survivorItem, true)
}

func TestDeletePodcastRemovesAllFoldersForUniqueTitle(t *testing.T) {
	dataDir := filepath.Join(setupRetentionTestDB(t), "assets")

	podcast := createPodcast(t, "Unique Show", false)
	item := seedDownloadedEpisode(t, dataDir, podcast, "unique-episode")
	folder := sanitizeAssetName(podcast.Title)

	ownFolders := []string{filepath.Join(dataDir, folder)}
	writeAssetFile(t, ownFolders[0], "legacy.txt")
	for _, file := range seedCategoryFolders(t, dataDir, folder, "own.txt") {
		ownFolders = append(ownFolders, filepath.Dir(file))
	}
	neighborFiles := seedCategoryFolders(t, dataDir, "Neighbor Show", "neighbor.txt")

	if err := DeletePodcast(podcast.ID, true); err != nil {
		t.Fatalf("delete podcast: %v", err)
	}

	requirePathsRemoved(t, ownFolders...)
	requirePathsExist(t, neighborFiles...)
	for _, category := range deleteTestCategories {
		requirePathsExist(t, filepath.Join(dataDir, filepath.FromSlash(category)))
	}
	requirePodcastRows(t, podcast, item, false)
}

// A failed check must stop the delete: without it, shared folders cannot be
// told apart from this podcast's own folders.
func TestDeletePodcastDeletesNothingWhenSharedFolderCheckFails(t *testing.T) {
	dataDir := filepath.Join(setupRetentionTestDB(t), "assets")

	podcast := createPodcast(t, "Check Failure Show", false)
	item := seedDownloadedEpisode(t, dataDir, podcast, "check-failure-episode")

	errListFailed := errors.New("podcast list failed")
	failPodcastList := func(tx *gorm.DB) {
		if _, isPodcastList := tx.Statement.Dest.(*[]db.Podcast); isPodcastList {
			_ = tx.AddError(errListFailed)
		}
	}
	if err := db.DB.Callback().Query().Before("gorm:query").Register("test:fail_podcast_list", failPodcastList); err != nil {
		t.Fatalf("register query callback: %v", err)
	}

	if err := DeletePodcast(podcast.ID, true); !errors.Is(err, errListFailed) {
		t.Fatalf("expected the list error, got %v", err)
	}

	requirePathsExist(t, item.DownloadPath)
	requirePodcastRows(t, podcast, item, true)
}
