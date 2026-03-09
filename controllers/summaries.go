package controllers

import (
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/ctaylor1/briefcast/db"
	"github.com/gin-gonic/gin"
)

// SummaryListItem is the JSON shape returned for each summary in the list.
type SummaryListItem struct {
	ID           string `json:"id"`
	EpisodeTitle string `json:"episodeTitle"`
	PodcastID    string `json:"podcastId"`
	PodcastTitle string `json:"podcastTitle"`
	PodcastImage string `json:"podcastImage"`
	Duration     int    `json:"duration"`
	PubDate      string `json:"pubDate"`
	GeneratedAt  string `json:"generatedAt"`
	Model        string `json:"model"`
	Excerpt      string `json:"excerpt"`
	ReadTime     int    `json:"readTime"`
	IsPlayed     bool   `json:"isPlayed"`
	HasSummary   bool   `json:"hasSummary"`
	IsFavorited  bool   `json:"isFavorited"`
}

// GetSummaries returns a paginated list of episodes that have available AI summaries.
func GetSummaries(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	count, _ := strconv.Atoi(c.DefaultQuery("count", "20"))
	q := c.Query("q")
	sorting := c.DefaultQuery("sorting", "newest")
	podcastIds := c.QueryArray("podcastIds[]")
	favoritesOnly := c.Query("favoritesOnly") == "true"

	if page < 1 {
		page = 1
	}
	if count < 1 || count > 100 {
		count = 20
	}

	items, totalCount, err := db.GetPaginatedSummaries(page, count, q, podcastIds, sorting, favoritesOnly)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load summaries"})
		return
	}

	totalPages := int(math.Ceil(float64(totalCount) / float64(count)))
	nextPage := 0
	previousPage := 0
	if page < totalPages {
		nextPage = page + 1
	}
	if page > 1 {
		previousPage = page - 1
	}

	summaries := make([]SummaryListItem, 0, len(*items))
	for _, item := range *items {
		excerpt := extractExcerpt(item.LLMSummary, 160)
		readTime := estimateReadTime(item.LLMSummary)

		generatedAt := ""
		if item.LLMSummaryDate != nil {
			generatedAt = item.LLMSummaryDate.Format("2006-01-02T15:04:05Z")
		}

		summaries = append(summaries, SummaryListItem{
			ID:           item.ID,
			EpisodeTitle: item.Title,
			PodcastID:    item.PodcastID,
			PodcastTitle: item.Podcast.Title,
			PodcastImage: item.Podcast.Image,
			Duration:     item.Duration,
			PubDate:      item.PubDate.Format("2006-01-02T15:04:05Z"),
			GeneratedAt:  generatedAt,
			Model:        item.LLMSummaryModel,
			Excerpt:      excerpt,
			ReadTime:     readTime,
			IsPlayed:     item.IsPlayed,
			HasSummary:   true,
			IsFavorited:  item.IsSummaryFavorited,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"summaries": summaries,
		"filter": gin.H{
			"page":         page,
			"count":        count,
			"totalCount":   totalCount,
			"totalPages":   totalPages,
			"nextPage":     nextPage,
			"previousPage": previousPage,
		},
	})
}

// FavoriteSummary marks a summary as favorited.
func FavoriteSummary(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing id"})
		return
	}
	if err := db.SetSummaryFavorited(id, true); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

// UnfavoriteSummary removes the favorite mark from a summary.
func UnfavoriteSummary(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing id"})
		return
	}
	if err := db.SetSummaryFavorited(id, false); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

func extractExcerpt(text string, maxLen int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	// Strip leading markdown headings for excerpt
	lines := strings.SplitN(text, "\n", 10)
	var clean []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Strip bold/italic markers for cleaner excerpt
		trimmed = strings.ReplaceAll(trimmed, "**", "")
		trimmed = strings.ReplaceAll(trimmed, "__", "")
		clean = append(clean, trimmed)
	}
	joined := strings.Join(clean, " ")
	if len(joined) <= maxLen {
		return joined
	}
	// Find a word boundary
	cut := maxLen
	for cut > 0 && joined[cut] != ' ' {
		cut--
	}
	if cut == 0 {
		cut = maxLen
	}
	return joined[:cut] + "..."
}

func estimateReadTime(text string) int {
	words := len(strings.Fields(text))
	minutes := int(math.Ceil(float64(words) / 200.0))
	if minutes < 1 {
		minutes = 1
	}
	return minutes
}
