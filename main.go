package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ctaylor1/briefcast/controllers"
	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/logging"
	"github.com/ctaylor1/briefcast/internal/telemetry"
	"github.com/ctaylor1/briefcast/service"
	"github.com/gin-contrib/location"
	"github.com/gin-gonic/gin"
	_ "github.com/joho/godotenv/autoload"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

const defaultRepoURL = "https://github.com/ctaylor1/briefcast"

var appVersion = "dev"
var appRepoURL = defaultRepoURL

type versionResponse struct {
	Version string `json:"version"`
	RepoURL string `json:"repoUrl"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(resolveRunningVersion())
		return
	}

	defer logging.Sync()
	appLogger := logging.Sugar()

	var err error
	db.DB, err = db.Init()
	if err != nil {
		appLogger.Fatalw("database initialization failed", "error", err)
		return
	}
	if err := db.Migrate(); err != nil {
		// Serving on a half-migrated schema risks writes the next migration
		// cannot repair. Restore from backup or fix the cause, then restart.
		appLogger.Fatalw("database migration failed; not starting", "error", err)
		return
	}
	telemetry.SetVersion(resolveRunningVersion())
	service.RegisterPipelineMetrics()
	r := buildRouter()
	service.SafeGo("websocket hub", controllers.DefaultHub.Run)

	service.SafeGo("startup configuration log", assetEnv)
	service.SafeGo("cron scheduler", intiCron)

	srv := newHTTPServer(r)
	appLogger.Infow("http server listening", "addr", srv.Addr)
	if err := serveUntilSignal(srv, appLogger); err != nil {
		appLogger.Fatalw("http server terminated", "error", err)
	}
	appLogger.Infow("http server stopped")
}

func buildRouter() *gin.Engine {
	r := gin.New()
	// The request logger stays outermost so it records the 500 that Recovery
	// writes; Recovery wraps every other middleware and handler.
	r.Use(logging.RequestLoggerMiddleware())
	r.Use(gin.Recovery())
	r.Use(limitRequestBody(defaultMaxRequestBodyBytes))
	r.Use(clacksOverhead())
	r.Use(location.Default())

	// Legacy HTML templates removed; modern Vue app is the only UI.
	pass := os.Getenv("PASSWORD")
	registerRoutes(authRouterGroup(r, pass))
	return r
}

func authRouterGroup(r *gin.Engine, pass string) *gin.RouterGroup {
	if pass != "" {
		return r.Group("/", gin.BasicAuth(gin.Accounts{
			"briefcast": pass,
		}))
	}
	return &r.RouterGroup
}

func registerRoutes(router *gin.RouterGroup) {
	router.Static("/webassets", "./webassets")
	router.Static("/app/assets", "./frontend/dist/assets")
	router.StaticFile("/app/favicon.ico", "./frontend/dist/favicon.ico")
	router.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/app")
	})
	router.GET("/player", func(c *gin.Context) {
		target := "/app/#/player"
		if c.Request.URL.RawQuery != "" {
			target = target + "?" + c.Request.URL.RawQuery
		}
		c.Redirect(http.StatusFound, target)
	})
	router.GET("/app", serveModernApp)
	router.GET("/app/", serveModernApp)
	router.POST("/podcasts", controllers.AddPodcast)
	router.GET("/podcasts", controllers.GetAllPodcasts)
	router.GET("/podcasts/:id", controllers.GetPodcastByID)
	router.GET("/podcasts/:id/image", controllers.GetPodcastImageByID)
	router.DELETE("/podcasts/:id", controllers.DeletePodcastByID)
	router.GET("/podcasts/:id/items", controllers.GetPodcastItemsByPodcastID)
	router.POST("/podcasts/:id/download", controllers.DownloadAllEpisodesByPodcastID)
	router.DELETE("/podcasts/:id/items", controllers.DeletePodcastEpisodesByID)
	router.DELETE("/podcasts/:id/podcast", controllers.DeleteOnlyPodcastByID)
	router.PATCH("/podcasts/:id/pause", controllers.PausePodcastByID)
	router.PATCH("/podcasts/:id/unpause", controllers.UnpausePodcastByID)
	router.PATCH("/podcasts/:id/retention", controllers.PatchPodcastRetention)
	router.PATCH("/podcasts/:id/sponsor-skip", controllers.PatchPodcastSponsorSkip)
	router.PATCH("/podcasts/:id/briefpoint", controllers.PatchPodcastBriefpoint)
	router.POST("/podcasts/:id/briefpoint/sync", controllers.SyncPodcastToBriefpoint)
	router.GET("/podcasts/:id/alternate-feeds", controllers.GetPodcastAlternateFeeds)
	router.PATCH("/podcasts/:id/alternate-feeds", controllers.PatchPodcastAlternateFeeds)
	router.GET("/podcasts/:id/rss", controllers.GetRssForPodcastByID)

	router.GET("/podcastitems", controllers.GetAllPodcastItems)
	router.GET("/podcastitems/:id", controllers.GetPodcastItemByID)
	router.GET("/podcastitems/:id/image", controllers.GetPodcastItemImageByID)
	router.GET("/podcastitems/:id/file", controllers.GetPodcastItemFileByID)
	router.PATCH("/podcastitems/:id/markUnplayed", controllers.MarkPodcastItemAsUnplayed)
	router.PATCH("/podcastitems/:id/markPlayed", controllers.MarkPodcastItemAsPlayed)
	router.PATCH("/podcastitems/:id/bookmark", controllers.BookmarkPodcastItem)
	router.PATCH("/podcastitems/:id/unbookmark", controllers.UnbookmarkPodcastItem)
	router.PATCH("/podcastitems/:id", controllers.PatchPodcastItemByID)
	router.PATCH("/podcastitems/:id/position", controllers.PatchPodcastItemPosition)
	router.POST("/podcastitems/:id/download", controllers.DownloadPodcastItem)
	router.GET("/podcastitems/:id/chapters", controllers.GetPodcastItemChapters)
	router.GET("/podcastitems/:id/transcript", controllers.GetPodcastItemTranscript)
	router.POST("/podcastitems/:id/retranscribe", controllers.RetranscribePodcastItem)
	router.GET("/podcastitems/:id/summary", controllers.GetPodcastItemSummary)
	router.GET("/podcastitems/:id/links", controllers.GetPodcastItemLinks)
	router.POST("/podcastitems/:id/cancel", controllers.CancelPodcastItemDownload)
	router.POST("/podcastitems/:id/resume", controllers.ResumePodcastItemDownload)
	router.DELETE("/podcastitems/:id/delete", controllers.DeletePodcastItem)

	router.GET("/links/top", controllers.GetTopLinks)
	router.GET("/links/targets/:id", controllers.GetLinkTarget)

	router.GET("/summaries", controllers.GetSummaries)
	router.POST("/summaries/:id/favorite", controllers.FavoriteSummary)
	router.POST("/summaries/:id/unfavorite", controllers.UnfavoriteSummary)

	router.GET("/downloads/queue", controllers.GetDownloadQueue)
	router.POST("/downloads/pause", controllers.PauseDownloads)
	router.POST("/downloads/resume", controllers.ResumeDownloads)
	router.POST("/downloads/cancel", controllers.CancelAllDownloads)

	router.GET("/tags", controllers.GetAllTags)
	router.GET("/tags/:id", controllers.GetTagByID)
	router.GET("/tags/:id/rss", controllers.GetRssForTagByID)
	router.DELETE("/tags/:id", controllers.DeleteTagByID)
	router.POST("/tags", controllers.AddTag)
	router.POST("/podcasts/:id/tags/:tagID", controllers.AddTagToPodcast)
	router.DELETE("/podcasts/:id/tags/:tagID", controllers.RemoveTagFromPodcast)

	router.GET("/search", controllers.Search)
	router.GET("/search/local", controllers.SearchLocalRecords)
	router.GET("/settings", controllers.GetSettings)
	router.GET("/settings/logs", controllers.GetAppLogs)
	router.GET("/settings/repair-work", controllers.GetRepairWork)
	router.POST("/settings/repair-work", controllers.StartRepairWork)
	router.PATCH("/settings", controllers.PatchSettings)
	router.POST("/settings/backfill-summaries", controllers.BackfillSummaries)
	router.GET("/settings/backfill-summaries", controllers.GetBackfillSummariesStatus)
	router.POST("/settings/resummarize", controllers.ResummarizeSummaries)
	router.GET("/settings/summary-models", controllers.GetSummaryModels)
	router.GET("/settings/openrouter-models", controllers.GetOpenRouterModels)
	router.POST("/settings/backfill-links", controllers.BackfillLinks)
	router.GET("/settings/backfill-links", controllers.GetBackfillLinksStatus)
	router.POST("/settings/export-all", controllers.ExportAll)
	router.GET("/settings/export-all", controllers.GetExportAllStatus)
	router.GET("/settings/monitoring", controllers.GetMonitoring)
	// Prometheus scrape endpoints (same auth as the rest of the app).
	router.GET("/metrics", gin.WrapH(telemetry.Handler()))
	router.GET("/metrics/worker", controllers.GetWorkerMetrics)
	router.GET("/settings/prompt-versions", controllers.GetPromptVersions)
	router.POST("/settings/prompt-versions/:id/restore", controllers.RestorePromptVersion)
	router.GET("/version", func(c *gin.Context) {
		c.JSON(http.StatusOK, versionResponse{
			Version: resolveRunningVersion(),
			RepoURL: resolveRepositoryURL(),
		})
	})
	router.POST("/settings", controllers.UpdateSetting)
	router.POST("/opml", controllers.UploadOpml)
	router.GET("/opml", controllers.GetOPML)
	router.GET("/rss", controllers.GetRss)
	router.GET("/ws", controllers.DefaultHub.Handler)
}

// setupSettings puts the settings row in the request context. It is no longer
// installed globally; handlers that need settings read them on demand.
func setupSettings() gin.HandlerFunc {
	return func(c *gin.Context) {
		setting := db.GetOrCreateSetting()
		c.Set("setting", setting)
		c.Writer.Header().Set("X-Clacks-Overhead", "GNU Terry Pratchett")
		c.Next()
	}
}

// clacksOverhead sets the header every response has carried, without the
// per-request settings read that setupSettings did.
func clacksOverhead() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("X-Clacks-Overhead", "GNU Terry Pratchett")
		c.Next()
	}
}

// cronJobSet holds the scheduled jobs. The three that take a logger log their
// details to the scheduler's job logger, so each run has one job_id.
type cronJobSet struct {
	RefreshEpisodes          func(*zap.SugaredLogger) error
	CheckMissingFiles        func() error
	ApplyRetentionPolicies   func(*zap.SugaredLogger) error
	UnlockMissedJobs         func()
	UpdateAllFileSizes       func()
	DownloadMissingImages    func() error
	TranscribePendingEpisode func(*zap.SugaredLogger) error
	CreateBackup             func() (string, error)
}

var defaultCronJobs = cronJobSet{
	RefreshEpisodes:          service.RefreshEpisodesJob,
	CheckMissingFiles:        service.CheckMissingFiles,
	ApplyRetentionPolicies:   service.ApplyRetentionPoliciesJob,
	UnlockMissedJobs:         service.UnlockMissedJobs,
	UpdateAllFileSizes:       service.UpdateAllFileSizes,
	DownloadMissingImages:    service.DownloadMissingImages,
	TranscribePendingEpisode: service.TranscribePendingEpisodesJob,
	CreateBackup:             service.CreateBackupIfDue,
}

func resolveRunningVersion() string {
	if version := strings.TrimSpace(os.Getenv("BRIEFCAST_VERSION")); version != "" {
		return version
	}
	if version := strings.TrimSpace(appVersion); version != "" {
		return version
	}
	return "dev"
}

func resolveRepositoryURL() string {
	if repoURL := strings.TrimSpace(os.Getenv("BRIEFCAST_REPO_URL")); repoURL != "" {
		return repoURL
	}
	if repoURL := strings.TrimSpace(appRepoURL); repoURL != "" {
		return repoURL
	}
	return defaultRepoURL
}

func resolveCheckFrequency(raw string, appLogger *zap.SugaredLogger) int {
	checkFrequency, err := strconv.Atoi(raw)
	if err != nil || checkFrequency <= 0 {
		checkFrequency = 30
		if err != nil {
			appLogger.Warnw("invalid CHECK_FREQUENCY, using fallback", "error", err, "check_frequency_minutes", checkFrequency)
		}
	}
	return checkFrequency
}

func resolveWhisperXFrequency(checkFrequency int, raw string, appLogger *zap.SugaredLogger) int {
	whisperxFrequency := checkFrequency
	if strings.TrimSpace(raw) != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(raw))
		if err == nil && parsed > 0 {
			whisperxFrequency = parsed
		} else {
			appLogger.Warnw("invalid WHISPERX_CHECK_FREQUENCY, using fallback", "value", raw, "fallback_minutes", whisperxFrequency)
		}
	}
	return whisperxFrequency
}

// runCronJob logs the run through service.RunJob, once, and records it for
// the scheduled-job metrics.
func runCronJob(name string, job func(*zap.SugaredLogger) error) {
	start := time.Now()
	err := service.RunJob(name, job)
	telemetry.ObserveScheduledJob(name, err, time.Since(start))
}

func scheduleCronJobs(scheduler *cron.Cron, checkFrequency, whisperxFrequency int, jobs cronJobSet, appLogger *zap.SugaredLogger) {
	add := func(spec, name string, job func(*zap.SugaredLogger) error) {
		if _, err := scheduler.AddFunc(spec, func() { runCronJob(name, job) }); err != nil {
			appLogger.Errorw("failed to schedule cron job", "job_name", name, "spec", spec, "error", err)
		}
	}
	// plain adapts a job that does no logging of its own.
	plain := func(fn func() error) func(*zap.SugaredLogger) error {
		return func(*zap.SugaredLogger) error { return fn() }
	}

	minutes := fmt.Sprintf("@every %dm", checkFrequency)
	add(minutes, "RefreshEpisodes", jobs.RefreshEpisodes)
	add(minutes, "CheckMissingFiles", plain(jobs.CheckMissingFiles))
	add("@every 24h", "RetentionCleanup", jobs.ApplyRetentionPolicies)
	add(fmt.Sprintf("@every %dm", checkFrequency*2), "UnlockMissedJobs", plain(func() error {
		jobs.UnlockMissedJobs()
		return nil
	}))
	add(fmt.Sprintf("@every %dm", checkFrequency*3), "UpdateAllFileSizes", plain(func() error {
		jobs.UpdateAllFileSizes()
		return nil
	}))
	add(minutes, "DownloadMissingImages", plain(jobs.DownloadMissingImages))
	add(fmt.Sprintf("@every %dm", whisperxFrequency), "TranscribePendingEpisodes", jobs.TranscribePendingEpisode)
	// Hourly, but it backs up only when the newest archive is older than
	// service.BackupInterval, so restarts no longer postpone backups.
	add("@every 1h", "CreateBackup", plain(func() error {
		_, err := jobs.CreateBackup()
		return err
	}))
}

// runStartupJobs clears stale job locks before starting the stats warmup.
// Started first, the warmup's long read held the database past the busy
// timeout and made the lock reset fail with "database is locked".
func runStartupJobs(unlockAllJobs func(), startStatsWarmup func()) {
	unlockAllJobs()
	startStatsWarmup()
}

func intiCron() {
	appLogger := logging.Sugar()
	checkFrequency := resolveCheckFrequency(os.Getenv("CHECK_FREQUENCY"), appLogger)
	runStartupJobs(service.UnlockAllJobs, service.StartPipelineStatsWarmup)

	scheduler := cron.New(cron.WithChain(cron.Recover(cron.DefaultLogger)))
	whisperxFrequency := resolveWhisperXFrequency(checkFrequency, os.Getenv("WHISPERX_CHECK_FREQUENCY"), appLogger)
	scheduleCronJobs(scheduler, checkFrequency, whisperxFrequency, defaultCronJobs, appLogger)

	scheduler.Start()
	select {}
}

func assetEnv() {
	appLogger := logging.Sugar()
	appLogger.Infow("runtime configuration", "config_dir", os.Getenv("CONFIG"), "assets_dir", os.Getenv("DATA"), "check_frequency_mins", os.Getenv("CHECK_FREQUENCY"), "database_driver", db.CurrentDriver())
	warnIfAuthDisabled(appLogger, os.Getenv("PASSWORD"))
	if os.Getenv("DATABASE_URL") == "" {
		appLogger.Infow("database URL not set, using sqlite default")
	} else {
		appLogger.Infow("database URL configured")
	}
}

// warnIfAuthDisabled uses the same condition as authRouterGroup, so the warning
// appears exactly when every route is unauthenticated.
func warnIfAuthDisabled(logger *zap.SugaredLogger, password string) {
	if password == "" {
		logger.Warnw("PASSWORD is not set: authentication is disabled, so every route is open to anyone who can reach this port")
	}
}

func serveModernApp(c *gin.Context) {
	indexPath := path.Join("frontend", "dist", "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		c.String(http.StatusServiceUnavailable, "Frontend app is not built. Run `npm --prefix frontend install && npm --prefix frontend run build`.")
		return
	}
	c.File(indexPath)
}

const (
	// No WriteTimeout or ReadTimeout: media and exports are streamed and
	// OPML uploads can be slow. Header and idle timeouts bound idle clients.
	httpReadHeaderTimeout = 10 * time.Second
	httpIdleTimeout       = 120 * time.Second
	httpShutdownTimeout   = 10 * time.Second

	// defaultMaxRequestBodyBytes caps request bodies on every route that does
	// not set its own limit. JSON settings and edits are far smaller.
	defaultMaxRequestBodyBytes int64 = 1 << 20
)

// routesWithOwnBodyLimit read large bodies under their own, larger cap.
var routesWithOwnBodyLimit = map[string]bool{
	"/opml": true,
}

// resolveListenAddr matches gin.Engine.Run: PORT when set, otherwise :8080.
func resolveListenAddr() string {
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		return ":" + port
	}
	return ":8080"
}

func newHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              resolveListenAddr(),
		Handler:           handler,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		IdleTimeout:       httpIdleTimeout,
	}
}

// serveUntilSignal runs srv until it fails or SIGINT/SIGTERM arrives, then
// shuts down, waiting at most httpShutdownTimeout for in-flight requests.
func serveUntilSignal(srv *http.Server, logger *zap.SugaredLogger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	listener, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return err
	}
	return serveUntilDone(ctx, srv, listener, logger)
}

func serveUntilDone(ctx context.Context, srv *http.Server, listener net.Listener, logger *zap.SugaredLogger) error {
	serveErr := make(chan error, 1)
	// Not SafeGo: this is the server itself. A recovered panic here would leave
	// main waiting forever instead of exiting.
	go func() { serveErr <- srv.Serve(listener) }()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	logger.Infow("shutting down http server", "timeout_seconds", int(httpShutdownTimeout.Seconds()))
	shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		// Long streams (media, exports) can outlast the deadline; cut them off.
		logger.Warnw("http server did not finish in-flight requests before the deadline", "error", err)
		if closeErr := srv.Close(); closeErr != nil {
			logger.Warnw("closing http server failed", "error", closeErr)
		}
	}
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// limitRequestBody caps request bodies at max bytes. Requests that declare a
// larger Content-Length get 413 immediately; others fail when the handler
// reads past the cap.
func limitRequestBody(max int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil || c.Request.Body == http.NoBody || routesWithOwnBodyLimit[c.FullPath()] {
			c.Next()
			return
		}
		if c.Request.ContentLength > max {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max)
		c.Next()
	}
}
