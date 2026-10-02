// Package whisperxworker implements the private execution journal. It never
// imports the application service, database models or configuration package.
package whisperxworker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/ctaylor1/briefcast/internal/transcription"
)

type Config struct {
	StateDir, MediaRoot, TokenFile                             string
	EngineProfile, WorkerVersion, Device, ComputeType          string
	BatchSize, ExecutionTimeoutSeconds, MaxRunning, MaxWaiting int
	ResultLimit                                                int64
	MinFreeBytes                                               uint64
	Models, DiarizationModels                                  []string
	PackageVersions                                            map[string]string
	PipelineSHA256                                             string
}
type RunSpec struct {
	JobID, Directory, AudioPath string
	Request                     transcription.JobRequest
	Effective                   transcription.EffectiveConfig
	Resume                      json.RawMessage
	ResultLimit                 int64
	OwnerLock                   *os.File
}
type Runner interface {
	Ready(context.Context) error
	Run(context.Context, RunSpec, func(transcription.ProgressUpdate) error) error
}
type Worker struct {
	cfg         Config
	store       *store
	media       *os.Root
	runner      Runner
	mu          sync.Mutex
	active      map[string]context.CancelFunc
	fault       error
	stopping    bool
	wake        chan struct{}
	done        chan struct{}
	wg          sync.WaitGroup
	tokenDigest [32]byte
}

func Open(ctx context.Context, cfg Config, runner Runner) (_ *Worker, err error) {
	if cfg.StateDir == "" || cfg.MediaRoot == "" || cfg.TokenFile == "" || cfg.EngineProfile == "" || cfg.WorkerVersion == "" || (cfg.Device != "cpu" && cfg.Device != "cuda") || cfg.ComputeType == "" || cfg.BatchSize < 1 || cfg.ExecutionTimeoutSeconds < 1 || cfg.MaxRunning < 1 || cfg.MaxRunning > 32 || cfg.MaxWaiting < 0 || cfg.MaxWaiting > 1000 || cfg.ResultLimit < 1024 || len(cfg.Models) == 0 || len(cfg.DiarizationModels) == 0 || runner == nil {
		return nil, errors.New("invalid worker configuration")
	}
	token, err := readToken(cfg.TokenFile)
	if err != nil {
		return nil, err
	}
	w := &Worker{cfg: cfg, runner: runner, active: map[string]context.CancelFunc{}, wake: make(chan struct{}, 1), done: make(chan struct{}), tokenDigest: token}
	w.store, err = openStore(cfg.StateDir)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			w.store.close()
			if w.media != nil {
				w.media.Close()
			}
		}
	}()
	w.media, err = os.OpenRoot(cfg.MediaRoot)
	if err != nil {
		return nil, err
	}
	if err = runner.Ready(ctx); err != nil {
		return nil, err
	}
	fingerprint, hashErr := transcription.MarshalCanonical(struct {
		Packages                          map[string]string
		Pipeline, Script, Device, Compute string
		Batch                             int
	}{cfg.PackageVersions, transcription.PipelineVersion, cfg.PipelineSHA256, cfg.Device, cfg.ComputeType, cfg.BatchSize})
	if hashErr != nil {
		return nil, hashErr
	}
	key := "engine:" + cfg.EngineProfile
	if _, err = w.store.db.Exec("INSERT OR IGNORE INTO metadata(key,value) VALUES (?,?)", key, transcription.Digest(fingerprint)); err != nil {
		return nil, err
	}
	var storedFingerprint string
	if err = w.store.db.QueryRow("SELECT value FROM metadata WHERE key=?", key).Scan(&storedFingerprint); err != nil {
		return nil, err
	}
	if storedFingerprint != transcription.Digest(fingerprint) {
		return nil, errors.New("engine profile was reused with a different runtime; select a new immutable profile")
	}
	if err = w.recover(ctx); err != nil {
		return nil, err
	}
	w.wg.Add(1)
	go w.dispatch()
	w.notify()
	return w, nil
}
func (w *Worker) notify() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}
func (w *Worker) unavailable(err error) {
	w.fault = err
	for _, cancel := range w.active {
		cancel()
	}
	slog.Error("worker_state_unavailable")
}
func terminal(state string) bool {
	return state == "succeeded" || state == "failed" || state == "cancelled"
}
func touch(r *record) { r.Status.Revision++; r.Status.UpdatedAt = time.Now().UTC() }
func finish(r *record, state string, err *transcription.WorkerError) {
	touch(r)
	r.Status.State = state
	r.Status.Stage = state
	r.Status.Error = err
	now := time.Now().UTC()
	r.Status.FinishedAt = &now
}
func publicError(err error) *transcription.WorkerError {
	var typed *transcription.WorkerError
	if errors.As(err, &typed) {
		return typed
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return transcription.Error("TIMEOUT", "Execution budget exhausted", true)
	}
	if errors.Is(err, context.Canceled) {
		return transcription.Error("CANCELLED", "Job was cancelled", true)
	}
	return transcription.Error("INTERNAL", "Execution failed; inspect private worker diagnostics", true)
}

