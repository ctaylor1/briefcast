package db

import (
	"testing"
	"time"
)

func TestMarkdownProvenanceStorageExistsAfterMigrate(t *testing.T) {
	setupDBForTest(t)
	if !DB.Migrator().HasColumn(&PodcastItem{}, "summary_provenance_json") {
		t.Fatal("missing summary provenance storage")
	}
	if !DB.Migrator().HasTable(&MarkdownRevision{}) {
		t.Fatal("missing Markdown revision storage")
	}
}

func TestDeletingEpisodeDeletesPrivateMarkdownRevisions(t *testing.T) {
	setupDBForTest(t)
	podcast := newPodcast(t, "Private export fixture", "https://example.com/feed.xml")
	item := newPodcastItem(t, podcast.ID, "episode-guid", "Private episode", NotDownloaded, time.Now().UTC())
	record := MarkdownRevision{DocumentID: "document", EpisodeID: item.ID, Revision: 1, ContentType: "transcript", Fingerprint: "fingerprint", Content: "private transcript"}
	if err := DB.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	if err := DeletePodcastItemByID(item.ID); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := DB.Unscoped().Model(&MarkdownRevision{}).Where("episode_id = ?", item.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("deleted episode retained private export history")
	}
}

func newShowNoteLink(t *testing.T, podcastID, podcastItemID, rawURL string) ShowNoteLink {
	t.Helper()
	link := ShowNoteLink{PodcastID: podcastID, PodcastItemID: podcastItemID, URL: rawURL, Domain: "example.com"}
	if err := DB.Create(&link).Error; err != nil {
		t.Fatalf("create show note link: %v", err)
	}
	return link
}

func countShowNoteLinks(t *testing.T, column, value string) int64 {
	t.Helper()
	var count int64
	if err := DB.Unscoped().Model(&ShowNoteLink{}).Where(column+" = ?", value).Count(&count).Error; err != nil {
		t.Fatalf("count show note links: %v", err)
	}
	return count
}

func TestDeletingEpisodeDeletesOnlyItsShowNoteLinks(t *testing.T) {
	setupDBForTest(t)
	podcast := newPodcast(t, "Links fixture", "https://example.com/links.xml")
	deleted := newPodcastItem(t, podcast.ID, "deleted-guid", "Deleted episode", NotDownloaded, time.Now().UTC())
	kept := newPodcastItem(t, podcast.ID, "kept-guid", "Kept episode", NotDownloaded, time.Now().UTC())
	newShowNoteLink(t, podcast.ID, deleted.ID, "https://x.com/chamath")
	newShowNoteLink(t, podcast.ID, deleted.ID, "https://www.reuters.com/article")
	newShowNoteLink(t, podcast.ID, kept.ID, "https://x.com/chamath")

	if err := DeletePodcastItemByID(deleted.ID); err != nil {
		t.Fatal(err)
	}

	if got := countShowNoteLinks(t, "podcast_item_id", deleted.ID); got != 0 {
		t.Fatalf("deleted episode retained %d show note links", got)
	}
	if got := countShowNoteLinks(t, "podcast_item_id", kept.ID); got != 1 {
		t.Fatalf("sibling episode show note links = %d, want 1", got)
	}
}

func TestDeletingPodcastDeletesOnlyItsShowNoteLinks(t *testing.T) {
	setupDBForTest(t)
	deletedPodcast := newPodcast(t, "Deleted podcast", "https://example.com/deleted.xml")
	keptPodcast := newPodcast(t, "Kept podcast", "https://example.com/kept.xml")
	deletedItem := newPodcastItem(t, deletedPodcast.ID, "deleted-guid", "Deleted episode", NotDownloaded, time.Now().UTC())
	keptItem := newPodcastItem(t, keptPodcast.ID, "kept-guid", "Kept episode", NotDownloaded, time.Now().UTC())
	newShowNoteLink(t, deletedPodcast.ID, deletedItem.ID, "https://x.com/friedberg")
	newShowNoteLink(t, keptPodcast.ID, keptItem.ID, "https://x.com/friedberg")

	if err := DeletePodcastByID(deletedPodcast.ID); err != nil {
		t.Fatal(err)
	}

	if got := countShowNoteLinks(t, "podcast_id", deletedPodcast.ID); got != 0 {
		t.Fatalf("deleted podcast retained %d show note links", got)
	}
	if got := countShowNoteLinks(t, "podcast_id", keptPodcast.ID); got != 1 {
		t.Fatalf("other podcast show note links = %d, want 1", got)
	}
}

