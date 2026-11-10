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

// Migrate brings the schema up to date. It stops at the first failure and
// returns it: later migrations can depend on earlier ones (a dedupe before a
// unique index), and serving on a half-migrated schema risks bad writes.
func Migrate() error {
	if err := DB.AutoMigrate(&Podcast{}, &PodcastItem{}, &Setting{}, &Migration{}, &JobLock{}, &Tag{}, &ShowNoteLink{}, &LinkTarget{}, &PromptVersion{}, &MarkdownRevision{}); err != nil {
		logging.Sugar().Errorw("database automigrate failed", "error", err)
		return fmt.Errorf("automigrate: %w", err)
	}
	return RunMigrations()
}

// SQLiteSnapshot writes a consistent copy of the open SQLite database to
// dest with VACUUM INTO. dest must not exist. It is an error on Postgres.
func SQLiteSnapshot(dest string) error {
	if activeDriver != DriverSQLite {
		return fmt.Errorf("sqlite snapshot requested on %s", activeDriver)
	}
	return DB.Exec("VACUUM INTO ?", dest).Error
}

// SQLiteFilePath returns the file path of the configured SQLite database,
// without a file: prefix or query options.
func SQLiteFilePath() (string, error) {
	config, err := ResolveDatabaseConfig()
	if err != nil {
		return "", err
	}
	if config.Driver != DriverSQLite {
		return "", fmt.Errorf("database driver is %s, not sqlite", config.Driver)
	}
	source := strings.TrimPrefix(config.DataSource, "file:")
	if i := strings.IndexByte(source, '?'); i >= 0 {
		source = source[:i]
	}
	return source, nil
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
