package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/transcription"
)

type stubTranscriptionBackend struct {
	preflight  func(context.Context) error
	transcribe func(context.Context, string, string, func(transcription.ProgressUpdate)) ([]byte, error)
}

func (b stubTranscriptionBackend) Preflight(ctx context.Context) error {
	return b.preflight(ctx)
}

func (b stubTranscriptionBackend) Transcribe(ctx context.Context, audioPath, checkpoint string, onProgress func(transcription.ProgressUpdate)) ([]byte, error) {
	return b.transcribe(ctx, audioPath, checkpoint, onProgress)
}

func TestTranscriptionBackendOrchestrationWithoutLocalRuntime(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "failure"
		}
		t.Run(name, func(t *testing.T) {
			_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, dir string) {
				item.DownloadPath = filepath.Join(dir, "backend.mp3")
				if err := os.WriteFile(item.DownloadPath, []byte(strings.Repeat("a", 2048)), 0o644); err != nil {
					t.Fatal(err)
				}
				item.TranscriptJSON, item.CanonicalTranscript = "", ""
				item.TranscriptStatus, item.TranscriptProgressStage = "processing", "transcribing"
				item.TranscriptCheckpointJSON = `{"completed_seconds":120,"total_seconds":240}`
				item.TranscriptProgressPct, item.TranscriptRetryCount = 50, 0
				item.TranscriptNextAttempt = nil
			})
			t.Setenv("LLM_ENABLED", "false")
			cfg := LoadWhisperXConfig()
			cfg.Enabled, cfg.Diarization = true, false
			cfg.MaxConcurrency, cfg.MaxItemsPerRun = 1, 1
			cfg.Python = filepath.Join(t.TempDir(), "missing-python")
			cfg.Script = filepath.Join(t.TempDir(), "missing-script.py")
			cfg.Model = "backend-test-model"
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			preflights, executions := 0, 0
			backendErr := errors.New("backend execution unavailable")
			checkpoint := `{"completed_seconds":180,"total_seconds":240}`
			output := []byte(`{"segments":[{"start":0,"end":1,"text":"backend transcript"}]}`)
			backend := stubTranscriptionBackend{
				preflight: func(gotCtx context.Context) error {
					preflights++
					if gotCtx != ctx {
						t.Error("preflight did not receive caller context")
					}
					return nil
				},
				transcribe: func(gotCtx context.Context, path, resume string, progress func(transcription.ProgressUpdate)) ([]byte, error) {
					executions++
					if gotCtx != ctx || path != item.DownloadPath || resume != item.TranscriptCheckpointJSON {
						t.Errorf("backend received wrong context/input/resume: path=%q resume=%q", path, resume)
					}
					progress(transcription.ProgressUpdate{Stage: "transcribing", Percent: 75, Checkpoint: json.RawMessage(checkpoint)})
					var got db.PodcastItem
					if err := db.GetPodcastItemByID(item.ID, &got); err != nil {
						return nil, err
					}
					if got.TranscriptStatus != "processing" || got.TranscriptProgressPct != 75 || got.TranscriptCheckpointJSON != checkpoint {
						t.Errorf("backend progress was not persisted: %+v", got)
					}
					if fail {
						return nil, backendErr
					}
					return output, nil
				},
			}
			err := transcribePendingEpisodes(ctx, cfg, backend)
			if preflights != 1 || executions != 1 {
				t.Fatalf("preflights=%d executions=%d", preflights, executions)
			}
			got := loadCharacterizationPodcastItem(t, item.ID)
			if fail {
				if !errors.Is(err, backendErr) || got.TranscriptStatus != "failed" || got.TranscriptRetryCount != 1 || got.TranscriptNextAttempt == nil || got.TranscriptCheckpointJSON != checkpoint {
					t.Fatalf("backend failure lost retry/checkpoint state: err=%v item=%+v", err, got)
				}
			} else {
				if err != nil || got.TranscriptJSON != string(output) || got.CanonicalTranscript != "backend transcript" || got.TranscriptStatus != "available" || got.TranscriptProgressPct != 100 || got.TranscriptCheckpointJSON != "" || got.TranscriptModel != cfg.Model {
					t.Fatalf("backend success was not committed: err=%v item=%+v", err, got)
				}
			}
			if _, err := os.Stat(item.DownloadPath); err != nil {
				t.Fatalf("backend processing removed valid audio: %v", err)
			}
		})
	}
}

func TestTranscriptionBackendPreflightFailureLeavesQueueUntouchedAndReleasesLock(t *testing.T) {
	_, item := seedCharacterizationPodcastItem(t, func(item *db.PodcastItem, _ string) {
		item.TranscriptStatus = "pending_whisperx"
		item.TranscriptJSON = ""
		item.TranscriptNextAttempt = nil
	})
	before := snapshotPodcastItemRow(t, item.ID)
	cfg := LoadWhisperXConfig()
	cfg.Enabled = true
	preflightErr := errors.New("backend preflight unavailable")
	backend := stubTranscriptionBackend{
		preflight: func(context.Context) error { return preflightErr },
		transcribe: func(context.Context, string, string, func(transcription.ProgressUpdate)) ([]byte, error) {
			t.Error("execution ran after failed preflight")
			return nil, nil
		},
	}
	if err := transcribePendingEpisodes(context.Background(), cfg, backend); !errors.Is(err, preflightErr) {
		t.Fatalf("preflight error not returned: %v", err)
	}
	assertPodcastItemChangedColumns(t, before, snapshotPodcastItemRow(t, item.ID))
	lock, acquired, err := db.TryLock("TranscribePendingEpisodes", cfg.LockDurationMins)
	if err != nil || !acquired {
		t.Fatalf("preflight failure leaked scheduler lock: acquired=%v err=%v", acquired, err)
	}
	defer db.UnlockByID(lock.ID)
}

func TestTranscriptionBackendDisabledSkipsPreflight(t *testing.T) {
	backend := stubTranscriptionBackend{
		preflight: func(context.Context) error {
			t.Error("disabled transcription ran preflight")
			return nil
		},
	}
	if err := transcribePendingEpisodes(context.Background(), WhisperXConfig{}, backend); err != nil {
		t.Fatal(err)
	}
}

func TestLocalTranscriptionBackendCancellationPreservesCheckpoint(t *testing.T) {
	python := requireWorkingPython(t)
	dir := t.TempDir()
	script := writeTempScript(t, dir, "cancelled.py", `raise RuntimeError("child should not run")`)
	backend := localTranscriptionBackend{cfg: WhisperXConfig{Python: python, Script: script}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := backend.Preflight(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("preflight did not honor cancellation: %v", err)
	}
	audio := filepath.Join(dir, "cancelled.mp3")
	sidecar := audio + ".briefcast.whisperx.resume.json"
	checkpoint := []byte(`{"segments":[{"text":"preserved"}]}`)
	if err := os.WriteFile(sidecar, checkpoint, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Transcribe(ctx, audio, "", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("execution did not honor cancellation: %v", err)
	}
	got, err := os.ReadFile(sidecar)
	if err != nil || string(got) != string(checkpoint) {
		t.Fatalf("cancellation changed checkpoint: %q err=%v", got, err)
	}
}
