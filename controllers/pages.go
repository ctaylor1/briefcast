package controllers

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/service"
	"github.com/gin-gonic/gin"
)

// SearchGPodderData represents a public type.
type SearchGPodderData struct {
	Q            string `binding:"required" form:"q" json:"q" query:"q"`
	SearchSource string `binding:"required" form:"searchSource" json:"searchSource" query:"searchSource"`
}

// SettingModel represents a public type.
type SettingModel struct {
	DownloadOnAdd                 bool   `form:"downloadOnAdd" json:"downloadOnAdd" query:"downloadOnAdd"`
	InitialDownloadCount          int    `form:"initialDownloadCount" json:"initialDownloadCount" query:"initialDownloadCount"`
	InitialDownloadMode           string `form:"initialDownloadMode" json:"initialDownloadMode" query:"initialDownloadMode"`
	InitialDownloadMonths         int    `form:"initialDownloadMonths" json:"initialDownloadMonths" query:"initialDownloadMonths"`
	AutoDownload                  bool   `form:"autoDownload" json:"autoDownload" query:"autoDownload"`
	AppendDateToFileName          bool   `form:"appendDateToFileName" json:"appendDateToFileName" query:"appendDateToFileName"`
	AppendEpisodeNumberToFileName bool   `form:"appendEpisodeNumberToFileName" json:"appendEpisodeNumberToFileName" query:"appendEpisodeNumberToFileName"`
	DarkMode                      bool   `form:"darkMode" json:"darkMode" query:"darkMode"`
	DownloadEpisodeImages         bool   `form:"downloadEpisodeImages" json:"downloadEpisodeImages" query:"downloadEpisodeImages"`
	GenerateNFOFile               bool   `form:"generateNFOFile" json:"generateNFOFile" query:"generateNFOFile"`
	DontDownloadDeletedFromDisk   bool   `form:"dontDownloadDeletedFromDisk" json:"dontDownloadDeletedFromDisk" query:"dontDownloadDeletedFromDisk"`
	BaseURL                       string `form:"baseURL" json:"baseURL" query:"baseURL"`
	MaxDownloadConcurrency        int    `form:"maxDownloadConcurrency" json:"maxDownloadConcurrency" query:"maxDownloadConcurrency"`
	UserAgent                     string `form:"userAgent" json:"userAgent" query:"userAgent"`
}

var searchOptions = map[string]string{
	"itunes":       "iTunes",
	"podcastindex": "PodcastIndex",
	"gpodder":      "gpodder.net",
}
var searchProvider = map[string]service.SearchService{
	"itunes":       new(service.ItunesService),
	"podcastindex": new(service.PodcastIndexService),
	"gpodder":      new(service.GpodderService),
}

// enrichSearchResults is swappable so handler tests stay offline.
var enrichSearchResults = service.EnrichSearchResults

func getItemsToPlay(itemIds []string, podcastID string, tagIds []string) []db.PodcastItem {
	var items []db.PodcastItem
	if len(itemIds) > 0 {
		toAdd, _ := service.GetAllPodcastItemsByIds(itemIds)
		items = *toAdd

	} else if podcastID != "" {
		pod := service.GetPodcastByID(podcastID)
		items = pod.PodcastItems
	} else if len(tagIds) != 0 {
		tags := service.GetTagsByIds(tagIds)
		var podIds []string
		for _, tag := range *tags {
			for _, pod := range tag.Podcasts {
				podIds = append(podIds, pod.ID)
			}
		}
		items = *service.GetAllPodcastItemsByPodcastIds(podIds)
	}
	return items
}

// Search handles the corresponding operation.
func Search(c *gin.Context) {
	var searchQuery SearchGPodderData
	if c.ShouldBindQuery(&searchQuery) == nil {
		var searcher service.SearchService
		var isValidSearchProvider bool
		if searcher, isValidSearchProvider = searchProvider[searchQuery.SearchSource]; !isValidSearchProvider {
			searcher = new(service.PodcastIndexService)
		}

		data := searcher.Query(searchQuery.Q)
		enrichSearchResults(data)
		allPodcasts := service.GetAllPodcasts("")

		urls := make(map[string]string, len(*allPodcasts))
		for _, pod := range *allPodcasts {
			urls[pod.URL] = pod.ID
		}
		for _, pod := range data {
			_, ok := urls[pod.URL]
			pod.AlreadySaved = ok
		}
		c.JSON(200, data)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
	}
}

// GetOPML handles the corresponding operation.
func GetOPML(c *gin.Context) {

	useBriefcastLink := c.DefaultQuery("useBriefcastLink", "false") == "true"

	data, err := service.ExportOPML(useBriefcastLink, getBaseURL(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid request"})
		return
	}
	c.Header("Content-Disposition", "attachment; filename=Briefcast-export.opml")
	c.Data(200, "text/xml", data)
}

// UploadOpml handles the corresponding operation.
func UploadOpml(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.MaxOPMLUploadBytes)
	content, err := readOPMLUpload(c.Request)
	if err != nil {
		if errors.Is(err, service.ErrOPMLTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid request"})
		return
	}

	err = service.AddOpml(content)
	if err != nil {
		if errors.Is(err, service.ErrOPMLTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
	} else {
		c.JSON(200, gin.H{"success": "File uploaded"})
	}
}

func readOPMLUpload(r *http.Request) (string, error) {
	reader, err := r.MultipartReader()
	if err != nil {
		return "", err
	}

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if isRequestBodyTooLarge(err) {
				return "", service.ErrOPMLTooLarge
			}
			return "", err
		}
		if part.FormName() != "file" || part.FileName() == "" {
			_ = part.Close()
			continue
		}

		var buf bytes.Buffer
		n, copyErr := io.Copy(&buf, io.LimitReader(part, service.MaxOPMLContentBytes+1))
		_ = part.Close()
		if copyErr != nil {
			if isRequestBodyTooLarge(copyErr) {
				return "", service.ErrOPMLTooLarge
			}
			return "", copyErr
		}
		if n > service.MaxOPMLContentBytes {
			return "", service.ErrOPMLTooLarge
		}
		return buf.String(), nil
	}

	return "", errors.New("missing OPML upload file")
}

func isRequestBodyTooLarge(err error) bool {
	return err != nil && strings.Contains(err.Error(), "request body too large")
}
