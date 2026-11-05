package controllers

import (
	"errors"
	"net/http"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/service"
	"github.com/gin-gonic/gin"
)

// RetranscribePodcastItem discards a finished transcript and queues the episode
// for WhisperX again (POST /podcastitems/:id/retranscribe).
func RetranscribePodcastItem(c *gin.Context) {
	var query SearchByIDQuery
	if c.ShouldBindUri(&query) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	var item db.PodcastItem
	if err := db.GetPodcastItemByID(query.ID, &item); err != nil || item.ID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Episode not found"})
		return
	}
	if err := service.RequeueTranscription(query.ID); err != nil {
		if errors.Is(err, service.ErrTranscriptNotRequeueable) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		controllerLogger.Warnw("failed to requeue transcription", "podcast_item_id", query.ID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue transcription"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"transcriptStatus": "pending_whisperx"})
}
