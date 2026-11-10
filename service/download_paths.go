package service

import (
	"encoding/json"
	"os"
	"path"
	"strings"

	"github.com/ctaylor1/briefcast/db"
)

// downloadSidecarSuffix names the file that records who owns a partial download.
const downloadSidecarSuffix = ".briefcast-partial.json"

// downloadSidecar records which episode and URL a partial file belongs to, and
// the validators the server sent, so a resume can be checked with If-Range.
type downloadSidecar struct {
	PodcastItemID string `json:"podcast_item_id"`
	URL           string `json:"url"`
	ETag          string `json:"etag,omitempty"`
	LastModified  string `json:"last_modified,omitempty"`
}

// ifRangeValidator returns a validator usable in If-Range. A weak ETag cannot
// be used there, so Last-Modified is the fallback.
func (s downloadSidecar) ifRangeValidator() string {
	if etag := strings.TrimSpace(s.ETag); etag != "" && !strings.HasPrefix(etag, "W/") {
		return etag
	}
	return strings.TrimSpace(s.LastModified)
}

func readDownloadSidecar(sidecarPath string) (downloadSidecar, bool) {
	// #nosec G304 -- sidecarPath is derived from the sanitized download path.
	raw, err := os.ReadFile(sidecarPath)
	if err != nil {
		return downloadSidecar{}, false
	}
	var sidecar downloadSidecar
	if err := json.Unmarshal(raw, &sidecar); err != nil {
		return downloadSidecar{}, false
	}
	return sidecar, true
}

func writeDownloadSidecar(sidecarPath string, sidecar downloadSidecar) error {
	raw, err := json.Marshal(sidecar)
	if err != nil {
		return err
	}
	return os.WriteFile(sidecarPath, raw, 0o600)
}

// resolveEpisodeDownloadPath picks where an episode's audio is written. File
// names come from titles, so two episodes can map to one path ("Trailer",
// "Bonus"). When the plain path belongs to another episode (a finished file,
// another episode's partial download, or a download running now), a short form
// of the episode ID is added to the name. The returned release frees the
// in-process claim on the path.
func resolveEpisodeDownloadPath(downloadID, owner, folder, fileName string) (string, func()) {
	candidate := path.Join(folder, fileName)
	if !downloadPathOwnedByOther(candidate, downloadID) {
		if release, ok := claimDownloadPath(candidate, owner); ok {
			return candidate, release
		}
	}
	ext := path.Ext(fileName)
	suffix := shortEpisodeID(downloadID)
	if suffix == "" {
		suffix = shortEpisodeID(strings.TrimPrefix(owner, "url:"))
	}
	unique := path.Join(folder, strings.TrimSuffix(fileName, ext)+"-"+suffix+ext)
	release, ok := claimDownloadPath(unique, owner)
	if !ok {
		// Only a second download of the same episode reaches this; the
		// per-episode guard stops that before Download is called.
		return unique, func() {}
	}
	return unique, release
}

func downloadPathOwnedByOther(candidate, downloadID string) bool {
	if downloadID == "" {
		return false
	}
	other, err := db.GetOtherPodcastItemIDByDownloadPath(candidate, downloadID)
	if err != nil {
		// Unknown ownership: a unique name is always safe.
		Logger.Warnw("could not check download path ownership; using a unique name", "podcast_item_id", downloadID, "error", err)
		return true
	}
	if other != "" {
		return true
	}
	if sidecar, ok := readDownloadSidecar(candidate + downloadSidecarSuffix); ok && sidecar.PodcastItemID != downloadID {
		return true
	}
	return false
}

// shortEpisodeID returns up to eight file-name-safe characters of an ID.
func shortEpisodeID(id string) string {
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			if b.Len() == 8 {
				break
			}
		}
	}
	return b.String()
}
