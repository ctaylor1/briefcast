package service

import (
	"archive/tar"
	"compress/gzip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/sanitize"
	stringy "github.com/gobeam/stringy"
)

var (
	// ErrDownloadCancelled is a public variable.
	ErrDownloadCancelled = errors.New("download cancelled")
	// ErrDownloadPaused is a public variable.
	ErrDownloadPaused = errors.New("download paused")
	backupNow         = func() time.Time { return time.Now().UTC() }
)

const (
	defaultHTTPTimeoutSeconds = 900
	httpTimeoutEnv            = "HTTP_TIMEOUT_SECONDS"
)

// Download handles the corresponding operation.
func Download(downloadID string, link string, episodeTitle string, podcastName string, prefix string) (string, error) {
	if link == "" {
		return "", errors.New("Download path empty")
	}
	client := httpClient()

	req, err := getRequest(link)
	if err != nil {
		logError("error creating request", err, "url", link)
		return "", err
	}
	fileName := getFileName(link, episodeTitle, ".mp3")
	if prefix != "" {
		fileName = fmt.Sprintf("%s-%s", prefix, fileName)
	}
	folder := createAudioFolderIfNotExists(podcastName)
	finalPath := path.Join(folder, fileName)

	var resumeOffset int64
	if info, statErr := os.Stat(finalPath); statErr == nil {
		resumeOffset = info.Size()
	}
	// If a partial file already exists, request the remaining bytes.
	// Callers treat non-2xx responses as hard failures and reset item state.
	if resumeOffset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeOffset))
	}

	resp, err := doRequestWithHostLimit(client, req)
	if err != nil {
		logError("error getting response", err, "url", link)
		return "", err
	}

	if resp.StatusCode >= http.StatusBadRequest {
		if resumeOffset > 0 && resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
			resp.Body.Close()
			// Some hosts reject stale resume offsets (416). Drop the partial file and retry once.
			if removeErr := os.Remove(finalPath); removeErr != nil && !os.IsNotExist(removeErr) {
				Logger.Warnw("failed to reset partial file after range rejection", "path", finalPath, "error", removeErr)
			} else {
				Logger.Warnw("range resume rejected; retrying full download", "url", link, "path", finalPath, "resume_offset", resumeOffset)
				return Download(downloadID, link, episodeTitle, podcastName, prefix)
			}
		}
		resp.Body.Close()
		// Keep upstream status visible in logs/UI (for example 416 resume mismatches).
		return "", fmt.Errorf("download failed with status %s", resp.Status)
	}

	var file *os.File
	if resumeOffset > 0 && resp.StatusCode == http.StatusPartialContent {
		// Continue writing to an existing partial file only for 206 responses.
		// #nosec G304 -- finalPath is derived from sanitized filename + configured data directory.
		file, err = os.OpenFile(finalPath, os.O_WRONLY|os.O_APPEND, 0o644)
	} else {
		// Servers that ignore range requests can still succeed with a full-body response.
		resumeOffset = 0
		// #nosec G304 -- finalPath is derived from sanitized filename + configured data directory.
		file, err = os.Create(finalPath)
	}
	if err != nil {
		logError("error creating file", err, "path", finalPath, "url", link)
		return "", err
	}
	defer resp.Body.Close()

	buffer := make([]byte, 32*1024)
	downloadedBytes := resumeOffset
	totalBytes := resolveTotalBytes(resp, resumeOffset)
	if downloadID != "" && totalBytes > 0 {
		_ = db.UpdatePodcastItemDownloadProgress(downloadID, downloadedBytes, totalBytes)
	}

	lastReport := time.Now()
	lastReportedBytes := downloadedBytes
	const minReportBytes = int64(256 * 1024)
	const minReportInterval = 750 * time.Millisecond
	for {
		if DownloadsPaused() || (downloadID != "" && IsDownloadPaused(downloadID)) {
			_ = file.Close()
			_ = resp.Body.Close()
			if downloadID != "" {
				ClearDownloadPause(downloadID)
			}
			return "", ErrDownloadPaused
		}
		if downloadID != "" && IsDownloadCancelled(downloadID) {
			_ = file.Close()
			_ = resp.Body.Close()
			_ = os.Remove(finalPath)
			ClearDownloadCancellation(downloadID)
			return "", ErrDownloadCancelled
		}
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			downloadedBytes += int64(n)
			if _, writeErr := file.Write(buffer[:n]); writeErr != nil {
				logError("error saving file", writeErr, "path", finalPath, "url", link)
				return "", writeErr
			}
			if downloadID != "" {
				if downloadedBytes-lastReportedBytes >= minReportBytes || time.Since(lastReport) >= minReportInterval {
					_ = db.UpdatePodcastItemDownloadProgress(downloadID, downloadedBytes, totalBytes)
					lastReport = time.Now()
					lastReportedBytes = downloadedBytes
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			logError("error saving file", readErr, "path", finalPath, "url", link)
			return "", readErr
		}
	}
	if downloadID != "" {
		_ = db.UpdatePodcastItemDownloadProgress(downloadID, downloadedBytes, totalBytes)
	}
	defer file.Close()
	changeOwnership(finalPath)
	return finalPath, nil

}

