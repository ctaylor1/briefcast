package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	glebarezsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setSQLiteTestEnv(t *testing.T, busyTimeoutMS string) {
	t.Helper()
	t.Setenv("DATABASE_URL", filepath.Join(t.TempDir(), "briefcast.db"))
	t.Setenv("DB_DRIVER", "")
	t.Setenv("DATABASE_DRIVER", "")
	t.Setenv("DB_MAX_OPEN_CONNS", "")
	t.Setenv("SQLITE_BUSY_TIMEOUT_MS", busyTimeoutMS)
}

func sqlHandle(t *testing.T, conn *gorm.DB) *sql.DB {
	t.Helper()
	sqlDB, err := conn.DB()
	if err != nil {
		t.Fatalf("get sql handle: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return sqlDB
}

// openThroughInit uses the production path. Without cgo Init cannot open SQLite,
// so the test falls back to the same data source on the pure-Go driver.
func openThroughInit(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := Init()
	if err != nil {
		if strings.Contains(err.Error(), "CGO_ENABLED=0") || strings.Contains(err.Error(), "requires cgo") {
			return openWithPureGoDriver(t)
		}
		t.Fatalf("db init failed: %v", err)
	}
	return sqlHandle(t, conn)
}

// openWithPureGoDriver mirrors Init on glebarez/sqlite. It runs under every build,
// so a cgo run covers both drivers.
func openWithPureGoDriver(t *testing.T) *sql.DB {
	t.Helper()
	config, err := ResolveDatabaseConfig()
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	dataSource, _ := connectionDataSource(config)
	conn, err := gorm.Open(glebarezsqlite.Open(dataSource), &gorm.Config{})
	if err != nil {
		t.Fatalf("open pure-Go sqlite: %v", err)
	}
	sqlDB := sqlHandle(t, conn)
	applyConnectionPool(sqlDB, config.Driver)
	return sqlDB
}

var sqliteOpeners = map[string]func(t *testing.T) *sql.DB{
	"production path": openThroughInit,
	"pure-Go driver":  openWithPureGoDriver,
}

func TestSQLiteBusyTimeoutReachesEveryPooledConnection(t *testing.T) {
	for name, open := range sqliteOpeners {
		t.Run(name, func(t *testing.T) {
			setSQLiteTestEnv(t, "7321")
			sqlDB := open(t)

			// Holding the connections at once forces the pool to open distinct ones.
			const held = 4
			ctx := context.Background()
			conns := make([]*sql.Conn, 0, held)
			for range held {
				conn, err := sqlDB.Conn(ctx)
				if err != nil {
					t.Fatalf("acquire connection: %v", err)
				}
				t.Cleanup(func() { _ = conn.Close() })
				conns = append(conns, conn)
			}
			for i, conn := range conns {
				var got int
				if err := conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&got); err != nil {
					t.Fatalf("read busy_timeout on connection %d: %v", i, err)
				}
				if got != 7321 {
					t.Errorf("connection %d: expected busy_timeout 7321, got %d", i, got)
				}
			}
		})
	}
}

func TestSQLitePoolDefaultIsSmallAndJournalModeIsUntouched(t *testing.T) {
	for name, open := range sqliteOpeners {
		t.Run(name, func(t *testing.T) {
			setSQLiteTestEnv(t, "")
			sqlDB := open(t)

			if got := sqlDB.Stats().MaxOpenConnections; got != 4 {
				t.Errorf("expected default sqlite pool of 4, got %d", got)
			}
			var journalMode string
			if err := sqlDB.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
				t.Fatalf("read journal_mode: %v", err)
			}
			if !strings.EqualFold(journalMode, "delete") {
				t.Errorf("expected journal_mode to stay delete, got %q", journalMode)
			}
		})
	}
}