func (w *Worker) healthLocked() error {
	if w.fault != nil {
		return w.fault
	}
	if w.stopping {
		return errors.New("worker stopping")
	}
	if _, err := w.store.db.Exec("SELECT 1"); err != nil {
		return err
	}
	if w.cfg.MinFreeBytes > 0 {
		available, err := freeBytes(w.store.dir)
		if err != nil {
			return err
		}
		if available < w.cfg.MinFreeBytes {
			return errors.New("insufficient worker state space")
		}
	}
	probe := filepath.Join(w.store.dir, ".readiness")
	if err := durableWrite(probe, []byte("ok")); err != nil {
		return err
	}
	if err := os.Remove(probe); err != nil {
		return err
	}
	if _, err := w.media.Stat("."); err != nil {
		return err
	}
	return nil
}
func (w *Worker) Status(id string) (transcription.JobStatus, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	r, err := w.store.get(id)
	if err != nil {
		return transcription.JobStatus{}, err
	}
	return r.Status, nil
}

func (w *Worker) Submit(ctx context.Context, id string, raw []byte) (transcription.JobStatus, bool, error) {
	if !transcription.ValidUUID(id) {
		return transcription.JobStatus{}, false, transcription.Error("INVALID_CONFIG", "Invalid job UUID", false)
	}
	request, canonical, err := transcription.DecodeRequest(raw)
	if err != nil {
		return transcription.JobStatus{}, false, transcription.Error("INVALID_CONFIG", "Malformed or unsupported request", false)
	}
	digest := transcription.Digest(canonical)
	w.mu.Lock()
	existing, getErr := w.store.get(id)
	w.mu.Unlock()
	if getErr == nil {
		if existing.Status.RequestDigest != digest {
			return existing.Status, false, errConflict
		}
		return existing.Status, false, nil
	}
	if !errors.Is(getErr, sql.ErrNoRows) {
		return transcription.JobStatus{}, false, getErr
	}
	if request.EngineProfile != w.cfg.EngineProfile || !slices.Contains(w.cfg.Models, request.Options.Model) || !slices.Contains(w.cfg.DiarizationModels, request.Options.DiarizationModel) {
		return transcription.JobStatus{}, false, transcription.Error("INVALID_CONFIG", "Unsupported engine profile or model", false)
	}
	if err = verifyMedia(ctx, w.media, request.Input); err != nil {
		return transcription.JobStatus{}, false, err
	}
	if err = w.runner.Ready(ctx); err != nil {
		return transcription.JobStatus{}, false, transcription.Error("MODEL_UNAVAILABLE", "Execution prerequisites are unavailable", true)
	}
	options, err := transcription.MarshalCanonical(request.Options)
	if err != nil {
		return transcription.JobStatus{}, false, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	existing, getErr = w.store.get(id)
	if getErr == nil {
		if existing.Status.RequestDigest != digest {
			return existing.Status, false, errConflict
		}
		return existing.Status, false, nil
	}
	if !errors.Is(getErr, sql.ErrNoRows) {
		return transcription.JobStatus{}, false, getErr
	}
	if err = w.healthLocked(); err != nil {
		return transcription.JobStatus{}, false, err
	}
	counts, err := w.store.counts()
	if err != nil {
		return transcription.JobStatus{}, false, err
	}
	if !counts.Accepting {
		return transcription.JobStatus{}, false, transcription.Error("WORKER_UNAVAILABLE", "Worker is draining", true)
	}
	if counts.Queued+counts.Running >= w.cfg.MaxRunning+w.cfg.MaxWaiting {
		return transcription.JobStatus{}, false, transcription.Error("BUSY", "Worker capacity exhausted", true)
	}
	now := time.Now().UTC()
	r := &record{Request: request, PipelineVersion: transcription.PipelineVersion, Status: transcription.JobStatus{ContractVersion: 1, JobID: id, RequestDigest: digest, WorkerInstanceID: w.store.instance, State: "queued", Revision: 1, Stage: "queued", Attempt: request.Attempt, Checkpoint: transcription.CheckpointSummary{Format: transcription.CheckpointFormat, Compatible: true, ResumeFromJobID: request.ResumeFromJobID}, EffectiveConfig: transcription.EffectiveConfig{EngineProfile: request.EngineProfile, OptionsSHA256: transcription.Digest(options), Device: w.cfg.Device, ComputeType: w.cfg.ComputeType, BatchSize: w.cfg.BatchSize, ExecutionTimeoutSeconds: w.cfg.ExecutionTimeoutSeconds}, CreatedAt: now, UpdatedAt: now}}
	if request.ResumeFromJobID != nil {
		old, err := w.store.get(*request.ResumeFromJobID)
		if err != nil {
			return r.Status, false, transcription.Error("CHECKPOINT_INCOMPATIBLE", "Resume source is unavailable", false)
		}
		if (old.Status.State != "failed" && old.Status.State != "cancelled") || old.Request.DeploymentID != request.DeploymentID || old.Request.EpisodeID != request.EpisodeID || old.Request.GenerationID != request.GenerationID || old.Request.Input.SHA256 != request.Input.SHA256 || old.Request.Input.SizeBytes != request.Input.SizeBytes || old.Status.EffectiveConfig.OptionsSHA256 != r.Status.EffectiveConfig.OptionsSHA256 || old.Status.EffectiveConfig.EngineProfile != request.EngineProfile || old.PipelineVersion != transcription.PipelineVersion || old.Checkpoint == nil {
			return r.Status, false, transcription.Error("CHECKPOINT_INCOMPATIBLE", "Resume source is incompatible", false)
		}
		if _, err = w.readCheckpoint(old); err != nil {
			return r.Status, false, transcription.Error("CHECKPOINT_INCOMPATIBLE", "Resume data failed verification", false)
		}
		ref := *old.Checkpoint
		r.Checkpoint = &ref
		r.Status.Checkpoint.CompletedSeconds = ref.CompletedSeconds
		r.Status.Checkpoint.SegmentCount = ref.SegmentCount
	}
	if err = os.MkdirAll(w.store.jobDir(id), 0700); err != nil {
		return r.Status, false, err
	}
	if err = syncDirectory(filepath.Join(w.store.dir, "jobs")); err != nil {
		return r.Status, false, err
	}
	if err = w.store.save(r, true); err != nil {
		w.unavailable(err)
		return r.Status, false, err
	}
	w.notify()
	slog.Info("worker_job_accepted", "job_id", id)
	return r.Status, true, nil
}

func (w *Worker) dispatch() {
	defer w.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-w.done:
			return
		case <-w.wake:
		case <-ticker.C:
		}
		w.mu.Lock()
		if w.stopping || w.fault != nil {
			w.mu.Unlock()
			continue
		}
		for len(w.active) < w.cfg.MaxRunning {
			list, err := w.store.list("WHERE state='queued'")
			if err != nil {
				w.unavailable(err)
				break
			}
			if len(list) == 0 {
				break
			}
			r := list[0]
			if r.Request.EngineProfile != w.cfg.EngineProfile || r.PipelineVersion != transcription.PipelineVersion {
				finish(r, "failed", transcription.Error("INVALID_CONFIG", "Accepted engine profile is unavailable", false))
				if err = w.store.save(r, false); err != nil {
					w.unavailable(err)
					break
				}
				continue
			}
			remaining := time.Duration(r.Status.EffectiveConfig.ExecutionTimeoutSeconds)*time.Second - r.BudgetUsed
			if remaining <= 0 {
				finish(r, "failed", transcription.Error("TIMEOUT", "Execution budget exhausted", true))
				if err = w.store.save(r, false); err != nil {
					w.unavailable(err)
					break
				}
				continue
			}
			if removeErr := os.Remove(filepath.Join(w.store.jobDir(r.Status.JobID), "stop")); removeErr != nil && !os.IsNotExist(removeErr) {
				w.unavailable(removeErr)
				break
			}
			now := time.Now().UTC()
			r.RunSince = &now
			if r.Status.StartedAt == nil {
				r.Status.StartedAt = &now
			}
			touch(r)
			r.Status.State = "running"
			r.Status.Stage = "starting"
			if err = w.store.save(r, false); err != nil {
				w.unavailable(err)
				break
			}
			ctx, cancel := context.WithTimeout(context.Background(), remaining)
			w.active[r.Status.JobID] = cancel
			w.wg.Add(1)
			go w.execute(ctx, r)
		}
		w.mu.Unlock()
	}
}

