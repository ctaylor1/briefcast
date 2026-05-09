package controllers

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/model"
	"github.com/ctaylor1/briefcast/service"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type mockSearchService struct {
	result []*model.CommonSearchResultModel
}

// Query handles the corresponding operation.
func (m mockSearchService) Query(_ string) []*model.CommonSearchResultModel {
	return m.result
}

func makeExpandedRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		setting := db.GetOrCreateSetting()
		if setting.BaseURL == "" {
			setting.BaseURL = "http://localhost"
			_ = db.UpdateSettings(setting)
		}
		c.Set("setting", setting)
		c.Next()
	})
	router.SetHTMLTemplate(template.Must(template.New("pages").Parse(`
{{define "addPodcast.html"}}add{{end}}
{{define "index.html"}}home{{end}}
{{define "episodes.html"}}episodes{{end}}
{{define "player.html"}}player{{end}}
{{define "settings.html"}}settings{{end}}
{{define "backups.html"}}backups{{end}}
{{define "episodes_new.html"}}episodes-new{{end}}
{{define "tags.html"}}tags{{end}}
`)))

	router.GET("/add", AddPage)
	router.GET("/home", HomePage)
	router.GET("/podcast/:id", PodcastPage)
	router.GET("/player", PlayerPage)
	router.GET("/settings-page", SettingsPage)
	router.GET("/backups", BackupsPage)
	router.GET("/episodes", AllEpisodesPage)
	router.GET("/tags-page", AllTagsPage)
	router.GET("/search", Search)
	router.GET("/opml", GetOPML)
	router.POST("/opml", UploadOpml)
	router.POST("/add-new", AddNewPodcast)

	router.POST("/podcasts", AddPodcast)
	router.GET("/podcasts", GetAllPodcasts)
	router.GET("/podcasts/:id", GetPodcastByID)
	router.GET("/podcasts/:id/image", GetPodcastImageByID)
	router.GET("/podcasts/:id/items", GetPodcastItemsByPodcastID)
	router.GET("/podcasts/:id/download", DownloadAllEpisodesByPodcastID)
	router.GET("/podcasts/:id/pause", PausePodcastByID)
	router.GET("/podcasts/:id/unpause", UnpausePodcastByID)
	router.PATCH("/podcasts/:id/retention", PatchPodcastRetention)
	router.PATCH("/podcasts/:id/sponsor-skip", PatchPodcastSponsorSkip)
	router.DELETE("/podcasts/:id/items", DeletePodcastEpisodesByID)
	router.DELETE("/podcasts/:id/items-only", DeletePodcasDeleteOnlyPodcasttEpisodesByID)
	router.DELETE("/podcasts/:id/podcast", DeleteOnlyPodcastByID)
	router.DELETE("/podcasts/:id", DeletePodcastByID)
	router.GET("/podcasts/:id/rss", GetRssForPodcastByID)

	router.GET("/podcastitems", GetAllPodcastItems)
	router.GET("/podcastitems/:id", GetPodcastItemByID)
	router.GET("/podcastitems/:id/image", GetPodcastItemImageByID)
	router.GET("/podcastitems/:id/file", GetPodcastItemFileByID)
	router.GET("/podcastitems/:id/markUnplayed", MarkPodcastItemAsUnplayed)
	router.GET("/podcastitems/:id/markPlayed", MarkPodcastItemAsPlayed)
	router.GET("/podcastitems/:id/bookmark", BookmarkPodcastItem)
	router.GET("/podcastitems/:id/unbookmark", UnbookmarkPodcastItem)
	router.PATCH("/podcastitems/:id", PatchPodcastItemByID)
	router.GET("/podcastitems/:id/download", DownloadPodcastItem)
	router.GET("/podcastitems/:id/delete", DeletePodcastItem)
	router.POST("/podcastitems/:id/resume", ResumePodcastItemDownload)

	router.GET("/tags", GetAllTags)
	router.GET("/tags/:id", GetTagByID)
	router.GET("/tags/:id/rss", GetRssForTagByID)
	router.POST("/tags", AddTag)
	router.POST("/podcasts/:id/tags/:tagID", AddTagToPodcast)
	router.DELETE("/podcasts/:id/tags/:tagID", RemoveTagFromPodcast)
	router.DELETE("/tags/:id", DeleteTagByID)

	router.POST("/settings", UpdateSetting)
	router.GET("/rss", GetRss)
	router.GET("/ws", DefaultHub.Handler)

	return router
}

