package controllers

import (
	"errors"
	"math"
	"net/http"

	"github.com/ctaylor1/briefcast/db"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// maxPlaybackPositionSeconds bounds a saved position (one week).
const maxPlaybackPositionSeconds = 7 * 24 * 60 * 60

// PlaybackPositionPatch is the body of PATCH /podcastitems/:id/position.
type PlaybackPositionPatch struct {
	PositionSeconds *float64 `json:"positionSeconds"`
}

// PatchPodcastItemPosition saves where the web player stopped, so playback can resume.
// It writes only the position column. Saves happen every few seconds while playing, so
// success is not logged.
func PatchPodcastItemPosition(c *gin.Context) {
	var uri SearchByIDQuery
	if c.ShouldBindUri(&uri) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	var patch PlaybackPositionPatch
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	if patch.PositionSeconds == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "positionSeconds is required"})
		return
	}
	value := *patch.PositionSeconds
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > maxPlaybackPositionSeconds {
		c.JSON(http.StatusBadRequest, gin.H{"error": "positionSeconds must be between 0 and 604800"})
		return
	}
	seconds := int(math.Floor(value))
	if err := db.UpdatePodcastItemColumns(uri.ID, map[string]interface{}{"playback_position_seconds": seconds}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Episode not found"})
			return
		}
		controllerLogger.Warnw("failed to save playback position", "podcast_item_id", uri.ID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not save the playback position"})
		return
	}
	c.Status(http.StatusNoContent)
}
