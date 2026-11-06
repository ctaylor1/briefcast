package db

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/ctaylor1/briefcast/internal/logging"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// DB is
var DB *gorm.DB
var activeDriver DatabaseDriver = DriverSQLite

// Init is used to Initialize Database
func Init() (*gorm.DB, error) {
	config, err := ResolveDatabaseConfig()
	if err != nil {
		logging.Sugar().Errorw("database configuration resolution failed", "error", err)
		return nil, err
	}

	dataSource, sqliteBusyTimeoutMS := connectionDataSource(config)
	var dialector gorm.Dialector
	switch config.Driver {
	case DriverSQLite:
		dialector = sqlite.Open(dataSource)
	case DriverPostgres:
		dialector = postgres.Open(dataSource)
	default:
		return nil, fmt.Errorf("unsupported database driver %q", config.Driver)
	}

	logging.Sugar().Infow("database driver configured", "driver", config.Driver)
	if config.DisplaySource != "" {
		logging.Sugar().Infow("database source configured", "source", strings.TrimSpace(config.DisplaySource))
	}

	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		logging.Sugar().Errorw("database connection open failed", "error", err)
		return nil, err
	}

	localDB, err := db.DB()
	if err == nil {
		maxOpenConns := applyConnectionPool(localDB, config.Driver)
		if config.Driver == DriverSQLite {
			logSQLiteSettings(localDB, sqliteBusyTimeoutMS, maxOpenConns)
		}
	}

	DB = db
	activeDriver = config.Driver
	return DB, nil
}

func logSQLiteSettings(localDB *sql.DB, busyTimeoutMS int, maxOpenConns int) {
	var journalMode string
	if err := localDB.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		logging.Sugar().Warnw("could not read sqlite journal mode", "error", err)
	}
	logging.Sugar().Infow("sqlite connection settings", "busy_timeout_ms", busyTimeoutMS, "max_open_conns", maxOpenConns, "journal_mode", journalMode)
}

// Migrate Database
func Migrate() {
	if err := DB.AutoMigrate(&Podcast{}, &PodcastItem{}, &Setting{}, &Migration{}, &JobLock{}, &Tag{}, &ShowNoteLink{}, &LinkTarget{}, &PromptVersion{}, &MarkdownRevision{}); err != nil {
		logging.Sugar().Errorw("database automigrate failed", "error", err)
		return
	}
	RunMigrations()
}

// GetDB handles the corresponding operation.
// Using this function to get a connection, you can create your connection pool here.
func GetDB() *gorm.DB {
	return DB
}

// CurrentDriver handles the corresponding operation.
func CurrentDriver() DatabaseDriver {
	return activeDriver
}
