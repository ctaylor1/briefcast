package whisperxworker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/internal/transcription"
	"github.com/google/uuid"
)

type fakeRunner struct {
	run func(context.Context, RunSpec, func(transcription.ProgressUpdate) error) error
}

func (fakeRunner) Ready(context.Context) error { return nil }
func (f fakeRunner) Run(ctx context.Context, s RunSpec, p func(transcription.ProgressUpdate) error) error {
	return f.run(ctx, s, p)
}

func workerFixture(t *testing.T) (Config, transcription.JobRequest, []byte) {
	t.Helper()
	dir := t.TempDir()
	media := filepath.Join(dir, "media")
	if err := os.MkdirAll(filepath.Join(media, "audio/example"), 0700); err != nil {
		t.Fatal(err)
	}
	audio := []byte("synthetic media")
	if err := os.WriteFile(filepath.Join(media, "audio/example/episode.mp3"), audio, 0600); err != nil {
		t.Fatal(err)
	}
	token := filepath.Join(dir, "token")
	if err := os.WriteFile(token, []byte(strings.Repeat("t", 32)), 0600); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../docs/whisperx-separation/fixtures/put-job.json")
	if err != nil {
		t.Fatal(err)
	}
	request, _, err := transcription.DecodeRequest(raw)
	if err != nil {
		t.Fatal(err)
	}
	request.Input.SizeBytes = int64(len(audio))
	request.Input.SHA256 = transcription.Digest(audio)
	cfg := Config{StateDir: filepath.Join(dir, "state"), MediaRoot: media, TokenFile: token, EngineProfile: request.EngineProfile, WorkerVersion: "test", Device: "cuda", ComputeType: "float16", BatchSize: 16, ExecutionTimeoutSeconds: 10, MaxRunning: 1, MaxWaiting: 1, ResultLimit: transcription.DefaultResultLimit, Models: []string{request.Options.Model}, DiarizationModels: []string{request.Options.DiarizationModel}, PackageVersions: map[string]string{"whisperx": "3.8.6", "torch": "2.8.0+cu128"}}
	result, err := os.ReadFile("../../docs/whisperx-separation/fixtures/job-result.json")
	if err != nil {
		t.Fatal(err)
	}
	var envelope transcription.JobResult
	if err = json.Unmarshal(result, &envelope); err != nil {
		t.Fatal(err)
	}
	return cfg, request, envelope.Transcript
}
func openTestWorker(t *testing.T, cfg Config, runner Runner) *Worker {
	t.Helper()
	w, err := Open(context.Background(), cfg, runner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = w.Close(ctx)
	})
	return w
}
func submitTest(t *testing.T, w *Worker, id string, r transcription.JobRequest) transcription.JobStatus {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	status, created, err := w.Submit(context.Background(), id, b)
	if err != nil || !created {
		t.Fatalf("submit: created=%v err=%v", created, err)
	}
	return status
}
func waitState(t *testing.T, w *Worker, id, state string) transcription.JobStatus {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, err := w.Status(id)
		if err != nil {
			t.Fatal(err)
		}
		if status.State == state {
			return status
		}
		time.Sleep(10 * time.Millisecond)
	}
	status, _ := w.Status(id)
	t.Fatalf("job did not reach %s: %+v", state, status)
	return status
}
func successRunner(output []byte) fakeRunner {
	return fakeRunner{run: func(ctx context.Context, s RunSpec, p func(transcription.ProgressUpdate) error) error {
		return durableWrite(filepath.Join(s.Directory, "stdout.json"), output)
	}}
}
func callHTTP(w *Worker, method, path string, raw []byte, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	w.Handler().ServeHTTP(rec, r)
	return rec
}

