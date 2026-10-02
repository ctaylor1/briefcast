package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSettingsExposesProviderWithoutAPIKey(t *testing.T) {
	setupControllersTestDB(t)
	t.Setenv("LLM_PROVIDER", "openrouter")
	t.Setenv("LLM_API_KEY", "test-secret-not-for-response")
	response := httptest.NewRecorder()
	makeRouter().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/settings", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET /settings returned %d: %s", response.Code, response.Body.String())
	}
	var settings SettingsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &settings); err != nil {
		t.Fatal(err)
	}
	if settings.DefaultProvider != "openrouter" {
		t.Fatalf("expected OpenRouter provider, got %q", settings.DefaultProvider)
	}
	if strings.Contains(response.Body.String(), "test-secret-not-for-response") {
		t.Fatal("settings response exposed the LLM API key")
	}
}

func TestOpenRouterCatalogFailureDoesNotExposeAPIKey(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "openrouter")
	t.Setenv("LLM_API_KEY", "test-secret-not-for-response")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodGet, "/settings/openrouter-models?refresh=true", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	makeRouter().ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 on canceled catalog request, got %d: %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "test-secret-not-for-response") {
		t.Fatal("catalog error exposed the LLM API key")
	}
}
