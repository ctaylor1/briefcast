package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/logging"
	"github.com/ctaylor1/briefcast/service"
	glebarezsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func main() {
	defer logging.Sync()

	batchSize := flag.Int("batch-size", 500, "number of rows to process per batch")
	targetVersion := flag.Int("version", 1, "canonical transcript version target")
	flag.Parse()

	conn, err := db.Init()
	if err != nil {
		cfg, cfgErr := db.ResolveDatabaseConfig()
		if cfgErr == nil && cfg.Driver == db.DriverSQLite && isSQLiteCGOStubError(err) {
			conn, err = gorm.Open(glebarezsqlite.Open(cfg.DataSource), &gorm.Config{})
			if err == nil {
				db.DB = conn
			}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "database init failed: %v\n", err)
			os.Exit(1)
		}
	}
	if conn != nil {
		db.DB = conn
	}
	db.Migrate()

	updated, err := service.BackfillCanonicalTranscripts(*batchSize, *targetVersion)
	if err != nil {
		fmt.Fprintf(os.Stderr, "canonical transcript backfill failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf(
		"canonical transcript backfill complete: updated=%d batch_size=%d version=%d\n",
		updated,
		*batchSize,
		*targetVersion,
	)
}

func isSQLiteCGOStubError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "cgo_enabled=0") || strings.Contains(lower, "requires cgo")
}
