package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/service"
	"github.com/gin-gonic/gin"
)

type DownloadQueueQuery struct {
	Limit int `form:"limit" query:"limit"`
}

const (
	defaultDownloadQueueLimit = 50
	maxDownloadQueueLimit     = 200
)

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

	items, err := db.GetPodcastItemsByDownloadStatuses([]db.DownloadStatus{db.NotDownloaded, db.Downloading, db.Paused}, limit)
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

func PauseDownloads(c *gin.Context) {
	if err := service.PauseAllDownloads(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to pause downloads."})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

func ResumeDownloads(c *gin.Context) {
	if err := service.ResumeAllDownloads(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resume downloads."})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

func CancelAllDownloads(c *gin.Context) {
	if err := service.CancelAllDownloads(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to stop downloads."})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

func CancelPodcastItemDownload(c *gin.Context) {
	var searchByIdQuery SearchByIdQuery
	if c.ShouldBindUri(&searchByIdQuery) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	if err := service.CancelEpisodeDownload(searchByIdQuery.Id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}
