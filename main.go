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
)

func main() {
	defer logging.Sync()
	appLogger := logging.Sugar()

	var err error
	db.DB, err = db.Init()
	if err != nil {
		appLogger.Fatalw("database initialization failed", "error", err)
		return
	}
	db.Migrate()
	r := gin.New()

	r.Use(logging.RequestLoggerMiddleware())
	r.Use(setupSettings())
	r.Use(gin.Recovery())
	r.Use(location.Default())

	// Legacy HTML templates removed; modern Vue app is the only UI.

	pass := os.Getenv("PASSWORD")
	var router *gin.RouterGroup
	if pass != "" {
		router = r.Group("/", gin.BasicAuth(gin.Accounts{
			"briefcast": pass,
		}))
	} else {
		router = &r.RouterGroup
	}

	dataPath := os.Getenv("DATA")
	backupPath := path.Join(os.Getenv("CONFIG"), "backups")

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
	router.GET("/podcasts/:id", controllers.GetPodcastById)
	router.GET("/podcasts/:id/image", controllers.GetPodcastImageById)
	router.DELETE("/podcasts/:id", controllers.DeletePodcastById)
	router.GET("/podcasts/:id/items", controllers.GetPodcastItemsByPodcastId)
	router.GET("/podcasts/:id/download", controllers.DownloadAllEpisodesByPodcastId)
	router.DELETE("/podcasts/:id/items", controllers.DeletePodcastEpisodesById)
	router.DELETE("/podcasts/:id/podcast", controllers.DeleteOnlyPodcastById)
	router.GET("/podcasts/:id/pause", controllers.PausePodcastById)
	router.GET("/podcasts/:id/unpause", controllers.UnpausePodcastById)
	router.PATCH("/podcasts/:id/retention", controllers.PatchPodcastRetention)
	router.PATCH("/podcasts/:id/sponsor-skip", controllers.PatchPodcastSponsorSkip)
	router.GET("/podcasts/:id/rss", controllers.GetRssForPodcastById)

	router.GET("/podcastitems", controllers.GetAllPodcastItems)
	router.GET("/podcastitems/:id", controllers.GetPodcastItemById)
	router.GET("/podcastitems/:id/image", controllers.GetPodcastItemImageById)
	router.GET("/podcastitems/:id/file", controllers.GetPodcastItemFileById)
	router.GET("/podcastitems/:id/markUnplayed", controllers.MarkPodcastItemAsUnplayed)
	router.GET("/podcastitems/:id/markPlayed", controllers.MarkPodcastItemAsPlayed)
	router.GET("/podcastitems/:id/bookmark", controllers.BookmarkPodcastItem)
	router.GET("/podcastitems/:id/unbookmark", controllers.UnbookmarkPodcastItem)
	router.PATCH("/podcastitems/:id", controllers.PatchPodcastItemById)
	router.GET("/podcastitems/:id/download", controllers.DownloadPodcastItem)
	router.GET("/podcastitems/:id/chapters", controllers.GetPodcastItemChapters)
	router.GET("/podcastitems/:id/transcript", controllers.GetPodcastItemTranscript)
	router.POST("/podcastitems/:id/cancel", controllers.CancelPodcastItemDownload)
	router.POST("/podcastitems/:id/resume", controllers.ResumePodcastItemDownload)
	router.GET("/podcastitems/:id/delete", controllers.DeletePodcastItem)

	router.GET("/downloads/queue", controllers.GetDownloadQueue)
	router.POST("/downloads/pause", controllers.PauseDownloads)
	router.POST("/downloads/resume", controllers.ResumeDownloads)
	router.POST("/downloads/cancel", controllers.CancelAllDownloads)

	router.GET("/tags", controllers.GetAllTags)
	router.GET("/tags/:id", controllers.GetTagById)
	router.GET("/tags/:id/rss", controllers.GetRssForTagById)
	router.DELETE("/tags/:id", controllers.DeleteTagById)
	router.POST("/tags", controllers.AddTag)
	router.POST("/podcasts/:id/tags/:tagId", controllers.AddTagToPodcast)
	router.DELETE("/podcasts/:id/tags/:tagId", controllers.RemoveTagFromPodcast)

	router.GET("/search", controllers.Search)
	router.GET("/search/local", controllers.SearchLocalRecords)
	router.GET("/settings", controllers.GetSettings)
	router.PATCH("/settings", controllers.PatchSettings)
	router.POST("/settings", controllers.UpdateSetting)
	router.POST("/opml", controllers.UploadOpml)
	router.GET("/opml", controllers.GetOmpl)
	router.GET("/rss", controllers.GetRss)

	r.GET("/ws", controllers.Wshandler)
	go controllers.HandleWebsocketMessages()

	go assetEnv()
	go intiCron()

	if err := r.Run(); err != nil {
		appLogger.Fatalw("http server terminated", "error", err)
	}

}
func setupSettings() gin.HandlerFunc {
	return func(c *gin.Context) {

		setting := db.GetOrCreateSetting()
		c.Set("setting", setting)
		c.Writer.Header().Set("X-Clacks-Overhead", "GNU Terry Pratchett")

		c.Next()
	}
}