func resolveTotalBytes(resp *http.Response, resumeOffset int64) int64 {
	if resp == nil {
		return 0
	}
	if contentRange := resp.Header.Get("Content-Range"); contentRange != "" {
		if total := parseContentRangeTotal(contentRange); total > 0 {
			return total
		}
	}
	if resp.ContentLength > 0 {
		if resp.StatusCode == http.StatusPartialContent && resumeOffset > 0 {
			return resumeOffset + resp.ContentLength
		}
		return resp.ContentLength
	}
	return 0
}

func parseContentRangeTotal(contentRange string) int64 {
	// format: bytes start-end/total
	parts := strings.Split(contentRange, "/")
	if len(parts) != 2 {
		return 0
	}
	totalPart := strings.TrimSpace(parts[1])
	if totalPart == "*" {
		return 0
	}
	total, err := strconv.ParseInt(totalPart, 10, 64)
	if err != nil {
		return 0
	}
	return total
}

// GetPodcastLocalImagePath handles the corresponding operation.
func GetPodcastLocalImagePath(link string, podcastName string) string {
	fileName := getFileName(link, "folder", ".jpg")
	folder := createDataFolderIfNotExists(podcastName)

	finalPath := path.Join(folder, fileName)
	return finalPath
}

// CreateNfoFile handles the corresponding operation.
func CreateNfoFile(podcast *db.Podcast) error {
	fileName := "album.nfo"
	folder := createDataFolderIfNotExists(podcast.Title)

	finalPath := path.Join(folder, fileName)

	type NFO struct {
		XMLName xml.Name `xml:"album"`
		Title   string   `xml:"title"`
		Type    string   `xml:"type"`
		Thumb   string   `xml:"thumb"`
	}

	toSave := NFO{
		Title: podcast.Title,
		Type:  "Broadcast",
		Thumb: podcast.Image,
	}
	out, err := xml.MarshalIndent(toSave, " ", "  ")
	if err != nil {
		return err
	}
	toPersist := xml.Header + string(out)
	return os.WriteFile(finalPath, []byte(toPersist), 0o644)
}

// DownloadPodcastCoverImage handles the corresponding operation.
func DownloadPodcastCoverImage(link string, podcastName string) (string, error) {
	if link == "" {
		return "", errors.New("Download path empty")
	}
	client := httpClient()
	req, err := getRequest(link)
	if err != nil {
		logError("error creating request", err, "url", link)
		return "", err
	}

	resp, err := doRequestWithHostLimit(client, req)
	if err != nil {
		logError("error getting response", err, "url", link)
		return "", err
	}

	fileName := getFileName(link, "folder", ".jpg")
	folder := createDataFolderIfNotExists(podcastName)

	finalPath := path.Join(folder, fileName)
	if _, err := os.Stat(finalPath); !os.IsNotExist(err) {
		changeOwnership(finalPath)
		return finalPath, nil
	}

	// #nosec G304 -- finalPath is derived from sanitized filename + configured data directory.
	file, err := os.Create(finalPath)
	if err != nil {
		logError("error creating file", err, "path", finalPath, "url", link)
		return "", err
	}
	defer resp.Body.Close()
	_, erra := io.Copy(file, resp.Body)
	defer file.Close()
	if erra != nil {
		logError("error saving file", erra, "path", finalPath, "url", link)
		return "", erra
	}
	changeOwnership(finalPath)
	return finalPath, nil
}

