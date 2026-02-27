package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/service"
	"github.com/gin-gonic/gin"
)

// DownloadQueueQuery represents a public type.
type DownloadQueueQuery struct {
	Limit int `form:"limit" query:"limit"`
}

const (
	defaultDownloadQueueLimit = 50
	maxDownloadQueueLimit     = 200
)

// GetDownloadQueue handles the corresponding operation.
func GetDownloadQueue(c *gin.Context) {
	// Bound limit to avoid unbounded queue payloads under heavy libraries.
	limit := defaultDownloadQueueLimit
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be an integer"})
			return
		}
		if parsed <= 0 || parsed > maxDownloadQueueLimit {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 200"})
			return
		}
		limit = parsed
	}

	items, err := db.GetPodcastItemsByDownloadStatuses([]db.DownloadStatus{db.NotDownloaded, db.Downloading, db.Paused, db.Downloaded}, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load download queue."})
		return
	}

	counts := map[string]int{
		"queued":      0,
		"downloading": 0,
		"downloaded":  0,
		"paused":      0,
	}
	if stats, err := db.GetPodcastEpisodeStats(); err == nil {
		for _, stat := range *stats {
			switch stat.DownloadStatus {
			case db.NotDownloaded:
				counts["queued"] = stat.Count
			case db.Downloading:
				counts["downloading"] = stat.Count
			case db.Downloaded:
				counts["downloaded"] = stat.Count
			case db.Paused:
				counts["paused"] = stat.Count
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"paused": service.DownloadsPaused(),
		"counts": counts,
		"items":  items,
	})
}

// PauseDownloads handles the corresponding operation.
func PauseDownloads(c *gin.Context) {
	if err := service.PauseAllDownloads(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to pause downloads."})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

// ResumeDownloads handles the corresponding operation.
func ResumeDownloads(c *gin.Context) {
	if err := service.ResumeAllDownloads(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resume downloads."})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

// CancelAllDownloads handles the corresponding operation.
func CancelAllDownloads(c *gin.Context) {
	if err := service.CancelAllDownloads(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to stop downloads."})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

// CancelPodcastItemDownload handles the corresponding operation.
func CancelPodcastItemDownload(c *gin.Context) {
	var searchByIDQuery SearchByIDQuery
	if c.ShouldBindUri(&searchByIDQuery) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	if err := service.CancelEpisodeDownload(searchByIDQuery.ID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

// ResumePodcastItemDownload handles the corresponding operation.
func ResumePodcastItemDownload(c *gin.Context) {
	var searchByIDQuery SearchByIDQuery
	if c.ShouldBindUri(&searchByIDQuery) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	shouldStart, err := service.ResumeEpisodeDownload(searchByIDQuery.ID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if shouldStart {
		go func(podcastItemID string) {
			if runErr := service.DownloadSingleEpisode(podcastItemID); runErr != nil {
				controllerLogger.Warnw("failed to resume podcast item download", "podcast_item_id", podcastItemID, "error", runErr)
			}
		}(searchByIDQuery.ID)
	}

	c.JSON(http.StatusOK, gin.H{})
}
