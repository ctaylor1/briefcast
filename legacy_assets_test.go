package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// The legacy templates and their libraries were removed; RSS feeds still link
// to /webassets/blank.png as the default artwork.
func TestOnlyTheBlankImageRemainsUnderWebassets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerRoutes(&engine.RouterGroup)

	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/webassets/blank.png", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("blank.png: expected 200, got %d", resp.Code)
	}
	resp = httptest.NewRecorder()
	engine.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/webassets/axios.min.js", nil))
	if resp.Code != http.StatusNotFound {
		t.Fatalf("legacy library still served: %d", resp.Code)
	}
}
