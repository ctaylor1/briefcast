package db

import (
	"testing"
	"time"
)

func testTarget(key, kind string, version int) *LinkTarget {
	return &LinkTarget{
		NormalizedKey:     key,
		CanonicalURL:      "https://example.test/" + key,
		Kind:              kind,
		Platform:          "x",
		Domain:            "x.com",
		ExtractionVersion: version,
	}
}

// testLinkWrite records the link under its target's version (1 without one).
func testLinkWrite(podcastID, itemID, rawURL string, position int, target, parent *LinkTarget) ShowNoteLinkWrite {
	version := 1
	if target != nil {
		version = target.ExtractionVersion
	}
	return ShowNoteLinkWrite{
		Link: ShowNoteLink{
			PodcastID: podcastID, PodcastItemID: itemID, URL: rawURL, Domain: "x.com",
			Position: position, ExtractionVersion: version,
		},
		Target: target,
		Parent: parent,
	}
}

func countRows(t *testing.T, model any, where string, args ...any) int64 {
	t.Helper()
	var count int64
	query := DB.Model(model)
	if where != "" {
		query = query.Where(where, args...)
	}
	if err := query.Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	return count
}

func linkFor(t *testing.T, itemID, rawURL string) ShowNoteLink {
	t.Helper()
	var link ShowNoteLink
	if err := DB.Where("podcast_item_id = ? AND url = ?", itemID, rawURL).First(&link).Error; err != nil {
		t.Fatalf("load link %q: %v", rawURL, err)
	}
	return link
}

func TestSaveShowNoteLinksIsIdempotent(t *testing.T) {
	setupDBForTest(t)
	podcast := newPodcast(t, "Idempotent", "https://example.com/idempotent.xml")
	item := newPodcastItem(t, podcast.ID, "guid", "Episode", NotDownloaded, time.Now().UTC())
	writes := []ShowNoteLinkWrite{
		testLinkWrite(podcast.ID, item.ID, "https://x.com/chamath", 0, testTarget("x:chamath", "profile", 1), nil),
		testLinkWrite(podcast.ID, item.ID, "https://example.com/a", 1, testTarget("web:example.com/a", "web", 1), nil),
	}

	for run := 0; run < 2; run++ {
		if err := SaveShowNoteLinks(writes); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
	}

	if got := countRows(t, &ShowNoteLink{}, "podcast_item_id = ?", item.ID); got != 2 {
		t.Fatalf("show note links = %d, want 2", got)
	}
	if got := countRows(t, &LinkTarget{}, ""); got != 2 {
		t.Fatalf("link targets = %d, want 2", got)
	}
	target, err := GetLinkTargetByKey("x:chamath")
	if err != nil {
		t.Fatal(err)
	}
	link := linkFor(t, item.ID, "https://x.com/chamath")
	if link.LinkTargetID == nil || *link.LinkTargetID != target.ID {
		t.Fatalf("link target id = %v, want %s", link.LinkTargetID, target.ID)
	}
}

func TestSaveShowNoteLinksLinksPostToParentProfile(t *testing.T) {
	setupDBForTest(t)
	podcast := newPodcast(t, "Parents", "https://example.com/parents.xml")
	item := newPodcastItem(t, podcast.ID, "guid", "Episode", NotDownloaded, time.Now().UTC())
	profile := testTarget("x:andrewbenson", "profile", 1)
	writes := []ShowNoteLinkWrite{
		testLinkWrite(podcast.ID, item.ID, "https://x.com/AndrewBenson/status/1", 0, testTarget("x:status:1", "post", 1), profile),
		// The same episode also links the author's profile directly.
		testLinkWrite(podcast.ID, item.ID, "https://x.com/AndrewBenson", 1, profile, nil),
	}
	if err := SaveShowNoteLinks(writes); err != nil {
		t.Fatal(err)
	}

	parent, err := GetLinkTargetByKey("x:andrewbenson")
	if err != nil {
		t.Fatal(err)
	}
	post, err := GetLinkTargetByKey("x:status:1")
	if err != nil {
		t.Fatal(err)
	}
	if post.ParentTargetID == nil || *post.ParentTargetID != parent.ID {
		t.Fatalf("post parent = %v, want %s", post.ParentTargetID, parent.ID)
	}
	if parent.ParentTargetID != nil {
		t.Fatalf("profile parent = %v, want nil", *parent.ParentTargetID)
	}
	if got := countRows(t, &LinkTarget{}, ""); got != 2 {
		t.Fatalf("link targets = %d, want 2 (profile reused)", got)
	}
	profileLink := linkFor(t, item.ID, "https://x.com/AndrewBenson")
	if profileLink.LinkTargetID == nil || *profileLink.LinkTargetID != parent.ID {
		t.Fatalf("profile link target = %v, want %s", profileLink.LinkTargetID, parent.ID)
	}
}

