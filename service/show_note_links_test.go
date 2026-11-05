package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
	strip "github.com/grokify/html-strip-tags-go"
)

func createShowNotesItem(t *testing.T, podcast db.Podcast, guid, summaryHTML string) db.PodcastItem {
	t.Helper()
	item := db.PodcastItem{
		PodcastID:   podcast.ID,
		Title:       guid,
		GUID:        guid,
		SummaryHTML: summaryHTML,
		PubDate:     time.Now().UTC(),
	}
	if err := db.CreatePodcastItem(&item); err != nil {
		t.Fatalf("create podcast item: %v", err)
	}
	return item
}

func countModel(t *testing.T, model any, where string, args ...any) int64 {
	t.Helper()
	var count int64
	query := db.DB.Model(model)
	if where != "" {
		query = query.Where(where, args...)
	}
	if err := query.Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	return count
}

func TestPersistShowNoteLinksAllInFixtureIsIdempotent(t *testing.T) {
	setupRetentionTestDB(t)
	raw, err := os.ReadFile(filepath.Join("..", "internal", "feedmeta", "testdata", "allin_show_notes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	podcast := createPodcast(t, "allin", true)
	item := createShowNotesItem(t, podcast, "allin-episode", "")

	for run := 0; run < 2; run++ {
		saved, err := persistShowNoteLinks(podcast.ID, item.ID, "", string(raw))
		if err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		if saved != 27 {
			t.Fatalf("run %d saved %d links, want 27", run, saved)
		}
	}

	if got := countModel(t, &db.ShowNoteLink{}, "podcast_item_id = ?", item.ID); got != 27 {
		t.Fatalf("show note links = %d, want 27", got)
	}
	if got := countModel(t, &db.ShowNoteLink{}, "podcast_item_id = ? AND link_target_id IS NULL", item.ID); got != 0 {
		t.Fatalf("links without a target = %d, want 0", got)
	}
	// 27 linked targets plus the 8 post authors, none of whom are also
	// linked directly in this fixture.
	if got := countModel(t, &db.LinkTarget{}, ""); got != 35 {
		t.Fatalf("link targets = %d, want 35", got)
	}
	if got := countModel(t, &db.LinkTarget{}, "kind = ?", "post"); got != 8 {
		t.Fatalf("post targets = %d, want 8", got)
	}

	post, err := db.GetLinkTargetByKey("x:status:2115458726396310008")
	if err != nil {
		t.Fatal(err)
	}
	author, err := db.GetLinkTargetByKey("x:andrewbenson")
	if err != nil {
		t.Fatal(err)
	}
	if post.ParentTargetID == nil || *post.ParentTargetID != author.ID {
		t.Fatalf("post parent = %v, want %s", post.ParentTargetID, author.ID)
	}

	var showAccount db.ShowNoteLink
	if err := db.DB.Where("podcast_item_id = ? AND url = ?", item.ID, "https://x.com/theallinpod").First(&showAccount).Error; err != nil {
		t.Fatal(err)
	}
	if showAccount.ContextLabel != "Follow on X" {
		t.Fatalf("show account label = %q, want %q", showAccount.ContextLabel, "Follow on X")
	}
	if got := countModel(t, &db.ShowNoteLink{}, "podcast_item_id = ? AND context_label = ?", item.ID, "Referenced in the show"); got != 16 {
		t.Fatalf("links labeled 'Referenced in the show' = %d, want 16", got)
	}
}

func TestPersistShowNoteLinksSkipsOverlongURLs(t *testing.T) {
	setupRetentionTestDB(t)
	podcast := createPodcast(t, "overlong", true)
	item := createShowNotesItem(t, podcast, "overlong-episode", "")
	overlong := "https://example.com/" + strings.Repeat("a", maxShowNoteLinkURLBytes)
	saved, err := persistShowNoteLinks(podcast.ID, item.ID, "", overlong+" https://example.com/ok")
	if err != nil {
		t.Fatal(err)
	}
	if saved != 1 {
		t.Fatalf("saved %d links, want 1", saved)
	}
	if got := countModel(t, &db.ShowNoteLink{}, "podcast_item_id = ?", item.ID); got != 1 {
		t.Fatalf("show note links = %d, want 1", got)
	}
}

func TestBackfillShowNoteLinksFillsMissingAndUnnormalizedLinks(t *testing.T) {
	setupRetentionTestDB(t)
	podcast := createPodcast(t, "backfill", true)
	notes := `<p><a href="https://x.com/Jason">Jason</a> and https://www.reuters.com/business/story</p>`

	missing := createShowNotesItem(t, podcast, "missing", notes)
	// A link stored before normalization existed has no target.
	legacy := createShowNotesItem(t, podcast, "legacy", notes)
	legacyLink := db.ShowNoteLink{PodcastID: podcast.ID, PodcastItemID: legacy.ID, URL: "https://x.com/Jason", Title: "Jason", Domain: "x.com"}
	if err := db.DB.Create(&legacyLink).Error; err != nil {
		t.Fatal(err)
	}
	current := createShowNotesItem(t, podcast, "current", notes)
	if _, err := persistShowNoteLinks(podcast.ID, current.ID, notes, strip.StripTags(notes)); err != nil {
		t.Fatal(err)
	}
	createShowNotesItem(t, podcast, "no-notes", "")

	succeeded, skipped, err := BackfillShowNoteLinks()
	if err != nil {
		t.Fatal(err)
	}
	if succeeded != 2 || skipped != 1 {
		t.Fatalf("succeeded=%d skipped=%d, want 2 and 1", succeeded, skipped)
	}

	for _, item := range []db.PodcastItem{missing, legacy, current} {
		if got := countModel(t, &db.ShowNoteLink{}, "podcast_item_id = ?", item.ID); got != 2 {
			t.Errorf("%s: show note links = %d, want 2", item.GUID, got)
		}
		if got := countModel(t, &db.ShowNoteLink{}, "podcast_item_id = ? AND link_target_id IS NULL", item.ID); got != 0 {
			t.Errorf("%s: links without a target = %d, want 0", item.GUID, got)
		}
	}
	jason, err := db.GetLinkTargetByKey("x:jason")
	if err != nil {
		t.Fatal(err)
	}
	if got := countModel(t, &db.ShowNoteLink{}, "link_target_id = ?", jason.ID); got != 3 {
		t.Fatalf("links pointing at x:jason = %d, want 3", got)
	}

	// A second run has nothing left to do.
	succeeded, skipped, err = BackfillShowNoteLinks()
	if err != nil {
		t.Fatal(err)
	}
	if succeeded != 0 || skipped != 3 {
		t.Fatalf("second run succeeded=%d skipped=%d, want 0 and 3", succeeded, skipped)
	}
}
