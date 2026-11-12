package service

import (
	"context"
	"errors"
	"testing"

	"github.com/ctaylor1/briefcast/internal/transcription"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func observedJobLogger() (*zap.SugaredLogger, *observer.ObservedLogs) {
	core, logs := observer.New(zap.DebugLevel)
	return zap.New(core).Sugar(), logs
}

func jobMessageCounts(logs *observer.ObservedLogs) map[string]int {
	counts := map[string]int{}
	for _, entry := range logs.All() {
		counts[entry.Message]++
	}
	return counts
}

// A scheduled run used to be logged twice, under two job IDs: by the scheduler
// and by the job itself, so one failure showed as job_completed_with_errors and
// job_failed. RunJob logs one start and one outcome.
func TestRunJobLogsOneStartAndOneOutcome(t *testing.T) {
	failure := errors.New("feed unreachable")
	for name, tc := range map[string]struct {
		job     func(*zap.SugaredLogger) error
		outcome string
	}{
		"success": {func(*zap.SugaredLogger) error { return nil }, "job_completed"},
		"failure": {func(jobLogger *zap.SugaredLogger) error {
			jobLogger.Errorw("failed to refresh podcast feed", "error", failure)
			return failure
		}, "job_failed"},
	} {
		t.Run(name, func(t *testing.T) {
			logger, logs := observedJobLogger()
			err := runJobWith(logger, tc.job)
			if (tc.outcome == "job_failed") != errors.Is(err, failure) {
				t.Fatalf("unexpected error %v", err)
			}
			counts := jobMessageCounts(logs)
			if counts["job_started"] != 1 || counts[tc.outcome] != 1 || counts["job_completed"]+counts["job_failed"] != 1 {
				t.Fatalf("expected one job_started and one %s, got %v", tc.outcome, counts)
			}
		})
	}
}

func TestRunJobLogsAPanicThenLetsTheSchedulerRecover(t *testing.T) {
	logger, logs := observedJobLogger()
	defer func() {
		if recover() == nil {
			t.Fatal("expected the panic to reach the scheduler's recovery")
		}
		if counts := jobMessageCounts(logs); counts["job_started"] != 1 || counts["job_failed"] != 1 {
			t.Fatalf("expected job_started and job_failed, got %v", counts)
		}
	}()
	_ = runJobWith(logger, func(*zap.SugaredLogger) error { panic("boom") })
}

// The scheduled job bodies leave the run's start and outcome to RunJob, so a
// scheduled run is logged once.
func TestScheduledJobBodiesLeaveStartAndOutcomeToRunJob(t *testing.T) {
	setupRetentionTestDB(t)
	cfg := LoadWhisperXConfig()
	cfg.Enabled = true
	backend := stubTranscriptionBackend{
		preflight: func(context.Context) error { return nil },
		transcribe: func(context.Context, string, string, func(transcription.ProgressUpdate)) ([]byte, error) {
			return nil, errors.New("no episode is queued")
		},
	}
	for name, body := range map[string]func(*zap.SugaredLogger) error{
		"refresh":   RefreshEpisodesJob,
		"retention": ApplyRetentionPoliciesJob,
		"transcribe": func(jobLogger *zap.SugaredLogger) error {
			return transcribePendingEpisodesWith(context.Background(), cfg, backend, jobLogger)
		},
	} {
		t.Run(name, func(t *testing.T) {
			logger, logs := observedJobLogger()
			_ = runJobWith(logger, body)
			counts := jobMessageCounts(logs)
			if counts["job_started"] != 1 || counts["job_completed"]+counts["job_failed"] != 1 {
				t.Fatalf("expected one start and one outcome, got %v", counts)
			}
			for _, old := range []string{"job_finished", "job_completed_successfully", "job_completed_with_errors"} {
				if counts[old] != 0 {
					t.Fatalf("the job body still logs %s: %v", old, counts)
				}
			}
		})
	}
}