func performRequest(router *gin.Engine, method, target string, body io.Reader, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, body)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

// TestPodcastAndPagesHandlersCoverage handles the corresponding operation.
func TestPodcastAndPagesHandlersCoverage(t *testing.T) {
	setupControllersTestDB(t)
	service.ResumeDownloads()
	t.Cleanup(service.ResumeDownloads)
	router := makeExpandedRouter(t)

	podcast, item := createControllerPodcastAndItem(t)
	podcast.Image = "https://example.com/podcast-cover.jpg"
	if err := db.UpdatePodcast(&podcast); err != nil {
		t.Fatalf("update podcast failed: %v", err)
	}
	var refreshedItem db.PodcastItem
	if err := db.GetPodcastItemByID(item.ID, &refreshedItem); err != nil {
		t.Fatalf("reload podcast item failed: %v", err)
	}
	item = refreshedItem

	existingURL := podcast.URL
	tag, err := service.AddTag("controller-expanded", "desc")
	if err != nil {
		t.Fatalf("add tag failed: %v", err)
	}
	if err := db.AddTagToPodcast(podcast.ID, tag.ID); err != nil {
		t.Fatalf("add tag to podcast failed: %v", err)
	}

	episodeImagePath := filepath.Join(os.TempDir(), "controller-episode-image.jpg")
	if err := os.WriteFile(episodeImagePath, []byte("img"), 0o644); err != nil {
		t.Fatalf("write episode image failed: %v", err)
	}
	item.LocalImage = episodeImagePath
	episodeFilePath := filepath.Join(os.TempDir(), "controller-episode-file.mp3")
	if err := os.WriteFile(episodeFilePath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write episode file failed: %v", err)
	}
	item.DownloadPath = episodeFilePath
	item.FileURL = "https://example.com/fallback.mp3"
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("update item paths failed: %v", err)
	}

	podcastLocalImagePath := service.GetPodcastLocalImagePath(podcast.Image, podcast.Title)
	if err := os.MkdirAll(filepath.Dir(podcastLocalImagePath), 0o755); err != nil {
		t.Fatalf("mkdir podcast local image dir failed: %v", err)
	}
	if err := os.WriteFile(podcastLocalImagePath, []byte("podcast-image"), 0o644); err != nil {
		t.Fatalf("write podcast local image failed: %v", err)
	}

	restoreSearchProvider := searchProvider
	searchProvider = map[string]service.SearchService{
		"itunes": mockSearchService{
			result: []*model.CommonSearchResultModel{
				{URL: existingURL, Title: "Already Saved"},
				{URL: "https://example.com/new.xml", Title: "New"},
			},
		},
		"podcastindex": mockSearchService{
			result: []*model.CommonSearchResultModel{
				{URL: "https://example.com/pidx.xml", Title: "PodcastIndex"},
			},
		},
	}
	t.Cleanup(func() {
		searchProvider = restoreSearchProvider
	})

	// pages.go handlers
	resp := performRequest(router, http.MethodGet, "/add", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /add expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/home", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /home expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/podcast/"+podcast.ID+"?page=1&count=1", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /podcast/:id expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/player?itemIds="+item.ID, nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /player itemIds expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/player?podcastID="+podcast.ID, nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /player podcastID expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/player?tagIds="+tag.ID, nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /player tagIds expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/player", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /player latest expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/settings-page", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /settings-page expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/backups", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /backups expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/episodes?page=1&count=10", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /episodes expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/tags-page?page=1&count=10", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /tags-page expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/search?q=test&searchSource=itunes", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /search expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/search?q=test&searchSource=unknown", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /search unknown provider expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/opml", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /opml expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodPost, "/add-new", strings.NewReader("url="+existingURL), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("POST /add-new expected 400 for duplicate, got %d", resp.Code)
	}

	var uploadBody bytes.Buffer
	uploader := multipart.NewWriter(&uploadBody)
	fileWriter, err := uploader.CreateFormFile("file", "bad.opml")
	if err != nil {
		t.Fatalf("create form file failed: %v", err)
	}
	if _, err := fileWriter.Write([]byte("invalid-opml")); err != nil {
		t.Fatalf("write opml content failed: %v", err)
	}
	if err := uploader.Close(); err != nil {
		t.Fatalf("close multipart writer failed: %v", err)
	}
	resp = performRequest(router, http.MethodPost, "/opml", &uploadBody, map[string]string{
		"Content-Type": uploader.FormDataContentType(),
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("POST /opml expected 400 for invalid payload, got %d", resp.Code)
	}

	// podcast.go handlers
	resp = performRequest(router, http.MethodGet, "/podcasts", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /podcasts expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/podcasts?sort=name&order=desc", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /podcasts sorted expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/podcasts/"+podcast.ID, nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /podcasts/:id expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/podcasts/missing", nil, nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("GET /podcasts/missing expected 400, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/podcasts/"+podcast.ID+"/pause", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /podcasts/:id/pause expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/podcasts/"+podcast.ID+"/unpause", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /podcasts/:id/unpause expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodPatch, "/podcasts/"+podcast.ID+"/retention", strings.NewReader(`{"keepAll":true}`), map[string]string{
		"Content-Type": "application/json",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("PATCH /podcasts/:id/retention expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodPatch, "/podcasts/"+podcast.ID+"/retention", strings.NewReader(`{}`), map[string]string{
		"Content-Type": "application/json",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("PATCH /podcasts/:id/retention empty expected 400, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodPatch, "/podcasts/"+podcast.ID+"/sponsor-skip", strings.NewReader(`{"autoSkipSponsorChapters":true}`), map[string]string{
		"Content-Type": "application/json",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("PATCH /podcasts/:id/sponsor-skip expected 200, got %d", resp.Code)
	}

	resp = performRequest(router, http.MethodGet, "/podcasts/"+podcast.ID+"/items", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /podcasts/:id/items expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/podcasts/missing/download", nil, nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("GET /podcasts/missing/download expected 400, got %d", resp.Code)
	}

	resp = performRequest(router, http.MethodGet, "/podcastitems", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /podcastitems expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/podcastitems/"+item.ID, nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /podcastitems/:id expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/podcastitems/"+item.ID+"/image", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /podcastitems/:id/image local expected 200, got %d", resp.Code)
	}
	if err := os.Remove(episodeImagePath); err != nil {
		t.Fatalf("remove episode image failed: %v", err)
	}
	resp = performRequest(router, http.MethodGet, "/podcastitems/"+item.ID+"/image", nil, nil)
	if resp.Code != http.StatusFound {
		t.Fatalf("GET /podcastitems/:id/image remote expected 302, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/podcasts/"+podcast.ID+"/image", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /podcasts/:id/image local expected 200, got %d", resp.Code)
	}
	if err := os.Remove(podcastLocalImagePath); err != nil {
		t.Fatalf("remove podcast local image failed: %v", err)
	}
	resp = performRequest(router, http.MethodGet, "/podcasts/"+podcast.ID+"/image", nil, nil)
	if resp.Code != http.StatusFound {
		t.Fatalf("GET /podcasts/:id/image remote expected 302, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/podcastitems/"+item.ID+"/file", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /podcastitems/:id/file local expected 200, got %d", resp.Code)
	}
	item.DownloadPath = filepath.Join(os.TempDir(), "missing-controller-episode.mp3")
	if err := db.UpdatePodcastItem(&item); err != nil {
		t.Fatalf("update item missing download path failed: %v", err)
	}
	resp = performRequest(router, http.MethodGet, "/podcastitems/"+item.ID+"/file", nil, nil)
	if resp.Code != http.StatusFound {
		t.Fatalf("GET /podcastitems/:id/file remote expected 302, got %d", resp.Code)
	}

	resp = performRequest(router, http.MethodGet, "/podcastitems/"+item.ID+"/markUnplayed", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /podcastitems/:id/markUnplayed expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/podcastitems/"+item.ID+"/markPlayed", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /podcastitems/:id/markPlayed expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/podcastitems/"+item.ID+"/bookmark", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /podcastitems/:id/bookmark expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/podcastitems/"+item.ID+"/unbookmark", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /podcastitems/:id/unbookmark expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodPatch, "/podcastitems/"+item.ID, strings.NewReader(`{"title":"patched-title","isPlayed":true}`), map[string]string{
		"Content-Type": "application/json",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("PATCH /podcastitems/:id expected 200, got %d", resp.Code)
	}

	service.PauseDownloads()
	resp = performRequest(router, http.MethodGet, "/podcastitems/"+item.ID+"/download", nil, nil)
	if resp.Code != http.StatusConflict {
		t.Fatalf("GET /podcastitems/:id/download while paused expected 409, got %d", resp.Code)
	}
	service.ResumeDownloads()

	resp = performRequest(router, http.MethodPost, "/podcasts", strings.NewReader(`{"url":"`+existingURL+`"}`), map[string]string{
		"Content-Type": "application/json",
	})
	if resp.Code != http.StatusConflict {
		t.Fatalf("POST /podcasts duplicate expected 409, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodPost, "/podcasts", strings.NewReader(`{}`), map[string]string{
		"Content-Type": "application/json",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("POST /podcasts invalid expected 400, got %d", resp.Code)
	}

	resp = performRequest(router, http.MethodGet, "/tags", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /tags expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/tags/"+tag.ID, nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /tags/:id expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/rss", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /rss expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/podcasts/"+podcast.ID+"/rss", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /podcasts/:id/rss expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/tags/"+tag.ID+"/rss", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /tags/:id/rss expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodPost, "/tags", strings.NewReader(`{"label":"controller-expanded","description":"dupe"}`), map[string]string{
		"Content-Type": "application/json",
	})
	if resp.Code != http.StatusConflict {
		t.Fatalf("POST /tags duplicate expected 409, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodPost, "/podcasts/"+podcast.ID+"/tags/"+tag.ID, nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /podcasts/:id/tags/:tagID expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodDelete, "/podcasts/"+podcast.ID+"/tags/"+tag.ID, nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("DELETE /podcasts/:id/tags/:tagID expected 200, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodPost, "/settings", strings.NewReader(`{"downloadOnAdd":true,"initialDownloadCount":1,"autoDownload":true,"appendDateToFileName":false,"appendEpisodeNumberToFileName":false,"darkMode":false,"downloadEpisodeImages":true,"generateNFOFile":false,"dontDownloadDeletedFromDisk":true,"baseURL":"http://localhost","maxDownloadConcurrency":2,"userAgent":"briefcast-test"}`), map[string]string{
		"Content-Type": "application/json",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /settings expected 200, got %d", resp.Code)
	}

	resp = performRequest(router, http.MethodDelete, "/tags/"+tag.ID, nil, nil)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("DELETE /tags/:id expected 204, got %d", resp.Code)
	}

	// Delete flows near the end because they mutate all linked data.
	resp = performRequest(router, http.MethodDelete, "/podcasts/"+podcast.ID+"/items", nil, nil)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("DELETE /podcasts/:id/items expected 204, got %d", resp.Code)
	}
	resp = performRequest(router, http.MethodDelete, "/podcasts/"+podcast.ID+"/podcast", nil, nil)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("DELETE /podcasts/:id/podcast expected 204, got %d", resp.Code)
	}

	podcastToDelete, _ := createControllerPodcastAndItem(t)
	resp = performRequest(router, http.MethodDelete, "/podcasts/"+podcastToDelete.ID+"/items-only", nil, nil)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("DELETE /podcasts/:id/items-only expected 204, got %d", resp.Code)
	}

	resp = performRequest(router, http.MethodDelete, "/podcasts/"+podcastToDelete.ID, nil, nil)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("DELETE /podcasts/:id expected 204, got %d", resp.Code)
	}

	resp = performRequest(router, http.MethodPatch, "/podcasts/"+podcastToDelete.ID+"/sponsor-skip", strings.NewReader(`{}`), map[string]string{
		"Content-Type": "application/json",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("PATCH /podcasts/:id/sponsor-skip empty expected 400, got %d", resp.Code)
	}

	resp = performRequest(router, http.MethodPost, "/tags", strings.NewReader(`{}`), map[string]string{
		"Content-Type": "application/json",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("POST /tags invalid payload expected 400, got %d", resp.Code)
	}

	resp = performRequest(router, http.MethodPost, "/settings", strings.NewReader(`{"maxDownloadConcurrency":"nope"}`), map[string]string{
		"Content-Type": "application/json",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("POST /settings invalid payload expected 400, got %d", resp.Code)
	}

	resp = performRequest(router, http.MethodPost, "/add-new", strings.NewReader(""), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("POST /add-new empty expected 400, got %d", resp.Code)
	}

	resp = performRequest(router, http.MethodGet, "/ws", nil, nil)
	if resp.Code != http.StatusOK && resp.Code != http.StatusBadRequest {
		t.Fatalf("GET /ws expected 200 or 400 in upgrade-failure path, got %d", resp.Code)
	}
}

// TestControllerHelpers handles the corresponding operation.
func TestControllerHelpers(t *testing.T) {
	setupControllersTestDB(t)
	podcast, item := createControllerPodcastAndItem(t)
	item.TranscriptStatus = ""
	item.TranscriptJSON = ""
	item.ChaptersJSON = ""
	item.ID3ChaptersJSON = "[]"
	decoratePodcastItem(&item)
	if item.TranscriptStatus != "missing" || !item.HasChapters {
		t.Fatalf("decoratePodcastItem did not set expected fields: %+v", item)
	}

	if got := GetFileContentType(filepath.Join(os.TempDir(), "does-not-exist.bin")); got == "" {
		t.Fatalf("expected default content type for missing file")
	}

	options := getSortOptions()
	b, err := json.Marshal(options)
	if err != nil || !bytes.Contains(b, []byte("release_asc")) {
		t.Fatalf("unexpected sort options payload: %v %s", err, string(b))
	}

	items := getItemsToPlay([]string{item.ID}, "", nil)
	if len(items) != 1 {
		t.Fatalf("expected itemIds branch to return one item, got %d", len(items))
	}
	items = getItemsToPlay(nil, podcast.ID, nil)
	if len(items) == 0 {
		t.Fatalf("expected podcast branch to return items")
	}
	tag, err := service.AddTag("helper-tag-"+time.Now().Format("150405"), "desc")
	if err != nil {
		t.Fatalf("add tag failed: %v", err)
	}
	if err := db.AddTagToPodcast(podcast.ID, tag.ID); err != nil {
		t.Fatalf("add tag to podcast failed: %v", err)
	}
	items = getItemsToPlay(nil, "", []string{tag.ID})
	if len(items) == 0 {
		t.Fatalf("expected tagIds branch to return items")
	}
}

// TestHandleWebsocketMessagesWithoutConnections handles the corresponding operation.
func TestHandleWebsocketMessagesWithoutConnections(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	hub.broadcast <- Message{Identifier: "alpha", MessageType: "RegisterPlayer"}
	hub.broadcast <- Message{Identifier: "alpha", MessageType: "PlayerRemoved"}
	hub.broadcast <- Message{
		Identifier:  "alpha",
		MessageType: "Enqueue",
		Payload:     `{"itemIds":[],"podcastID":"","tagIds":[]}`,
	}
}

// TestControllerMissingIDErrorBranches handles the corresponding operation.
func TestControllerMissingIDErrorBranches(t *testing.T) {
	setupControllersTestDB(t)
	router := makeExpandedRouter(t)

	cases := []struct {
		method string
		path   string
		body   string
		status int
	}{
		{method: http.MethodGet, path: "/podcasts/missing/pause", status: http.StatusBadRequest},
		{method: http.MethodGet, path: "/podcasts/missing/unpause", status: http.StatusBadRequest},
		{method: http.MethodDelete, path: "/podcasts/missing", status: http.StatusBadRequest},
		{method: http.MethodDelete, path: "/podcasts/missing/podcast", status: http.StatusBadRequest},
		{method: http.MethodDelete, path: "/podcasts/missing/items", status: http.StatusBadRequest},
		{method: http.MethodDelete, path: "/podcasts/missing/items-only", status: http.StatusBadRequest},
		{method: http.MethodGet, path: "/podcasts/missing/items", status: http.StatusOK},
		{method: http.MethodGet, path: "/podcasts/missing/download", status: http.StatusBadRequest},
		{method: http.MethodGet, path: "/podcastitems/missing", status: http.StatusBadRequest},
		{method: http.MethodGet, path: "/podcastitems/missing/markUnplayed", status: http.StatusBadRequest},
		{method: http.MethodGet, path: "/podcastitems/missing/markPlayed", status: http.StatusBadRequest},
		{method: http.MethodGet, path: "/podcastitems/missing/bookmark", status: http.StatusBadRequest},
		{method: http.MethodGet, path: "/podcastitems/missing/unbookmark", status: http.StatusBadRequest},
		{method: http.MethodPost, path: "/podcastitems/missing/resume", status: http.StatusBadRequest},
	}

	for _, tc := range cases {
		var body io.Reader
		headers := map[string]string{}
		if tc.body != "" {
			body = strings.NewReader(tc.body)
			headers["Content-Type"] = "application/json"
		}
		resp := performRequest(router, tc.method, tc.path, body, headers)
		if resp.Code != tc.status {
			t.Fatalf("%s %s expected %d, got %d", tc.method, tc.path, tc.status, resp.Code)
		}
	}

	resp := performRequest(router, http.MethodPatch, "/podcasts/missing/retention", strings.NewReader(`{"keepAll":true}`), map[string]string{
		"Content-Type": "application/json",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("PATCH /podcasts/missing/retention expected 200 (no-op update path), got %d", resp.Code)
	}

	resp = performRequest(router, http.MethodPatch, "/podcasts/missing/sponsor-skip", strings.NewReader(`{"autoSkipSponsorChapters":true}`), map[string]string{
		"Content-Type": "application/json",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("PATCH /podcasts/missing/sponsor-skip expected 200 (no-op update path), got %d", resp.Code)
	}

	resp = performRequest(router, http.MethodPatch, "/podcastitems/missing", strings.NewReader(`{"title":"x"}`), map[string]string{
		"Content-Type": "application/json",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("PATCH /podcastitems/missing expected 400, got %d", resp.Code)
	}
}

// TestDownloadAndSearchControllersHandleDatabaseErrors handles the corresponding operation.
func TestDownloadAndSearchControllersHandleDatabaseErrors(t *testing.T) {
	setupControllersTestDB(t)
	router := makeRouter()

	sqlDB, err := db.DB.DB()
	if err != nil {
		t.Fatalf("failed to get sql db handle: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("failed to close sql db handle: %v", err)
	}

	for _, path := range []string{"/downloads/pause", "/downloads/resume", "/downloads/cancel"} {
		resp := performRequest(router, http.MethodPost, path, nil, nil)
		if resp.Code != http.StatusInternalServerError {
			t.Fatalf("POST %s expected 500 when DB is closed, got %d", path, resp.Code)
		}
	}

	resp := performRequest(router, http.MethodGet, "/search/local?q=hello", nil, nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("GET /search/local expected 400 when DB is closed, got %d", resp.Code)
	}
}

// TestWebsocketHandlerRoundTrip handles the corresponding operation.
func TestWebsocketHandlerRoundTrip(t *testing.T) {
	setupControllersTestDB(t)
	hub := NewHub()
	go hub.Run()

	_, item := createControllerPodcastAndItem(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/ws", hub.Handler)
	server := httptest.NewServer(router)
	defer server.Close()

	wsURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse httptest server url: %v", err)
	}
	wsURL.Scheme = "ws"
	wsURL.Path = "/ws"

	playerConn, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial player websocket: %v", err)
	}
	defer playerConn.Close()

	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	if err != nil {
		t.Fatalf("failed to dial client websocket: %v", err)
	}
	defer clientConn.Close()

	if err := playerConn.WriteJSON(Message{Identifier: "player-1", MessageType: "RegisterPlayer"}); err != nil {
		t.Fatalf("failed to write RegisterPlayer: %v", err)
	}

	_ = playerConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var playerAck Message
	if err := playerConn.ReadJSON(&playerAck); err != nil {
		t.Fatalf("failed to read player register ack: %v", err)
	}
	if playerAck.MessageType != "PlayerExists" {
		t.Fatalf("expected PlayerExists ack for player registration, got %+v", playerAck)
	}

	if err := clientConn.WriteJSON(Message{Identifier: "player-1", MessageType: "Register"}); err != nil {
		t.Fatalf("failed to write Register from client: %v", err)
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var registerAck Message
	if err := clientConn.ReadJSON(&registerAck); err != nil {
		t.Fatalf("failed to read client register ack: %v", err)
	}
	if registerAck.MessageType != "PlayerExists" {
		t.Fatalf("expected PlayerExists for registered client, got %+v", registerAck)
	}

	payload := `{"itemIds":["` + item.ID + `"],"podcastID":"","tagIds":[]}`
	if err := clientConn.WriteJSON(Message{
		Identifier:  "player-1",
		MessageType: "Enqueue",
		Payload:     payload,
	}); err != nil {
		t.Fatalf("failed to write Enqueue message: %v", err)
	}

	_ = playerConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var enqueueMsg Message
	if err := playerConn.ReadJSON(&enqueueMsg); err != nil {
		t.Fatalf("failed to read enqueue payload on player connection: %v", err)
	}
	if enqueueMsg.MessageType != "Enqueue" {
		t.Fatalf("expected Enqueue message type, got %+v", enqueueMsg)
	}
	if !strings.Contains(enqueueMsg.Payload, item.ID) {
		t.Fatalf("expected enqueue payload to include item id %s, got %s", item.ID, enqueueMsg.Payload)
	}

	// Closing the player should trigger the player-removed broadcast path.
	if err := playerConn.Close(); err != nil {
		t.Fatalf("failed to close player connection: %v", err)
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var removedMsg Message
	if err := clientConn.ReadJSON(&removedMsg); err != nil {
		t.Fatalf("failed to read player removal notice: %v", err)
	}
	if removedMsg.MessageType != "NoPlayer" {
		t.Fatalf("expected NoPlayer after player disconnect, got %+v", removedMsg)
	}
}
