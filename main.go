package main

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/akhilrex/briefcast/controllers"
	"github.com/akhilrex/briefcast/db"
	"github.com/akhilrex/briefcast/internal/logging"
	"github.com/akhilrex/briefcast/service"
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
		appLogger.Errorw("database initialization failed", "error", err)
	} else {
		db.Migrate()
	}
	r := gin.New()

	r.Use(logging.RequestLoggerMiddleware())
	r.Use(setupSettings())
	r.Use(gin.Recovery())
	r.Use(location.Default())

	funcMap := template.FuncMap{
		"intRange": func(start, end int) []int {
			n := end - start + 1
			result := make([]int, n)
			for i := 0; i < n; i++ {
				result[i] = start + i
			}
			return result
		},
		"removeStartingSlash": func(raw string) string {
			if string(raw[0]) == "/" {
				return raw
			}
			return "/" + raw
		},
		"isDateNull": func(raw time.Time) bool {
			return raw == (time.Time{})
		},
		"formatDate": func(raw time.Time) string {
			if raw == (time.Time{}) {
				return ""
			}

			return raw.Format("Jan 2 2006")
		},
		"naturalDate": func(raw time.Time) string {
			return service.NatualTime(time.Now(), raw)
			//return raw.Format("Jan 2 2006")
		},
		"latestEpisodeDate": func(podcastItems []db.PodcastItem) string {
			var latest time.Time
			for _, item := range podcastItems {
				if item.PubDate.After(latest) {
					latest = item.PubDate
				}
			}
			return latest.Format("Jan 2 2006")
		},
		"downloadedEpisodes": func(podcastItems []db.PodcastItem) int {
			count := 0
			for _, item := range podcastItems {
				if item.DownloadStatus == db.Downloaded {
					count++
				}
			}
			return count
		},
		"downloadingEpisodes": func(podcastItems []db.PodcastItem) int {
			count := 0
			for _, item := range podcastItems {
				if item.DownloadStatus == db.NotDownloaded {
					count++
				}
			}
			return count
		},
		"formatFileSize": func(inputSize int64) string {
			size := float64(inputSize)
			const divisor float64 = 1024
			if size < divisor {
				return fmt.Sprintf("%.0f bytes", size)
			}
			size = size / divisor
			if size < divisor {
				return fmt.Sprintf("%.2f KB", size)
			}
			size = size / divisor
			if size < divisor {
				return fmt.Sprintf("%.2f MB", size)
			}
			size = size / divisor
			if size < divisor {
				return fmt.Sprintf("%.2f GB", size)
			}
			size = size / divisor
			return fmt.Sprintf("%.2f TB", size)
		},
		"formatDuration": func(total int) string {
			if total <= 0 {
				return ""
			}
			mins := total / 60
			secs := total % 60
			hrs := 0
			if mins >= 60 {
				hrs = mins / 60
				mins = mins % 60
			}
			if hrs > 0 {
				return fmt.Sprintf("%02d:%02d:%02d", hrs, mins, secs)
			}
			return fmt.Sprintf("%02d:%02d", mins, secs)
		},
	}
	tmpl := template.Must(template.New("main").Funcs(funcMap).ParseGlob("client/*"))

	//r.LoadHTMLGlob("client/*")
	r.SetHTMLTemplate(tmpl)

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
	router.GET("/podcastitems/:id/delete", controllers.DeletePodcastItem)

	router.GET("/tags", controllers.GetAllTags)
	router.GET("/tags/:id", controllers.GetTagById)
	router.GET("/tags/:id/rss", controllers.GetRssForTagById)
	router.DELETE("/tags/:id", controllers.DeleteTagById)
	router.POST("/tags", controllers.AddTag)
	router.POST("/podcasts/:id/tags/:tagId", controllers.AddTagToPodcast)
	router.DELETE("/podcasts/:id/tags/:tagId", controllers.RemoveTagFromPodcast)

	router.GET("/add", controllers.AddPage)
	router.GET("/search", controllers.Search)
	router.GET("/", controllers.HomePage)
	router.GET("/podcasts/:id/view", controllers.PodcastPage)
	router.GET("/episodes", controllers.AllEpisodesPage)
	router.GET("/allTags", controllers.AllTagsPage)
	router.GET("/settings", controllers.SettingsPage)
	router.POST("/settings", controllers.UpdateSetting)
	router.GET("/backups", controllers.BackupsPage)
	router.POST("/opml", controllers.UploadOpml)
	router.GET("/opml", controllers.GetOmpl)
	router.GET("/player", controllers.PlayerPage)
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
	add(fmt.Sprintf("@every %dm", checkFrequency*2), "UnlockMissedJobs", func() error {
		service.UnlockMissedJobs()
		return nil
	})
	add(fmt.Sprintf("@every %dm", checkFrequency*3), "UpdateAllFileSizes", func() error {
		service.UpdateAllFileSizes()
		return nil
	})
	add(minutes, "DownloadMissingImages", service.DownloadMissingImages)
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
