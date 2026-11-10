package service

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/internal/transcription"
	"github.com/ctaylor1/briefcast/internal/whisperxworker"
)

// A blocked episode whose audio or engine profile changed can only be repaired
// by a new submission. A manual retry must start one instead of reusing the
// pinned envelope, which the worker or the local audio check rejects again.
func TestRemoteManualRetryResetsEnvelopeWhenAudioOrProfileChanged(t *testing.T) {
	for _, change := range []string{"audio-after-success", "audio-after-terminal", "profile"} {
		t.Run(change, func(t *testing.T) {
			f := newRemoteTestFixture(t)
			replaced := []byte(strings.Repeat("c", 2048))
			f.runner.run = func(ctx context.Context, s whisperxworker.RunSpec, p func(transcription.ProgressUpdate) error) error {
				if f.runner.calls.Load() != 1 {
					return nil
				}
				if change == "audio-after-success" {
					// The job succeeds, but the app rejects its result because the
					// audio changed underneath it (the envelope stays non-terminal).
					return os.WriteFile(f.item.DownloadPath, replaced, 0600)
				}
				return transcription.Error("MODEL_UNAVAILABLE", "private diagnostic", false)
			}
			if err := f.run(t); err == nil {
				t.Fatal("first attempt should block")
			}
			blocked := loadCharacterizationPodcastItem(t, f.item.ID)
			old, err := decodeRemoteEnvelope(blocked.TranscriptCheckpointJSON)
			if err != nil {
				t.Fatal(err)
			}
			if blocked.TranscriptStatus != "failed" || blocked.TranscriptProgressStage != "blocked" || !old.Permanent {
				t.Fatalf("first attempt not blocked: status=%s stage=%s permanent=%v", blocked.TranscriptStatus, blocked.TranscriptProgressStage, old.Permanent)
			}
			switch change {
			case "audio-after-terminal":
				if err = os.WriteFile(f.item.DownloadPath, replaced, 0600); err != nil {
					t.Fatal(err)
				}
			case "profile":
				// The operator deployed a worker with a new immutable profile.
				next, err := whisperxworker.Open(context.Background(), whisperxworker.Config{StateDir: filepath.Join(t.TempDir(), "state"), MediaRoot: os.Getenv("DATA"), TokenFile: f.cfg.ServiceTokenFile, EngineProfile: "stage-e-next", WorkerVersion: "test", Device: "cpu", ComputeType: "int8", BatchSize: 16, ExecutionTimeoutSeconds: 10, MaxRunning: 1, MaxWaiting: 4, ResultLimit: transcription.DefaultResultLimit, Models: []string{f.cfg.Model}, DiarizationModels: []string{f.cfg.DiarizationModel}, PackageVersions: map[string]string{"whisperx": "3.8.6", "torch": "2.8.0+cu128"}}, f.runner)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					_ = next.Close(ctx)
				})
				handler := next.Handler()
				f.intercept = func(rw http.ResponseWriter, r *http.Request) bool {
					handler.ServeHTTP(rw, r)
					return true
				}
				f.cfg.EngineProfile = "stage-e-next"
			}
			count, err := ForceFailedTranscriptRetriesDue(time.Now().UTC())
			if err != nil || count != 1 {
				t.Fatalf("manual repair: %v, %d", err, count)
			}
			if err = f.run(t); err != nil {
				t.Fatalf("manual retry re-blocked: %v", err)
			}
			got := loadCharacterizationPodcastItem(t, f.item.ID)
			if got.TranscriptStatus != "available" || got.TranscriptJSON == "" {
				t.Fatalf("manual retry did not transcribe: status=%s stage=%s error=%s", got.TranscriptStatus, got.TranscriptProgressStage, got.TranscriptLastError)
			}
			if f.runner.calls.Load() != 2 {
				t.Fatalf("expected exactly one new execution, got %d runs", f.runner.calls.Load())
			}
			if change != "profile" && transcription.Digest(replaced) == old.InputSHA256 {
				t.Fatal("fixture did not change the audio")
			}
		})
	}
}

// Without a manual retry the strict behaviour stays: changed audio blocks.
func TestRemoteAutomaticRetryStillRejectsChangedAudio(t *testing.T) {
	f := newRemoteTestFixture(t)
	f.cfg.RetryFailed = true
	f.runner.run = func(ctx context.Context, s whisperxworker.RunSpec, p func(transcription.ProgressUpdate) error) error {
		if f.runner.calls.Load() == 1 {
			return transcription.Error("MODEL_UNAVAILABLE", "private diagnostic", true)
		}
		return nil
	}
	if err := f.run(t); err == nil {
		t.Fatal("first attempt should fail")
	}
	if err := os.WriteFile(f.item.DownloadPath, []byte(strings.Repeat("d", 2048)), 0600); err != nil {
		t.Fatal(err)
	}
	f.due(t)
	if err := f.run(t); err == nil {
		t.Fatal("automatic retry accepted changed audio")
	}
	got := loadCharacterizationPodcastItem(t, f.item.ID)
	if got.TranscriptProgressStage != "blocked" || f.runner.calls.Load() != 1 {
		t.Fatalf("automatic retry: stage=%s runs=%d", got.TranscriptProgressStage, f.runner.calls.Load())
	}
}
