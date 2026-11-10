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
	if err := appdb.Migrate(); err != nil {
		fmt.Fprintf(os.Stderr, "schema setup failed: %v\n", err)
		os.Exit(1)
	}
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

// requiredParents lists foreign keys that Postgres enforces and SQLite never did, so a
// long-lived source can break them. Dropping such rows silently would lose data, so
// the migration stops and leaves the choice to the operator.
var requiredParents = []struct{ table, column, parent string }{
	{"podcast_items", "podcast_id", "podcasts"},
	{"markdown_revisions", "episode_id", "podcast_items"},
}

func checkSource(src *gorm.DB) error {
	for _, check := range requiredParents {
		if !src.Migrator().HasTable(check.table) {
			continue
		}
		var orphans int64
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s NOT IN (SELECT id FROM %s)", check.table, check.column, check.parent)
		if err := src.Raw(query).Scan(&orphans).Error; err != nil {
			return fmt.Errorf("check %s: %w", check.table, err)
		}
		if orphans > 0 {
			return fmt.Errorf("%d rows in %s point at a missing %s row, which Postgres would reject. Nothing was written. Remove them from a copy of the source, then run this again", orphans, check.table, check.parent)
		}
	}
	return nil
}

func checkTargetEmpty(dst *gorm.DB) error {
	for _, table := range []string{"settings", "podcasts", "podcast_items"} {
		var count int64
		if err := dst.Table(table).Count(&count).Error; err != nil {
			return fmt.Errorf("check target %s: %w", table, err)
		}
		if count > 0 {
			return fmt.Errorf("target is not empty: %s already has %d rows. Use a new empty database, and do not start the app against it before migrating", table, count)
		}
	}
	return nil
}

func migrateAll(src, dst *gorm.DB, dryRun bool) error {
	if err := checkSource(src); err != nil {
		return err
	}
	if dryRun {
		return copyTables(src, dst, true)
	}
	if err := checkTargetEmpty(dst); err != nil {
		return err
	}
	// One transaction, so a failure leaves the target empty and a retry is safe.
	return dst.Transaction(func(tx *gorm.DB) error {
		return copyTables(src, tx, false)
	})
}

func copyTables(src, dst *gorm.DB, dryRun bool) error {
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
	var total int64
	if err := src.Model(&podcastTag{}).Count(&total).Error; err != nil {
		return fmt.Errorf("count podcast_tags: %w", err)
	}
	// SQLite let deleted podcasts leave these links behind. They point at nothing and
	// Postgres would reject them, so skipping them loses no information.
	var rows []podcastTag
	if err := src.Where("podcast_id IN (SELECT id FROM podcasts) AND tag_id IN (SELECT id FROM tags)").Find(&rows).Error; err != nil {
		return fmt.Errorf("read podcast_tags: %w", err)
	}
	fmt.Printf("%-20s %d rows", "podcast_tags", len(rows))
	if skipped := total - int64(len(rows)); skipped > 0 {
		fmt.Printf(", %d orphaned links skipped", skipped)
	}
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
