package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/logging"
	"github.com/gin-gonic/gin"
	glebarezsqlite "github.com/glebarez/sqlite"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

// TestAuthRouterGroup handles the corresponding operation.
func TestAuthRouterGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	withAuth := authRouterGroup(engine, "secret")
	if withAuth == nil {
		t.Fatal("expected auth router group")
	}
	if len(withAuth.Handlers) == 0 {
		t.Fatal("expected auth middleware to be attached")
	}

	withoutAuth := authRouterGroup(engine, "")
	if withoutAuth == nil {
		t.Fatal("expected default router group")
	}
	if len(withoutAuth.Handlers) != 0 {
		t.Fatal("expected no middleware on default router group")
	}
}

// TestBuildRouterRegistersRoutes handles the corresponding operation.
func TestBuildRouterRegistersRoutes(t *testing.T) {
	t.Setenv("DATA", t.TempDir())
	t.Setenv("CONFIG", "config-test")
	t.Setenv("PASSWORD", "")

	engine := buildRouter()
	routes := engine.Routes()
	if len(routes) == 0 {
		t.Fatal("expected routes to be registered")
	}

	required := map[string]bool{
		"GET /app":       false,
		"GET /app/":      false,
		"POST /podcasts": false,
		"GET /ws":        false,
	}
	for _, route := range routes {
		key := route.Method + " " + route.Path
		if _, ok := required[key]; ok {
			required[key] = true
		}
	}
	for key, found := range required {
		if !found {
			t.Fatalf("expected route %s to be registered", key)
		}
	}
}

// TestBuildRouterWithPassword handles the corresponding operation.
func TestBuildRouterWithPassword(t *testing.T) {
	t.Setenv("DATA", t.TempDir())
	t.Setenv("CONFIG", "config-test")
	t.Setenv("PASSWORD", "pw")

	engine := buildRouter()
	if len(engine.Routes()) == 0 {
		t.Fatal("expected routes to be registered when password is set")
	}
}

// TestRegisterRoutesPlayerRedirect handles the corresponding operation.
func TestRegisterRoutesPlayerRedirect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerRoutes(engine, &engine.RouterGroup, t.TempDir(), "/backups")

	req := httptest.NewRequest(http.MethodGet, "/player?foo=bar", nil)
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusFound {
		t.Fatalf("expected redirect, got %d", resp.Code)
	}
	location := resp.Header().Get("Location")
	if location != "/app/#/player?foo=bar" {
		t.Fatalf("unexpected redirect target %q", location)
	}
}

