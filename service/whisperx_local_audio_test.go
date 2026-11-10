package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ctaylor1/briefcast/db"
)

// A stat error other than "does not exist" (here ENOTDIR, because a path
// component is a regular file) must not reset the episode for re-download.
func TestLocalTranscriptionStatErrorSchedulesRetryInsteadOfReset(t *testing.T) {
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, dir string) {
		blocker := filepath.Join(dir, "not-a-directory")
		if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		item.DownloadPath = filepath.Join(blocker, "episode.mp3")
		item.TranscriptJSON, item.TranscriptCheckpointJSON = "", ""
		item.TranscriptStatus = "pending_whisperx"
		item.TranscriptNextAttempt = nil
		item.TranscriptRetryCount = 0
	})
	configureCharacterizationWhisperX(t, "import sys\nsys.exit(0)\n")
	if err := TranscribePendingEpisodes(); err != nil {
		t.Fatalf("TranscribePendingEpisodes: %v", err)
	}
	got := loadCharacterizationPodcastItem(t, item.ID)
	if got.DownloadStatus != db.Downloaded || got.DownloadPath != item.DownloadPath {
		t.Fatalf("unknown stat error must keep download state: %+v", got)
	}
	if got.TranscriptStatus != "failed" || got.TranscriptRetryCount != 1 || got.TranscriptNextAttempt == nil {
		t.Fatalf("unknown stat error must schedule a retry: %+v", got)
	}
}

func TestLocalAudioMissingClassifiesStatErrors(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "present.mp3")
	if err := os.WriteFile(present, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if missing, err := localAudioMissing(present); missing || err != nil {
		t.Fatalf("present file: missing=%v err=%v", missing, err)
	}
	if missing, err := localAudioMissing(filepath.Join(dir, "absent.mp3")); !missing || err != nil {
		t.Fatalf("absent file: missing=%v err=%v", missing, err)
	}
	if missing, err := localAudioMissing(filepath.Join(present, "child.mp3")); missing || err == nil {
		t.Fatalf("ENOTDIR must be unknown, not missing: missing=%v err=%v", missing, err)
	}
}
