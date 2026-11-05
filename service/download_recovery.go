package service

import (
	"sync"
	"time"

	"github.com/ctaylor1/briefcast/db"
)

const (
	staleDownloadMinAge = time.Hour
	downloadBackoffBase = 30 * time.Minute
	downloadBackoffMax  = 24 * time.Hour
)

// staleDownloadAge is how long a "downloading" row may go without an update
// before it is treated as abandoned. Live downloads write progress at least
// every 750 ms while bytes arrive and are aborted by HTTP_TIMEOUT_SECONDS, so
// twice the timeout (at least an hour) cannot belong to a live download.
func staleDownloadAge() time.Duration {
	age := staleDownloadMinAge
	if seconds := getEnvInt(httpTimeoutEnv, defaultHTTPTimeoutSeconds); seconds > 0 {
		if d := 2 * time.Duration(seconds) * time.Second; d > age {
			age = d
		}
	}
	return age
}

// requeueStaleDownloads returns episodes left "downloading" by an interrupted
// process (a restart or crash mid-download) to the download queue. Progress
// bytes are kept so the next attempt can resume. Call it while holding the
// DownloadMissingEpisodes lock.
func requeueStaleDownloads(now time.Time) (int64, error) {
	cutoff := now.Add(-staleDownloadAge())
	result := db.DB.Model(&db.PodcastItem{}).
		Where("download_status = ? AND deleted_at IS NULL AND updated_at < ?", db.Downloading, cutoff).
		Updates(map[string]any{"download_status": db.NotDownloaded})
	return result.RowsAffected, result.Error
}

// Per-episode backoff after consecutive failed scheduled downloads, so a host
// that refuses an episode is not retried on every 15-minute run. The first
// failure retries on the next run; later ones wait 30 min, 1 h, 2 h ... up to
// 24 h. State is in memory: a restart retries everything once, which is the
// same behaviour as before this existed.
type downloadBackoffEntry struct {
	failures int
	next     time.Time
}

var (
	downloadBackoffMu sync.Mutex
	downloadBackoff   = make(map[string]downloadBackoffEntry)
)

func downloadRetryDelay(failures int) time.Duration {
	if failures < 2 {
		return 0
	}
	steps := failures - 2
	if steps > 10 {
		return downloadBackoffMax
	}
	if d := downloadBackoffBase << steps; d < downloadBackoffMax {
		return d
	}
	return downloadBackoffMax
}

// recordDownloadFailure counts a failed attempt and returns the consecutive
// failure count and the earliest next scheduled attempt.
func recordDownloadFailure(id string, now time.Time) (int, time.Time) {
	downloadBackoffMu.Lock()
	defer downloadBackoffMu.Unlock()
	entry := downloadBackoff[id]
	entry.failures++
	entry.next = now.Add(downloadRetryDelay(entry.failures))
	downloadBackoff[id] = entry
	return entry.failures, entry.next
}

func clearDownloadFailures(id string) {
	downloadBackoffMu.Lock()
	delete(downloadBackoff, id)
	downloadBackoffMu.Unlock()
}

// splitDownloadBackoff separates items that are due from those still backing off.
func splitDownloadBackoff(items []db.PodcastItem, now time.Time) (due []db.PodcastItem, waiting int) {
	downloadBackoffMu.Lock()
	defer downloadBackoffMu.Unlock()
	due = items[:0:0]
	for _, item := range items {
		if entry, ok := downloadBackoff[item.ID]; ok && now.Before(entry.next) {
			waiting++
			continue
		}
		due = append(due, item)
	}
	return due, waiting
}