// TestSetupSettingsMiddleware handles the corresponding operation.
func TestSetupSettingsMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previous := db.DB
	t.Cleanup(func() {
		db.DB = previous
	})

	t.Setenv("DATABASE_URL", "")
	t.Setenv("DB_DRIVER", "")
	t.Setenv("DATABASE_DRIVER", "")
	t.Setenv("CONFIG", t.TempDir())

	conn, err := gorm.Open(glebarezsqlite.Open(filepath.Join(t.TempDir(), "briefcast.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("init sqlite db: %v", err)
	}
	db.DB = conn
	sqlDB, err := conn.DB()
	if err != nil {
		t.Fatalf("init sql db handle: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	db.Migrate()

	engine := gin.New()
	engine.Use(setupSettings())

	settingPresent := false
	engine.GET("/ok", func(c *gin.Context) {
		_, settingPresent = c.Get("setting")
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if !settingPresent {
		t.Fatal("expected setting to be injected in context")
	}
	if resp.Header().Get("X-Clacks-Overhead") != "GNU Terry Pratchett" {
		t.Fatal("expected X-Clacks-Overhead header to be set")
	}
}

// TestServeModernApp handles the corresponding operation.
func TestServeModernApp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Chdir(t.TempDir())

	// Missing frontend build should return 503.
	respMissing := httptest.NewRecorder()
	ctxMissing, _ := gin.CreateTestContext(respMissing)
	ctxMissing.Request = httptest.NewRequest(http.MethodGet, "/app", nil)
	serveModernApp(ctxMissing)
	if respMissing.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for missing frontend build, got %d", respMissing.Code)
	}

	distDir := filepath.Join("frontend", "dist")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatalf("mkdir dist: %v", err)
	}
	indexPath := filepath.Join(distDir, "index.html")
	const body = "<html><body>briefcast test</body></html>"
	if err := os.WriteFile(indexPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	respBuilt := httptest.NewRecorder()
	ctxBuilt, _ := gin.CreateTestContext(respBuilt)
	ctxBuilt.Request = httptest.NewRequest(http.MethodGet, "/app", nil)
	serveModernApp(ctxBuilt)
	if respBuilt.Code != http.StatusOK {
		t.Fatalf("expected 200 for built frontend, got %d", respBuilt.Code)
	}
	if !strings.Contains(respBuilt.Body.String(), "briefcast test") {
		t.Fatalf("expected served index content, got %q", respBuilt.Body.String())
	}
}

// TestAssetEnv handles the corresponding operation.
func TestAssetEnv(t *testing.T) {
	t.Setenv("CONFIG", t.TempDir())
	t.Setenv("DATA", t.TempDir())
	t.Setenv("CHECK_FREQUENCY", "30")
	t.Setenv("DATABASE_URL", "")
	assetEnv()
}

// TestResolveCheckFrequency handles the corresponding operation.
func TestResolveCheckFrequency(t *testing.T) {
	logger := logging.Sugar()
	if got := resolveCheckFrequency("15", logger); got != 15 {
		t.Fatalf("expected 15, got %d", got)
	}
	if got := resolveCheckFrequency("0", logger); got != 30 {
		t.Fatalf("expected fallback 30 for zero, got %d", got)
	}
	if got := resolveCheckFrequency("bad", logger); got != 30 {
		t.Fatalf("expected fallback 30 for invalid input, got %d", got)
	}
}

// TestResolveWhisperXFrequency handles the corresponding operation.
func TestResolveWhisperXFrequency(t *testing.T) {
	logger := logging.Sugar()
	if got := resolveWhisperXFrequency(20, "40", logger); got != 40 {
		t.Fatalf("expected 40, got %d", got)
	}
	if got := resolveWhisperXFrequency(20, "  ", logger); got != 20 {
		t.Fatalf("expected fallback 20 for empty override, got %d", got)
	}
	if got := resolveWhisperXFrequency(20, "invalid", logger); got != 20 {
		t.Fatalf("expected fallback 20 for invalid override, got %d", got)
	}
}

// TestRunCronJob handles the corresponding operation.
func TestRunCronJob(t *testing.T) {
	runCronJob("success", func() error { return nil })
	runCronJob("failure", func() error { return errors.New("boom") })
}

// TestScheduleCronJobs handles the corresponding operation.
func TestScheduleCronJobs(t *testing.T) {
	logger := logging.Sugar()
	scheduler := cron.New()

	var refreshCalls int
	var missingCalls int
	var retentionCalls int
	var unlockCalls int
	var sizesCalls int
	var imagesCalls int
	var transcribeCalls int
	var backupCalls int

	jobs := cronJobSet{
		RefreshEpisodes: func() error {
			refreshCalls++
			return nil
		},
		CheckMissingFiles: func() error {
			missingCalls++
			return errors.New("missing failure")
		},
		ApplyRetentionPolicies: func() error {
			retentionCalls++
			return nil
		},
		UnlockMissedJobs: func() {
			unlockCalls++
		},
		UpdateAllFileSizes: func() {
			sizesCalls++
		},
		DownloadMissingImages: func() error {
			imagesCalls++
			return nil
		},
		TranscribePendingEpisode: func() error {
			transcribeCalls++
			return nil
		},
		CreateBackup: func() (string, error) {
			backupCalls++
			return "backup.tar", nil
		},
	}

	scheduleCronJobs(scheduler, 15, 45, jobs, logger)
	entries := scheduler.Entries()
	if len(entries) != 8 {
		t.Fatalf("expected 8 cron entries, got %d", len(entries))
	}

	for _, entry := range entries {
		entry.Job.Run()
	}

	if refreshCalls != 1 || missingCalls != 1 || retentionCalls != 1 {
		t.Fatalf("expected core recurring jobs to run once, got refresh=%d missing=%d retention=%d", refreshCalls, missingCalls, retentionCalls)
	}
	if unlockCalls != 1 || sizesCalls != 1 || imagesCalls != 1 {
		t.Fatalf("expected maintenance jobs to run once, got unlock=%d sizes=%d images=%d", unlockCalls, sizesCalls, imagesCalls)
	}
	if transcribeCalls != 1 || backupCalls != 1 {
		t.Fatalf("expected transcript+backup jobs to run once, got transcribe=%d backup=%d", transcribeCalls, backupCalls)
	}
}
