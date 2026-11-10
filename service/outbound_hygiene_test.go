package service

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ctaylor1/briefcast/db"
)

func TestImageDownloadRejectsErrorResponses(t *testing.T) {
	setupRetentionTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "<html>not found</html>", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	link := server.URL + "/cover.jpg"

	if _, err := DownloadImage(link, "episode-1", "Image Errors"); err == nil {
		t.Fatal("expected a 404 to fail")
	}
	saved := path.Join(createImagesFolderIfNotExists("Image Errors"), getFileName(link, "episode-1", ".jpg"))
	if _, err := os.Stat(saved); !os.IsNotExist(err) {
		t.Fatalf("an error page was saved as an image: %v", err)
	}
}

func TestImageDownloadCapsSize(t *testing.T) {
	setupRetentionTestDB(t)
	previous := maxImageDownloadBytes
	t.Cleanup(func() { maxImageDownloadBytes = previous })
	maxImageDownloadBytes = 16

	for _, declared := range []bool{true, false} {
		declared := declared
		t.Run(fmt.Sprintf("declared=%v", declared), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !declared {
					w.Header().Set("Transfer-Encoding", "chunked")
					w.(http.Flusher).Flush()
				}
				_, _ = w.Write([]byte(strings.Repeat("x", 64)))
			}))
			t.Cleanup(server.Close)
			link := server.URL + "/big.jpg"
			if _, err := DownloadPodcastCoverImage(link, "Big Images"); err == nil {
				t.Fatal("expected an oversized image to fail")
			}
			folder := createImagesFolderIfNotExists("Big Images")
			leftovers, _ := filepath.Glob(filepath.Join(folder, "*"))
			if len(leftovers) != 0 {
				t.Fatalf("oversized image left files: %v", leftovers)
			}
		})
	}
}

func TestImageDownloadSkipsRequestWhenFileExists(t *testing.T) {
	setupRetentionTestDB(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("image"))
	}))
	t.Cleanup(server.Close)
	link := server.URL + "/cover.jpg"

	first, err := DownloadImage(link, "episode-2", "Cached Images")
	if err != nil {
		t.Fatal(err)
	}
	second, err := DownloadImage(link, "episode-2", "Cached Images")
	if err != nil || second != first {
		t.Fatalf("second call: %q %v", second, err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("an existing image was fetched again: %d requests", got)
	}
}

func TestOutboundRedirectsStopAfterTen(t *testing.T) {
	setupRetentionTestDB(t)
	var hits atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, server.URL+"/loop", http.StatusFound)
	}))
	t.Cleanup(server.Close)

	req, err := getRequest(server.URL + "/start")
	if err != nil {
		t.Fatal(err)
	}
	_, err = httpClient().Do(req)
	if !errors.Is(err, errTooManyRedirects) {
		t.Fatalf("expected the redirect cap, got %v", err)
	}
	if got := hits.Load(); got != maxOutboundRedirects {
		t.Fatalf("expected %d requests, got %d", maxOutboundRedirects, got)
	}
}

func TestOutboundTransportIsSharedPerPurpose(t *testing.T) {
	if outboundHTTPTransport(outboundPurposeHTTP) != outboundHTTPTransport(outboundPurposeHTTP) {
		t.Fatal("each call built a new transport")
	}
	if outboundHTTPTransport(outboundPurposeHTTP) == outboundHTTPTransport(outboundPurposeLLM) {
		t.Fatal("purposes must not share a transport: their URL policies differ")
	}
}

func TestHelperEnvironmentKeepsSecretsOut(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:secret@db/briefcast")
	t.Setenv("PASSWORD", "secret")
	t.Setenv("LLM_API_KEY", "sk-secret")
	t.Setenv("PODCASTINDEX_SECRET", "secret")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_OUTPUT", "stdout,file")
	t.Setenv("PATH", "/usr/local/bin:/usr/bin")

	env := helperCommandEnv()
	joined := "\n" + strings.Join(env, "\n") + "\n"
	for _, secret := range []string{"DATABASE_URL=", "PASSWORD=", "LLM_API_KEY=", "PODCASTINDEX_SECRET="} {
		if strings.Contains(joined, "\n"+secret) {
			t.Fatalf("helper environment leaks %s", secret)
		}
	}
	for _, kept := range []string{"\nPATH=/usr/local/bin:/usr/bin\n", "\nLOG_LEVEL=debug\n"} {
		if !strings.Contains(joined, kept) {
			t.Fatalf("helper environment lost %q", strings.TrimSpace(kept))
		}
	}
	if strings.Count(joined, "\nLOG_OUTPUT=") != 1 {
		t.Fatalf("expected exactly one sanitized LOG_OUTPUT, got:%s", joined)
	}
}

// The local transcription child parses untrusted audio; it must not see secrets.
func TestLocalTranscriptionChildDoesNotSeeSecrets(t *testing.T) {
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, dir string) {
		item.DownloadPath = filepath.Join(dir, "episode.mp3")
		if err := os.WriteFile(item.DownloadPath, []byte(strings.Repeat("a", 2048)), 0o644); err != nil {
			t.Fatal(err)
		}
		item.TranscriptJSON, item.CanonicalTranscript, item.TranscriptCheckpointJSON = "", "", ""
		item.TranscriptStatus = "pending_whisperx"
		item.TranscriptNextAttempt = nil
	})
	t.Setenv("BRIEFCAST_SENTINEL_SECRET", "must-not-leak")
	t.Setenv("PASSWORD", "must-not-leak")
	configureCharacterizationWhisperX(t, `import json, os, sys
leaked = [k for k in ("BRIEFCAST_SENTINEL_SECRET", "PASSWORD", "DATABASE_URL") if k in os.environ]
if leaked:
    print("leaked: " + ",".join(leaked), file=sys.stderr)
    sys.exit(3)
print(json.dumps({"segments": [{"text": "clean"}]}))
`)
	if err := TranscribePendingEpisodes(); err != nil {
		t.Fatalf("child saw secrets or failed: %v", err)
	}
	if got := loadCharacterizationPodcastItem(t, item.ID); got.TranscriptStatus != "available" {
		t.Fatalf("transcription did not complete: %q %q", got.TranscriptStatus, got.TranscriptLastError)
	}
}
