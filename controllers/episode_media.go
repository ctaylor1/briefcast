package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/logging"
	pm "github.com/ctaylor1/briefcast/internal/podcastmarkdown"
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
	if c.Query("format") == "markdown-package" {
		writeEpisodeMarkdownBundle(c, &item, "transcript")
		return
	}
	if status == "" {
		status = "missing"
	}

	payload := gin.H{
		"status":      status,
		"isFavorited": isPodcastItemFavorited(item),
	}

	if strings.TrimSpace(item.CanonicalTranscript) != "" {
		payload["canonicalTranscript"] = item.CanonicalTranscript
	}
	if strings.TrimSpace(item.CanonicalTranscript) != "" || strings.TrimSpace(item.TranscriptJSON) != "" {
		addEpisodeMarkdownPayload(payload, &item, "transcript")
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

// GetPodcastItemLinks handles the corresponding operation.
func GetPodcastItemLinks(c *gin.Context) {
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

	links, err := db.GetShowNoteLinksByPodcastItemID(searchByIDQuery.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve links"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"episodeId": item.ID,
		"podcastId": item.PodcastID,
		"count":     len(links),
		"links":     links,
	})
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
	if c.Query("format") == "markdown-package" {
		writeEpisodeMarkdownBundle(c, &item, "summary")
		return
	}
	if status == "" {
		status = "not_attempted"
	}

	payload := gin.H{
		"status":      status,
		"isFavorited": isPodcastItemFavorited(item),
	}

	if strings.TrimSpace(item.LLMSummary) != "" {
		payload["summary"] = item.LLMSummary
		addEpisodeMarkdownPayload(payload, &item, "summary")
	}
	if item.LLMSummaryDate != nil {
		payload["generatedAt"] = item.LLMSummaryDate
	}
	if strings.TrimSpace(item.LLMSummaryModel) != "" {
		payload["model"] = item.LLMSummaryModel
	}

	c.JSON(http.StatusOK, payload)
}

func writeEpisodeMarkdownBundle(c *gin.Context, item *db.PodcastItem, kind string) {
	data, filename, err := service.EpisodeMarkdownBundle(item, kind)
	if err != nil {
		logging.Sugar().Warnw("failed to prepare Markdown package", "podcast_item_id", item.ID, "content_type", kind, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Markdown package is unavailable. Check the server logs."})
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	c.Data(http.StatusOK, "application/zip", data)
}

func addEpisodeMarkdownPayload(payload gin.H, item *db.PodcastItem, kind string) {
	content, err := service.EpisodeMarkdown(item, kind)
	if err != nil {
		logging.Sugar().Warnw("failed to prepare UI Markdown export", "podcast_item_id", item.ID, "content_type", kind, "error", err)
		payload["markdownError"] = "Markdown export is unavailable. Check the server logs."
		return
	}
	if content == "" {
		return
	}
	meta, _, err := pm.Parse(content)
	if err != nil {
		return
	}
	payload["markdown"] = content
	payload["markdownFileName"] = fmt.Sprintf("Briefcast-%s-%s-r%v.md", strings.TrimPrefix(fmt.Sprint(meta["document_id"]), "urn:uuid:"), kind, meta["document_revision"])
}
