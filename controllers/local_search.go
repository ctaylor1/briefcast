package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ctaylor1/briefcast/service"
	"github.com/gin-gonic/gin"
)

const (
	defaultLocalSearchLimit = 50
	maxLocalSearchLimit     = 200
)

func SearchLocalRecords(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		c.JSON(http.StatusOK, []service.LocalSearchResult{})
		return
	}
	// Bound limit to keep queries predictable and protect DB/search performance.
	limit := defaultLocalSearchLimit
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be an integer"})
			return
		}
		if parsed <= 0 || parsed > maxLocalSearchLimit {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 200"})
			return
		}
		limit = parsed
	}

	results, err := service.SearchLocalRecords(query, limit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, results)
}
