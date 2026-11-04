package whisperxworker

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/internal/transcription"
	"github.com/google/uuid"
)

func stubGPU(t *testing.T, out string) {
	t.Helper()
	orig := queryGPU
	queryGPU = func(context.Context) ([]byte, error) { return []byte(out), nil }
	t.Cleanup(func() { queryGPU = orig })
}

func scrape(t *testing.T, w *Worker) string {
	t.Helper()
	got := callHTTP(w, "GET", "/v1/metrics", nil, strings.Repeat("t", 32))
	if got.Code != 200 {
		t.Fatalf("metrics status=%d body=%s", got.Code, got.Body)
	}
	return got.Body.String()
}

func requireLines(t *testing.T, body string, lines ...string) {
	t.Helper()
	for _, line := range lines {
		if !strings.Contains(body, line) {
			t.Errorf("metrics missing %q", line)
		}
	}
}

func TestWorkerMetricsRecordJobLifecycleBehindToken(t *testing.T) {
	stubGPU(t, "0, 3000, 12282, 45\n")
	cfg, request, output := workerFixture(t)
	runner := fakeRunner{run: func(ctx context.Context, s RunSpec, p func(transcription.ProgressUpdate) error) error {
		for _, stage := range []string{"model_loaded", "transcribing", "aligning"} {
			if err := p(transcription.ProgressUpdate{Stage: stage, Percent: 50, TotalSeconds: 120}); err != nil {
				return err
			}
			time.Sleep(5 * time.Millisecond)
		}
		return durableWrite(filepath.Join(s.Directory, "stdout.json"), output)
	}}
	w := openTestWorker(t, cfg, runner)
	if got := callHTTP(w, "GET", "/v1/metrics", nil, "wrong"); got.Code != 401 {
		t.Fatalf("metrics must require the worker token, got %d", got.Code)
	}

	id := uuid.NewString()
	submitTest(t, w, id, request)
	waitState(t, w, id, "succeeded")

	body := scrape(t, w)
	requireLines(t, body,
		"whisperx_jobs_accepted_total 1",
		"whisperx_job_runs_started_total 1",
		`whisperx_jobs_finished_total{error_code="none",state="succeeded"} 1`,
		"whisperx_job_queue_wait_seconds_count 1",
		`whisperx_job_run_seconds_count{state="succeeded"} 1`,
		`whisperx_job_stage_seconds_count{stage="starting"} 1`,
		`whisperx_job_stage_seconds_count{stage="model_loaded"} 1`,
		`whisperx_job_stage_seconds_count{stage="transcribing"} 1`,
		`whisperx_job_stage_seconds_count{stage="aligning"} 1`,
		"whisperx_transcribed_audio_seconds_total 120",
		"whisperx_job_speed_ratio_count 1",
		`whisperx_jobs{state="running"} 0`,
		"whisperx_worker_healthy 1",
		"whisperx_worker_accepting 1",
		`whisperx_worker_capacity{kind="running"} 1`,
		`whisperx_gpu_memory_used_bytes{gpu="0"} 3.145728e+09`,
		`whisperx_gpu_utilization_ratio{gpu="0"} 0.45`,
		`engine_profile="`+cfg.EngineProfile+`"`,
	)
	if strings.Contains(body, id) {
		t.Fatalf("job IDs must not appear in metric labels")
	}
}

func TestWorkerMetricsCountFailureCodes(t *testing.T) {
	stubGPU(t, "")
	cfg, request, _ := workerFixture(t)
	runner := fakeRunner{run: func(ctx context.Context, s RunSpec, p func(transcription.ProgressUpdate) error) error {
		return transcription.Error("MODEL_UNAVAILABLE", "missing model", true)
	}}
	w := openTestWorker(t, cfg, runner)
	id := uuid.NewString()
	submitTest(t, w, id, request)
	waitState(t, w, id, "failed")
	body := scrape(t, w)
	requireLines(t, body, `whisperx_jobs_finished_total{error_code="MODEL_UNAVAILABLE",state="failed"} 1`,
		`whisperx_job_run_seconds_count{state="failed"} 1`)
	if strings.Contains(body, "whisperx_gpu_memory_used_bytes{") {
		t.Fatalf("no GPU series expected when nvidia-smi reports nothing")
	}
	if strings.Contains(body, "whisperx_job_speed_ratio_count 1") {
		t.Fatalf("failed runs must not contribute to speed")
	}
}

func TestGPUQueryParsingAndLabelBounds(t *testing.T) {
	got := parseGPUQuery("0, 1024, 2048, 50\n1, 10, 20, 0\nbad line\n2, x, 1, 1\n")
	if got == nil || len(got.devices) != 2 || got.devices[0].usedBytes != 1024*1024*1024 || got.devices[0].utilization != 0.5 || got.devices[1].index != "1" {
		t.Fatalf("unexpected GPU parse: %+v", got)
	}
	if parseGPUQuery("No devices were found") != nil {
		t.Fatalf("expected nil reading for non-numeric output")
	}
	if stageLabel("transcribing") != "transcribing" || stageLabel("chunk 17 of 40") != "other" {
		t.Fatalf("stage labels must be bounded")
	}
	if errorCodeLabel(nil) != "none" || errorCodeLabel(transcription.Error("TIMEOUT", "x", true)) != "TIMEOUT" ||
		errorCodeLabel(&transcription.WorkerError{Code: "free text with spaces"}) != "OTHER" {
		t.Fatalf("error code labels must be bounded")
	}
}
