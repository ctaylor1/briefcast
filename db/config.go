package db

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DatabaseDriver represents a public type.
type DatabaseDriver string

const (
	// DriverSQLite is a public constant.
	DriverSQLite DatabaseDriver = "sqlite"
	// DriverPostgres is a public constant.
	DriverPostgres DatabaseDriver = "postgres"
)

const (
	defaultMaxOpenConns        = 25
	defaultSQLiteMaxOpenConns  = 4
	defaultSQLiteBusyTimeoutMS = 5000
)

var sqliteBusyTimeoutPragma = regexp.MustCompile(`(?i)^\s*busy_timeout\s*[(=]\s*(\d+)`)

// DatabaseConfig represents a public type.
type DatabaseConfig struct {
	Driver        DatabaseDriver
	DataSource    string
	DisplaySource string
}

// ResolveDatabaseConfig handles the corresponding operation.
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

// connectionDataSource returns the data source to open and, for SQLite, the
// effective busy timeout. ResolveDatabaseConfig stays a plain path because the
// command-line tools under cmd/ read it directly.
func connectionDataSource(config DatabaseConfig) (string, int) {
	if config.Driver != DriverSQLite {
		return config.DataSource, 0
	}
	return withSQLiteBusyTimeout(config.DataSource)
}

// withSQLiteBusyTimeout adds a busy timeout as data source parameters, because
// both drivers apply those to every connection they open. A PRAGMA run after Open
// would reach only one pooled connection. mattn/go-sqlite3 reads _busy_timeout,
// glebarez/sqlite reads _pragma, and each ignores the other's parameter. A timeout
// already in the data source wins over SQLITE_BUSY_TIMEOUT_MS.
func withSQLiteBusyTimeout(dataSource string) (string, int) {
	timeoutMS := configuredSQLiteBusyTimeoutMS()
	_, rawQuery, _ := strings.Cut(dataSource, "?")
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		// Leave a malformed data source for the driver to reject.
		return dataSource, timeoutMS
	}

	// mattn/go-sqlite3 prefers _timeout when both spellings are present.
	mattnValue := firstNonEmpty(query.Get("_timeout"), query.Get("_busy_timeout"))
	pragmaValue := sqliteBusyTimeoutPragmaValue(query["_pragma"])
	if explicit, err := strconv.Atoi(firstNonEmpty(mattnValue, pragmaValue)); err == nil {
		timeoutMS = explicit
	}

	var additions []string
	if mattnValue == "" {
		additions = append(additions, fmt.Sprintf("_busy_timeout=%d", timeoutMS))
	}
	if pragmaValue == "" {
		additions = append(additions, fmt.Sprintf("_pragma=busy_timeout(%d)", timeoutMS))
	}
	if len(additions) == 0 {
		return dataSource, timeoutMS
	}
	separator := "?"
	if strings.Contains(dataSource, "?") {
		separator = "&"
	}
	return dataSource + separator + strings.Join(additions, "&"), timeoutMS
}

func configuredSQLiteBusyTimeoutMS() int {
	timeoutMS := getEnvInt("SQLITE_BUSY_TIMEOUT_MS", defaultSQLiteBusyTimeoutMS)
	if timeoutMS < 0 {
		return defaultSQLiteBusyTimeoutMS
	}
	return timeoutMS
}

func sqliteBusyTimeoutPragmaValue(pragmas []string) string {
	for _, pragma := range pragmas {
		if match := sqliteBusyTimeoutPragma.FindStringSubmatch(pragma); match != nil {
			return match[1]
		}
	}
	return ""
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

// applyConnectionPool configures db and returns the DB_MAX_OPEN_CONNS value in effect.
func applyConnectionPool(db ConnPoolConfigurable, driver DatabaseDriver) int {
	defaultMaxOpen := defaultMaxOpenConns
	if driver == DriverSQLite {
		defaultMaxOpen = defaultSQLiteMaxOpenConns
	}
	maxIdle := getEnvInt("DB_MAX_IDLE_CONNS", 10)
	maxOpen := getEnvInt("DB_MAX_OPEN_CONNS", defaultMaxOpen)
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
	return maxOpen
}

// ConnPoolConfigurable represents a public type.
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
