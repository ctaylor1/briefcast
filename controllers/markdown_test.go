package controllers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ctaylor1/briefcast/db"
	pm "github.com/ctaylor1/briefcast/internal/podcastmarkdown"
	"github.com/ctaylor1/briefcast/service"
)

func TestMarkdownUIExportsUseSameServerDocuments(t *testing.T) {
	setupControllersTestDB(t)
	_, item := createControllerPodcastAndItem(t)
	item.LLMSummary = "## Takeaway\n\nA qualification."
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatal(err)
	}
	router := makeRouter()
	for _, kind := range []string{"transcript", "summary"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/podcastitems/"+item.ID+"/"+kind, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("unexpected status %d", response.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		content, ok := payload["markdown"].(string)
		if !ok {
			t.Fatalf("missing Markdown for %s", kind)
		}
		meta, _, err := pm.Parse(content)
		if err != nil {
			t.Fatal(err)
		}
		if err := pm.Validate(meta); err != nil {
			t.Fatal(err)
		}
		expected, err := service.EpisodeMarkdown(&item, kind)
		if err != nil || content != expected {
			t.Fatal("UI changed document identity or provenance", err)
		}
		if !strings.HasSuffix(payload["markdownFileName"].(string), "-r1.md") {
			t.Fatal("missing revision in filename")
		}
		pack := httptest.NewRecorder()
		router.ServeHTTP(pack, httptest.NewRequest(http.MethodGet, "/podcastitems/"+item.ID+"/"+kind+"?format=markdown-package", nil))
		if pack.Code != http.StatusOK || pack.Header().Get("Content-Type") != "application/zip" || !strings.Contains(pack.Header().Get("Content-Disposition"), ".zip") {
			t.Fatal("Markdown package response failed")
		}
		if _, err := zip.NewReader(bytes.NewReader(pack.Body.Bytes()), int64(pack.Body.Len())); err != nil {
			t.Fatal("invalid ZIP response", err)
		}
	}
}
