package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/transcription"
	"github.com/ctaylor1/briefcast/internal/whisperxworker"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type remoteTestRunner struct {
	t     *testing.T
	calls atomic.Int32
	run   func(context.Context, whisperxworker.RunSpec, func(transcription.ProgressUpdate) error) error
}

func (r *remoteTestRunner) Ready(context.Context) error { return nil }
func (r *remoteTestRunner) Run(ctx context.Context, s whisperxworker.RunSpec, p func(transcription.ProgressUpdate) error) error {
	r.calls.Add(1)
	if r.run != nil {
		if err := r.run(ctx, s, p); err != nil {
			return err
		}
	}
	raw, err := os.ReadFile("../docs/whisperx-separation/fixtures/job-result.json")
	if err != nil {
		return err
	}
	var result transcription.JobResult
	_ = json.Unmarshal(raw, &result)
	var body map[string]any
	_ = json.Unmarshal(result.Transcript, &body)
	body["model"] = s.Request.Options.Model
	body["device"] = s.Effective.Device
	body["compute_type"] = s.Effective.ComputeType
	body["batch_size"] = s.Effective.BatchSize
	body["asr_options"] = s.Request.Options.ASROptions
	body["vad_options"] = s.Request.Options.VADOptions
	body["alignment"] = map[string]any{"requested": s.Request.Options.Align, "used": s.Request.Options.Align, "error": ""}
	body["aligned"] = s.Request.Options.Align
	body["diarization"] = map[string]any{"enabled": s.Request.Options.Diarization, "used": false, "model": s.Request.Options.DiarizationModel, "min_speakers": s.Request.Options.MinSpeakers, "max_speakers": s.Request.Options.MaxSpeakers, "error": ""}
	data, _ := json.Marshal(body)
	return os.WriteFile(filepath.Join(s.Directory, "stdout.json"), data, 0600)
}

type remoteTestFixture struct {
	item       db.PodcastItem
	cfg        WhisperXConfig
	runner     *remoteTestRunner
	worker     *whisperxworker.Worker
	server     *httptest.Server
	puts, acks atomic.Int32
	intercept  func(http.ResponseWriter, *http.Request) bool
}

