package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/telemetry"
	"github.com/ctaylor1/briefcast/internal/transcription"
	"github.com/prometheus/common/expfmt"
)

func seedTelemetryItems(t *testing.T, now time.Time) {
	t.Helper()
	podcast := db.Podcast{Title: "Telemetry Show", URL: "https://feeds.example/telemetry"}
	if err := db.DB.Create(&podcast).Error; err != nil {
		t.Fatalf("create podcast: %v", err)
	}
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	recent, old := ago(time.Hour), ago(10*24*time.Hour)
	items := []db.PodcastItem{
		{Title: "recent transcript", GUID: "g1", FileURL: "https://media.example/1.mp3", DownloadStatus: db.Downloaded, DownloadDate: ago(2 * time.Hour),
			DownloadPath: "/assets/1.mp3", TranscriptStatus: "available", TranscriptJSON: "{}", CanonicalUpdatedAt: &recent, Duration: 600},
		{Title: "waiting three days", GUID: "g2", FileURL: "https://media.example/2.mp3", DownloadStatus: db.Downloaded, DownloadDate: ago(72 * time.Hour),
			DownloadPath: "/assets/2.mp3", TranscriptStatus: "pending_whisperx"},
		{Title: "blocked", GUID: "g3", FileURL: "https://media.example/3.mp3", DownloadStatus: db.Downloaded, DownloadDate: ago(48 * time.Hour),
			DownloadPath: "/assets/3.mp3", TranscriptStatus: "failed", TranscriptProgressStage: "blocked"},
		{Title: "not downloaded", GUID: "g4", FileURL: "https://media.example/4.mp3", DownloadStatus: db.NotDownloaded, TranscriptStatus: "pending_whisperx"},
		{Title: "old transcript", GUID: "g5", FileURL: "https://media.example/5.mp3", DownloadStatus: db.Downloaded, DownloadDate: old,
			DownloadPath: "/assets/5.mp3", TranscriptStatus: "available", TranscriptJSON: "{}", CanonicalUpdatedAt: &old, Duration: 900},
		// Soft-deleted rows are ignored everywhere.
		{Base: db.Base{DeletedAt: &recent}, Title: "deleted", GUID: "g6", FileURL: "https://media.example/6.mp3", DownloadStatus: db.Downloaded,
			DownloadDate: ago(100 * time.Hour), DownloadPath: "/assets/6.mp3", TranscriptStatus: "pending_whisperx"},
	}
	for i := range items {
		items[i].PodcastID = podcast.ID
		if err := db.DB.Create(&items[i]).Error; err != nil {
			t.Fatalf("create item: %v", err)
		}
	}
}