func TestAuthenticatedContractAndIdempotentConcurrentAcceptance(t *testing.T) {
	cfg, request, output := workerFixture(t)
	var executions atomic.Int32
	runner := fakeRunner{run: func(ctx context.Context, s RunSpec, p func(transcription.ProgressUpdate) error) error {
		executions.Add(1)
		return durableWrite(filepath.Join(s.Directory, "stdout.json"), output)
	}}
	w := openTestWorker(t, cfg, runner)
	id := uuid.NewString()
	body, _ := json.Marshal(request)
	for _, endpoint := range []struct{ method, path string }{{"GET", "/v1/capabilities"}, {"PUT", "/v1/jobs/" + id}, {"GET", "/v1/jobs/" + id}, {"GET", "/v1/jobs/" + id + "/result"}, {"POST", "/v1/jobs/" + id + "/cancel"}, {"POST", "/v1/jobs/" + id + "/ack"}, {"POST", "/v1/admin/drain"}, {"POST", "/v1/admin/resume"}} {
		if got := callHTTP(w, endpoint.method, endpoint.path, body, "wrong"); got.Code != 401 {
			t.Errorf("unauthenticated %s: %d", endpoint.path, got.Code)
		}
	}
	if got := callHTTP(w, "GET", "/health/live", nil, ""); got.Code != 200 {
		t.Fatal(got.Code)
	}
	var wg sync.WaitGroup
	var created atomic.Int32
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := callHTTP(w, "PUT", "/v1/jobs/"+id, body, strings.Repeat("t", 32))
			if got.Code == 201 {
				created.Add(1)
			} else if got.Code != 200 {
				t.Errorf("replay status=%d body=%s", got.Code, got.Body)
			}
		}()
	}
	wg.Wait()
	waitState(t, w, id, "succeeded")
	if created.Load() != 1 || executions.Load() != 1 {
		t.Fatalf("created=%d executed=%d", created.Load(), executions.Load())
	}
	changed := request
	changed.Attempt++
	raw, _ := json.Marshal(changed)
	if got := callHTTP(w, "PUT", "/v1/jobs/"+id, raw, strings.Repeat("t", 32)); got.Code != 409 {
		t.Fatal(got.Code)
	}
	if got := callHTTP(w, "PUT", "/v1/jobs/"+uuid.NewString(), []byte(strings.Repeat(" ", transcription.ControlLimit+1)), strings.Repeat("t", 32)); got.Code != 413 {
		t.Fatalf("oversized request=%d", got.Code)
	}
}

