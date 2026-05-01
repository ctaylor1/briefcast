package service

import (
	"os"
	"path"
	"regexp"
	"strings"
)

const (
	assetAudioDir      = "audio"
	assetImagesDir     = "images"
	assetTranscriptDir = "transcripts"
	assetSummariesDir  = "summaries"
)

var (
	specialAssetNameChars = regexp.MustCompile(`[^A-Za-z0-9._ -]+`)
	repeatedAssetSpaces   = regexp.MustCompile(`\s+`)
)

func resolveAssetsDir() string {
	return strings.TrimSpace(os.Getenv("DATA"))
}

func sanitizeAssetName(name string) string {
	safe := strings.TrimSpace(cleanFileName(name))
	safe = specialAssetNameChars.ReplaceAllString(safe, "")
	safe = repeatedAssetSpaces.ReplaceAllString(safe, " ")
	safe = strings.Trim(safe, " .-")
	if safe == "" {
		return "_"
	}
	return safe
}

func dataPodcastFolderPath(podcastName string) string {
	return path.Join(resolveAssetsDir(), sanitizeAssetName(podcastName))
}

func dataCategoryPodcastFolderPath(category string, podcastName string) string {
	return path.Join(resolveAssetsDir(), category, sanitizeAssetName(podcastName))
}

func createDataCategoryPodcastFolderIfNotExists(category string, podcastName string) string {
	categoryFolder := createFolder(category, resolveAssetsDir())
	return createFolder(podcastName, categoryFolder)
}

func createAudioFolderIfNotExists(podcastName string) string {
	return createDataCategoryPodcastFolderIfNotExists(assetAudioDir, podcastName)
}

func createImagesFolderIfNotExists(podcastName string) string {
	return createDataCategoryPodcastFolderIfNotExists(assetImagesDir, podcastName)
}