func TestDeleteOrphanedShowNoteLinksMigrationRemovesOnlyOrphans(t *testing.T) {
	setupDBForTest(t)
	podcast := newPodcast(t, "Orphan fixture", "https://example.com/orphan.xml")
	item := newPodcastItem(t, podcast.ID, "live-guid", "Live episode", NotDownloaded, time.Now().UTC())
	newShowNoteLink(t, podcast.ID, item.ID, "https://x.com/DavidSacks")
	newShowNoteLink(t, podcast.ID, "episode-deleted-before-fix", "https://x.com/DavidSacks")

	var query string
	for _, mig := range migrations {
		if mig.Name == "2026_11_05_01_00_DeleteOrphanedShowNoteLinks" {
			query = mig.Query
		}
	}
	if query == "" {
		t.Fatal("orphaned show note link migration is not registered")
	}
	// Migrate() already recorded this migration on the empty test DB, so run
	// the query directly against the seeded orphan.
	if err := executeMigrationQuery("orphan-cleanup-test", query); err != nil {
		t.Fatal(err)
	}

	if got := countShowNoteLinks(t, "podcast_item_id", "episode-deleted-before-fix"); got != 0 {
		t.Fatalf("orphaned show note links remaining = %d, want 0", got)
	}
	if got := countShowNoteLinks(t, "podcast_item_id", item.ID); got != 1 {
		t.Fatalf("live episode show note links = %d, want 1", got)
	}
}

// TestParseAddColumnIfNotExists handles the corresponding operation.
func TestParseAddColumnIfNotExists(t *testing.T) {
	query := "alter table settings add column if not exists retention_keep_all boolean default true"
	table, column, ok := parseAddColumnIfNotExists(query)
	if !ok {
		t.Fatalf("expected parse to succeed")
	}
	if table != "settings" || column != "retention_keep_all" {
		t.Fatalf("unexpected parse result table=%q column=%q", table, column)
	}

	if _, _, ok := parseAddColumnIfNotExists("update settings set x=1"); ok {
		t.Fatalf("expected parse to fail for non-alter query")
	}
}

// TestExecuteAndSaveMigrationRunsOnce handles the corresponding operation.
func TestExecuteAndSaveMigrationRunsOnce(t *testing.T) {
	setupDBForTest(t)

	name := "test_migration_" + time.Now().Format("150405")
	query := "create table if not exists migration_test_table (id text primary key)"
	if err := ExecuteAndSaveMigration(name, query); err != nil {
		t.Fatalf("ExecuteAndSaveMigration first run failed: %v", err)
	}
	if err := ExecuteAndSaveMigration(name, query); err != nil {
		t.Fatalf("ExecuteAndSaveMigration second run failed: %v", err)
	}

	var migrations []Migration
	if err := DB.Where("name=?", name).Find(&migrations).Error; err != nil {
		t.Fatalf("failed to query migrations: %v", err)
	}
	if len(migrations) != 1 {
		t.Fatalf("expected exactly one migration row, got %d", len(migrations))
	}
}

// TestExecuteMigrationQuerySQLiteSkipsExistingColumn handles the corresponding operation.
func TestExecuteMigrationQuerySQLiteSkipsExistingColumn(t *testing.T) {
	setupDBForTest(t)

	err := executeMigrationQuery(
		"test_skip_existing_column",
		"alter table settings add column if not exists retention_keep_all boolean default true",
	)
	if err != nil {
		t.Fatalf("expected existing-column migration to be skipped without error, got %v", err)
	}
}

// TestPodcastItemCanonicalTranscriptColumnsExistAfterMigrate handles the corresponding operation.
func TestPodcastItemCanonicalTranscriptColumnsExistAfterMigrate(t *testing.T) {
	setupDBForTest(t)

	if !DB.Migrator().HasColumn(&PodcastItem{}, "canonical_transcript") {
		t.Fatalf("expected canonical_transcript column to exist after migrate")
	}
	if !DB.Migrator().HasColumn(&PodcastItem{}, "canonical_transcript_version") {
		t.Fatalf("expected canonical_transcript_version column to exist after migrate")
	}
	if !DB.Migrator().HasColumn(&PodcastItem{}, "canonical_updated_at") {
		t.Fatalf("expected canonical_updated_at column to exist after migrate")
	}
}

func TestInitialDownloadPolicyColumnsExistAfterMigrate(t *testing.T) {
	setupDBForTest(t)

	if !DB.Migrator().HasColumn(&Setting{}, "initial_download_mode") {
		t.Fatalf("expected initial_download_mode column to exist after migrate")
	}
	if !DB.Migrator().HasColumn(&Setting{}, "initial_download_months") {
		t.Fatalf("expected initial_download_months column to exist after migrate")
	}
}

func TestObsidianColumnsExistAfterMigrate(t *testing.T) {
	setupDBForTest(t)

	if !DB.Migrator().HasColumn(&Setting{}, "obsidian_vault") {
		t.Fatalf("expected obsidian_vault column to exist after migrate")
	}
	if !DB.Migrator().HasColumn(&Setting{}, "obsidian_folder") {
		t.Fatalf("expected obsidian_folder column to exist after migrate")
	}

	setting := GetOrCreateSetting()
	if setting.ObsidianVault != DefaultObsidianVault {
		t.Fatalf("expected default obsidian vault %q, got %q", DefaultObsidianVault, setting.ObsidianVault)
	}
	if setting.ObsidianFolder != DefaultObsidianFolder {
		t.Fatalf("expected default obsidian folder %q, got %q", DefaultObsidianFolder, setting.ObsidianFolder)
	}
}
