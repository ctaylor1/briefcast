package controllers

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ctaylor1/briefcast/db"
	"github.com/gin-gonic/gin"
)

func TestUploadOpmlSuccessAndAllTagsErrorBranches(t *testing.T) {
	setupControllersTestDB(t)
	router := makeExpandedRouter(t)

	podcast, _ := createControllerPodcastAndItem(t)
	opml := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>Briefcast Import</title></head>
  <body>
    <outline text="Controller Podcast" type="rss" xmlUrl="` + podcast.URL + `" />
  </body>
</opml>`

	var uploadBody bytes.Buffer
	uploader := multipart.NewWriter(&uploadBody)
	fileWriter, err := uploader.CreateFormFile("file", "import.opml")
	if err != nil {
		t.Fatalf("failed to create multipart form file: %v", err)
	}
	if _, err := fileWriter.Write([]byte(opml)); err != nil {
		t.Fatalf("failed to write opml payload: %v", err)
	}
	if err := uploader.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}

	resp := performRequest(router, http.MethodPost, "/opml", &uploadBody, map[string]string{
		"Content-Type": uploader.FormDataContentType(),
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("expected OPML upload success (200), got %d", resp.Code)
	}

	resp = performRequest(router, http.MethodGet, "/tags-page?page=bad&count=10", nil, nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid pagination query to return 400, got %d", resp.Code)
	}

	sqlDB, err := db.DB.DB()
	if err != nil {
		t.Fatalf("failed to get sql db handle: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("failed to close sql db handle: %v", err)
	}

	resp = performRequest(router, http.MethodGet, "/tags-page?page=1&count=10", nil, nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected database error branch on tags page to return 400, got %d", resp.Code)
	}
}

func TestPodcastHandlerUriValidationBranches(t *testing.T) {
	setupControllersTestDB(t)

	{
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPatch, "/podcasts/missing/retention", strings.NewReader(`{"keepAll":true}`))
		c.Request.Header.Set("Content-Type", "application/json")
		PatchPodcastRetention(c)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected retention uri validation failure to return 400, got %d", rec.Code)
		}
	}

	{
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPatch, "/podcasts/missing/sponsor-skip", strings.NewReader(`{"autoSkipSponsorChapters":true}`))
		c.Request.Header.Set("Content-Type", "application/json")
		PatchPodcastSponsorSkip(c)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected sponsor-skip uri validation failure to return 400, got %d", rec.Code)
		}
	}

	{
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/podcasts/missing/download", nil)
		DownloadAllEpisodesByPodcastId(c)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected queue-all uri validation failure to return 400, got %d", rec.Code)
		}
	}
}
