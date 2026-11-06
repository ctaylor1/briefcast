package controllers

import (
	"errors"
	"net/http"

	"github.com/ctaylor1/briefcast/service"
	"github.com/gin-gonic/gin"
)

// Swappable for handler tests.
var (
	buildMonitoringSnapshot = service.BuildMonitoringSnapshot
	fetchWorkerMetrics      = service.FetchWorkerMetrics
)

// GetMonitoring returns queue state, recent throughput and worker health for the Settings panel.
func GetMonitoring(c *gin.Context) {
	snapshot, err := buildMonitoringSnapshot(c.Request.Context())
	if err != nil {
		controllerLogger.Warnw("monitoring snapshot failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Monitoring data unavailable"})
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

// GetWorkerMetrics relays the WhisperX worker's Prometheus metrics. The worker has
// no published port, so this is how a local Prometheus can scrape it.
func GetWorkerMetrics(c *gin.Context) {
	body, contentType, err := fetchWorkerMetrics(c.Request.Context())
	if errors.Is(err, service.ErrWorkerMetricsUnavailable) {
		c.String(http.StatusNotFound, "whisperx worker is not configured\n")
		return
	}
	if err != nil {
		controllerLogger.Warnw("worker metrics relay failed", "error", err)
		c.String(http.StatusBadGateway, "whisperx worker metrics unavailable\n")
		return
	}
	if contentType == "" {
		contentType = "text/plain; version=0.0.4; charset=utf-8"
	}
	c.Data(http.StatusOK, contentType, body)
}