func intiCron() {
	appLogger := logging.Sugar()
	checkFrequency, err := strconv.Atoi(os.Getenv("CHECK_FREQUENCY"))
	if err != nil || checkFrequency <= 0 {
		checkFrequency = 30
		if err != nil {
			appLogger.Warnw("invalid CHECK_FREQUENCY, using fallback", "error", err, "check_frequency_minutes", checkFrequency)
		}
	}
	service.UnlockMissedJobs()

	run := func(name string, fn func() error) {
		jobLogger, _ := logging.NewJobSugar(name)
		start := time.Now()
		jobLogger.Infow("job_started")

		if err := fn(); err != nil {
			jobLogger.Errorw("job_failed", "duration_ms", time.Since(start).Milliseconds(), "error", err)
			return
		}
		jobLogger.Infow("job_completed", "duration_ms", time.Since(start).Milliseconds())
	}

	scheduler := cron.New(cron.WithChain(cron.Recover(cron.DefaultLogger)))
	add := func(spec, name string, fn func() error) {
		if _, err := scheduler.AddFunc(spec, func() { run(name, fn) }); err != nil {
			appLogger.Errorw("failed to schedule cron job", "job_name", name, "spec", spec, "error", err)
		}
	}

	minutes := fmt.Sprintf("@every %dm", checkFrequency)
	add(minutes, "RefreshEpisodes", service.RefreshEpisodes)
	add(minutes, "CheckMissingFiles", service.CheckMissingFiles)
	add("@every 24h", "RetentionCleanup", service.ApplyRetentionPolicies)
	add(fmt.Sprintf("@every %dm", checkFrequency*2), "UnlockMissedJobs", func() error {
		service.UnlockMissedJobs()
		return nil
	})
	add(fmt.Sprintf("@every %dm", checkFrequency*3), "UpdateAllFileSizes", func() error {
		service.UpdateAllFileSizes()
		return nil
	})
	add(minutes, "DownloadMissingImages", service.DownloadMissingImages)
	whisperxFrequency := checkFrequency
	if raw := strings.TrimSpace(os.Getenv("WHISPERX_CHECK_FREQUENCY")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			whisperxFrequency = parsed
		} else {
			appLogger.Warnw("invalid WHISPERX_CHECK_FREQUENCY, using fallback", "value", raw, "fallback_minutes", whisperxFrequency)
		}
	}
	add(fmt.Sprintf("@every %dm", whisperxFrequency), "TranscribePendingEpisodes", service.TranscribePendingEpisodes)
	add("@every 48h", "CreateBackup", func() error {
		_, err := service.CreateBackup()
		return err
	})

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
