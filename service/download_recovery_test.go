package service

import (
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
)

func resetDownloadBackoffForTests() {
	downloadBackoffMu.Lock()
	downloadBackoff = make(map[string]downloadBackoffEntry)
	downloadBackoffMu.Unlock()
}

func TestRequeueStaleDownloadsOnlyTouchesAbandonedRows(t *testing.T) {
	setupRetentionTestDB(t)
	t.Setenv(httpTimeoutEnv, "")
	podcast := createPodcast(t, "stale-downloads", false)

	create := func(title string, status db.DownloadStatus) db.PodcastItem {
		item := db.PodcastItem{PodcastID: podcast.ID, Title: title, FileURL: "https://media.example.com/" + title + ".mp3", DownloadStatus: status, DownloadedBytes: 4096, DownloadTotalBytes: 8192}
		if err := db.CreatePodcastItem(&item); err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
		return item
	}
	stale := create("stale", db.Downloading)
	live := create("live", db.Downloading)
	staleDownloaded := create("done", db.Downloaded)
	old := time.Now().Add(-3 * time.Hour)
	for _, id := range []string{stale.ID, staleDownloaded.ID} {
		if err := db.DB.Model(&db.PodcastItem{}).Where("id = ?", id).UpdateColumn("updated_at", old).Error; err != nil {
			t.Fatalf("age row: %v", err)
		}
	}

	requeued, err := requeueStaleDownloads(time.Now())
	if err != nil || requeued != 1 {
		t.Fatalf("requeueStaleDownloads = %d, %v; want 1", requeued, err)
	}
	want := map[string]db.DownloadStatus{stale.ID: db.NotDownloaded, live.ID: db.Downloading, staleDownloaded.ID: db.Downloaded}
	for id, status := range want {
		var got db.PodcastItem
		if err := db.GetPodcastItemByID(id, &got); err != nil {
			t.Fatalf("reload: %v", err)
		}
		if got.DownloadStatus != status {
			t.Fatalf("%s status = %v, want %v", got.Title, got.DownloadStatus, status)
		}
		if id == stale.ID && got.DownloadedBytes != 4096 {
			t.Fatalf("requeue should keep resume progress, got %d bytes", got.DownloadedBytes)
		}
	}
}

func TestStaleDownloadAgeCoversHTTPTimeout(t *testing.T) {
	t.Setenv(httpTimeoutEnv, "")
	if got := staleDownloadAge(); got != time.Hour {
		t.Fatalf("default stale age = %v, want 1h (900 s timeout)", got)
	}
	t.Setenv(httpTimeoutEnv, "3600")
	if got := staleDownloadAge(); got != 2*time.Hour {
		t.Fatalf("stale age = %v, want twice the timeout", got)
	}
	t.Setenv(httpTimeoutEnv, "0")
	if got := staleDownloadAge(); got != time.Hour {
		t.Fatalf("disabled timeout stale age = %v, want 1h", got)
	}
}

func TestDownloadRetryDelaySchedule(t *testing.T) {
	want := map[int]time.Duration{0: 0, 1: 0, 2: 30 * time.Minute, 3: time.Hour, 4: 2 * time.Hour, 7: 16 * time.Hour, 8: 24 * time.Hour, 50: 24 * time.Hour, 200: 24 * time.Hour}
	for failures, delay := range want {
		if got := downloadRetryDelay(failures); got != delay {
			t.Fatalf("downloadRetryDelay(%d) = %v, want %v", failures, got, delay)
		}
	}
}

func TestDownloadBackoffSkipsRepeatedFailuresUntilDue(t *testing.T) {
	resetDownloadBackoffForTests()
	t.Cleanup(resetDownloadBackoffForTests)
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	items := []db.PodcastItem{{Base: db.Base{ID: "a"}}, {Base: db.Base{ID: "b"}}}

	if failures, next := recordDownloadFailure("a", now); failures != 1 || !next.Equal(now) {
		t.Fatalf("first failure = %d, %v; want immediate retry", failures, next)
	}
	if due, waiting := splitDownloadBackoff(items, now); len(due) != 2 || waiting != 0 {
		t.Fatalf("after one failure due=%d waiting=%d", len(due), waiting)
	}

	_, next := recordDownloadFailure("a", now)
	if !next.Equal(now.Add(30 * time.Minute)) {
		t.Fatalf("second failure next = %v", next)
	}
	due, waiting := splitDownloadBackoff(items, now.Add(15*time.Minute))
	if len(due) != 1 || due[0].ID != "b" || waiting != 1 {
		t.Fatalf("during backoff due=%v waiting=%d", due, waiting)
	}
	if due, _ := splitDownloadBackoff(items, now.Add(30*time.Minute)); len(due) != 2 {
		t.Fatalf("backoff should expire at next attempt, due=%d", len(due))
	}

	clearDownloadFailures("a")
	if failures, _ := recordDownloadFailure("a", now); failures != 1 {
		t.Fatalf("success should reset the failure count, got %d", failures)
	}
}
