package db

import (
	"path/filepath"
	"testing"
	"time"
)

type fakeConnPool struct {
	maxIdle     int
	maxOpen     int
	maxLifetime time.Duration
}

func (f *fakeConnPool) SetMaxIdleConns(n int) {
	f.maxIdle = n
}

func (f *fakeConnPool) SetMaxOpenConns(n int) {
	f.maxOpen = n
}

func (f *fakeConnPool) SetConnMaxLifetime(d time.Duration) {
	f.maxLifetime = d
}

func TestResolveDatabaseConfigDefaultsToSQLite(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DB_DRIVER", "")
	t.Setenv("DATABASE_DRIVER", "")
	t.Setenv("CONFIG", configDir)

	cfg, err := ResolveDatabaseConfig()
	if err != nil {
		t.Fatalf("ResolveDatabaseConfig failed: %v", err)
	}
	if cfg.Driver != DriverSQLite {
		t.Fatalf("expected sqlite driver, got %q", cfg.Driver)
	}
	expectedPath := filepath.Join(configDir, "briefcast.db")
	if cfg.DataSource != expectedPath {
		t.Fatalf("expected sqlite path %q, got %q", expectedPath, cfg.DataSource)
	}
}

func TestResolveDatabaseConfigRejectsMissingURLForPostgres(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DB_DRIVER", "postgres")
	t.Setenv("DATABASE_DRIVER", "")

	if _, err := ResolveDatabaseConfig(); err == nil {
		t.Fatalf("expected error when postgres driver is set without DATABASE_URL")
	}
}

func TestResolveDatabaseConfigPostgres(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:secret@localhost:5432/briefcast?sslmode=disable")
	t.Setenv("DB_DRIVER", "")
	t.Setenv("DATABASE_DRIVER", "")

	cfg, err := ResolveDatabaseConfig()
	if err != nil {
		t.Fatalf("ResolveDatabaseConfig failed: %v", err)
	}
	if cfg.Driver != DriverPostgres {
		t.Fatalf("expected postgres driver, got %q", cfg.Driver)
	}
	if cfg.DataSource == cfg.DisplaySource {
		t.Fatalf("expected display source to be sanitized")
	}
	if cfg.DisplaySource != "postgres://user:xxxxx@localhost:5432/briefcast?sslmode=disable" {
		t.Fatalf("unexpected display source %q", cfg.DisplaySource)
	}
}

func TestNormalizeDriverAndInferDriver(t *testing.T) {
	if driver, err := normalizeDriver("sqlite3"); err != nil || driver != DriverSQLite {
		t.Fatalf("expected sqlite driver, got %q, %v", driver, err)
	}
	if driver, err := normalizeDriver("postgresql"); err != nil || driver != DriverPostgres {
		t.Fatalf("expected postgres driver, got %q, %v", driver, err)
	}
	if _, err := normalizeDriver("mysql"); err == nil {
		t.Fatalf("expected unsupported driver error")
	}

	if driver, ok := inferDriverFromURL("sqlite:///tmp/db.sqlite"); !ok || driver != DriverSQLite {
		t.Fatalf("expected sqlite inference, got %q, %v", driver, ok)
	}
	if driver, ok := inferDriverFromURL("postgres://a:b@localhost/db"); !ok || driver != DriverPostgres {
		t.Fatalf("expected postgres inference, got %q, %v", driver, ok)
	}
	if _, ok := inferDriverFromURL("unknown://x"); ok {
		t.Fatalf("expected unknown scheme to fail inference")
	}
}

func TestNormalizeSQLiteDSN(t *testing.T) {
	if dsn, err := normalizeSQLiteDSN("sqlite:///tmp/briefcast.db"); err != nil || dsn != "/tmp/briefcast.db" {
		t.Fatalf("expected /tmp/briefcast.db, got %q, %v", dsn, err)
	}
	if dsn, err := normalizeSQLiteDSN("file:briefcast.db?cache=shared"); err != nil || dsn != "file:briefcast.db?cache=shared" {
		t.Fatalf("expected file dsn passthrough, got %q, %v", dsn, err)
	}
	if dsn, err := normalizeSQLiteDSN("relative.db"); err != nil || dsn != "relative.db" {
		t.Fatalf("expected relative path passthrough, got %q, %v", dsn, err)
	}
}

func TestSanitizeDataSource(t *testing.T) {
	source := "postgres://user:secret@localhost:5432/briefcast"
	got := sanitizeDataSource(source)
	if got != "postgres://user:xxxxx@localhost:5432/briefcast" {
		t.Fatalf("unexpected sanitized source %q", got)
	}

	plain := "sqlite:///tmp/briefcast.db"
	if sanitizeDataSource(plain) != plain {
		t.Fatalf("expected sqlite source unchanged")
	}
}

func TestApplyConnectionPool(t *testing.T) {
	t.Setenv("DB_MAX_IDLE_CONNS", "11")
	t.Setenv("DB_MAX_OPEN_CONNS", "22")
	t.Setenv("DB_CONN_MAX_LIFETIME_MINUTES", "3")

	mock := &fakeConnPool{}
	applyConnectionPool(mock)

	if mock.maxIdle != 11 {
		t.Fatalf("expected max idle 11, got %d", mock.maxIdle)
	}
	if mock.maxOpen != 22 {
		t.Fatalf("expected max open 22, got %d", mock.maxOpen)
	}
	if mock.maxLifetime != 3*time.Minute {
		t.Fatalf("expected max lifetime 3m, got %s", mock.maxLifetime)
	}
}

func TestHelpers(t *testing.T) {
	t.Setenv("DB_TEST_INT", "9")
	if got := getEnvInt("DB_TEST_INT", 2); got != 9 {
		t.Fatalf("expected env int 9, got %d", got)
	}
	t.Setenv("DB_TEST_INT", "bad")
	if got := getEnvInt("DB_TEST_INT", 2); got != 2 {
		t.Fatalf("expected fallback env int 2, got %d", got)
	}

	if got := firstNonEmpty("", "a", "b"); got != "a" {
		t.Fatalf("expected first non-empty value 'a', got %q", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}