func newRemoteTestFixture(t *testing.T) *remoteTestFixture {
	t.Helper()
	_, item := seedCharacterizationPodcastItem(t, func(i *db.PodcastItem, dir string) {
		i.DownloadPath = filepath.Join(os.Getenv("DATA"), "remote.mp3")
		if err := os.WriteFile(i.DownloadPath, []byte(strings.Repeat("a", 2048)), 0600); err != nil {
			t.Fatal(err)
		}
		i.TranscriptJSON = ""
		i.CanonicalTranscript = ""
		i.TranscriptCheckpointJSON = ""
		i.TranscriptStatus = "pending_whisperx"
		i.TranscriptProgressPct = 0
		i.TranscriptRetryCount = 0
		i.TranscriptNextAttempt = nil
		i.LLMSummary = ""
		i.LLMSummaryStatus = "pending"
	})
	t.Setenv("LLM_ENABLED", "false")
	t.Setenv(whisperxTimeoutEnv, "10")
	t.Setenv(whisperxProgressPollEnv, "5")
	token := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(token, []byte(strings.Repeat("t", 32)), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := LoadWhisperXConfig()
	cfg.Backend = "http"
	cfg.Enabled = true
	cfg.Diarization = false
	cfg.Device = "cpu"
	cfg.ComputeType = "int8"
	cfg.BatchSize = 16
	cfg.EngineProfile = "stage-e-test"
	cfg.ServiceTokenFile = token
	cfg.MaxItemsPerRun = 1
	cfg.MaxConcurrency = 1
	cfg.LockRefreshSecs = 1
	runner := &remoteTestRunner{t: t}
	w, err := whisperxworker.Open(context.Background(), whisperxworker.Config{StateDir: filepath.Join(t.TempDir(), "state"), MediaRoot: os.Getenv("DATA"), TokenFile: token, EngineProfile: cfg.EngineProfile, WorkerVersion: "test", Device: "cpu", ComputeType: "int8", BatchSize: 16, ExecutionTimeoutSeconds: 10, MaxRunning: 1, MaxWaiting: 4, ResultLimit: transcription.DefaultResultLimit, Models: []string{cfg.Model}, DiarizationModels: []string{cfg.DiarizationModel}, PackageVersions: map[string]string{"whisperx": "3.8.6", "torch": "2.8.0+cu128"}}, runner)
	if err != nil {
		t.Fatal(err)
	}
	f := &remoteTestFixture{item: item, cfg: cfg, worker: w, runner: runner}
	handler := w.Handler()
	f.server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			f.puts.Add(1)
			current := loadCharacterizationPodcastItem(t, item.ID)
			e, err := decodeRemoteEnvelope(current.TranscriptCheckpointJSON)
			if err != nil || e.JobID != strings.TrimPrefix(r.URL.Path, "/v1/jobs/") {
				t.Error("submission identity was not durable before PUT")
			}
		}
		if strings.HasSuffix(r.URL.Path, "/ack") {
			f.acks.Add(1)
			current := loadCharacterizationPodcastItem(t, item.ID)
			if current.TranscriptStatus != "available" || current.TranscriptJSON == "" {
				t.Error("ACK preceded DB commit")
			}
		}
		if f.intercept != nil && f.intercept(rw, r) {
			return
		}
		handler.ServeHTTP(rw, r)
	}))
	f.cfg.ServiceURL = f.server.URL
	t.Cleanup(func() {
		f.server.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := w.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return f
}
func (f *remoteTestFixture) run(t *testing.T) error {
	t.Helper()
	b, err := newHTTPTranscriptionBackend(f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return transcribePendingEpisodes(ctx, f.cfg, b)
}
func (f *remoteTestFixture) due(t *testing.T) {
	t.Helper()
	if err := db.DB.Model(&db.PodcastItem{}).Where("id = ?", f.item.ID).Update("transcript_next_attempt", nil).Error; err != nil {
		t.Fatal(err)
	}
}

func TestRemoteCommitPreservesConcurrentLibraryEditsAndAcknowledges(t *testing.T) {
	f := newRemoteTestFixture(t)
	bookmark := time.Now().UTC().Truncate(time.Second)
	f.runner.run = func(ctx context.Context, s whisperxworker.RunSpec, p func(transcription.ProgressUpdate) error) error {
		return db.DB.Model(&db.PodcastItem{}).Where("id = ?", f.item.ID).Updates(map[string]any{"is_played": true, "bookmark_date": bookmark, "title": "Concurrent title"}).Error
	}
	if err := f.run(t); err != nil {
		t.Fatal(err)
	}
	got := loadCharacterizationPodcastItem(t, f.item.ID)
	if !got.IsPlayed || !got.BookmarkDate.Equal(bookmark) || got.Title != "Concurrent title" || got.TranscriptStatus != "available" || got.TranscriptProgressPct != 100 || got.TranscriptCheckpointJSON != "" || f.runner.calls.Load() != 1 || f.acks.Load() != 1 || got.CanonicalTranscript != "Hello world." {
		t.Fatalf("invalid commit/receipt: %+v", got)
	}
	for _, variant := range transcriptExportVariants {
		path := filepath.Join(resolveExportDir(), variant.dir, sanitizeAssetName(podcastTitle(&got)), pubDateSubdir(got.PubDate), exportEpisodeFileName(&got)+variant.extension)
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRemoteCommitFailureRetainsResultAndSkipsDownstream(t *testing.T) {
	f := newRemoteTestFixture(t)
	if err := db.DB.Exec("CREATE TRIGGER stage_e_commit_failure BEFORE UPDATE OF transcript_status ON podcast_items WHEN NEW.transcript_status = 'available' BEGIN SELECT RAISE(ABORT, 'injected commit failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := f.run(t); err == nil {
		t.Fatal("commit failure was swallowed")
	}
	got := loadCharacterizationPodcastItem(t, f.item.ID)
	e, err := decodeRemoteEnvelope(got.TranscriptCheckpointJSON)
	if err != nil {
		t.Fatal(err)
	}
	if got.TranscriptJSON != "" || got.TranscriptStatus != "processing" || f.acks.Load() != 0 || f.runner.calls.Load() != 1 {
		t.Fatal("uncommitted result acknowledged or lost")
	}
	if _, err := os.Stat(filepath.Join(resolveExportDir(), "transcripts")); !os.IsNotExist(err) {
		t.Fatal("export occurred before commit")
	}
	if _, err := f.worker.Result(e.JobID); err != nil {
		t.Fatal("result was not retained")
	}
	if err := db.DB.Exec("DROP TRIGGER stage_e_commit_failure").Error; err != nil {
		t.Fatal(err)
	}
	f.cfg.BeamSize = 9 // Reconnect must use the durable old options.
	if err := f.run(t); err != nil {
		t.Fatal(err)
	}
	if f.runner.calls.Load() != 1 || f.puts.Load() != 1 || f.acks.Load() != 1 {
		t.Fatal("DB recovery repeated inference")
	}
}

func dropTestConnection(t *testing.T, rw http.ResponseWriter) {
	t.Helper()
	conn, _, err := rw.(http.Hijacker).Hijack()
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
}
func TestRemoteLostAcceptanceResponseReusesSubmission(t *testing.T) {
	f := newRemoteTestFixture(t)
	var dropped atomic.Bool
	f.intercept = func(rw http.ResponseWriter, r *http.Request) bool {
		if r.Method == "PUT" && dropped.CompareAndSwap(false, true) {
			rec := httptest.NewRecorder()
			f.worker.Handler().ServeHTTP(rec, r)
			if rec.Code != 201 {
				t.Errorf("PUT failed: %d", rec.Code)
			}
			dropTestConnection(t, rw)
			return true
		}
		return false
	}
	if err := f.run(t); err == nil {
		t.Fatal("lost response was not visible")
	}
	got := loadCharacterizationPodcastItem(t, f.item.ID)
	e, err := decodeRemoteEnvelope(got.TranscriptCheckpointJSON)
	if err != nil {
		t.Fatal(err)
	}
	if got.TranscriptRetryCount != 0 || got.TranscriptStatus != "processing" || e.WorkerInstanceID != nil {
		t.Fatal("transport ambiguity consumed attempt")
	}
	f.due(t)
	f.cfg.Model = "changed-model"
	f.cfg.InitialPrompt = "changed options"
	if err := f.run(t); err != nil {
		t.Fatal(err)
	}
	if f.puts.Load() != 1 || f.runner.calls.Load() != 1 {
		t.Fatal("lost response caused duplicate execution")
	}
}

func TestRemoteReceiptReconcilesAfterLostAckResponse(t *testing.T) {
	f := newRemoteTestFixture(t)
	var dropped atomic.Bool
	f.intercept = func(rw http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/ack") && dropped.CompareAndSwap(false, true) {
			f.worker.Handler().ServeHTTP(httptest.NewRecorder(), r)
			dropTestConnection(t, rw)
			return true
		}
		return false
	}
	if err := f.run(t); err == nil {
		t.Fatal("lost ACK response not surfaced")
	}
	got := loadCharacterizationPodcastItem(t, f.item.ID)
	e, err := decodeRemoteEnvelope(got.TranscriptCheckpointJSON)
	if err != nil || e.ReceiptResultSHA256 == nil || got.TranscriptStatus != "available" {
		t.Fatalf("receipt lost: %v", err)
	}
	f.due(t)
	if err := f.run(t); err != nil {
		t.Fatal(err)
	}
	got = loadCharacterizationPodcastItem(t, f.item.ID)
	if got.TranscriptCheckpointJSON != "" || f.acks.Load() != 2 || f.runner.calls.Load() != 1 {
		t.Fatal("receipt reconciliation reran inference or lost ACK")
	}
}

func TestRemoteGenerationOrInputChangeRejectsResult(t *testing.T) {
	for _, change := range []string{"generation", "input", "deleted", "lease"} {
		t.Run(change, func(t *testing.T) {
			f := newRemoteTestFixture(t)
			f.runner.run = func(ctx context.Context, s whisperxworker.RunSpec, p func(transcription.ProgressUpdate) error) error {
				switch change {
				case "generation":
					return db.DB.Model(&db.PodcastItem{}).Where("id = ?", f.item.ID).Updates(map[string]any{"transcript_checkpoint_json": "", "transcript_status": "pending_whisperx"}).Error
				case "deleted":
					return db.DB.Delete(&db.PodcastItem{}, "id = ?", f.item.ID).Error
				case "lease":
					return db.DB.Model(&db.JobLock{}).Where("name = ?", "TranscribePendingEpisodes").Update("date", time.Now().UTC().Add(time.Minute)).Error
				default:
					return os.WriteFile(f.item.DownloadPath, []byte(strings.Repeat("b", 2048)), 0600)
				}
			}
			if err := f.run(t); err == nil {
				t.Fatal("stale work committed")
			}
			var got db.PodcastItem
			if err := db.DB.Unscoped().First(&got, "id = ?", f.item.ID).Error; change == "deleted" && errors.Is(err, gorm.ErrRecordNotFound) {
				if f.acks.Load() != 0 {
					t.Fatal("deleted episode ACKed")
				}
				return
			} else if err != nil {
				t.Fatal(err)
			}
			if got.TranscriptJSON != "" || f.acks.Load() != 0 {
				t.Fatal("stale result committed/ACKed")
			}
			if change == "lease" {
				lock := db.GetLock("TranscribePendingEpisodes")
				if lock.Duration == 0 {
					t.Fatal("old run released successor lease")
				}
			}
		})
	}
}

func TestRemoteHTTPFailuresPreserveAudioAndDoNotConsumeInferenceRetries(t *testing.T) {
	for _, code := range []int{401, 403, 409, 429, 500, 503} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			f := newRemoteTestFixture(t)
			f.intercept = func(rw http.ResponseWriter, r *http.Request) bool {
				if r.Method == "PUT" {
					rw.Header().Set("Retry-After", "2")
					rw.WriteHeader(code)
					_, _ = rw.Write([]byte(`{"error":{"code":"INPUT_MISSING","message":"No such file or directory secret","retryable":true}}`))
					return true
				}
				return false
			}
			if err := f.run(t); err == nil {
				t.Fatal("failed worker request succeeded")
			}
			got := loadCharacterizationPodcastItem(t, f.item.ID)
			if got.DownloadPath != f.item.DownloadPath || got.DownloadStatus != db.Downloaded || got.TranscriptRetryCount != 0 || strings.Contains(got.TranscriptLastError, "secret") {
				t.Fatal("remote infrastructure failure altered media or leaked diagnostic")
			}
			if _, err := os.Stat(f.item.DownloadPath); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRemotePaginationSkipsBlockedAttemptsBeforeLimit(t *testing.T) {
	f := newRemoteTestFixture(t)
	for i := 0; i < 105; i++ {
		item := db.PodcastItem{PodcastID: f.item.PodcastID, DownloadPath: f.item.DownloadPath, DownloadStatus: db.Downloaded, TranscriptStatus: "failed", TranscriptProgressStage: "blocked"}
		if err := db.CreatePodcastItem(&item); err != nil {
			t.Fatal(err)
		}
	}
	// Put the real eligible episode after an entire blocked page by UUID order.
	id := "ffffffff-ffff-4fff-8fff-ffffffffffff"
	if err := db.DB.Model(&db.PodcastItem{}).Where("id = ?", f.item.ID).Updates(map[string]any{"id": id, "transcript_status": "failed", "transcript_progress_stage": "failed"}).Error; err != nil {
		t.Fatal(err)
	}
	f.item.ID = id
	items, err := remoteQueueItems(f.cfg, time.Now().UTC())
	if err != nil || len(items) != 1 || items[0].ID != id {
		t.Fatalf("blocked page starved eligible item: %v, %d", err, len(items))
	}
}

func TestRemoteLegacyAndUnknownCheckpointHandling(t *testing.T) {
	f := newRemoteTestFixture(t)
	sidecar := f.item.DownloadPath + ".briefcast.whisperx.resume.json"
	if err := os.WriteFile(sidecar, []byte(`{"segments":[{"text":"old"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	legacy := `{"completed_seconds":120,"total_seconds":240,"segments_file":` + string(mustJSON(t, sidecar)) + `}`
	item := f.item
	item.TranscriptCheckpointJSON = legacy
	b, err := newHTTPTranscriptionBackend(f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer b.client.CloseIdleConnections()
	if err = b.Preflight(context.Background()); err != nil {
		t.Fatal(err)
	}
	e, err := b.prepare(context.Background(), item)
	if err != nil || e.LegacyImport == nil || e.RestartReason == "" {
		t.Fatalf("legacy evidence lost: %v", err)
	}
	r, _, _ := transcription.DecodeRequest([]byte(e.RequestJSON))
	if r.ResumeFromJobID != nil {
		t.Fatal("unverified legacy offset resumed")
	}
	if _, err = os.Stat(sidecar); err != nil {
		t.Fatal(err)
	}
	for _, cp := range []string{`{"version":2,"backend":"http"}`, `{"version":1,"backend":"other"}`, `{"broken"`} {
		item.TranscriptCheckpointJSON = cp
		if _, err = b.prepare(context.Background(), item); err == nil {
			t.Fatal("unknown checkpoint was dispatched")
		}
	}
}
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestRemoteLeaseRenewalAndMissingOwnerFailClosed(t *testing.T) {
	setupRetentionTestDB(t)
	lock, ok, err := db.TryLock("TranscribePendingEpisodes", 1)
	if err != nil || !ok {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l := &remoteLease{ctx: ctx, cancel: cancel, lock: *lock}
	if err = l.renew(); err != nil {
		t.Fatal(err)
	}
	if err = db.DB.Delete(&db.JobLock{}, "id = ?", lock.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = l.renew(); !errors.Is(err, errRemoteOwnership) || ctx.Err() == nil {
		t.Fatal("missing lock refreshed silently")
	}
}

func TestRemoteCancelledAppWaitLeavesAcceptedJobRunning(t *testing.T) {
	f := newRemoteTestFixture(t)
	started := make(chan struct{})
	release := make(chan struct{})
	f.runner.run = func(ctx context.Context, s whisperxworker.RunSpec, p func(transcription.ProgressUpdate) error) error {
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	b, err := newHTTPTranscriptionBackend(f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- transcribePendingEpisodes(ctx, f.cfg, b) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("worker never started")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	got := loadCharacterizationPodcastItem(t, f.item.ID)
	e, err := decodeRemoteEnvelope(got.TranscriptCheckpointJSON)
	if err != nil {
		t.Fatal(err)
	}
	s, err := f.worker.Status(e.JobID)
	if err != nil || s.State != "running" {
		t.Fatal("app shutdown cancelled accepted execution")
	}
	close(release)
}

func TestRemoteResultValidationAndResponseBounds(t *testing.T) {
	for _, mode := range []string{"digest", "provenance", "malformed", "oversized", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			f := newRemoteTestFixture(t)
			f.cfg.ServiceResultLimit = 4096
			f.intercept = func(rw http.ResponseWriter, r *http.Request) bool {
				if !strings.HasSuffix(r.URL.Path, "/result") {
					return false
				}
				switch mode {
				case "malformed":
					_, _ = rw.Write([]byte(`{"broken"`))
				case "oversized":
					_, _ = rw.Write([]byte(strings.Repeat(" ", 4097)))
				case "redirect":
					http.Redirect(rw, r, "http://127.0.0.1:1/token", 302)
				default:
					rec := httptest.NewRecorder()
					f.worker.Handler().ServeHTTP(rec, r)
					var result transcription.JobResult
					_ = json.Unmarshal(rec.Body.Bytes(), &result)
					if mode == "digest" {
						result.ResultSHA256 = strings.Repeat("0", 64)
					} else {
						var body map[string]any
						_ = json.Unmarshal(result.Transcript, &body)
						body["metadata"].(map[string]any)["input_sha256"] = strings.Repeat("0", 64)
						result.Transcript, _ = transcription.MarshalCanonical(body)
						result.ResultSHA256 = transcription.Digest(result.Transcript)
					}
					_ = json.NewEncoder(rw).Encode(result)
				}
				return true
			}
			if err := f.run(t); err == nil {
				t.Fatal("invalid result committed")
			}
			got := loadCharacterizationPodcastItem(t, f.item.ID)
			if got.TranscriptJSON != "" || f.acks.Load() != 0 {
				t.Fatal("invalid result accepted")
			}
		})
	}
}

func TestRemotePreparedRequestContainsCanonicalDurableIdentity(t *testing.T) {
	f := newRemoteTestFixture(t)
	b, err := newHTTPTranscriptionBackend(f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer b.client.CloseIdleConnections()
	if err = b.Preflight(context.Background()); err != nil {
		t.Fatal(err)
	}
	e, err := b.prepare(context.Background(), f.item)
	if err != nil {
		t.Fatal(err)
	}
	if !transcription.ValidUUID(e.GenerationID) || !transcription.ValidUUID(e.JobID) || e.JobID == uuid.Nil.String() {
		t.Fatal("missing attempt identity")
	}
	if _, err = decodeRemoteEnvelope(string(mustJSON(t, e))); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteTerminalRetryUsesNewAttemptAndManualRepairRearmsPermanentFailure(t *testing.T) {
	for _, retryable := range []bool{true, false} {
		name := "retryable"
		if !retryable {
			name = "manual"
		}
		t.Run(name, func(t *testing.T) {
			f := newRemoteTestFixture(t)
			f.runner.run = func(ctx context.Context, s whisperxworker.RunSpec, p func(transcription.ProgressUpdate) error) error {
				if f.runner.calls.Load() == 1 {
					return transcription.Error("MODEL_UNAVAILABLE", "private diagnostic", retryable)
				}
				return nil
			}
			if err := f.run(t); err == nil {
				t.Fatal("terminal failure not surfaced")
			}
			got := loadCharacterizationPodcastItem(t, f.item.ID)
			old, err := decodeRemoteEnvelope(got.TranscriptCheckpointJSON)
			if err != nil {
				t.Fatal(err)
			}
			if old.TerminalState != "failed" || got.TranscriptRetryCount != 1 || old.TerminalRetryable != retryable {
				t.Fatalf("terminal status lost: %+v", old)
			}
			if !retryable {
				if !old.Permanent || got.TranscriptNextAttempt != nil {
					t.Fatal("permanent failure was automatically retried")
				}
				if err := f.run(t); err != nil {
					t.Fatal("blocked item should be skipped", err)
				}
				if f.runner.calls.Load() != 1 {
					t.Fatal("permanent job was rerun")
				}
				count, err := ForceFailedTranscriptRetriesDue(time.Now().UTC())
				if err != nil || count != 1 {
					t.Fatalf("manual repair failed: %v, %d", err, count)
				}
			} else {
				f.due(t)
			}
			var newID string
			f.intercept = func(rw http.ResponseWriter, r *http.Request) bool {
				if r.Method == "PUT" {
					got := loadCharacterizationPodcastItem(t, f.item.ID)
					e, err := decodeRemoteEnvelope(got.TranscriptCheckpointJSON)
					if err != nil {
						t.Error(err)
					} else {
						newID = e.JobID
						if e.Attempt != 2 || e.GenerationID != old.GenerationID || e.JobID == old.JobID {
							t.Error("incorrect retry identity")
						}
					}
				}
				return false
			}
			if err := f.run(t); err != nil {
				t.Fatal(err)
			}
			if newID == "" || f.runner.calls.Load() != 2 || f.acks.Load() != 1 {
				t.Fatal("retry did not execute exactly once")
			}
		})
	}
}

func TestRemoteLostWorkerStateDoesNotResubmitAcceptedJob(t *testing.T) {
	f := newRemoteTestFixture(t)
	if err := db.DB.Exec("CREATE TRIGGER stage_e_commit_failure BEFORE UPDATE OF transcript_status ON podcast_items WHEN NEW.transcript_status = 'available' BEGIN SELECT RAISE(ABORT, 'injected commit failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := f.run(t); err == nil {
		t.Fatal("expected commit failure")
	}
	f.intercept = func(rw http.ResponseWriter, r *http.Request) bool {
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/jobs/") {
			rw.WriteHeader(404)
			return true
		}
		return false
	}
	if err := f.run(t); err == nil {
		t.Fatal("accepted missing job silently recreated")
	}
	if f.puts.Load() != 1 || f.runner.calls.Load() != 1 || f.acks.Load() != 0 {
		t.Fatal("lost state triggered duplicate work")
	}
}

func TestRemoteInitialCheckpointCommitFailurePreventsSubmission(t *testing.T) {
	f := newRemoteTestFixture(t)
	if err := db.DB.Exec("CREATE TRIGGER stage_e_prepare_failure BEFORE UPDATE OF transcript_checkpoint_json ON podcast_items BEGIN SELECT RAISE(ABORT, 'injected prepare failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := f.run(t); err == nil {
		t.Fatal("checkpoint write failed silently")
	}
	if f.puts.Load() != 0 || f.runner.calls.Load() != 0 {
		t.Fatal("submission preceded durable identity")
	}
}

func TestRemoteStatusRevisionsCannotRegressProgress(t *testing.T) {
	f := newRemoteTestFixture(t)
	var responses atomic.Int32
	f.intercept = func(rw http.ResponseWriter, r *http.Request) bool {
		if r.Method != "GET" || strings.HasSuffix(r.URL.Path, "/result") || !strings.HasPrefix(r.URL.Path, "/v1/jobs/") {
			return false
		}
		current := loadCharacterizationPodcastItem(t, f.item.ID)
		e, err := decodeRemoteEnvelope(current.TranscriptCheckpointJSON)
		if err != nil || e.WorkerInstanceID == nil {
			return false
		}
		count := responses.Add(1)
		if count > 2 {
			return false
		}
		s, err := f.worker.Status(e.JobID)
		if err != nil {
			t.Error(err)
			return false
		}
		if count == 1 {
			s.Revision = 100
			s.State = "running"
			s.Percent = 80
			s.Stage = "transcribing"
		} else {
			s.Revision = 99
			s.State = "running"
			s.Percent = 10
			s.Stage = "starting"
		}
		_ = json.NewEncoder(rw).Encode(s)
		if count == 2 {
			got := loadCharacterizationPodcastItem(t, f.item.ID)
			if got.TranscriptProgressPct != 80 {
				t.Error("newer progress was not persisted")
			}
		}
		return true
	}
	// Let the worker revision catch up only after its deliberately older replies.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	b, err := newHTTPTranscriptionBackend(f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Stall the real worker so the first poll installs a running status.
	f.runner.run = func(ctx context.Context, s whisperxworker.RunSpec, p func(transcription.ProgressUpdate) error) error {
		<-ctx.Done()
		return ctx.Err()
	}
	if err = transcribePendingEpisodes(ctx, f.cfg, b); err == nil {
		t.Fatal("test should stop waiting")
	}
	got := loadCharacterizationPodcastItem(t, f.item.ID)
	if got.TranscriptProgressPct != 80 {
		t.Fatalf("stale status changed progress: %d", got.TranscriptProgressPct)
	}
}

func TestRemoteSummaryWritesPreserveConcurrentEdits(t *testing.T) {
	f := newRemoteTestFixture(t)
	var summaryCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		summaryCalls.Add(1)
		got := loadCharacterizationPodcastItem(t, f.item.ID)
		if got.TranscriptStatus != "available" || got.TranscriptJSON == "" {
			t.Error("summary preceded transcript commit")
		}
		if err := db.DB.Model(&db.PodcastItem{}).Where("id = ?", f.item.ID).Updates(map[string]any{"is_played": true, "title": "Edited during summary"}).Error; err != nil {
			t.Error(err)
		}
		_, _ = rw.Write([]byte(`{"choices":[{"message":{"content":"A safe summary."}}]}`))
	}))
	defer server.Close()
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "synthetic-key")
	t.Setenv("LLM_BASE_URL", server.URL)
	setting := db.GetOrCreateSetting()
	setting.SummarizationEnabled = true
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatal(err)
	}
	if err := f.run(t); err != nil {
		t.Fatal(err)
	}
	got := loadCharacterizationPodcastItem(t, f.item.ID)
	if !got.IsPlayed || got.Title != "Edited during summary" || got.LLMSummary != "A safe summary." || got.LLMSummaryStatus != "available" || summaryCalls.Load() != 1 {
		t.Fatal("summary persistence overwrote library edit")
	}
}

func TestRemoteOutageLeavesQueueUntouchedAndRequiresNoLocalRuntime(t *testing.T) {
	f := newRemoteTestFixture(t)
	f.cfg.Python = filepath.Join(t.TempDir(), "missing-python")
	f.cfg.Script = filepath.Join(t.TempDir(), "missing-script")
	before := snapshotPodcastItemRow(t, f.item.ID)
	f.intercept = func(rw http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/v1/capabilities" {
			rw.WriteHeader(503)
			return true
		}
		return false
	}
	if err := f.run(t); err == nil {
		t.Fatal("outage was not reported")
	}
	assertPodcastItemChangedColumns(t, before, snapshotPodcastItemRow(t, f.item.ID))
	f.intercept = nil
	if err := f.run(t); err != nil {
		t.Fatal("outage recovery required local runtime", err)
	}
}

func TestLocalBackendRejectsRemoteCheckpointWithoutLaunchingPython(t *testing.T) {
	b := localTranscriptionBackend{cfg: WhisperXConfig{Python: "missing-python", Script: "missing-script"}}
	if _, err := b.Transcribe(context.Background(), "audio", `{"version":1,"backend":"http"}`, nil); err == nil || !strings.Contains(err.Error(), "rollback reconciliation") {
		t.Fatal("local adapter consumed remote checkpoint", err)
	}
}

func TestRemoteRetryResumesOnlyVerifiedWorkerCheckpoint(t *testing.T) {
	f := newRemoteTestFixture(t)
	var sourceJob string
	f.runner.run = func(ctx context.Context, s whisperxworker.RunSpec, p func(transcription.ProgressUpdate) error) error {
		if f.runner.calls.Load() == 1 {
			sourceJob = s.JobID
			cp := map[string]any{"segments": []map[string]any{{"start": 0, "end": 1, "text": "Preserved chunk."}}, "completed_seconds": 120, "total_seconds": 240, "language": "en", "worker": map[string]any{"format": transcription.CheckpointFormat, "input_sha256": s.Request.Input.SHA256, "options_sha256": s.Effective.OptionsSHA256, "pipeline_version": transcription.PipelineVersion, "engine_profile": s.Request.EngineProfile, "deployment_id": s.Request.DeploymentID, "episode_id": s.Request.EpisodeID, "generation_id": s.Request.GenerationID}}
			if err := os.WriteFile(filepath.Join(s.Directory, "segments.json"), mustJSON(t, cp), 0600); err != nil {
				return err
			}
			if err := p(transcription.ProgressUpdate{Stage: "transcribing", Percent: 50, CompletedSeconds: 120, TotalSeconds: 240, Checkpoint: json.RawMessage(`{"completed_seconds":120,"total_seconds":240,"language":"en"}`)}); err != nil {
				return err
			}
			return transcription.Error("INTERNAL", "private test error", true)
		}
		if s.Request.ResumeFromJobID == nil || *s.Request.ResumeFromJobID != sourceJob || !strings.Contains(string(s.Resume), "Preserved chunk.") {
			t.Error("verified checkpoint was not resumed")
		}
		return nil
	}
	if err := f.run(t); err == nil {
		t.Fatal("failed chunk not visible")
	}
	f.due(t)
	if err := f.run(t); err != nil {
		t.Fatal(err)
	}
	if f.runner.calls.Load() != 2 {
		t.Fatal("incorrect attempt count")
	}
}

func TestRemotePreparedIdentityDetectsWorkerReplacementAfterLostResponse(t *testing.T) {
	f := newRemoteTestFixture(t)
	var lost atomic.Bool
	f.intercept = func(rw http.ResponseWriter, r *http.Request) bool {
		if r.Method == "PUT" && lost.CompareAndSwap(false, true) {
			f.worker.Handler().ServeHTTP(httptest.NewRecorder(), r)
			dropTestConnection(t, rw)
			return true
		}
		return false
	}
	if err := f.run(t); err == nil {
		t.Fatal("expected lost response")
	}
	f.due(t)
	f.intercept = func(rw http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/v1/capabilities" {
			rec := httptest.NewRecorder()
			f.worker.Handler().ServeHTTP(rec, r)
			var caps transcription.Capabilities
			_ = json.Unmarshal(rec.Body.Bytes(), &caps)
			caps.WorkerInstanceID = uuid.NewString()
			_ = json.NewEncoder(rw).Encode(caps)
			return true
		}
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/jobs/") {
			rw.WriteHeader(404)
			return true
		}
		return false
	}
	if err := f.run(t); err == nil {
		t.Fatal("lost worker state recreated ambiguous attempt")
	}
	if f.puts.Load() != 1 {
		t.Fatal("replacement triggered duplicate submission")
	}
}

func TestRemoteAppMediaValidationPreservesFiles(t *testing.T) {
	for _, mode := range []string{"outside", "directory", "missing", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			f := newRemoteTestFixture(t)
			path := f.item.DownloadPath
			ctx := context.Background()
			switch mode {
			case "outside":
				path = filepath.Join(t.TempDir(), "outside.mp3")
				if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			case "directory":
				path = os.Getenv("DATA")
			case "missing":
				path = filepath.Join(os.Getenv("DATA"), "missing.mp3")
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if _, err := fingerprintRemoteMedia(ctx, os.Getenv("DATA"), path); err == nil {
				t.Fatal("invalid audio accepted")
			}
			if mode == "outside" || mode == "directory" || mode == "cancelled" {
				if _, err := os.Stat(path); err != nil {
					t.Fatal("validation altered file", err)
				}
			}
		})
	}
}

func TestRemoteCompletedLegacyRowsDoNotStarveReceipts(t *testing.T) {
	f := newRemoteTestFixture(t)
	var dropped atomic.Bool
	f.intercept = func(rw http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/ack") && dropped.CompareAndSwap(false, true) {
			f.worker.Handler().ServeHTTP(httptest.NewRecorder(), r)
			dropTestConnection(t, rw)
			return true
		}
		return false
	}
	if err := f.run(t); err == nil {
		t.Fatal("expected lost ACK response")
	}
	f.due(t)
	for i := 0; i < 105; i++ {
		item := db.PodcastItem{PodcastID: f.item.PodcastID, DownloadPath: f.item.DownloadPath, DownloadStatus: db.Downloaded, TranscriptStatus: "available", TranscriptJSON: `{"segments":[{"text":"legacy"}]}`, TranscriptCheckpointJSON: `{"completed_seconds":120}`}
		if err := db.CreatePodcastItem(&item); err != nil {
			t.Fatal(err)
		}
		if err := db.DB.Model(&db.PodcastItem{}).Where("id = ?", item.ID).Update("updated_at", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.run(t); err != nil {
		t.Fatal(err)
	}
	got := loadCharacterizationPodcastItem(t, f.item.ID)
	if got.TranscriptCheckpointJSON != "" || f.acks.Load() != 2 || f.runner.calls.Load() != 1 {
		t.Fatal("completed legacy rows blocked receipt reconciliation")
	}
}

func TestRemoteCorruptReceiptIsRetainedAndDeferredWithoutAck(t *testing.T) {
	f := newRemoteTestFixture(t)
	f.intercept = func(rw http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/ack") {
			rw.WriteHeader(503)
			return true
		}
		return false
	}
	if err := f.run(t); err == nil {
		t.Fatal("expected ACK failure")
	}
	f.due(t)
	got := loadCharacterizationPodcastItem(t, f.item.ID)
	e, err := decodeRemoteEnvelope(got.TranscriptCheckpointJSON)
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("0", 64)
	e.ReceiptResultSHA256 = &digest
	raw := string(mustJSON(t, e))
	if err := db.DB.Model(&db.PodcastItem{}).Where("id = ?", f.item.ID).Update("transcript_checkpoint_json", raw).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.run(t); err == nil {
		t.Fatal("corrupt receipt was accepted")
	}
	got = loadCharacterizationPodcastItem(t, f.item.ID)
	if got.TranscriptStatus != "available" || got.TranscriptCheckpointJSON != raw || got.TranscriptNextAttempt == nil || !got.TranscriptNextAttempt.After(time.Now().UTC()) || f.acks.Load() != 1 || f.runner.calls.Load() != 1 {
		t.Fatal("corrupt receipt lost, ACKed or not deferred")
	}
}

func TestRemoteStructuredPermanentErrorStopsAutomaticDispatch(t *testing.T) {
	f := newRemoteTestFixture(t)
	f.intercept = func(rw http.ResponseWriter, r *http.Request) bool {
		if r.Method == "PUT" {
			rw.WriteHeader(503)
			_, _ = rw.Write([]byte(`{"error":{"code":"MODEL_ACCESS_DENIED","message":"private credential details","retryable":false}}`))
			return true
		}
		return false
	}
	if err := f.run(t); err == nil {
		t.Fatal("permanent model error was swallowed")
	}
	got := loadCharacterizationPodcastItem(t, f.item.ID)
	if got.TranscriptStatus != "failed" || got.TranscriptProgressStage != "blocked" || got.TranscriptRetryCount != 0 || got.TranscriptNextAttempt != nil || strings.Contains(got.TranscriptLastError, "credential") {
		t.Fatal("structured permanent error was retried or leaked")
	}
	if err := f.run(t); err != nil {
		t.Fatal("blocked job should be skipped", err)
	}
	if f.puts.Load() != 1 || f.runner.calls.Load() != 0 {
		t.Fatal("permanent submission was repeated")
	}
}

func TestRemoteInterruptedResultTransferReconnectsWithoutInferenceRetry(t *testing.T) {
	f := newRemoteTestFixture(t)
	var interrupted atomic.Bool
	f.intercept = func(rw http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/result") && interrupted.CompareAndSwap(false, true) {
			rec := httptest.NewRecorder()
			f.worker.Handler().ServeHTTP(rec, r)
			rw.Header().Set("Content-Length", strconv.Itoa(rec.Body.Len()))
			rw.WriteHeader(200)
			_, _ = rw.Write(rec.Body.Bytes()[:rec.Body.Len()/2])
			return true
		}
		return false
	}
	if err := f.run(t); err == nil {
		t.Fatal("partial transfer was not surfaced")
	}
	got := loadCharacterizationPodcastItem(t, f.item.ID)
	e, err := decodeRemoteEnvelope(got.TranscriptCheckpointJSON)
	if err != nil {
		t.Fatal(err)
	}
	if got.TranscriptStatus != "processing" || got.TranscriptRetryCount != 0 || e.Permanent || f.acks.Load() != 0 {
		t.Fatal("partial result became a permanent or inference failure")
	}
	f.due(t)
	if err := f.run(t); err != nil {
		t.Fatal(err)
	}
	if f.runner.calls.Load() != 1 || f.puts.Load() != 1 || f.acks.Load() != 1 {
		t.Fatal("result reconnect repeated inference")
	}
}