func TestSaveShowNoteLinksSharesTargetAcrossEpisodes(t *testing.T) {
	setupDBForTest(t)
	podcast := newPodcast(t, "Shared", "https://example.com/shared.xml")
	first := newPodcastItem(t, podcast.ID, "guid-1", "Episode 1", NotDownloaded, time.Now().UTC())
	second := newPodcastItem(t, podcast.ID, "guid-2", "Episode 2", NotDownloaded, time.Now().UTC())
	if err := SaveShowNoteLinks([]ShowNoteLinkWrite{
		testLinkWrite(podcast.ID, first.ID, "https://x.com/Jason", 0, testTarget("x:jason", "profile", 1), nil),
	}); err != nil {
		t.Fatal(err)
	}
	if err := SaveShowNoteLinks([]ShowNoteLinkWrite{
		testLinkWrite(podcast.ID, second.ID, "https://twitter.com/jason?s=20", 0, testTarget("x:jason", "profile", 1), nil),
	}); err != nil {
		t.Fatal(err)
	}

	target, err := GetLinkTargetByKey("x:jason")
	if err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, &LinkTarget{}, ""); got != 1 {
		t.Fatalf("link targets = %d, want 1", got)
	}
	if got := countRows(t, &ShowNoteLink{}, "link_target_id = ?", target.ID); got != 2 {
		t.Fatalf("links pointing at x:jason = %d, want 2", got)
	}
}

func TestSaveShowNoteLinksRepointsAfterNormalizerChange(t *testing.T) {
	setupDBForTest(t)
	podcast := newPodcast(t, "Renormalize", "https://example.com/renormalize.xml")
	item := newPodcastItem(t, podcast.ID, "guid", "Episode", NotDownloaded, time.Now().UTC())
	rawURL := "https://example.com/Article"
	if err := SaveShowNoteLinks([]ShowNoteLinkWrite{
		testLinkWrite(podcast.ID, item.ID, rawURL, 0, testTarget("web:example.com/Article", "web", 1), nil),
	}); err != nil {
		t.Fatal(err)
	}
	if err := SaveShowNoteLinks([]ShowNoteLinkWrite{
		testLinkWrite(podcast.ID, item.ID, rawURL, 0, testTarget("web:example.com/article", "web", 2), nil),
	}); err != nil {
		t.Fatal(err)
	}

	newTarget, err := GetLinkTargetByKey("web:example.com/article")
	if err != nil {
		t.Fatal(err)
	}
	link := linkFor(t, item.ID, rawURL)
	if link.LinkTargetID == nil || *link.LinkTargetID != newTarget.ID {
		t.Fatalf("link target = %v, want %s", link.LinkTargetID, newTarget.ID)
	}
	if got := countRows(t, &ShowNoteLink{}, "podcast_item_id = ?", item.ID); got != 1 {
		t.Fatalf("show note links = %d, want 1", got)
	}
}

func TestSaveShowNoteLinksStoresLinkWithoutTarget(t *testing.T) {
	setupDBForTest(t)
	podcast := newPodcast(t, "No target", "https://example.com/no-target.xml")
	item := newPodcastItem(t, podcast.ID, "guid", "Episode", NotDownloaded, time.Now().UTC())
	if err := SaveShowNoteLinks([]ShowNoteLinkWrite{
		testLinkWrite(podcast.ID, item.ID, "https://unnormalizable.example", 0, nil, nil),
	}); err != nil {
		t.Fatal(err)
	}
	link := linkFor(t, item.ID, "https://unnormalizable.example")
	if link.LinkTargetID != nil {
		t.Fatalf("link target = %v, want nil", *link.LinkTargetID)
	}
}

func TestShowNoteLinksUniqueIndexRejectsDuplicateOccurrence(t *testing.T) {
	setupDBForTest(t)
	podcast := newPodcast(t, "Unique", "https://example.com/unique.xml")
	item := newPodcastItem(t, podcast.ID, "guid", "Episode", NotDownloaded, time.Now().UTC())
	newShowNoteLink(t, podcast.ID, item.ID, "https://x.com/friedberg")
	duplicate := ShowNoteLink{PodcastID: podcast.ID, PodcastItemID: item.ID, URL: "https://x.com/friedberg"}
	if err := DB.Create(&duplicate).Error; err == nil {
		t.Fatal("duplicate (episode, url) insert succeeded; unique index missing")
	}
}

