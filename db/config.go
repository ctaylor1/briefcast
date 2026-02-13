package db

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type DatabaseDriver string

const (
	DriverSQLite   DatabaseDriver = "sqlite"
	DriverPostgres DatabaseDriver = "postgres"
)

type DatabaseConfig struct {
	Driver        DatabaseDriver
	DataSource    string
	DisplaySource string
}

func ResolveDatabaseConfig() (DatabaseConfig, error) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	explicitDriver := firstNonEmpty(
		strings.TrimSpace(os.Getenv("DB_DRIVER")),
		strings.TrimSpace(os.Getenv("DATABASE_DRIVER")),
	)

	if databaseURL == "" {
		if explicitDriver != "" {
			driver, err := normalizeDriver(explicitDriver)
			if err != nil {
				return DatabaseConfig{}, err
			}
			if driver != DriverSQLite {
				return DatabaseConfig{}, fmt.Errorf("DATABASE_URL is required when driver is %q", driver)
			}
		}

		configPath := strings.TrimSpace(os.Getenv("CONFIG"))
		if configPath == "" {
			configPath = "."
		}
		dbPath := filepath.Join(configPath, "briefcast.db")
		return DatabaseConfig{
			Driver:        DriverSQLite,
			DataSource:    dbPath,
			DisplaySource: dbPath,
		}, nil
	}

	driver, err := resolveDriverFromURL(databaseURL, explicitDriver)
	if err != nil {
		return DatabaseConfig{}, err
	}

	switch driver {
	case DriverSQLite:
		dsn, err := normalizeSQLiteDSN(databaseURL)
		if err != nil {
			return DatabaseConfig{}, err
		}
		return DatabaseConfig{
			Driver:        DriverSQLite,
			DataSource:    dsn,
			DisplaySource: dsn,
		}, nil
	case DriverPostgres:
		return DatabaseConfig{
			Driver:        DriverPostgres,
			DataSource:    databaseURL,
			DisplaySource: sanitizeDataSource(databaseURL),
		}, nil
	default:
		return DatabaseConfig{}, fmt.Errorf("unsupported database driver %q", driver)
	}
}

func resolveDriverFromURL(databaseURL, explicitDriver string) (DatabaseDriver, error) {
	if explicitDriver != "" {
		return normalizeDriver(explicitDriver)
	}
	driver, ok := inferDriverFromURL(databaseURL)
	if !ok {
		return "", fmt.Errorf(
			"could not infer database driver from DATABASE_URL %q; set DB_DRIVER to sqlite or postgres",
			sanitizeDataSource(databaseURL),
		)
	}
	return driver, nil
}

func normalizeDriver(driver string) (DatabaseDriver, error) {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "sqlite", "sqlite3":
		return DriverSQLite, nil
	case "postgres", "postgresql":
		return DriverPostgres, nil
	default:
		return "", fmt.Errorf("unsupported DB driver %q", driver)
	}
}

func inferDriverFromURL(databaseURL string) (DatabaseDriver, bool) {
	raw := strings.TrimSpace(databaseURL)
	if raw == "" {
		return "", false
	}

	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "postgres://"), strings.HasPrefix(lower, "postgresql://"):
		return DriverPostgres, true
	case strings.HasPrefix(lower, "sqlite://"), strings.HasPrefix(lower, "sqlite3://"), strings.HasPrefix(lower, "file:"):
		return DriverSQLite, true
	case strings.HasPrefix(lower, "host="), strings.Contains(lower, " host="):
		return DriverPostgres, true
	case !strings.Contains(raw, "://"):
		return DriverSQLite, true
	default:
		return "", false
	}
}

func normalizeSQLiteDSN(databaseURL string) (string, error) {
	raw := strings.TrimSpace(databaseURL)
	lower := strings.ToLower(raw)

	if strings.HasPrefix(lower, "file:") {
		return raw, nil
	}

	if strings.HasPrefix(lower, "sqlite://") || strings.HasPrefix(lower, "sqlite3://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "", fmt.Errorf("invalid sqlite DATABASE_URL: %w", err)
		}

		dsn := parsed.Path
		if dsn == "" && parsed.Opaque != "" {
			dsn = parsed.Opaque
		}
		if dsn == "" && parsed.Host != "" {
			dsn = parsed.Host
		}
		if dsn == "" {
			dsn = "briefcast.db"
		}
		if parsed.RawQuery != "" {
			dsn = dsn + "?" + parsed.RawQuery
		}
		return dsn, nil
	}

	return raw, nil
}

func sanitizeDataSource(dataSource string) string {
	parsed, err := url.Parse(dataSource)
	if err != nil {
		return dataSource
	}
	if parsed.User == nil {
		return dataSource
	}
	username := parsed.User.Username()
	if _, hasPassword := parsed.User.Password(); !hasPassword {
		return dataSource
	}
	parsed.User = url.UserPassword(username, "xxxxx")
	return parsed.String()
}

func applyConnectionPool(db ConnPoolConfigurable) {
	maxIdle := getEnvInt("DB_MAX_IDLE_CONNS", 10)
	maxOpen := getEnvInt("DB_MAX_OPEN_CONNS", 25)
	connMaxLifetimeMinutes := getEnvInt("DB_CONN_MAX_LIFETIME_MINUTES", 0)

	if maxIdle >= 0 {
		db.SetMaxIdleConns(maxIdle)
	}
	if maxOpen > 0 {
		db.SetMaxOpenConns(maxOpen)
	}
	if connMaxLifetimeMinutes > 0 {
		db.SetConnMaxLifetime(time.Duration(connMaxLifetimeMinutes) * time.Minute)
	}
}

type ConnPoolConfigurable interface {
	SetMaxIdleConns(n int)
	SetMaxOpenConns(n int)
	SetConnMaxLifetime(d time.Duration)
}

func getEnvInt(name string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return defaultValue
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
