package db

import (
	"strings"
	"testing"
	"time"
)

func runMigrationsWithPrefix(t *testing.T, prefix string) {
	t.Helper()
	ran := 0
	for _, mig := range migrations {
		if strings.HasPrefix(mig.Name, prefix) {
			if err := executeMigrationQuery(mig.Name, mig.Query); err != nil {
				t.Fatalf("%s: %v", mig.Name, err)
			}
			ran++
		}
	}
	if ran == 0 {
		t.Fatalf("no migrations with prefix %s", prefix)
	}
}

func TestDuplicateEpisodeMigrationsKeepTheMostCompleteRow(t *testing.T) {
	setupDBForTest(t)
	// Recreate the state before the fix: no unique index yet.
	if err := DB.Exec("drop index if exists idx_podcast_items_podcast_guid").Error; err != nil {
		t.Fatal(err)
	}
	podcast := newPodcast(t, "Dupes", "https://example.com/dupes.xml")
	other := newPodcast(t, "Other", "https://example.com/other.xml")
	now := time.Now().UTC()
	bare := newPodcastItem(t, podcast.ID, "same-guid", "Episode", NotDownloaded, now)
	rich := newPodcastItem(t, podcast.ID, "same-guid", "Episode", NotDownloaded, now)
	if err := UpdatePodcastItemColumns(rich.ID, map[string]interface{}{"transcript_json": `{"segments":[]}`}); err != nil {
		t.Fatal(err)
	}
	unrelated := newPodcastItem(t, other.ID, "same-guid", "Episode", NotDownloaded, now)
	noGUID1 := newPodcastItem(t, podcast.ID, "", "No GUID 1", NotDownloaded, now)
	noGUID2 := newPodcastItem(t, podcast.ID, "", "No GUID 2", NotDownloaded, now)
	newShowNoteLink(t, podcast.ID, bare.ID, "https://x.com/bare")
	newShowNoteLink(t, podcast.ID, rich.ID, "https://x.com/rich")

	runMigrationsWithPrefix(t, "2026_11_09_02_")

	exists := func(id string) bool {
		var n int64
		if err := DB.Model(&PodcastItem{}).Where("id = ?", id).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n == 1
	}
	if exists(bare.ID) || !exists(rich.ID) {
		t.Fatalf("expected the row with a transcript to be kept: bare=%v rich=%v", exists(bare.ID), exists(rich.ID))
	}
	if !exists(unrelated.ID) || !exists(noGUID1.ID) || !exists(noGUID2.ID) {
		t.Fatal("rows that are not duplicates were deleted")
	}
	if got := countShowNoteLinks(t, "podcast_item_id", bare.ID); got != 0 {
		t.Fatalf("links of the removed duplicate remain: %d", got)
	}
	if got := countShowNoteLinks(t, "podcast_item_id", rich.ID); got != 1 {
		t.Fatalf("links of the kept row were removed: %d", got)
	}

	again := PodcastItem{PodcastID: podcast.ID, GUID: "same-guid", Title: "Episode"}
	if err := CreatePodcastItem(&again); err == nil {
		t.Fatal("the unique index must reject a second row with the same podcast and GUID")
	}
	blank := PodcastItem{PodcastID: podcast.ID, GUID: "", Title: "No GUID 3"}
	if err := CreatePodcastItem(&blank); err != nil {
		t.Fatalf("episodes without a GUID must still be allowed: %v", err)
	}
	if found, err := PodcastItemExistsWithGUID(podcast.ID, "same-guid"); err != nil || !found {
		t.Fatalf("PodcastItemExistsWithGUID: %v %v", found, err)
	}
}
