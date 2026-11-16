package controllers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/service"
	"github.com/gin-gonic/gin"
)

func monitoringRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/settings/monitoring", GetMonitoring)
	r.GET("/metrics/worker", GetWorkerMetrics)
	return r
}

func TestSendTestNotificationRoute(t *testing.T) {
	orig := sendTestNotification
	t.Cleanup(func() { sendTestNotification = orig })
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/settings/notifications/test", SendTestNotification)
	for _, tc := range []struct {
		err      error
		wantCode int
		want     string
	}{
		{nil, http.StatusAccepted, "sent"},
		{service.ErrNotificationsOff, http.StatusConflict, "BRIEFCAST_NOTIFY_URL"},
		{errors.New("the notification service answered HTTP 403"), http.StatusBadGateway, "HTTP 403"},
	} {
		sendTestNotification = func(context.Context) error { return tc.err }
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/settings/notifications/test", nil))
		if rec.Code != tc.wantCode || !strings.Contains(rec.Body.String(), tc.want) {
			t.Fatalf("%v: %d %s", tc.err, rec.Code, rec.Body)
		}
	}
}

func TestMonitoringHandlers(t *testing.T) {
	origSnap, origFetch := buildMonitoringSnapshot, fetchWorkerMetrics
	t.Cleanup(func() { buildMonitoringSnapshot, fetchWorkerMetrics = origSnap, origFetch })
	r := monitoringRouter()

	buildMonitoringSnapshot = func(context.Context) (service.MonitoringSnapshot, error) {
		return service.MonitoringSnapshot{GeneratedAt: time.Unix(0, 0).UTC(), Windows: []service.MonitoringWindow{{Label: "Last 24 hours", Hours: 24, Transcripts: 3}}}, nil
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/settings/monitoring", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"transcripts":3`) {
		t.Fatalf("monitoring: %d %s", rec.Code, rec.Body)
	}
	origCheck := checkLLMServer
	t.Cleanup(func() { checkLLMServer = origCheck })
	var checks int
	checkLLMServer = func(context.Context) (service.LLMServerHealth, bool) {
		checks++
		return service.LLMServerHealth{Reachable: true, Host: "llm.local:11434"}, true
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/settings/monitoring", nil))
	if checks != 0 || strings.Contains(rec.Body.String(), `"server"`) {
		t.Fatalf("the LLM server must only be checked on request: checks=%d %s", checks, rec.Body)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/settings/monitoring?checkLLM=1", nil))
	if checks != 1 || !strings.Contains(rec.Body.String(), `"server":{`) || !strings.Contains(rec.Body.String(), `"host":"llm.local:11434"`) {
		t.Fatalf("checkLLM=1 did not include the check: checks=%d %s", checks, rec.Body)
	}

	buildMonitoringSnapshot = func(context.Context) (service.MonitoringSnapshot, error) {
		return service.MonitoringSnapshot{}, errors.New("db down with secret detail")
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/settings/monitoring", nil))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "secret detail") {
		t.Fatalf("errors must not leak details: %d %s", rec.Code, rec.Body)
	}

	cases := []struct {
		err      error
		body     string
		wantCode int
		want     string
	}{
		{nil, "whisperx_worker_healthy 1\n", http.StatusOK, "whisperx_worker_healthy 1"},
		{service.ErrWorkerMetricsUnavailable, "", http.StatusNotFound, "not configured"},
		{errors.New("dial tcp: refused"), "", http.StatusBadGateway, "unavailable"},
	}
	for _, tc := range cases {
		fetchWorkerMetrics = func(context.Context) ([]byte, string, error) { return []byte(tc.body), "", tc.err }
		rec = httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics/worker", nil))
		if rec.Code != tc.wantCode || !strings.Contains(rec.Body.String(), tc.want) || strings.Contains(rec.Body.String(), "dial tcp") {
			t.Fatalf("worker metrics (%v): %d %q", tc.err, rec.Code, rec.Body)
		}
	}
}
