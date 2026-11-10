package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPodcastIndexSearchGivesUpOnStalledUpstream(t *testing.T) {
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		close(release)
		upstream.Close()
	})

	t.Setenv(outboundAllowPrivateEnv, "true")
	t.Setenv(PodcastIndexKeyEnv, "key")
	t.Setenv(PodcastIndexSecretEnv, "secret")
	previousURL, previousTimeout := podcastIndexBaseURL, podcastIndexSearchTimeout
	t.Cleanup(func() {
		podcastIndexBaseURL, podcastIndexSearchTimeout = previousURL, previousTimeout
	})
	podcastIndexBaseURL = upstream.URL + "/"
	podcastIndexSearchTimeout = 200 * time.Millisecond

	started := time.Now()
	results := PodcastIndexService{}.Query("anything")
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("search waited %v on a stalled upstream", elapsed)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results, got %d", len(results))
	}
}
