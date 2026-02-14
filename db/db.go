package db

import (
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

	var dialector gorm.Dialector
	switch config.Driver {
	case DriverSQLite:
		dialector = sqlite.Open(config.DataSource)
	case DriverPostgres:
		dialector = postgres.Open(config.DataSource)
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
		applyConnectionPool(localDB)
	}

	DB = db
	activeDriver = config.Driver
	return DB, nil
}

// Migrate Database
func Migrate() {
	DB.AutoMigrate(&Podcast{}, &PodcastItem{}, &Setting{}, &Migration{}, &JobLock{}, &Tag{})
	RunMigrations()
}

// Using this function to get a connection, you can create your connection pool here.
func GetDB() *gorm.DB {
	return DB
}

func CurrentDriver() DatabaseDriver {
	return activeDriver
}