func TestWithSQLiteBusyTimeout(t *testing.T) {
	cases := []struct {
		name        string
		databaseURL string
		want        string
		wantMS      int
	}{
		{
			name:        "plain path",
			databaseURL: "/config/briefcast.db",
			want:        "/config/briefcast.db?_busy_timeout=7000&_pragma=busy_timeout(7000)",
			wantMS:      7000,
		},
		{
			name:        "sqlite URL",
			databaseURL: "sqlite:///config/briefcast.db",
			want:        "/config/briefcast.db?_busy_timeout=7000&_pragma=busy_timeout(7000)",
			wantMS:      7000,
		},
		{
			name:        "file URL",
			databaseURL: "file:briefcast.db",
			want:        "file:briefcast.db?_busy_timeout=7000&_pragma=busy_timeout(7000)",
			wantMS:      7000,
		},
		{
			name:        "file URL keeps existing parameters",
			databaseURL: "file:briefcast.db?cache=shared&mode=rwc",
			want:        "file:briefcast.db?cache=shared&mode=rwc&_busy_timeout=7000&_pragma=busy_timeout(7000)",
			wantMS:      7000,
		},
		{
			name:        "sqlite URL keeps existing parameters",
			databaseURL: "sqlite:///config/briefcast.db?_foreign_keys=on",
			want:        "/config/briefcast.db?_foreign_keys=on&_busy_timeout=7000&_pragma=busy_timeout(7000)",
			wantMS:      7000,
		},
		{
			name:        "unrelated pragma is kept",
			databaseURL: "briefcast.db?_pragma=foreign_keys(1)",
			want:        "briefcast.db?_pragma=foreign_keys(1)&_busy_timeout=7000&_pragma=busy_timeout(7000)",
			wantMS:      7000,
		},
		{
			name:        "operator _busy_timeout wins and reaches both drivers",
			databaseURL: "briefcast.db?_busy_timeout=1500",
			want:        "briefcast.db?_busy_timeout=1500&_pragma=busy_timeout(1500)",
			wantMS:      1500,
		},
		{
			name:        "operator _timeout wins and reaches both drivers",
			databaseURL: "briefcast.db?_timeout=1500",
			want:        "briefcast.db?_timeout=1500&_pragma=busy_timeout(1500)",
			wantMS:      1500,
		},
		{
			name:        "operator pragma wins and reaches both drivers",
			databaseURL: "briefcast.db?_pragma=BUSY_TIMEOUT(1500)",
			want:        "briefcast.db?_pragma=BUSY_TIMEOUT(1500)&_busy_timeout=1500",
			wantMS:      1500,
		},
		{
			name:        "both parameters already set",
			databaseURL: "briefcast.db?_busy_timeout=1500&_pragma=busy_timeout(1500)",
			want:        "briefcast.db?_busy_timeout=1500&_pragma=busy_timeout(1500)",
			wantMS:      1500,
		},
		{
			name:        "malformed query is left for the driver to reject",
			databaseURL: "briefcast.db?cache=%zz",
			want:        "briefcast.db?cache=%zz",
			wantMS:      7000,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SQLITE_BUSY_TIMEOUT_MS", "7000")
			dsn, err := normalizeSQLiteDSN(tc.databaseURL)
			if err != nil {
				t.Fatalf("normalize dsn: %v", err)
			}
			got, gotMS := withSQLiteBusyTimeout(dsn)
			if got != tc.want || gotMS != tc.wantMS {
				t.Fatalf("expected (%q, %d), got (%q, %d)", tc.want, tc.wantMS, got, gotMS)
			}
		})
	}
}

func TestConfiguredSQLiteBusyTimeoutMS(t *testing.T) {
	cases := map[string]int{
		"":      5000,
		"7000":  7000,
		"0":     0,
		"-1":    5000,
		"later": 5000,
	}
	for value, want := range cases {
		t.Run(value, func(t *testing.T) {
			t.Setenv("SQLITE_BUSY_TIMEOUT_MS", value)
			if got := configuredSQLiteBusyTimeoutMS(); got != want {
				t.Fatalf("expected %d, got %d", want, got)
			}
		})
	}
}

func TestConnectionDataSourceLeavesPostgresUnchanged(t *testing.T) {
	t.Setenv("SQLITE_BUSY_TIMEOUT_MS", "7000")
	const dataSource = "postgres://user:secret@db.example.com:5432/briefcast?sslmode=disable"

	got, busyTimeoutMS := connectionDataSource(DatabaseConfig{Driver: DriverPostgres, DataSource: dataSource})

	if got != dataSource || busyTimeoutMS != 0 {
		t.Fatalf("expected postgres data source unchanged, got (%q, %d)", got, busyTimeoutMS)
	}
}

func TestApplyConnectionPoolDefaultsByDriver(t *testing.T) {
	cases := []struct {
		name     string
		driver   DatabaseDriver
		envValue string
		want     int
	}{
		{name: "sqlite default", driver: DriverSQLite, envValue: "", want: 4},
		{name: "sqlite explicit value wins", driver: DriverSQLite, envValue: "9", want: 9},
		{name: "postgres default is unchanged", driver: DriverPostgres, envValue: "", want: 25},
		{name: "postgres explicit value wins", driver: DriverPostgres, envValue: "9", want: 9},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DB_MAX_OPEN_CONNS", tc.envValue)
			mock := &fakeConnPool{}
			got := applyConnectionPool(mock, tc.driver)
			if got != tc.want || mock.maxOpen != tc.want {
				t.Fatalf("expected %d, returned %d and set %d", tc.want, got, mock.maxOpen)
			}
		})
	}
}
