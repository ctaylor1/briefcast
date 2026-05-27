package service

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	assetAudioDir              = "audio"
	assetImagesDir             = "images"
	assetTranscriptDir         = "transcripts"
	assetSummariesDir          = "summaries"
	assetMarkdownTranscriptDir = "markdown/transcripts"
	assetMarkdownSummariesDir  = "markdown/summaries"
)

var (
	specialAssetNameChars = regexp.MustCompile(`[^A-Za-z0-9._ -]+`)
	repeatedAssetSpaces   = regexp.MustCompile(`\s+`)
)

func resolveAssetsDir() string {
	return strings.TrimSpace(os.Getenv("DATA"))
}

func IsPathWithinAssetsDir(filePath string) bool {
	return isPathWithinDir(filePath, resolveAssetsDir())
}

func isPathWithinDir(filePath, dir string) bool {
	cleanPath := strings.TrimSpace(filePath)
	cleanDir := strings.TrimSpace(dir)
	if cleanPath == "" || cleanDir == "" {
		return false
	}

	root, err := filepath.Abs(cleanDir)
	if err != nil {
		return false
	}
	if resolvedRoot, err := filepath.EvalSymlinks(root); err == nil {
		root = resolvedRoot
	}

	target, err := filepath.Abs(cleanPath)
	if err != nil {
		return false
	}
	if resolvedTarget, err := filepath.EvalSymlinks(target); err == nil {
		target = resolvedTarget
	}

	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel))
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
