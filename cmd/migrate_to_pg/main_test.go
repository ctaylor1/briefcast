package main

import (
	"path/filepath"
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
