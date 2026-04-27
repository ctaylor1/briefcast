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

func migrateAll(src, dst *gorm.DB, dryRun bool) error {
	if err := migrateTable[appdb.Setting](src, dst, "settings", dryRun); err != nil {
		return err
	}
	if err := migrateTable[appdb.Podcast](src, dst, "podcasts", dryRun); err != nil {
		return err
	}
	if err := migrateTable[appdb.Tag](src, dst, "tags", dryRun); err != nil {
		return err
	}
	if err := migratePodcastTags(src, dst, dryRun); err != nil {
		return err
	}
	if err := migrateTable[appdb.PodcastItem](src, dst, "podcast_items", dryRun); err != nil {
		return err
	}
	if err := migrateTable[appdb.JobLock](src, dst, "job_locks", dryRun); err != nil {
		return err
	}
	// Skip migrations table — RunMigrations already populated it.
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