func TestDeleteDuplicateShowNoteLinksMigrationKeepsFirstOccurrence(t *testing.T) {
	setupDBForTest(t)
	// Simulate a database from before the unique index existed.
	if err := DB.Exec("drop index if exists idx_show_note_links_item_url").Error; err != nil {
		t.Fatal(err)
	}
	podcast := newPodcast(t, "Duplicates", "https://example.com/duplicates.xml")
	item := newPodcastItem(t, podcast.ID, "guid", "Episode", NotDownloaded, time.Now().UTC())
	other := newPodcastItem(t, podcast.ID, "guid-2", "Other", NotDownloaded, time.Now().UTC())
	for _, link := range []ShowNoteLink{
		{PodcastID: podcast.ID, PodcastItemID: item.ID, URL: "https://x.com/jason", Position: 3},
		{PodcastID: podcast.ID, PodcastItemID: item.ID, URL: "https://x.com/jason", Position: 0},
		{PodcastID: podcast.ID, PodcastItemID: other.ID, URL: "https://x.com/jason", Position: 5},
	} {
		link := link
		if err := DB.Create(&link).Error; err != nil {
			t.Fatal(err)
		}
	}

	for _, name := range []string{
		"2026_11_05_02_00_DeleteDuplicateShowNoteLinks",
		"2026_11_05_02_01_AddShowNoteLinksItemURLUniqueIndex",
	} {
		if err := executeMigrationQuery(name, migrationQueryByName(t, name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}

	var kept []ShowNoteLink
	if err := DB.Where("podcast_item_id = ?", item.ID).Find(&kept).Error; err != nil {
		t.Fatal(err)
	}
	if len(kept) != 1 || kept[0].Position != 0 {
		t.Fatalf("kept = %+v, want the single position-0 occurrence", kept)
	}
	if got := countRows(t, &ShowNoteLink{}, "podcast_item_id = ?", other.ID); got != 1 {
		t.Fatalf("other episode links = %d, want 1", got)
	}
}

func TestPodcastItemIDsNeedingLinkExtraction(t *testing.T) {
	setupDBForTest(t)
	podcast := newPodcast(t, "Stale", "https://example.com/stale.xml")
	legacy := newPodcastItem(t, podcast.ID, "guid-1", "Legacy", NotDownloaded, time.Now().UTC())
	stale := newPodcastItem(t, podcast.ID, "guid-2", "Stale", NotDownloaded, time.Now().UTC())
	current := newPodcastItem(t, podcast.ID, "guid-3", "Current", NotDownloaded, time.Now().UTC())
	unnormalizable := newPodcastItem(t, podcast.ID, "guid-4", "Unnormalizable", NotDownloaded, time.Now().UTC())
	noLinks := newPodcastItem(t, podcast.ID, "guid-5", "No links", NotDownloaded, time.Now().UTC())

	// Captured before versioning: extraction_version defaults to 0.
	newShowNoteLink(t, podcast.ID, legacy.ID, "https://x.com/a")
	// Saved under v1, then another episode re-saves the same shared target
	// under v2. The v1 episode must still be reported.
	if err := SaveShowNoteLinks([]ShowNoteLinkWrite{
		testLinkWrite(podcast.ID, stale.ID, "https://x.com/shared", 0, testTarget("x:shared", "profile", 1), nil),
	}); err != nil {
		t.Fatal(err)
	}
	unnormalizableWrite := testLinkWrite(podcast.ID, unnormalizable.ID, "https://bad.example", 0, nil, nil)
	unnormalizableWrite.Link.ExtractionVersion = 2
	if err := SaveShowNoteLinks([]ShowNoteLinkWrite{
		testLinkWrite(podcast.ID, current.ID, "https://twitter.com/shared", 0, testTarget("x:shared", "profile", 2), nil),
		unnormalizableWrite,
	}); err != nil {
		t.Fatal(err)
	}

	needs, err := PodcastItemIDsNeedingLinkExtraction(2)
	if err != nil {
		t.Fatal(err)
	}
	if !needs[legacy.ID] || !needs[stale.ID] || len(needs) != 2 {
		t.Fatalf("needing extraction = %v, want only legacy %s and stale %s", needs, legacy.ID, stale.ID)
	}

	withLinks, err := PodcastItemIDsWithShowNoteLinks()
	if err != nil {
		t.Fatal(err)
	}
	if len(withLinks) != 4 || withLinks[noLinks.ID] {
		t.Fatalf("with links = %v, want the four linked episodes", withLinks)
	}
}

func migrationQueryByName(t *testing.T, name string) string {
	t.Helper()
	for _, mig := range migrations {
		if mig.Name == name {
			return mig.Query
		}
	}
	t.Fatalf("migration %s is not registered", name)
	return ""
}
