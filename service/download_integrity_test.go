package service

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
)

// entityServer serves body with the given ETag through http.ServeContent, which
// implements Range and If-Range, and records the request headers it saw.
type entityServer struct {
	mu      sync.Mutex
	ranges  []string
	ifRange []string
}

func (e *entityServer) start(t *testing.T, etag string, body []byte) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		e.ranges = append(e.ranges, r.Header.Get("Range"))
		e.ifRange = append(e.ifRange, r.Header.Get("If-Range"))
		e.mu.Unlock()
		w.Header().Set("ETag", etag)
		http.ServeContent(w, r, "audio.mp3", time.Time{}, bytes.NewReader(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestDownloadDoesNotOverwriteAnotherEpisodesFile(t *testing.T) {
	setupRetentionTestDB(t)
	podcast := createPodcast(t, "Collision Podcast", false)
	server := (&entityServer{}).start(t, `"b"`, []byte("episode-b-audio"))
	link := server.URL + "/audio.mp3"
	folder := createAudioFolderIfNotExists(podcast.Title)
	shared := path.Join(folder, getFileName(link, "Trailer", ".mp3"))

	owner := db.PodcastItem{PodcastID: podcast.ID, Title: "Trailer", GUID: "a", DownloadPath: shared, DownloadStatus: db.Downloaded}
	if err := db.CreatePodcastItem(&owner); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shared, []byte("episode-a-audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := db.PodcastItem{PodcastID: podcast.ID, Title: "Trailer", GUID: "b"}
	if err := db.CreatePodcastItem(&other); err != nil {
		t.Fatal(err)
	}

	got, err := Download(other.ID, link, "Trailer", podcast.Title, "")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if got == shared || !strings.Contains(got, shortEpisodeID(other.ID)) {
		t.Fatalf("expected a unique path for the second episode, got %q", got)
	}
	if raw, _ := os.ReadFile(shared); string(raw) != "episode-a-audio" {
		t.Fatalf("the first episode's file was changed: %q", raw)
	}
	if raw, _ := os.ReadFile(got); string(raw) != "episode-b-audio" {
		t.Fatalf("second episode content %q", raw)
	}
}

func TestDownloadReplacesAPartialFileItDidNotStart(t *testing.T) {
	setupRetentionTestDB(t)
	server := &entityServer{}
	link := server.start(t, `"v1"`, []byte("full-audio")).URL + "/audio.mp3"
	target := path.Join(createAudioFolderIfNotExists("Orphan Podcast"), getFileName(link, "Orphan", ".mp3"))
	if err := os.WriteFile(target, []byte("someone-elses-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Download("", link, "Orphan", "Orphan Podcast", "")
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(got); string(raw) != "full-audio" {
		t.Fatalf("expected a fresh download, got %q", raw)
	}
	if server.ranges[0] != "" {
		t.Fatalf("must not resume a file without a matching sidecar; sent Range %q", server.ranges[0])
	}
}

func seedPartialDownload(t *testing.T, target string, partial []byte, sidecar downloadSidecar) {
	t.Helper()
	if err := os.WriteFile(target, partial, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeDownloadSidecar(target+downloadSidecarSuffix, sidecar); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadRestartsWhenTheRemoteFileChanged(t *testing.T) {
	setupRetentionTestDB(t)
	server := &entityServer{}
	link := server.start(t, `"v2"`, []byte("NEW-ENCODE-WITH-ADS")).URL + "/audio.mp3"
	target := path.Join(createAudioFolderIfNotExists("Changed Podcast"), getFileName(link, "Changed", ".mp3"))
	seedPartialDownload(t, target, []byte("OLD-EN"), downloadSidecar{URL: link, ETag: `"v1"`})

	got, err := Download("", link, "Changed", "Changed Podcast", "")
	if err != nil {
		t.Fatal(err)
	}
	if server.ifRange[0] != `"v1"` || server.ranges[0] != "bytes=6-" {
		t.Fatalf("expected a validated resume request, got Range %q If-Range %q", server.ranges[0], server.ifRange[0])
	}
	if raw, _ := os.ReadFile(got); string(raw) != "NEW-ENCODE-WITH-ADS" {
		t.Fatalf("a changed remote file must restart the download, got %q", raw)
	}
	if _, err := os.Stat(got + downloadSidecarSuffix); !os.IsNotExist(err) {
		t.Fatalf("sidecar must be removed after success: %v", err)
	}
}

func TestDownloadResumesTheSameRemoteFile(t *testing.T) {
	setupRetentionTestDB(t)
	server := &entityServer{}
	link := server.start(t, `"v1"`, []byte("0123456789")).URL + "/audio.mp3"
	target := path.Join(createAudioFolderIfNotExists("Same Podcast"), getFileName(link, "Same", ".mp3"))
	seedPartialDownload(t, target, []byte("01234"), downloadSidecar{URL: link, ETag: `"v1"`})

	got, err := Download("", link, "Same", "Same Podcast", "")
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(got); string(raw) != "0123456789" {
		t.Fatalf("resume produced %q", raw)
	}
	if len(server.ranges) != 1 || server.ranges[0] != "bytes=5-" {
		t.Fatalf("expected one ranged request, got %v", server.ranges)
	}
}

func TestDownloadDoesNotResumeAnotherEpisodesPartial(t *testing.T) {
	setupRetentionTestDB(t)
	podcast := createPodcast(t, "Partial Owner Podcast", false)
	server := &entityServer{}
	link := server.start(t, `"v1"`, []byte("mine")).URL + "/audio.mp3"
	shared := path.Join(createAudioFolderIfNotExists(podcast.Title), getFileName(link, "Bonus", ".mp3"))
	seedPartialDownload(t, shared, []byte("theirs-partial"), downloadSidecar{PodcastItemID: "another-episode", URL: link, ETag: `"v1"`})
	item := db.PodcastItem{PodcastID: podcast.ID, Title: "Bonus", GUID: "mine"}
	if err := db.CreatePodcastItem(&item); err != nil {
		t.Fatal(err)
	}

	got, err := Download(item.ID, link, "Bonus", podcast.Title, "")
	if err != nil {
		t.Fatal(err)
	}
	if got == shared {
		t.Fatal("must not take over another episode's partial download")
	}
	if raw, _ := os.ReadFile(shared); string(raw) != "theirs-partial" {
		t.Fatalf("the other partial was changed: %q", raw)
	}
}

func TestConcurrentDownloadsOfOneTitleGetDifferentPaths(t *testing.T) {
	setupRetentionTestDB(t)
	folder := createAudioFolderIfNotExists("Concurrent Podcast")
	firstPath, releaseFirst := resolveEpisodeDownloadPath("episode-one", "episode-one", folder, "same-title.mp3")
	defer releaseFirst()
	secondPath, releaseSecond := resolveEpisodeDownloadPath("episode-two", "episode-two", folder, "same-title.mp3")
	defer releaseSecond()
	if firstPath == secondPath {
		t.Fatalf("two running downloads were given one path: %q", firstPath)
	}
}

func TestDownloadSingleEpisodeRefusesASecondConcurrentDownload(t *testing.T) {
	setupRetentionTestDB(t)
	podcast := createPodcast(t, "Guard Podcast", false)
	item := db.PodcastItem{PodcastID: podcast.ID, Title: "Guarded", GUID: "g", FileURL: "http://127.0.0.1:1/a.mp3", DownloadStatus: db.Downloading}
	if err := db.CreatePodcastItem(&item); err != nil {
		t.Fatal(err)
	}
	release, ok := beginEpisodeDownload(item.ID)
	if !ok {
		t.Fatal("first begin must succeed")
	}
	defer release()

	if err := DownloadSingleEpisode(item.ID); !errors.Is(err, ErrDownloadInProgress) {
		t.Fatalf("expected ErrDownloadInProgress, got %v", err)
	}
	var got db.PodcastItem
	if err := db.GetPodcastItemByID(item.ID, &got); err != nil {
		t.Fatal(err)
	}
	if got.DownloadStatus != db.Downloading {
		t.Fatalf("the running download's state was changed to %v", got.DownloadStatus)
	}
	release()
	again, ok := beginEpisodeDownload(item.ID)
	if !ok {
		t.Fatal("the guard must be free after release")
	}
	again()
}
