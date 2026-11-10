package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/internal/transcription"
	"github.com/ctaylor1/briefcast/internal/whisperxworker"
)

func isJobStatusRequest(r *http.Request) bool {
	return r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/jobs/") && strings.Count(r.URL.Path, "/") == 3
}

// mismatchedStatus serves the real status with completed > total, the shape
// an older worker reported for resumed jobs; the app rejects it permanently.
func mismatchedStatus(t *testing.T, f *remoteTestFixture, rw http.ResponseWriter, r *http.Request) {
	rec := httptest.NewRecorder()
	f.worker.Handler().ServeHTTP(rec, r)
	var s transcription.JobStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Error(err)
	}
	s.CompletedSeconds, s.TotalSeconds = 5, 0
	rw.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(rw).Encode(s)
}

func waitWorkerState(t *testing.T, w *whisperxworker.Worker, id, state string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s, err := w.Status(id); err == nil && s.State == state {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	s, _ := w.Status(id)
	t.Fatalf("worker job did not reach %s: %+v", state, s)
}

func TestRemotePermanentGiveUpCancelsAcceptedJob(t *testing.T) {
	f := newRemoteTestFixture(t)
	var inject atomic.Bool
	var cancels atomic.Int32
	f.runner.run = func(ctx context.Context, s whisperxworker.RunSpec, p func(transcription.ProgressUpdate) error) error {
		inject.Store(true)
		<-ctx.Done()
		return ctx.Err()
	}
	f.intercept = func(rw http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/cancel") {
			cancels.Add(1)
			if got := loadCharacterizationPodcastItem(t, f.item.ID); got.TranscriptProgressStage != "blocked" {
				t.Error("cancel was sent before the blocked state was committed")
			}
			return false
		}
		if inject.Load() && isJobStatusRequest(r) {
			mismatchedStatus(t, f, rw, r)
			return true
		}
		return false
	}
	if err := f.run(t); err == nil {
		t.Fatal("mismatched status was accepted")
	}
	got := loadCharacterizationPodcastItem(t, f.item.ID)
	e, err := decodeRemoteEnvelope(got.TranscriptCheckpointJSON)
	if err != nil {
		t.Fatal(err)
	}
	if got.TranscriptProgressStage != "blocked" || cancels.Load() != 1 {
		t.Fatalf("stage=%s cancels=%d", got.TranscriptProgressStage, cancels.Load())
	}
	waitWorkerState(t, f.worker, e.JobID, "cancelled")
}

func TestRemoteRetryableFailureKeepsAcceptedJobRunning(t *testing.T) {
	f := newRemoteTestFixture(t)
	var inject atomic.Bool
	var cancels atomic.Int32
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	f.runner.run = func(ctx context.Context, s whisperxworker.RunSpec, p func(transcription.ProgressUpdate) error) error {
		inject.Store(true)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.intercept = func(rw http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/cancel") {
			cancels.Add(1)
			return false
		}
		if inject.Load() && isJobStatusRequest(r) {
			rw.WriteHeader(503)
			return true
		}
		return false
	}
	if err := f.run(t); err == nil {
		t.Fatal("worker outage was not surfaced")
	}
	got := loadCharacterizationPodcastItem(t, f.item.ID)
	e, err := decodeRemoteEnvelope(got.TranscriptCheckpointJSON)
	if err != nil {
		t.Fatal(err)
	}
	s, err := f.worker.Status(e.JobID)
	if err != nil || s.State != "running" || cancels.Load() != 0 || got.TranscriptStatus != "processing" {
		t.Fatalf("retryable failure abandoned the job: state=%s cancels=%d status=%s", s.State, cancels.Load(), got.TranscriptStatus)
	}
}

func TestRemoteManualResetCancelsSupersededRunningJob(t *testing.T) {
	f := newRemoteTestFixture(t)
	var inject atomic.Bool
	var cancels atomic.Int32
	f.runner.run = func(ctx context.Context, s whisperxworker.RunSpec, p func(transcription.ProgressUpdate) error) error {
		if f.runner.calls.Load() != 1 {
			return nil
		}
		inject.Store(true)
		<-ctx.Done()
		return ctx.Err()
	}
	f.intercept = func(rw http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/cancel") {
			if cancels.Add(1) == 1 {
				rw.WriteHeader(500) // the first, best-effort cancel is lost
				return true
			}
			return false
		}
		if inject.Load() && isJobStatusRequest(r) {
			mismatchedStatus(t, f, rw, r)
			return true
		}
		return false
	}
	if err := f.run(t); err == nil {
		t.Fatal("mismatched status was accepted")
	}
	blocked := loadCharacterizationPodcastItem(t, f.item.ID)
	old, err := decodeRemoteEnvelope(blocked.TranscriptCheckpointJSON)
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := f.worker.Status(old.JobID); s.State != "running" {
		t.Fatalf("lost cancel should leave the old job running, got %s", s.State)
	}
	inject.Store(false)
	if err = os.WriteFile(f.item.DownloadPath, []byte(strings.Repeat("e", 2048)), 0600); err != nil {
		t.Fatal(err)
	}
	if count, err := ForceFailedTranscriptRetriesDue(time.Now().UTC()); err != nil || count != 1 {
		t.Fatalf("manual repair: %v, %d", err, count)
	}
	if err = f.run(t); err != nil {
		t.Fatalf("manual reset run: %v", err)
	}
	got := loadCharacterizationPodcastItem(t, f.item.ID)
	if got.TranscriptStatus != "available" || cancels.Load() != 2 {
		t.Fatalf("status=%s cancels=%d", got.TranscriptStatus, cancels.Load())
	}
	waitWorkerState(t, f.worker, old.JobID, "cancelled")
}
