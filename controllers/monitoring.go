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
	checkLLMServer          = service.CheckLLMServer
	sendTestNotification    = service.SendTestNotification
)

// GetMonitoring returns queue state, recent throughput and worker health for the Settings panel.
// With checkLLM=1 it also checks the LLM server (cached for a minute), so only
// the Monitoring panel, not every poller, makes that outbound request.
func GetMonitoring(c *gin.Context) {
	snapshot, err := buildMonitoringSnapshot(c.Request.Context())
	if err != nil {
		controllerLogger.Warnw("monitoring snapshot failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Monitoring data unavailable"})
		return
	}
	if c.Query("checkLLM") == "1" {
		if health, checked := checkLLMServer(c.Request.Context()); checked {
			snapshot.Summaries.Server = &health
		}
	}
	c.JSON(http.StatusOK, snapshot)
}

// SendTestNotification sends one test message to the configured notification
// channel (POST /settings/notifications/test).
func SendTestNotification(c *gin.Context) {
	err := sendTestNotification(c.Request.Context())
	switch {
	case err == nil:
		c.JSON(http.StatusAccepted, gin.H{"message": "Test notification sent"})
	case errors.Is(err, service.ErrNotificationsOff):
		c.JSON(http.StatusConflict, gin.H{"error": "Notifications are off. Set BRIEFCAST_NOTIFY_URL and restart the app."})
	default:
		// The service's errors never include the destination URL.
		c.JSON(http.StatusBadGateway, gin.H{"error": "Test notification failed: " + err.Error()})
	}
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
