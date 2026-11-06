package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/ctaylor1/briefcast/db"
	"gorm.io/gorm"
)

var (
	episodeUpdateAssignments = regexp.MustCompile("(?s)^UPDATE `podcast_items` SET (.*) WHERE ")
	assignedColumnName       = regexp.MustCompile("`(\\w+)`=")
)

// recordEpisodeUpdateColumns collects the columns assigned by each UPDATE of
// podcast_items, sorted so they compare against a literal list.
func recordEpisodeUpdateColumns(t *testing.T) *[][]string {
	t.Helper()
	var updates [][]string
	record := func(tx *gorm.DB) {
		match := episodeUpdateAssignments.FindStringSubmatch(tx.Statement.SQL.String())
		if match == nil {
			return
		}
		var columns []string
		for _, assignment := range assignedColumnName.FindAllStringSubmatch(match[1], -1) {
			columns = append(columns, assignment[1])
		}
		slices.Sort(columns)
		updates = append(updates, columns)
	}
	if err := db.DB.Callback().Update().After("gorm:update").Register("test:record_episode_columns", record); err != nil {
		t.Fatalf("register update callback: %v", err)
	}
	return &updates
}

func TestDownloadStateSettersWriteOnlyTheirColumns(t *testing.T) {
	cases := []struct {
		name string
		run  func(id string, audioPath string) error
		want []string
	}{
		{
			name: "queued",
			run:  func(id, _ string) error { return SetPodcastItemAsQueuedForDownload(id) },
			want: []string{"download_status", "download_total_bytes", "downloaded_bytes", "updated_at"},
		},
		{
			name: "queued preserving progress",
			run:  func(id, _ string) error { return SetPodcastItemAsQueuedPreserveProgress(id) },
			want: []string{"download_status", "updated_at"},
		},
		{
			name: "downloading",
			run:  func(id, _ string) error { return SetPodcastItemAsDownloading(id) },
			want: []string{"download_status", "updated_at"},
		},
		{
			name: "paused",
			run:  func(id, _ string) error { return SetPodcastItemAsPaused(id) },
			want: []string{"download_status", "updated_at"},
		},
		{
			name: "not downloaded",
			run:  func(id, _ string) error { return SetPodcastItemAsNotDownloaded(id, db.Deleted) },
			want: []string{"download_date", "download_path", "download_status", "download_total_bytes", "downloaded_bytes", "updated_at"},
		},
		{
			name: "played",
			run:  func(id, _ string) error { return SetPodcastItemPlayedStatus(id, true) },
			want: []string{"is_played", "updated_at"},
		},
		{
			name: "bookmarked",
			run:  func(id, _ string) error { return SetPodcastItemBookmarkStatus(id, true) },
			want: []string{"bookmark_date", "is_summary_favorited", "updated_at"},
		},
		{
			name: "downloaded with a finished transcript",
			run:  func(id, audioPath string) error { return SetPodcastItemAsDownloaded(id, audioPath) },
			want: []string{
				"download_date", "download_path", "download_status", "download_total_bytes", "downloaded_bytes",
				"file_size", "transcript_checkpoint_json", "transcript_progress_pct", "transcript_progress_stage",
				"updated_at",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tempDir, item := seedCharacterizationPodcastItem(t, nil)
			audioPath := filepath.Join(tempDir, "episode.mp3")
			if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
				t.Fatalf("write audio: %v", err)
			}
			updates := recordEpisodeUpdateColumns(t)

			if err := tc.run(item.ID, audioPath); err != nil {
				t.Fatalf("run setter: %v", err)
			}

			if len(*updates) != 1 {
				t.Fatalf("expected one UPDATE, got %d", len(*updates))
			}
			if got := (*updates)[0]; !slices.Equal(got, tc.want) {
				t.Fatalf("expected columns %v, got %v", tc.want, got)
			}
		})
	}
}

