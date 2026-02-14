package controllers

import (
	"net/http"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/service"
	"github.com/gin-gonic/gin"
)

type DownloadQueueQuery struct {
	Limit int `form:"limit" query:"limit"`
}

func GetDownloadQueue(c *gin.Context) {
	var query DownloadQueueQuery
	_ = c.ShouldBindQuery(&query)
	if query.Limit <= 0 {
		query.Limit = 50
	}

	items, err := db.GetPodcastItemsByDownloadStatuses([]db.DownloadStatus{db.NotDownloaded, db.Downloading}, query.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load download queue."})
		return
	}

	counts := map[string]int{
		"queued":      0,
		"downloading": 0,
		"downloaded":  0,
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
	service.PauseDownloads()
	c.JSON(http.StatusOK, gin.H{})
}

func ResumeDownloads(c *gin.Context) {
	service.ResumeDownloads()
	c.JSON(http.StatusOK, gin.H{})
}

func CancelAllDownloads(c *gin.Context) {
	if err := service.CancelAllDownloads(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel downloads."})
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
