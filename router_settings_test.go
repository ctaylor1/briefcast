package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/ctaylor1/briefcast/db"
	"github.com/gin-gonic/gin"
	glebarezsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// Thumbnails and audio are requested many times per page. They must not read
// the settings row on every request.
func TestRouterDoesNotReadSettingsOnEveryRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := db.DB
	t.Cleanup(func() { db.DB = previous })
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DB_DRIVER", "")
	t.Setenv("DATABASE_DRIVER", "")
	t.Setenv("CONFIG", t.TempDir())
	t.Setenv("PASSWORD", "")

	conn, err := gorm.Open(glebarezsqlite.Open(filepath.Join(t.TempDir(), "briefcast.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.DB = conn
	sqlDB, err := conn.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}

	var settingsReads atomic.Int32
	if err := db.DB.Callback().Query().Before("gorm:query").Register("test:count_settings", func(tx *gorm.DB) {
		if tx.Statement.Table == "settings" {
			settingsReads.Add(1)
		}
	}); err != nil {
		t.Fatal(err)
	}

	engine := buildRouter()
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/podcastitems/does-not-exist/image", nil))
	if got := settingsReads.Load(); got != 0 {
		t.Fatalf("an image request read settings %d times", got)
	}
	if resp.Header().Get("X-Clacks-Overhead") != "GNU Terry Pratchett" {
		t.Fatal("expected the X-Clacks-Overhead header to remain")
	}
}