// DownloadImage handles the corresponding operation.
func DownloadImage(link string, episodeID string, podcastName string) (string, error) {
	if link == "" {
		return "", errors.New("Download path empty")
	}
	client := httpClient()
	req, err := getRequest(link)
	if err != nil {
		logError("error creating request", err, "url", link)
		return "", err
	}

	resp, err := doRequestWithHostLimit(client, req)
	if err != nil {
		logError("error getting response", err, "url", link)
		return "", err
	}

	fileName := getFileName(link, episodeID, ".jpg")
	folder := createDataFolderIfNotExists(podcastName)
	imageFolder := createFolder("images", folder)
	finalPath := path.Join(imageFolder, fileName)

	if _, err := os.Stat(finalPath); !os.IsNotExist(err) {
		changeOwnership(finalPath)
		return finalPath, nil
	}

	// #nosec G304 -- finalPath is derived from sanitized filename + configured data directory.
	file, err := os.Create(finalPath)
	if err != nil {
		logError("error creating file", err, "path", finalPath, "url", link)
		return "", err
	}
	defer resp.Body.Close()
	_, erra := io.Copy(file, resp.Body)
	defer file.Close()
	if erra != nil {
		logError("error saving file", erra, "path", finalPath, "url", link)
		return "", erra
	}
	changeOwnership(finalPath)
	return finalPath, nil

}
func changeOwnership(path string) {
	uid, err1 := strconv.Atoi(os.Getenv("PUID"))
	gid, err2 := strconv.Atoi(os.Getenv("PGID"))
	Logger.Debugw("attempting ownership update", "path", path)
	if err1 == nil && err2 == nil {
		Logger.Debugw("changing ownership", "path", path, "uid", uid, "gid", gid)
		// #nosec G703 -- path is scoped to configured app data/config directories.
		if err := os.Chown(path, uid, gid); err != nil {
			Logger.Warnw("failed to update ownership", "path", path, "uid", uid, "gid", gid, "error", err)
		}
	}

}

// DeleteFile handles the corresponding operation.
func DeleteFile(filePath string) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(filePath); err != nil {
		return err
	}
	return nil
}

// FileExists handles the corresponding operation.
func FileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil

}

// GetAllBackupFiles handles the corresponding operation.
func GetAllBackupFiles() ([]string, error) {
	var files []string
	folder := createConfigFolderIfNotExists("backups")
	err := filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	return files, err
}

// GetFileSize handles the corresponding operation.
func GetFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func deleteOldBackup() {
	files, err := GetAllBackupFiles()
	if err != nil {
		return
	}
	if len(files) <= 5 {
		return
	}

	toDelete := files[5:]
	for _, file := range toDelete {
		Logger.Infow("deleting old backup file", "path", file)
		if err := DeleteFile(file); err != nil && !os.IsNotExist(err) {
			Logger.Warnw("failed to delete old backup file", "path", file, "error", err)
		}
	}
}

// GetFileSizeFromURL handles the corresponding operation.
func GetFileSizeFromURL(url string) (int64, error) {
	req, err := getRequestWithMethod(http.MethodHead, url)
	if err != nil {
		return 0, err
	}
	resp, err := doRequestWithHostLimit(httpClient(), req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	// Is our request ok?

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("Did not receive 200")
	}

	size, err := strconv.Atoi(resp.Header.Get("Content-Length"))
	if err != nil {
		return 0, err
	}

	return int64(size), nil
}

// CreateBackup handles the corresponding operation.
func CreateBackup() (string, error) {
	backupFileName := "briefcast_backup_" + backupNow().Format("2006.01.02_150405") + ".tar.gz"
	folder := createConfigFolderIfNotExists("backups")
	configPath := os.Getenv("CONFIG")
	tarballFilePath := path.Join(folder, backupFileName)
	// #nosec G304 -- backup destination is under configured app config directory.
	file, err := os.Create(tarballFilePath)
	if err != nil {
		return "", fmt.Errorf("could not create tarball file %q: %w", tarballFilePath, err)
	}
	defer file.Close()

	dbPath := path.Join(configPath, "briefcast.db")
	// #nosec G703 -- dbPath is rooted at configured app config directory.
	_, err = os.Stat(dbPath)
	if err != nil {
		return "", fmt.Errorf("could not find db file %q: %w", dbPath, err)
	}
	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	err = addFileToTarWriter(dbPath, tarWriter)
	if err == nil {
		deleteOldBackup()
	}
	return backupFileName, err
}