func TestQueueSnapshotWindowsAndPipelineGauges(t *testing.T) {
	setupRetentionTestDB(t)
	t.Setenv("WHISPERX_ENABLED", "false")
	now := time.Now().UTC()
	seedTelemetryItems(t, now)

	q, err := loadQueueSnapshot(now)
	if err != nil {
		t.Fatalf("queue snapshot: %v", err)
	}
	if q.EpisodesByDownloadState["downloaded"] != 4 || q.EpisodesByDownloadState["not_downloaded"] != 1 {
		t.Fatalf("unexpected download states: %v", q.EpisodesByDownloadState)
	}
	want := map[string]int64{"available": 2, "pending_whisperx": 1, "blocked": 1, "failed": 0, "processing": 0}
	for state, n := range want {
		if q.TranscriptsByState[state] != n {
			t.Fatalf("transcripts[%s]=%d want %d (all: %v)", state, q.TranscriptsByState[state], n, q.TranscriptsByState)
		}
	}
	if q.OldestPendingSeconds < 71*3600 || q.OldestPendingSeconds > 73*3600 {
		t.Fatalf("oldest pending should be about 72h, got %.0fs", q.OldestPendingSeconds)
	}

	snap, err := BuildMonitoringSnapshot(context.Background())
	if err != nil {
		t.Fatalf("monitoring snapshot: %v", err)
	}
	if len(snap.Windows) != 2 {
		t.Fatalf("expected two windows, got %+v", snap.Windows)
	}
	day, week := snap.Windows[0], snap.Windows[1]
	if day.Downloads != 1 || day.Transcripts != 1 || day.TranscribedAudioSeconds != 600 {
		t.Fatalf("unexpected 24h window: %+v", day)
	}
	if week.Downloads != 3 || week.Transcripts != 1 {
		t.Fatalf("unexpected 7d window: %+v", week)
	}
	if snap.Worker.Configured || snap.Worker.Reachable {
		t.Fatalf("worker should be reported as not configured: %+v", snap.Worker)
	}

	RegisterPipelineMetrics()
	families, err := telemetry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	found := false
	for _, f := range families {
		if f.GetName() != "briefcast_downloaded_episode_transcripts" {
			continue
		}
		for _, m := range f.GetMetric() {
			if metricLabel(m, "state") == "blocked" && m.GetGauge().GetValue() == 1 {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected blocked transcript gauge from the database collector")
	}
}

const sampleWorkerMetrics = `# TYPE whisperx_worker_healthy gauge
whisperx_worker_healthy 1
# TYPE whisperx_worker_accepting gauge
whisperx_worker_accepting 1
# TYPE whisperx_jobs gauge
whisperx_jobs{state="queued"} 2
whisperx_jobs{state="running"} 1
# TYPE whisperx_worker_capacity gauge
whisperx_worker_capacity{kind="running"} 1
whisperx_worker_capacity{kind="waiting"} 4
# TYPE whisperx_worker_info gauge
whisperx_worker_info{compute_type="float16",device="cuda",engine_profile="baseline-wx3.8.6-cu128-v1",pipeline_version="v1",worker_version="dev"} 1
# TYPE whisperx_jobs_finished_total counter
whisperx_jobs_finished_total{error_code="none",state="succeeded"} 7
whisperx_jobs_finished_total{error_code="TIMEOUT",state="failed"} 1
# TYPE whisperx_transcribed_audio_seconds_total counter
whisperx_transcribed_audio_seconds_total 25200
# TYPE whisperx_job_speed_ratio histogram
whisperx_job_speed_ratio_bucket{le="+Inf"} 7
whisperx_job_speed_ratio_sum 266
whisperx_job_speed_ratio_count 7
# TYPE whisperx_job_stage_seconds histogram
whisperx_job_stage_seconds_bucket{stage="transcribing",le="+Inf"} 7
whisperx_job_stage_seconds_sum{stage="transcribing"} 700
whisperx_job_stage_seconds_count{stage="transcribing"} 7
# TYPE whisperx_gpu_memory_used_bytes gauge
whisperx_gpu_memory_used_bytes{gpu="0"} 9.0e+09
# TYPE whisperx_gpu_memory_total_bytes gauge
whisperx_gpu_memory_total_bytes{gpu="0"} 1.2e+10
# TYPE whisperx_gpu_utilization_ratio gauge
whisperx_gpu_utilization_ratio{gpu="0"} 0.8
`

func TestWorkerMetricsFetchAndSummary(t *testing.T) {
	setupRetentionTestDB(t)
	token := strings.Repeat("k", 40)
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte(token), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/metrics" || r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", string(expfmt.NewFormat(expfmt.TypeTextPlain)))
		_, _ = w.Write([]byte(sampleWorkerMetrics))
	}))
	defer server.Close()
	t.Setenv("WHISPERX_ENABLED", "true")
	t.Setenv("WHISPERX_BACKEND", "http")
	t.Setenv("WHISPERX_SERVICE_URL", server.URL)
	t.Setenv("WHISPERX_SERVICE_TOKEN_FILE", tokenFile)

	raw, contentType, err := FetchWorkerMetrics(context.Background())
	if err != nil || !strings.Contains(string(raw), "whisperx_worker_healthy 1") || !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("fetch: err=%v type=%q", err, contentType)
	}
	w := summarizeWorker(context.Background())
	if !w.Configured || !w.Reachable || !w.Healthy || !w.Accepting || w.Running != 1 || w.Queued != 2 || w.MaxRunning != 1 {
		t.Fatalf("unexpected worker summary: %+v", w)
	}
	if w.EngineProfile != "baseline-wx3.8.6-cu128-v1" || w.Device != "cuda" || w.JobsFinished["succeeded"] != 7 || w.JobsFinished["failed"] != 1 {
		t.Fatalf("unexpected worker identity or counts: %+v", w)
	}
	if w.AvgSpeedRatio != 38 || w.AvgStageSeconds["transcribing"] != 100 || w.TranscribedAudioSec != 25200 {
		t.Fatalf("unexpected worker averages: %+v", w)
	}
	if len(w.GPUs) != 1 || w.GPUs[0].MemoryTotalBytes != 1.2e10 || w.GPUs[0].Utilization != 0.8 {
		t.Fatalf("unexpected GPU summary: %+v", w.GPUs)
	}

	t.Setenv("WHISPERX_SERVICE_TOKEN_FILE", filepath.Join(t.TempDir(), "missing"))
	if got := summarizeWorker(context.Background()); !got.Configured || got.Reachable || got.Error == "" {
		t.Fatalf("an unreadable token must report the worker as unreachable: %+v", got)
	}
}

func TestTranscriptionProblemOutcomesAndTimings(t *testing.T) {
	before := telemetry.Summary().Transcriptions
	item := &db.PodcastItem{Base: db.Base{ID: "item-1"}}
	env := &remoteEnvelope{JobID: "job-1"}
	recordTranscriptionProblem(item, env, remoteProblem("BUSY", "busy", true), "processing", "reconnecting", 0, nil)
	recordTranscriptionProblem(item, env, remoteProblem("TIMEOUT", "slow", true), "failed", "failed", 1, nil)
	recordTranscriptionProblem(item, nil, remoteProblem("INVALID_CONFIG", "bad", false), "failed", "blocked", 1, nil)
	after := telemetry.Summary().Transcriptions
	for outcome, want := range map[string]float64{telemetry.OutcomeRetrying: 1, telemetry.OutcomeFailed: 1, telemetry.OutcomeBlocked: 1} {
		if got := after[outcome] - before[outcome]; got != want {
			t.Fatalf("%s delta=%v want %v", outcome, got, want)
		}
	}

	created := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	started, finished := created.Add(30*time.Second), created.Add(90*time.Second)
	queue, processing := transcriptionTimings(transcription.JobStatus{CreatedAt: created, StartedAt: &started, FinishedAt: &finished})
	if queue != 30*time.Second || processing != time.Minute {
		t.Fatalf("queue=%s processing=%s", queue, processing)
	}
	if q, p := transcriptionTimings(transcription.JobStatus{CreatedAt: created}); q != 0 || p != 0 {
		t.Fatalf("missing timestamps must yield zero durations")
	}

	if downloadOutcome(nil) != telemetry.OutcomeSuccess || downloadOutcome(ErrDownloadPaused) != telemetry.OutcomePaused ||
		downloadOutcome(ErrDownloadCancelled) != telemetry.OutcomeCancelled || downloadOutcome(os.ErrNotExist) != telemetry.OutcomeFailed {
		t.Fatalf("unexpected download outcome mapping")
	}
}
