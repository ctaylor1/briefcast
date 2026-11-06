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
