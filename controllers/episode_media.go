package controllers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/service"
	"github.com/gin-gonic/gin"
)

// GetPodcastItemChapters handles the corresponding operation.
func GetPodcastItemChapters(c *gin.Context) {
	var searchByIDQuery SearchByIDQuery
	if c.ShouldBindUri(&searchByIDQuery) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	var item db.PodcastItem
	if err := db.GetPodcastItemByID(searchByIDQuery.ID, &item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Episode not found"})
		return
	}

	service.RefreshChaptersFromID3(&item)
	response := service.BuildChapterResponse(item)
	c.JSON(http.StatusOK, response)
}

// GetPodcastItemTranscript handles the corresponding operation.
func GetPodcastItemTranscript(c *gin.Context) {
	var searchByIDQuery SearchByIDQuery
	if c.ShouldBindUri(&searchByIDQuery) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	var item db.PodcastItem
	if err := db.GetPodcastItemByID(searchByIDQuery.ID, &item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Episode not found"})
		return
	}

	status := strings.TrimSpace(item.TranscriptStatus)
	if status == "" {
		status = "missing"
	}

	payload := gin.H{
		"status": status,
	}

	if strings.TrimSpace(item.TranscriptJSON) == "" {
		c.JSON(http.StatusOK, payload)
		return
	}

	var decoded interface{}
	if err := json.Unmarshal([]byte(item.TranscriptJSON), &decoded); err == nil {
		payload["transcript"] = decoded
	} else {
		payload["transcript"] = item.TranscriptJSON
	}
	c.JSON(http.StatusOK, payload)
}

// GetPodcastItemSummary handles the corresponding operation.
func GetPodcastItemSummary(c *gin.Context) {
	var searchByIDQuery SearchByIDQuery
	if c.ShouldBindUri(&searchByIDQuery) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	var item db.PodcastItem
	if err := db.GetPodcastItemByID(searchByIDQuery.ID, &item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Episode not found"})
		return
	}

	status := strings.TrimSpace(item.LLMSummaryStatus)
	if status == "" {
		status = "not_attempted"
	}

	payload := gin.H{
		"status": status,
	}

	if strings.TrimSpace(item.LLMSummary) != "" {
		payload["summary"] = item.LLMSummary
	}
	if item.LLMSummaryDate != nil {
		payload["generatedAt"] = item.LLMSummaryDate
	}
	if strings.TrimSpace(item.LLMSummaryModel) != "" {
		payload["model"] = item.LLMSummaryModel
	}

	c.JSON(http.StatusOK, payload)
}