func TestCapacityDrainAndResultReceiptSurviveRestart(t *testing.T) {
	cfg, request, output := workerFixture(t)
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	runner := fakeRunner{run: func(ctx context.Context, s RunSpec, p func(transcription.ProgressUpdate) error) error {
		once.Do(func() { close(started) })
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
		return durableWrite(filepath.Join(s.Directory, "stdout.json"), output)
	}}
	w := openTestWorker(t, cfg, runner)
	var err error
	first, second := uuid.NewString(), uuid.NewString()
	submitTest(t, w, first, request)
	<-started
	submitTest(t, w, second, request)
	body, _ := json.Marshal(request)
	if got := callHTTP(w, "PUT", "/v1/jobs/"+uuid.NewString(), body, strings.Repeat("t", 32)); got.Code != 429 || got.Header().Get("Retry-After") == "" {
		t.Fatalf("capacity response=%d", got.Code)
	}
	drain := false
	if _, err = w.Maintenance(&drain); err != nil {
		t.Fatal(err)
	}
	if got := callHTTP(w, "GET", "/health/ready", nil, ""); got.Code != 503 {
		t.Fatal(got.Code)
	}
	close(release)
	waitState(t, w, first, "succeeded")
	waitState(t, w, second, "succeeded")
	before, err := w.Result(first)
	if err != nil {
		t.Fatal(err)
	}
	instance := w.store.instance
	if err = w.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	w = openTestWorker(t, cfg, successRunner(output))
	if w.store.instance != instance {
		t.Fatal("instance identity changed on restart")
	}
	status, _ := w.Maintenance(nil)
	if status.Accepting || status.UnacknowledgedResults != 2 {
		t.Fatalf("lost durable drain/results: %+v", status)
	}
	after, err := w.Result(first)
	if err != nil || string(before) != string(after) {
		t.Fatalf("lost unacknowledged result: %v", err)
	}
	if got := callHTTP(w, "PUT", "/v1/jobs/"+first, body, strings.Repeat("t", 32)); got.Code != 200 {
		t.Fatal("replay rejected while draining")
	}
	if got := callHTTP(w, "PUT", "/v1/jobs/"+uuid.NewString(), body, strings.Repeat("t", 32)); got.Code != 503 {
		t.Fatal(got.Code)
	}
	var result transcription.JobResult
	if err = json.Unmarshal(after, &result); err != nil {
		t.Fatal(err)
	}
	if _, err = w.Ack(first, strings.Repeat("0", 64)); !errors.Is(err, errConflict) {
		t.Fatal("accepted wrong receipt")
	}
	if err = w.PruneAcknowledged(time.Now().Add(30 * 24 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err = w.Result(first); err != nil {
		t.Fatal("unacknowledged result expired")
	}
	ack, err := w.Ack(first, result.ResultSHA256)
	if err != nil {
		t.Fatal(err)
	}
	again, err := w.Ack(first, result.ResultSHA256)
	if err != nil || again != ack {
		t.Fatal("ACK not idempotent")
	}
	if err = w.PruneAcknowledged(time.Now().Add(8 * 24 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err = w.Result(first); !errors.Is(err, errGone) {
		t.Fatal("acknowledged result not pruned")
	}
	if _, err = w.Ack(first, result.ResultSHA256); err != nil {
		t.Fatal("tombstone lost ACK")
	}
	if _, err = w.Result(second); err != nil {
		t.Fatal("pruning removed unacknowledged result")
	}
}

func TestCancellationIsTerminalOnlyAfterRunnerStops(t *testing.T) {
	cfg, request, _ := workerFixture(t)
	started, reaped := make(chan struct{}), make(chan struct{})
	runner := fakeRunner{run: func(ctx context.Context, s RunSpec, p func(transcription.ProgressUpdate) error) error {
		close(started)
		<-ctx.Done()
		<-reaped
		return ctx.Err()
	}}
	w := openTestWorker(t, cfg, runner)
	id := uuid.NewString()
	submitTest(t, w, id, request)
	<-started
	status, err := w.Cancel(id)
	if err != nil || status.State != "cancelling" {
		t.Fatalf("cancel: %+v %v", status, err)
	}
	if _, err = w.Result(id); !errors.Is(err, errConflict) {
		t.Fatal("cancelling job published result")
	}
	status, _ = w.Status(id)
	if status.State != "cancelling" {
		t.Fatal("reported terminal cancellation before reaping")
	}
	close(reaped)
	waitState(t, w, id, "cancelled")
	if status, err = w.Cancel(id); err != nil || status.State != "cancelled" {
		t.Fatal("terminal cancel changed state")
	}
}

func checkpointBytes(s RunSpec, seconds float64) []byte {
	r := &record{Request: s.Request, Status: transcription.JobStatus{EffectiveConfig: s.Effective}, PipelineVersion: transcription.PipelineVersion}
	raw, _ := json.Marshal(checkpointPayload{Segments: json.RawMessage(`[{"start":0,"end":1,"text":"earlier"}]`), CompletedSeconds: seconds, TotalSeconds: 240, Language: "en", Worker: binding(r)})
	return raw
}
func TestVerifiedCheckpointResumeAndFingerprintMismatch(t *testing.T) {
	cfg, request, output := workerFixture(t)
	var calls atomic.Int32
	var resumeSeen atomic.Bool
	runner := fakeRunner{run: func(ctx context.Context, s RunSpec, p func(transcription.ProgressUpdate) error) error {
		if calls.Add(1) == 1 {
			if err := durableWrite(filepath.Join(s.Directory, "segments.json"), checkpointBytes(s, 120)); err != nil {
				return err
			}
			if err := p(transcription.ProgressUpdate{Percent: 50, Stage: "transcribing", TotalSeconds: 240}); err != nil {
				return err
			}
			return errors.New("synthetic failure including secret")
		}
		if len(s.Resume) == 0 {
			return errors.New("resume missing")
		}
		resumeSeen.Store(true)
		return durableWrite(filepath.Join(s.Directory, "stdout.json"), output)
	}}
	w := openTestWorker(t, cfg, runner)
	id := uuid.NewString()
	submitTest(t, w, id, request)
	status := waitState(t, w, id, "failed")
	if status.Checkpoint.CompletedSeconds != 120 || strings.Contains(status.Error.Message, "secret") {
		t.Fatal("checkpoint missing or diagnostics exposed")
	}
	retry := request
	retry.ResumeFromJobID = &id
	retry.Attempt = 2
	second := uuid.NewString()
	submitTest(t, w, second, retry)
	waitState(t, w, second, "succeeded")
	if !resumeSeen.Load() {
		t.Fatal("did not resume")
	}
	retry.Options.ASROptions.BeamSize++
	body, _ := json.Marshal(retry)
	if _, _, err := w.Submit(context.Background(), uuid.NewString(), body); publicError(err).Code != "CHECKPOINT_INCOMPATIBLE" {
		t.Fatalf("accepted incompatible resume: %v", err)
	}
}

func TestRejectMaliciousMediaAndChangedInputWithoutDeletingAudio(t *testing.T) {
	cfg, request, output := workerFixture(t)
	w := openTestWorker(t, cfg, successRunner(output))
	for _, path := range []string{"../escape", "/absolute", "C:/escape", `\\host\share`, "audio/../escape", "audio//escape", "audio/./escape", "audio/episode:stream", "audio/\x00bad"} {
		r := request
		r.Input.RelativePath = path
		body, _ := json.Marshal(r)
		if _, _, err := w.Submit(context.Background(), uuid.NewString(), body); err == nil {
			t.Errorf("accepted %q", path)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.mp3")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(cfg.MediaRoot, "escape.mp3")
	if err := os.Symlink(outside, link); err == nil {
		r := request
		r.Input.RelativePath = "escape.mp3"
		r.Input.SizeBytes = 7
		r.Input.SHA256 = transcription.Digest([]byte("outside"))
		body, _ := json.Marshal(r)
		if _, _, err = w.Submit(context.Background(), uuid.NewString(), body); err == nil {
			t.Fatal("accepted symlink escape")
		}
	} else {
		t.Log("symlink creation unavailable on this host")
	}
	request.Input.SHA256 = strings.Repeat("0", 64)
	raw, _ := json.Marshal(request)
	if _, _, err := w.Submit(context.Background(), uuid.NewString(), raw); publicError(err).Code != "INPUT_CHANGED" {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfg.MediaRoot, "audio/example/episode.mp3")); err != nil {
		t.Fatal("valid source was removed")
	}
}

func TestStorageFailuresCannotClaimAcceptanceOrSuccess(t *testing.T) {
	for _, boundary := range []string{"acceptance", "result_file", "result_commit"} {
		t.Run(boundary, func(t *testing.T) {
			cfg, request, output := workerFixture(t)
			runner := successRunner(output)
			if boundary == "result_file" {
				runner.run = func(ctx context.Context, s RunSpec, p func(transcription.ProgressUpdate) error) error {
					if err := os.Mkdir(filepath.Join(s.Directory, "result.json"), 0700); err != nil {
						return err
					}
					return durableWrite(filepath.Join(s.Directory, "stdout.json"), output)
				}
			}
			w := openTestWorker(t, cfg, runner)
			id := uuid.NewString()
			trigger := "CREATE TRIGGER reject_write BEFORE UPDATE ON jobs WHEN NEW.state='succeeded' BEGIN SELECT RAISE(ABORT,'injected'); END"
			if boundary == "acceptance" {
				trigger = "CREATE TRIGGER reject_write BEFORE INSERT ON jobs BEGIN SELECT RAISE(ABORT,'injected'); END"
			}
			if boundary != "result_file" {
				if _, err := w.store.db.Exec(trigger); err != nil {
					t.Fatal(err)
				}
			}
			body, _ := json.Marshal(request)
			_, created, err := w.Submit(context.Background(), id, body)
			if boundary == "acceptance" {
				if err == nil || created {
					t.Fatal("claimed durable acceptance")
				}
				if _, err = w.Status(id); !errors.Is(err, sql.ErrNoRows) {
					t.Fatal("failed acceptance left journal record")
				}
				return
			}
			if err != nil || !created {
				t.Fatal(err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				w.mu.Lock()
				fault := w.fault
				w.mu.Unlock()
				if fault != nil {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			status, err := w.Status(id)
			if err != nil || status.State == "succeeded" {
				t.Fatalf("claimed false durable success: %+v %v", status, err)
			}
			if _, err = w.Result(id); err == nil {
				t.Fatal("published undurable result")
			}
		})
	}
}

func TestStateOwnershipAndRecoveryBudget(t *testing.T) {
	cfg, request, output := workerFixture(t)
	w := openTestWorker(t, cfg, successRunner(output))
	if second, err := Open(context.Background(), cfg, successRunner(output)); err == nil {
		second.Close(context.Background())
		t.Fatal("two workers owned state")
	}
	id := uuid.NewString()
	submitTest(t, w, id, request)
	waitState(t, w, id, "succeeded")
	// Simulate durable images at the crash boundary without leaving test children
	// behind. Each recovery has a previously running record with no new chunk.
	w.mu.Lock()
	defer w.mu.Unlock()
	r, err := w.store.get(id)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		r.Status.State = "running"
		r.ResultDigest = ""
		now := time.Now().UTC().Add(-time.Second)
		r.RunSince = &now
		if err = w.store.save(r, false); err != nil {
			t.Fatal(err)
		}
		if err = w.recover(context.Background()); err != nil {
			t.Fatal(err)
		}
		r, err = w.store.get(id)
		if err != nil {
			t.Fatal(err)
		}
		if r.BudgetUsed < time.Duration(i)*time.Second {
			t.Fatal("restart reset cumulative budget")
		}
		if i < 3 && r.Status.State != "queued" {
			t.Fatal(r.Status.State)
		}
	}
	if r.Status.State != "failed" || r.Status.Error.Code != "INTERNAL" {
		t.Fatal("unbounded recovery loop")
	}
}
