package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ctaylor1/briefcast/service"
	"github.com/gin-gonic/gin"
)

func SearchLocalRecords(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		c.JSON(http.StatusOK, []service.LocalSearchResult{})
		return
	}
	limit := 50
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	results, err := service.SearchLocalRecords(query, limit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, results)
}
