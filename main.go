package main

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/ctaylor1/briefcast/controllers"
	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/logging"
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
	db.Migrate()
	r := buildRouter()
	go controllers.HandleWebsocketMessages()

	go assetEnv()
	go intiCron()

	if err := r.Run(); err != nil {
		appLogger.Fatalw("http server terminated", "error", err)
	}

}

func buildRouter() *gin.Engine {
	r := gin.New()
	r.Use(logging.RequestLoggerMiddleware())
	r.Use(setupSettings())
	r.Use(gin.Recovery())
	r.Use(location.Default())

	// Legacy HTML templates removed; modern Vue app is the only UI.
	pass := os.Getenv("PASSWORD")
	dataPath := os.Getenv("DATA")
	backupPath := path.Join(os.Getenv("CONFIG"), "backups")
	registerRoutes(r, authRouterGroup(r, pass), dataPath, backupPath)
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

func registerRoutes(r *gin.Engine, router *gin.RouterGroup, dataPath, backupPath string) {
	router.Static("/webassets", "./webassets")
	router.Static("/assets", dataPath)
	router.Static(backupPath, backupPath)
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
	router.GET("/podcasts/:id/download", controllers.DownloadAllEpisodesByPodcastID)
	router.DELETE("/podcasts/:id/items", controllers.DeletePodcastEpisodesByID)
	router.DELETE("/podcasts/:id/podcast", controllers.DeleteOnlyPodcastByID)
	router.GET("/podcasts/:id/pause", controllers.PausePodcastByID)
	router.GET("/podcasts/:id/unpause", controllers.UnpausePodcastByID)
	router.PATCH("/podcasts/:id/retention", controllers.PatchPodcastRetention)
	router.PATCH("/podcasts/:id/sponsor-skip", controllers.PatchPodcastSponsorSkip)
	router.GET("/podcasts/:id/rss", controllers.GetRssForPodcastByID)

	router.GET("/podcastitems", controllers.GetAllPodcastItems)
	router.GET("/podcastitems/:id", controllers.GetPodcastItemByID)
	router.GET("/podcastitems/:id/image", controllers.GetPodcastItemImageByID)
	router.GET("/podcastitems/:id/file", controllers.GetPodcastItemFileByID)
	router.GET("/podcastitems/:id/markUnplayed", controllers.MarkPodcastItemAsUnplayed)
	router.GET("/podcastitems/:id/markPlayed", controllers.MarkPodcastItemAsPlayed)
	router.GET("/podcastitems/:id/bookmark", controllers.BookmarkPodcastItem)
	router.GET("/podcastitems/:id/unbookmark", controllers.UnbookmarkPodcastItem)
	router.PATCH("/podcastitems/:id", controllers.PatchPodcastItemByID)
	router.GET("/podcastitems/:id/download", controllers.DownloadPodcastItem)
	router.GET("/podcastitems/:id/chapters", controllers.GetPodcastItemChapters)
	router.GET("/podcastitems/:id/transcript", controllers.GetPodcastItemTranscript)
	router.GET("/podcastitems/:id/summary", controllers.GetPodcastItemSummary)
	router.POST("/podcastitems/:id/cancel", controllers.CancelPodcastItemDownload)
	router.POST("/podcastitems/:id/resume", controllers.ResumePodcastItemDownload)
	router.GET("/podcastitems/:id/delete", controllers.DeletePodcastItem)

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
	router.PATCH("/settings", controllers.PatchSettings)
	router.POST("/settings/backfill-summaries", controllers.BackfillSummaries)
	router.GET("/settings/backfill-summaries", controllers.GetBackfillSummariesStatus)
	router.POST("/settings/resummarize", controllers.ResummarizeSummaries)
	router.GET("/settings/summary-models", controllers.GetSummaryModels)
	router.POST("/settings/export-all", controllers.ExportAll)
	router.GET("/settings/export-all", controllers.GetExportAllStatus)
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
	r.GET("/ws", controllers.Wshandler)
}
func setupSettings() gin.HandlerFunc {
	return func(c *gin.Context) {

		setting := db.GetOrCreateSetting()
		c.Set("setting", setting)
		c.Writer.Header().Set("X-Clacks-Overhead", "GNU Terry Pratchett")

		c.Next()
	}
}

type cronJobSet struct {
	RefreshEpisodes          func() error
	CheckMissingFiles        func() error
	ApplyRetentionPolicies   func() error
	UnlockMissedJobs         func()
	UpdateAllFileSizes       func()
	DownloadMissingImages    func() error
	TranscribePendingEpisode func() error
	CreateBackup             func() (string, error)
}

var defaultCronJobs = cronJobSet{
	RefreshEpisodes:          service.RefreshEpisodes,
	CheckMissingFiles:        service.CheckMissingFiles,
	ApplyRetentionPolicies:   service.ApplyRetentionPolicies,
	UnlockMissedJobs:         service.UnlockMissedJobs,
	UpdateAllFileSizes:       service.UpdateAllFileSizes,
	DownloadMissingImages:    service.DownloadMissingImages,
	TranscribePendingEpisode: service.TranscribePendingEpisodes,
	CreateBackup:             service.CreateBackup,
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

func runCronJob(name string, fn func() error) {
	jobLogger, _ := logging.NewJobSugar(name)
	start := time.Now()
	jobLogger.Infow("job_started")

	if err := fn(); err != nil {
		jobLogger.Errorw("job_failed", "duration_ms", time.Since(start).Milliseconds(), "error", err)
		return
	}
	jobLogger.Infow("job_completed", "duration_ms", time.Since(start).Milliseconds())
}

func scheduleCronJobs(scheduler *cron.Cron, checkFrequency, whisperxFrequency int, jobs cronJobSet, appLogger *zap.SugaredLogger) {
	add := func(spec, name string, fn func() error) {
		if _, err := scheduler.AddFunc(spec, func() { runCronJob(name, fn) }); err != nil {
			appLogger.Errorw("failed to schedule cron job", "job_name", name, "spec", spec, "error", err)
		}
	}

	minutes := fmt.Sprintf("@every %dm", checkFrequency)
	add(minutes, "RefreshEpisodes", jobs.RefreshEpisodes)
	add(minutes, "CheckMissingFiles", jobs.CheckMissingFiles)
	add("@every 24h", "RetentionCleanup", jobs.ApplyRetentionPolicies)
	add(fmt.Sprintf("@every %dm", checkFrequency*2), "UnlockMissedJobs", func() error {
		jobs.UnlockMissedJobs()
		return nil
	})
	add(fmt.Sprintf("@every %dm", checkFrequency*3), "UpdateAllFileSizes", func() error {
		jobs.UpdateAllFileSizes()
		return nil
	})
	add(minutes, "DownloadMissingImages", jobs.DownloadMissingImages)
	add(fmt.Sprintf("@every %dm", whisperxFrequency), "TranscribePendingEpisodes", jobs.TranscribePendingEpisode)
	add("@every 48h", "CreateBackup", func() error {
		_, err := jobs.CreateBackup()
		return err
	})
}

func intiCron() {
	appLogger := logging.Sugar()
	checkFrequency := resolveCheckFrequency(os.Getenv("CHECK_FREQUENCY"), appLogger)
	service.UnlockMissedJobs()

	scheduler := cron.New(cron.WithChain(cron.Recover(cron.DefaultLogger)))
	whisperxFrequency := resolveWhisperXFrequency(checkFrequency, os.Getenv("WHISPERX_CHECK_FREQUENCY"), appLogger)
	scheduleCronJobs(scheduler, checkFrequency, whisperxFrequency, defaultCronJobs, appLogger)

	scheduler.Start()
	select {}
}

func assetEnv() {
	appLogger := logging.Sugar()
	appLogger.Infow("runtime configuration", "config_dir", os.Getenv("CONFIG"), "assets_dir", os.Getenv("DATA"), "check_frequency_mins", os.Getenv("CHECK_FREQUENCY"), "database_driver", db.CurrentDriver())
	if os.Getenv("DATABASE_URL") == "" {
		appLogger.Infow("database URL not set, using sqlite default")
	} else {
		appLogger.Infow("database URL configured")
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