func addFileToTarWriter(filePath string, tarWriter *tar.Writer) error {
	// #nosec G304,G703 -- filePath is a generated internal backup input path.
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("could not open file %q: %w", filePath, err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("could not stat file %q: %w", filePath, err)
	}

	header := &tar.Header{
		Name:    filePath,
		Size:    stat.Size(),
		Mode:    int64(stat.Mode()),
		ModTime: stat.ModTime(),
	}

	err = tarWriter.WriteHeader(header)
	if err != nil {
		return fmt.Errorf("could not write tar header for %q: %w", filePath, err)
	}

	_, err = io.Copy(tarWriter, file)
	if err != nil {
		return fmt.Errorf("could not copy file %q to tarball: %w", filePath, err)
	}

	return nil
}
func httpClient() *http.Client {
	timeoutSeconds := getEnvInt(httpTimeoutEnv, defaultHTTPTimeoutSeconds)
	client := http.Client{
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			//	r.URL.Opaque = r.URL.Path
			return nil
		},
	}
	if timeoutSeconds > 0 {
		client.Timeout = time.Duration(timeoutSeconds) * time.Second
	}

	return &client
}

func getRequest(url string) (*http.Request, error) {
	return getRequestWithMethod(http.MethodGet, url)
}

func getRequestWithMethod(method string, targetURL string) (*http.Request, error) {
	req, err := http.NewRequest(method, targetURL, nil)
	if err != nil {
		return nil, err
	}

	setting := db.GetOrCreateSetting()
	if len(setting.UserAgent) > 0 {
		req.Header.Add("User-Agent", setting.UserAgent)
	}

	return req, nil
}

func createFolder(folder string, parent string) string {
	folder = sanitizeAssetName(folder)
	//str := stringy.New(folder)
	folderPath := path.Join(parent, folder)
	// #nosec G703 -- folderPath is composed from sanitized folder names and configured parent roots.
	if _, err := os.Stat(folderPath); os.IsNotExist(err) {
		// #nosec G703 -- folderPath is composed from sanitized folder names and configured parent roots.
		if mkErr := os.MkdirAll(folderPath, 0o777); mkErr != nil {
			Logger.Warnw("failed to create folder", "path", folderPath, "error", mkErr)
			return folderPath
		}
		changeOwnership(folderPath)
	}
	return folderPath
}

func createDataFolderIfNotExists(folder string) string {
	dataPath := os.Getenv("DATA")
	return createFolder(folder, dataPath)
}
func createConfigFolderIfNotExists(folder string) string {
	dataPath := os.Getenv("CONFIG")
	return createFolder(folder, dataPath)
}

func deletePodcastFolder(folder string) error {
	var firstErr error
	paths := []string{
		dataPodcastFolderPath(folder),
		dataCategoryPodcastFolderPath(assetAudioDir, folder),
		dataCategoryPodcastFolderPath(assetTranscriptDir, folder),
		dataCategoryPodcastFolderPath(assetSummariesDir, folder),
	}
	for _, folderPath := range paths {
		if err := os.RemoveAll(folderPath); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func getFileName(link string, title string, defaultExtension string) string {
	parsed := ""
	fileURL, err := url.Parse(link)
	if err != nil {
		// Keep filename generation non-fatal; callers should still return the
		// upstream error from request construction/network execution.
		Logger.Warnw("failed to parse URL while building filename", "error", err, "link_length", len(strings.TrimSpace(link)))
	} else {
		parsed = fileURL.Path
	}
	ext := filepath.Ext(parsed)

	if len(ext) == 0 {
		ext = defaultExtension
	}
	//str := stringy.New(title)
	str := stringy.New(sanitizeAssetName(title))
	return str.KebabCase().Get() + ext

}

func cleanFileName(original string) string {
	return sanitize.Name(original)
}