func TestDownloadStateSettersReturnNotFoundForUnknownEpisode(t *testing.T) {
	setupRetentionTestDB(t)
	setters := map[string]func(string) error{
		"queued":                     SetPodcastItemAsQueuedForDownload,
		"queued preserving progress": SetPodcastItemAsQueuedPreserveProgress,
		"downloading":                SetPodcastItemAsDownloading,
		"paused":                     SetPodcastItemAsPaused,
		"not downloaded":             func(id string) error { return SetPodcastItemAsNotDownloaded(id, db.Deleted) },
		"played":                     func(id string) error { return SetPodcastItemPlayedStatus(id, true) },
		"bookmarked":                 func(id string) error { return SetPodcastItemBookmarkStatus(id, true) },
		"downloaded":                 func(id string) error { return SetPodcastItemAsDownloaded(id, "missing.mp3") },
	}
	for name, setter := range setters {
		t.Run(name, func(t *testing.T) {
			if err := setter("missing-episode"); !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("expected record not found, got %v", err)
			}
		})
	}
}

// serveEpisodeImage calls during once for each image request, while
// downloadImageLocally is holding the row it read before the download started.
func serveEpisodeImage(t *testing.T, during func(id string)) (*httptest.Server, *atomic.Value) {
	t.Helper()
	var episodeID atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		during(episodeID.Load().(string))
		_, _ = w.Write([]byte("image"))
	}))
	t.Cleanup(server.Close)
	return server, &episodeID
}

func TestDownloadImageLocallyKeepsChangesMadeDuringTheDownload(t *testing.T) {
	server, episodeID := serveEpisodeImage(t, func(id string) {
		if err := SetPodcastItemPlayedStatus(id, true); err != nil {
			t.Errorf("mark played during download: %v", err)
		}
	})
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, _ string) {
		item.Image = server.URL + "/cover.jpg"
		item.LocalImage = ""
	})
	episodeID.Store(item.ID)

	if err := downloadImageLocally(item.ID); err != nil {
		t.Fatalf("download image: %v", err)
	}

	got := loadCharacterizationPodcastItem(t, item.ID)
	if !got.IsPlayed {
		t.Fatal("expected is_played set during the download to survive the image save")
	}
	if got.LocalImage == "" {
		t.Fatal("expected local_image to be saved")
	}
}

func TestDownloadImageLocallyDoesNotRecreateAnEpisodeDeletedDuringTheDownload(t *testing.T) {
	server, episodeID := serveEpisodeImage(t, func(id string) {
		if err := db.DeletePodcastItemByID(id); err != nil {
			t.Errorf("delete during download: %v", err)
		}
	})
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, _ string) {
		item.Image = server.URL + "/cover.jpg"
		item.LocalImage = ""
	})
	episodeID.Store(item.ID)

	if err := downloadImageLocally(item.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected record not found, got %v", err)
	}

	var deleted db.PodcastItem
	if err := db.GetPodcastItemByID(item.ID, &deleted); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected the deleted episode to stay deleted, lookup err=%v", err)
	}
}

func TestDownloadWithFallbackKeepsChangesMadeAfterTheSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/alternate.mp3" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("audio"))
	}))
	t.Cleanup(server.Close)
	alternateURL := server.URL + "/alternate.mp3"
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, _ string) {
		item.FileURL = server.URL + "/primary.mp3"
		item.AlternateFileURLs = `["` + alternateURL + `"]`
	})

	snapshot := loadCharacterizationPodcastItem(t, item.ID)
	if err := SetPodcastItemPlayedStatus(item.ID, true); err != nil {
		t.Fatalf("mark played: %v", err)
	}

	filePath, err := DownloadWithFallback(item.ID, &snapshot, "")
	if err != nil {
		t.Fatalf("download with fallback: %v", err)
	}
	if filePath == "" {
		t.Fatal("expected a downloaded file path")
	}

	got := loadCharacterizationPodcastItem(t, item.ID)
	if !got.IsPlayed {
		t.Fatal("expected is_played to survive saving the alternate URL")
	}
	if got.FileURL != alternateURL || snapshot.FileURL != alternateURL {
		t.Fatalf("expected working URL %q saved and on the snapshot, got stored %q, snapshot %q", alternateURL, got.FileURL, snapshot.FileURL)
	}
}
