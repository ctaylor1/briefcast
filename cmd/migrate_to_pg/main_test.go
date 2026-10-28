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