func (w *Worker) execute(ctx context.Context, r *record) {
	defer w.wg.Done()
	id := r.Status.JobID
	var resume json.RawMessage
	var runErr error
	if r.Checkpoint != nil {
		resume, runErr = w.readCheckpoint(r)
	}
	if runErr != nil {
		w.mu.Lock()
		fresh, err := w.store.get(id)
		if err == nil {
			fresh.Checkpoint = nil
			fresh.Status.Checkpoint.Compatible = false
			fresh.Status.Checkpoint.Reason = "checkpoint_unverified_restarted_from_zero"
			fresh.Status.Checkpoint.CompletedSeconds = 0
			fresh.Status.Checkpoint.SegmentCount = 0
			fresh.Status.Stage = "restarting_checkpoint_invalid"
			touch(fresh)
			err = w.store.save(fresh, false)
		}
		if err != nil {
			w.unavailable(err)
		}
		w.mu.Unlock()
		resume = nil
		runErr = nil
	}
	if runErr = verifyMedia(ctx, w.media, r.Request.Input); runErr == nil {
		runErr = w.runner.Run(ctx, RunSpec{JobID: id, Directory: w.store.jobDir(id), AudioPath: filepath.Join(w.cfg.MediaRoot, filepath.FromSlash(r.Request.Input.RelativePath)), Request: r.Request, Effective: r.Status.EffectiveConfig, Resume: resume, ResultLimit: w.cfg.ResultLimit, OwnerLock: w.store.lock}, func(progress transcription.ProgressUpdate) error { return w.progress(id, progress) })
	}
	if runErr == nil {
		runErr = verifyMedia(ctx, w.media, r.Request.Input)
	}
	var result []byte
	var digest string
	if runErr == nil {
		result, digest, runErr = w.normalizeResult(r)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	cancel := w.active[id]
	if cancel != nil {
		cancel()
	}
	delete(w.active, id)
	w.notify()
	if w.fault != nil {
		return
	}
	fresh, err := w.store.get(id)
	if err != nil {
		w.unavailable(err)
		return
	}
	if fresh.RunSince != nil {
		fresh.BudgetUsed += max(time.Since(*fresh.RunSince), 0)
		fresh.RunSince = nil
	}
	// Final checkpoint capture includes short-lived failures and the last complete
	// chunk. It may never overstate progress after an artifact/storage failure.
	if cpErr := w.captureCheckpoint(fresh); cpErr != nil && !os.IsNotExist(cpErr) {
		runErr = cpErr
	}
	if fresh.Status.State == "cancelling" {
		finish(fresh, "cancelled", transcription.Error("CANCELLED", "Job was cancelled", true))
	} else if w.stopping && runErr != nil {
		touch(fresh)
		fresh.Status.State = "queued"
		fresh.Status.Stage = "interrupted"
	} else if runErr != nil {
		finish(fresh, "failed", publicError(runErr))
	} else {
		if err = durableWrite(w.store.resultPath(id), result); err != nil {
			w.unavailable(err)
			return
		}
		fresh.ResultDigest = digest
		finish(fresh, "succeeded", nil)
		fresh.Status.Percent = 100
	}
	if err = w.store.save(fresh, false); err != nil {
		w.unavailable(err)
		return
	}
	w.cleanupArtifacts(fresh)
	slog.Info("worker_job_finished", "job_id", id, "state", fresh.Status.State)
}

func (w *Worker) progress(id string, p transcription.ProgressUpdate) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.fault != nil {
		return w.fault
	}
	r, err := w.store.get(id)
	if err != nil {
		return err
	}
	if r.Status.State != "running" && r.Status.State != "cancelling" {
		return errors.New("progress for inactive job")
	}
	if err = w.captureCheckpoint(r); err != nil && !os.IsNotExist(err) {
		w.unavailable(err)
		return err
	}
	if p.Percent >= 0 && p.Percent <= 100 {
		r.Status.Percent = max(r.Status.Percent, min(p.Percent, 99))
	}
	if len(p.Stage) <= 64 && p.Stage != "" {
		r.Status.Stage = p.Stage
	}
	if p.TotalSeconds >= 0 {
		r.Status.TotalSeconds = max(r.Status.TotalSeconds, p.TotalSeconds)
	}
	if r.Checkpoint != nil {
		r.Status.CompletedSeconds = max(r.Status.CompletedSeconds, r.Checkpoint.CompletedSeconds)
	}
	touch(r)
	if err = w.store.save(r, false); err != nil {
		w.unavailable(err)
		return err
	}
	w.cleanupArtifacts(r)
	return nil
}

