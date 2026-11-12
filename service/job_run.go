package service

import (
	"fmt"
	"time"

	"github.com/ctaylor1/briefcast/internal/logging"
	"go.uber.org/zap"
)

const (
	refreshJobName   = "RefreshEpisodes"
	retentionJobName = "RetentionCleanup"
)

// RunJob runs a background job under a new job logger and logs its start and
// outcome once: job_started, then job_completed or job_failed with duration_ms
// (docs/MONITORING.md). The job logs its details to the logger it is given, so
// every line of a run carries the same job_id. The scheduler (main.go) and the
// code that starts jobs directly both run jobs through it; before, a scheduled
// run was logged twice, under two job IDs.
func RunJob(name string, job func(jobLogger *zap.SugaredLogger) error) error {
	jobLogger, _ := logging.NewJobSugar(name)
	return runJobWith(jobLogger, job)
}

func runJobWith(jobLogger *zap.SugaredLogger, job func(*zap.SugaredLogger) error) error {
	start := time.Now()
	jobLogger.Infow("job_started")
	defer func() {
		// Log the outcome, then let the scheduler's recovery handle the panic
		// as before.
		if recovered := recover(); recovered != nil {
			jobLogger.Errorw("job_failed", "duration_ms", time.Since(start).Milliseconds(), "panic", fmt.Sprint(recovered))
			panic(recovered)
		}
	}()
	if err := job(jobLogger); err != nil {
		jobLogger.Errorw("job_failed", "duration_ms", time.Since(start).Milliseconds(), "error", err)
		return err
	}
	jobLogger.Infow("job_completed", "duration_ms", time.Since(start).Milliseconds())
	return nil
}
