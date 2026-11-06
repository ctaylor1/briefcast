package main

import (
	"path/filepath"
	"strings"
	"testing"

	appdb "github.com/ctaylor1/briefcast/db"
	glebarezsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMigrationPreservesMarkdownHistoryAndSupportsOlderSources(t *testing.T) {
	for _, withHistory := range []bool{false, true} {
		name := "legacy"
		if withHistory {
			name = "with-history"
		}
		t.Run(name, func(t *testing.T) {
			open := func(name string) *gorm.DB {
				database, err := gorm.Open(glebarezsqlite.Open(filepath.Join(t.TempDir(), name+".db")), &gorm.Config{})
				if err != nil {
					t.Fatal(err)
				}
				sqlDB, err := database.DB()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { sqlDB.Close() })
				if err := database.AutoMigrate(&appdb.Podcast{}, &appdb.PodcastItem{}, &appdb.Setting{}, &appdb.Tag{}, &appdb.JobLock{}); err != nil {
					t.Fatal(err)
				}
				return database
			}
			source, target := open("source"), open("target")
			if err := target.AutoMigrate(&appdb.MarkdownRevision{}); err != nil {
				t.Fatal(err)
			}
			var original appdb.MarkdownRevision
			if withHistory {
				if err := source.AutoMigrate(&appdb.MarkdownRevision{}); err != nil {
					t.Fatal(err)
				}
				podcast := appdb.Podcast{Title: "Fixture", URL: "https://example.com/feed.xml"}
				if err := source.Create(&podcast).Error; err != nil {
					t.Fatal(err)
				}
				item := appdb.PodcastItem{PodcastID: podcast.ID, Title: "Fixture episode", SummaryProvenanceJSON: `{"historical":"retained"}`}
				if err := source.Create(&item).Error; err != nil {
					t.Fatal(err)
				}
				original = appdb.MarkdownRevision{EpisodeID: item.ID, DocumentID: "urn:uuid:00000000-0000-4000-8000-000000000001", Revision: 3, ContentType: "summary", Fingerprint: "fixture", Content: "exact immutable Markdown bytes"}
				if err := source.Create(&original).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := migrateAll(source, target, false); err != nil {
				t.Fatal(err)
			}
			var copied []appdb.MarkdownRevision
			if err := target.Find(&copied).Error; err != nil {
				t.Fatal(err)
			}
			if !withHistory && len(copied) != 0 {
				t.Fatal("invented export history")
			}
			if withHistory && (len(copied) != 1 || copied[0].ID != original.ID || copied[0].DocumentID != original.DocumentID || copied[0].Revision != original.Revision || copied[0].Content != original.Content) {
				t.Fatal("migration lost export identity or bytes")
			}
		})
	}
}

func openMigratedSQLite(t *testing.T, name string) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(glebarezsqlite.Open(filepath.Join(t.TempDir(), name+".db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	previous := appdb.DB
	appdb.DB = database
	t.Cleanup(func() { appdb.DB = previous })
	appdb.Migrate()
	return database
}

func TestEveryAppTableIsCopiedOrDeliberatelySkipped(t *testing.T) {
	database := openMigratedSQLite(t, "schema")
	var tables []string
	if err := database.Raw("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'").Scan(&tables).Error; err != nil {
		t.Fatal(err)
	}
	if len(tables) == 0 {
		t.Fatal("found no tables, so this test proves nothing")
	}
	copied := map[string]bool{}
	for _, step := range tableCopies() {
		copied[step.table] = true
	}
	for _, table := range tables {
		if !copied[table] && !notCopied[table] {
			t.Errorf("table %q would be lost in a migration: add it to tableCopies or notCopied", table)
		}
	}
}

func seedPodcastWithEpisode(t *testing.T, database *gorm.DB) appdb.PodcastItem {
	t.Helper()
	podcast := appdb.Podcast{Title: "Fixture", URL: "https://example.com/feed.xml"}
	if err := database.Create(&podcast).Error; err != nil {
		t.Fatal(err)
	}
	item := appdb.PodcastItem{PodcastID: podcast.ID, Title: "Fixture episode"}
	if err := database.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	return item
}

func countRows(t *testing.T, database *gorm.DB, table string) int64 {
	t.Helper()
	var count int64
	if err := database.Table(table).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

// SQLite never enforced these foreign keys, so a long-lived database can hold rows that
// Postgres would reject.
func TestMigrationSkipsOrphanedTagLinksAndKeepsTheRest(t *testing.T) {
	source := openMigratedSQLite(t, "source")
	item := seedPodcastWithEpisode(t, source)
	tag := appdb.Tag{Label: "news"}
	if err := source.Create(&tag).Error; err != nil {
		t.Fatal(err)
	}
	for _, link := range []podcastTag{
		{PodcastID: item.PodcastID, TagID: tag.ID},
		{PodcastID: "deleted-podcast", TagID: tag.ID},
		{PodcastID: item.PodcastID, TagID: "deleted-tag"},
	} {
		if err := source.Create(&link).Error; err != nil {
			t.Fatal(err)
		}
	}

	destination := openMigratedSQLite(t, "target")
	if err := migrateAll(source, destination, false); err != nil {
		t.Fatal(err)
	}

	var copied []podcastTag
	if err := destination.Find(&copied).Error; err != nil {
		t.Fatal(err)
	}
	if len(copied) != 1 || copied[0].PodcastID != item.PodcastID || copied[0].TagID != tag.ID {
		t.Fatalf("expected only the valid tag link, got %+v", copied)
	}
}

func TestMigrationStopsBeforeWritingWhenRowsAreOrphaned(t *testing.T) {
	cases := map[string]func(t *testing.T, source *gorm.DB){
		"episode without a podcast": func(t *testing.T, source *gorm.DB) {
			orphan := appdb.PodcastItem{PodcastID: "deleted-podcast", Title: "Orphan"}
			if err := source.Create(&orphan).Error; err != nil {
				t.Fatal(err)
			}
		},
		"revision without an episode": func(t *testing.T, source *gorm.DB) {
			if err := source.Exec("INSERT INTO markdown_revisions (id, document_id, revision, episode_id, content_type, fingerprint, content) VALUES ('r1', 'd1', 1, 'deleted-episode', 'summary', 'f', 'c')").Error; err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, makeOrphan := range cases {
		t.Run(name, func(t *testing.T) {
			source := openMigratedSQLite(t, "source")
			seedPodcastWithEpisode(t, source)
			makeOrphan(t, source)
			destination := openMigratedSQLite(t, "target")

			err := migrateAll(source, destination, false)

			if err == nil || !strings.Contains(err.Error(), "missing") {
				t.Fatalf("expected an orphaned row error, got %v", err)
			}
			if got := countRows(t, destination, "podcasts"); got != 0 {
				t.Fatalf("wrote %d podcasts before stopping", got)
			}
		})
	}
}

func TestMigrationWritesNothingWhenACopyFails(t *testing.T) {
	source := openMigratedSQLite(t, "source")
	seedPodcastWithEpisode(t, source)
	lock := appdb.JobLock{Name: "fixture"}
	if err := source.Create(&lock).Error; err != nil {
		t.Fatal(err)
	}
	destination := openMigratedSQLite(t, "target")
	// job_locks is copied last, so this fails after everything else has been written.
	if err := destination.Exec("DROP TABLE job_locks").Error; err != nil {
		t.Fatal(err)
	}

	if err := migrateAll(source, destination, false); err == nil {
		t.Fatal("expected the copy to fail")
	}

	for _, table := range []string{"podcasts", "podcast_items"} {
		if got := countRows(t, destination, table); got != 0 {
			t.Fatalf("left %d rows in %s after a failed migration", got, table)
		}
	}
}

func TestMigrationRefusesATargetThatAlreadyHasData(t *testing.T) {
	source := openMigratedSQLite(t, "source")
	seedPodcastWithEpisode(t, source)
	destination := openMigratedSQLite(t, "target")
	seedPodcastWithEpisode(t, destination)

	err := migrateAll(source, destination, false)

	if err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("expected a not-empty error, got %v", err)
	}
}

func TestMigrationCopiesLinksAndPromptHistory(t *testing.T) {
	source := openMigratedSQLite(t, "source")
	podcast := appdb.Podcast{Title: "Fixture", URL: "https://example.com/feed.xml"}
	if err := source.Create(&podcast).Error; err != nil {
		t.Fatal(err)
	}
	item := appdb.PodcastItem{PodcastID: podcast.ID, Title: "Fixture episode"}
	if err := source.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	target := appdb.LinkTarget{NormalizedKey: "x:fixture", CanonicalURL: "https://x.com/fixture", Kind: "profile", Platform: "x", ExtractionVersion: 1}
	if err := source.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	link := appdb.ShowNoteLink{PodcastItemID: item.ID, PodcastID: podcast.ID, URL: "https://x.com/fixture", LinkTargetID: &target.ID}
	if err := source.Create(&link).Error; err != nil {
		t.Fatal(err)
	}
	prompt := appdb.PromptVersion{PromptType: "summary", Content: "an earlier prompt"}
	if err := source.Create(&prompt).Error; err != nil {
		t.Fatal(err)
	}

	destination := openMigratedSQLite(t, "target")
	if err := migrateAll(source, destination, false); err != nil {
		t.Fatal(err)
	}

	var copiedTarget appdb.LinkTarget
	if err := destination.First(&copiedTarget, "id = ?", target.ID).Error; err != nil {
		t.Fatalf("link target was not copied: %v", err)
	}
	var copiedLink appdb.ShowNoteLink
	if err := destination.First(&copiedLink, "id = ?", link.ID).Error; err != nil {
		t.Fatalf("show note link was not copied: %v", err)
	}
	if copiedLink.LinkTargetID == nil || *copiedLink.LinkTargetID != target.ID {
		t.Fatalf("show note link lost its target: %+v", copiedLink)
	}
	var copiedPrompt appdb.PromptVersion
	if err := destination.First(&copiedPrompt, "id = ?", prompt.ID).Error; err != nil {
		t.Fatalf("prompt version was not copied: %v", err)
	}
	if copiedPrompt.Content != prompt.Content {
		t.Fatalf("prompt history changed: %q", copiedPrompt.Content)
	}
}