func (w *Worker) Cancel(id string) (transcription.JobStatus, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	r, err := w.store.get(id)
	if err != nil {
		return transcription.JobStatus{}, err
	}
	if terminal(r.Status.State) {
		return r.Status, nil
	}
	if r.Status.State == "queued" {
		finish(r, "cancelled", transcription.Error("CANCELLED", "Job was cancelled", true))
	} else {
		touch(r)
		r.Status.State = "cancelling"
		r.Status.Stage = "cancelling"
	}
	if err = w.store.save(r, false); err != nil {
		w.unavailable(err)
		return r.Status, err
	}
	if cancel := w.active[id]; cancel != nil {
		cancel()
	}
	return r.Status, nil
}
func (w *Worker) Maintenance(accepting *bool) (transcription.MaintenanceStatus, error) {
	if accepting != nil && *accepting {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := w.runner.Ready(ctx)
		cancel()
		if err != nil {
			return transcription.MaintenanceStatus{}, transcription.Error("MODEL_UNAVAILABLE", "Execution prerequisites are unavailable", true)
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if accepting != nil {
		if *accepting {
			if err := w.healthLocked(); err != nil {
				return transcription.MaintenanceStatus{}, err
			}
		}
		if err := w.store.accepting(*accepting); err != nil {
			w.unavailable(err)
			return transcription.MaintenanceStatus{}, err
		}
	}
	return w.store.counts()
}

func (w *Worker) Close(ctx context.Context) error {
	w.mu.Lock()
	if w.stopping {
		w.mu.Unlock()
		return errors.New("worker already closing")
	}
	w.stopping = true
	close(w.done)
	var stopErr error
	for id := range w.active {
		stopErr = errors.Join(stopErr, durableWrite(filepath.Join(w.store.jobDir(id), "stop"), []byte("stop")))
	}
	w.mu.Unlock()
	waited := make(chan struct{})
	go func() { w.wg.Wait(); close(waited) }()
	select {
	case <-waited:
	case <-ctx.Done():
		w.mu.Lock()
		for _, cancel := range w.active {
			cancel()
		}
		w.mu.Unlock()
		<-waited
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return errors.Join(stopErr, w.fault, w.media.Close(), w.store.close())
}

var errConflict = transcription.Error("INVALID_CONFIG", "Request identity or result digest conflict", false)

func (w *Worker) recover(ctx context.Context) error {
	records, err := w.store.list("")
	if err != nil {
		return err
	}
	for _, r := range records {
		if r.Status.State == "succeeded" && !r.Pruned {
			if _, err = w.resultLocked(r); err != nil {
				return fmt.Errorf("durable result verification failed for %s", r.Status.JobID)
			}
		}
		if r.Status.State != "running" && r.Status.State != "cancelling" {
			w.cleanupArtifacts(r)
			continue
		}
		if r.RunSince != nil {
			r.BudgetUsed += max(time.Since(*r.RunSince), 0)
			r.RunSince = nil
		}
		if err = w.captureCheckpoint(r); err != nil {
			r.Checkpoint = nil
			r.Status.Checkpoint.Compatible = false
			r.Status.Checkpoint.Reason = "checkpoint_unverified_restarted_from_zero"
			r.Status.Checkpoint.CompletedSeconds = 0
			r.Status.Checkpoint.SegmentCount = 0
			r.Status.Stage = "restarting_checkpoint_invalid"
		} else {
			r.Status.Stage = "recovered"
		}
		offset := 0.0
		if r.Checkpoint != nil {
			offset = r.Checkpoint.CompletedSeconds
		}
		if offset > r.RecoveryOffset {
			r.Recoveries = 0
		}
		r.RecoveryOffset = offset
		r.Recoveries++
		if r.Status.State == "cancelling" {
			finish(r, "cancelled", transcription.Error("CANCELLED", "Job was cancelled", true))
		} else if r.Recoveries >= 3 {
			finish(r, "failed", transcription.Error("INTERNAL", "Repeated crash recovery without checkpoint progress", true))
		} else {
			touch(r)
			r.Status.State = "queued"
		}
		if err = w.store.save(r, false); err != nil {
			return err
		}
		w.cleanupArtifacts(r)
	}
	return ctx.Err()
}
