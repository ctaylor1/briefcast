package controllers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/service"
	"github.com/gin-gonic/gin"
)

func GetPodcastItemChapters(c *gin.Context) {
	var searchByIdQuery SearchByIdQuery
	if c.ShouldBindUri(&searchByIdQuery) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	var item db.PodcastItem
	if err := db.GetPodcastItemById(searchByIdQuery.Id, &item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Episode not found"})
		return
	}

	response := service.BuildChapterResponse(item)
	c.JSON(http.StatusOK, response)
}

func GetPodcastItemTranscript(c *gin.Context) {
	var searchByIdQuery SearchByIdQuery
	if c.ShouldBindUri(&searchByIdQuery) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	var item db.PodcastItem
	if err := db.GetPodcastItemById(searchByIdQuery.Id, &item); err != nil {
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
