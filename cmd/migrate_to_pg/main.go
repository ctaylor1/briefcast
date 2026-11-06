package main

import (
	"flag"
	"fmt"
	"os"

	appdb "github.com/ctaylor1/briefcast/db"
	glebarezsqlite "github.com/glebarez/sqlite"
	_ "github.com/joho/godotenv/autoload"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

func main() {
	sqlitePath := flag.String("sqlite", "", "path to SQLite database (default: config/briefcast.db)")
	pgURL := flag.String("pg", "", "PostgreSQL connection URL (default: DATABASE_URL env var)")
	dryRun := flag.Bool("dry-run", false, "show what would be migrated without writing")
	flag.Parse()

	if *sqlitePath == "" {
		*sqlitePath = envOr("CONFIG", ".") + "/briefcast.db"
		if _, err := os.Stat(*sqlitePath); err != nil {
			*sqlitePath = "config/briefcast.db"
		}
	}
	if *pgURL == "" {
		*pgURL = os.Getenv("DATABASE_URL")
	}
	if *pgURL == "" {
		fmt.Fprintln(os.Stderr, "error: provide --pg or set DATABASE_URL in .env")
		os.Exit(1)
	}

	fmt.Printf("source:  %s\n", *sqlitePath)
	fmt.Printf("target:  %s\n", sanitize(*pgURL))
	fmt.Println()

	src, err := gorm.Open(glebarezsqlite.Open(*sqlitePath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening sqlite: %v\n", err)
		os.Exit(1)
	}

	dst, err := gorm.Open(postgres.Open(*pgURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening postgres: %v\n", err)
		os.Exit(1)
	}

	appdb.DB = dst
	fmt.Println("running AutoMigrate + migrations on target database...")
	appdb.Migrate()
	fmt.Println("schema ready.")
	fmt.Println()

	if err := migrateAll(src, dst, *dryRun); err != nil {
		fmt.Fprintf(os.Stderr, "\nmigration failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nmigration complete.")
}

// tableCopy copies one table. An optional table may be missing from an older source
// database, and is copied only when present, so the source needs no upgrade first.
type tableCopy struct {
	table    string
	optional bool
	run      func(src, dst *gorm.DB, dryRun bool) error
}

// notCopied lists tables deliberately left out. RunMigrations already fills migrations
// on the target. A test fails if a table is in neither this nor tableCopies, so a new
// table cannot be silently lost in a migration.
var notCopied = map[string]bool{"migrations": true}

func copyOf[T any](table string) func(src, dst *gorm.DB, dryRun bool) error {
	return func(src, dst *gorm.DB, dryRun bool) error {
		return migrateTable[T](src, dst, table, dryRun)
	}
}

// tableCopies lists every copied table, parents before the tables that refer to them.
func tableCopies() []tableCopy {
	return []tableCopy{
		{table: "settings", run: copyOf[appdb.Setting]("settings")},
		{table: "podcasts", run: copyOf[appdb.Podcast]("podcasts")},
		{table: "tags", run: copyOf[appdb.Tag]("tags")},
		{table: "podcast_tags", run: migratePodcastTags},
		{table: "podcast_items", run: copyOf[appdb.PodcastItem]("podcast_items")},
		{table: "link_targets", optional: true, run: copyOf[appdb.LinkTarget]("link_targets")},
		{table: "show_note_links", optional: true, run: copyOf[appdb.ShowNoteLink]("show_note_links")},
		{table: "prompt_versions", optional: true, run: copyOf[appdb.PromptVersion]("prompt_versions")},
		{table: "markdown_revisions", optional: true, run: copyOf[appdb.MarkdownRevision]("markdown_revisions")},
		{table: "job_locks", run: copyOf[appdb.JobLock]("job_locks")},
	}
}

func migrateAll(src, dst *gorm.DB, dryRun bool) error {
	for _, step := range tableCopies() {
		if step.optional && !src.Migrator().HasTable(step.table) {
			fmt.Printf("%-20s not in source (skip)\n", step.table)
			continue
		}
		if err := step.run(src, dst, dryRun); err != nil {
			return err
		}
	}
	return nil
}

func migrateTable[T any](src, dst *gorm.DB, label string, dryRun bool) error {
	var rows []T
	if err := src.Unscoped().Find(&rows).Error; err != nil {
		return fmt.Errorf("read %s: %w", label, err)
	}
	fmt.Printf("%-20s %d rows", label, len(rows))
	if len(rows) == 0 {
		fmt.Println(" (skip)")
		return nil
	}
	if dryRun {
		fmt.Println(" (dry-run)")
		return nil
	}

	batchSize := 100
	for i := 0; i < len(rows); i += batchSize {
		end := i + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		if err := dst.Session(&gorm.Session{SkipHooks: true}).Omit(clause.Associations).Create(rows[i:end]).Error; err != nil {
			return fmt.Errorf("write %s batch %d: %w", label, i/batchSize, err)
		}
	}
	fmt.Println(" (done)")
	return nil
}

type podcastTag struct {
	PodcastID string `gorm:"column:podcast_id"`
	TagID     string `gorm:"column:tag_id"`
}

func (podcastTag) TableName() string { return "podcast_tags" }

func migratePodcastTags(src, dst *gorm.DB, dryRun bool) error {
	var rows []podcastTag
	if err := src.Find(&rows).Error; err != nil {
		return fmt.Errorf("read podcast_tags: %w", err)
	}
	fmt.Printf("%-20s %d rows", "podcast_tags", len(rows))
	if len(rows) == 0 {
		fmt.Println(" (skip)")
		return nil
	}
	if dryRun {
		fmt.Println(" (dry-run)")
		return nil
	}
	if err := dst.Create(&rows).Error; err != nil {
		return fmt.Errorf("write podcast_tags: %w", err)
	}
	fmt.Println(" (done)")
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func sanitize(url string) string {
	for i, c := range url {
		if c == '@' {
			for j := i - 1; j >= 0; j-- {
				if url[j] == ':' && j > 10 {
					return url[:j+1] + "***" + url[i:]
				}
			}
		}
	}
	return url
}
