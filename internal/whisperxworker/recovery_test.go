package whisperxworker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/internal/transcription"
	"github.com/google/uuid"
)

func TestInputChangedBeforeCompletionRejectsResult(t *testing.T) {
	cfg, request, output := workerFixture(t)
	runner := fakeRunner{run: func(ctx context.Context, s RunSpec, p func(transcription.ProgressUpdate) error) error {
		if err := durableWrite(filepath.Join(s.Directory, "stdout.json"), output); err != nil {
			return err
		}
		return os.WriteFile(s.AudioPath, []byte("changed media!"), 0600)
	}}
	w := openTestWorker(t, cfg, runner)
	id := uuid.NewString()
	submitTest(t, w, id, request)
	status := waitState(t, w, id, "failed")
	if status.Error.Code != "INPUT_CHANGED" {
		t.Fatal(status.Error)
	}
	if _, err := w.Result(id); err == nil {
		t.Fatal("published stale result")
	}
	if _, err := os.Stat(filepath.Join(cfg.MediaRoot, filepath.FromSlash(request.Input.RelativePath))); err != nil {
		t.Fatal("source was deleted")
	}
}

func TestResultCommitFailureRecoversVerifiedCheckpoint(t *testing.T) {
	cfg, request, output := workerFixture(t)
	runner := fakeRunner{run: func(ctx context.Context, s RunSpec, p func(transcription.ProgressUpdate) error) error {
		if err := durableWrite(filepath.Join(s.Directory, "segments.json"), checkpointBytes(s, 120)); err != nil {
			return err
		}
		return durableWrite(filepath.Join(s.Directory, "stdout.json"), output)
	}}
	w := openTestWorker(t, cfg, runner)
	if _, err := w.store.db.Exec("CREATE TRIGGER reject_success BEFORE UPDATE ON jobs WHEN NEW.state='succeeded' BEGIN SELECT RAISE(ABORT,'injected'); END"); err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	submitTest(t, w, id, request)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		w.mu.Lock()
		failed := w.fault != nil
		w.mu.Unlock()
		if failed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(w.store.resultPath(id)); err != nil {
		t.Fatal("result artifact boundary not reached")
	}
	status, _ := w.Status(id)
	if status.State != "running" {
		t.Fatal("false committed success")
	}
	_ = w.Close(context.Background())
	s, err := openStore(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("DROP TRIGGER reject_success"); err != nil {
		t.Fatal(err)
	}
	s.close()
	var resumed atomic.Bool
	runner.run = func(ctx context.Context, s RunSpec, p func(transcription.ProgressUpdate) error) error {
		resumed.Store(len(s.Resume) > 0)
		return durableWrite(filepath.Join(s.Directory, "stdout.json"), output)
	}
	w = openTestWorker(t, cfg, runner)
	waitState(t, w, id, "succeeded")
	if !resumed.Load() {
		t.Fatal("crash lost the last verified chunk")
	}
	if _, err = w.Result(id); err != nil {
		t.Fatal(err)
	}
}

func TestCorruptCheckpointRecoveryRestartsAtZeroWithReason(t *testing.T) {
	cfg, request, output := workerFixture(t)
	w := openTestWorker(t, cfg, successRunner(output))
	id := uuid.NewString()
	submitTest(t, w, id, request)
	waitState(t, w, id, "succeeded")
	w.mu.Lock()
	r, err := w.store.get(id)
	if err != nil {
		t.Fatal(err)
	}
	r.Status.State = "running"
	r.ResultDigest = ""
	if err = durableWrite(filepath.Join(w.store.jobDir(id), "segments.json"), []byte(`{"completed_seconds":120,"segments_file":"/untrusted"}`)); err != nil {
		t.Fatal(err)
	}
	if err = w.store.save(r, false); err != nil {
		t.Fatal(err)
	}
	if err = w.recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	r, err = w.store.get(id)
	w.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if r.Checkpoint != nil || r.Status.Checkpoint.Compatible || r.Status.Checkpoint.Reason == "" || r.Status.State != "queued" {
		t.Fatalf("unsafe checkpoint recovery: %+v", r.Status)
	}
}

func TestCorruptResultFailsReadinessAndStartup(t *testing.T) {
	cfg, request, output := workerFixture(t)
	w := openTestWorker(t, cfg, successRunner(output))
	id := uuid.NewString()
	submitTest(t, w, id, request)
	waitState(t, w, id, "succeeded")
	if err := os.WriteFile(w.store.resultPath(id), []byte(`{"error":"corrupt"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Result(id); err == nil {
		t.Fatal("served corrupt artifact")
	}
	if response := callHTTP(w, "GET", "/health/ready", nil, ""); response.Code != 503 {
		t.Fatal("known corruption did not stop intake")
	}
	_ = w.Close(context.Background())
	if restarted, err := Open(context.Background(), cfg, successRunner(output)); err == nil {
		restarted.Close(context.Background())
		t.Fatal("startup accepted broken durable result")
	}
}

func TestProfileCannotBeReusedForDifferentRuntime(t *testing.T) {
	cfg, _, output := workerFixture(t)
	w := openTestWorker(t, cfg, successRunner(output))
	if err := w.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	cfg.ComputeType = "float32"
	if restarted, err := Open(context.Background(), cfg, successRunner(output)); err == nil {
		restarted.Close(context.Background())
		t.Fatal("profile reused with different effective runtime")
	}
	cfg.EngineProfile += "-new"
	w = openTestWorker(t, cfg, successRunner(output))
	if w == nil {
		t.Fatal("new profile could not start")
	}
}

func TestRecoveryRejectsJournalIdentityMismatch(t *testing.T) {
	cfg, request, output := workerFixture(t)
	w := openTestWorker(t, cfg, successRunner(output))
	id := uuid.NewString()
	submitTest(t, w, id, request)
	waitState(t, w, id, "succeeded")
	w.mu.Lock()
	r, err := w.store.get(id)
	w.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	_ = w.Close(context.Background())
	s, err := openStore(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	r.Status.JobID = "../outside"
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("UPDATE jobs SET data=? WHERE id=?", raw, id); err != nil {
		t.Fatal(err)
	}
	s.close()
	if restarted, err := Open(context.Background(), cfg, successRunner(output)); err == nil {
		restarted.Close(context.Background())
		t.Fatal("recovery trusted a corrupt job directory identity")
	}
}

func TestAcknowledgedArtifactPruningPreservesReferencedResumeSource(t *testing.T) {
	cfg, request, output := workerFixture(t)
	var calls atomic.Int32
	runner := fakeRunner{run: func(ctx context.Context, s RunSpec, p func(transcription.ProgressUpdate) error) error {
		seconds := 240.0
		if calls.Add(1) == 1 {
			seconds = 120
		}
		if err := durableWrite(filepath.Join(s.Directory, "segments.json"), checkpointBytes(s, seconds)); err != nil {
			return err
		}
		if seconds == 120 {
			return errors.New("first attempt fails")
		}
		return durableWrite(filepath.Join(s.Directory, "stdout.json"), output)
	}}
	w := openTestWorker(t, cfg, runner)
	first := uuid.NewString()
	submitTest(t, w, first, request)
	waitState(t, w, first, "failed")
	request.ResumeFromJobID = &first
	request.Attempt++
	second := uuid.NewString()
	submitTest(t, w, second, request)
	waitState(t, w, second, "succeeded")
	w.mu.Lock()
	source, err := w.store.get(first)
	success, secondErr := w.store.get(second)
	w.mu.Unlock()
	if err != nil || secondErr != nil {
		t.Fatal(err, secondErr)
	}
	successPath := filepath.Join(w.store.dir, filepath.FromSlash(success.Checkpoint.Path))
	if _, err = w.Ack(second, success.ResultDigest); err != nil {
		t.Fatal(err)
	}
	if err = w.PruneAcknowledged(time.Now().Add(8 * 24 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(successPath); !os.IsNotExist(err) {
		t.Fatal("acknowledged large checkpoint retained")
	}
	if _, err = w.readCheckpoint(source); err != nil {
		t.Fatal("referenced failed checkpoint was removed")
	}
	request.Attempt++
	third := uuid.NewString()
	submitTest(t, w, third, request)
	waitState(t, w, third, "succeeded")
}
