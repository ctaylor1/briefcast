package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/logging"
	"github.com/ctaylor1/briefcast/internal/telemetry"
	"github.com/ctaylor1/briefcast/internal/transcription"
)

// ---- Database-backed gauges --------------------------------------------------

var knownTranscriptStates = map[string]bool{"available": true, "pending_whisperx": true, "processing": true, "failed": true}

var downloadStateNames = map[db.DownloadStatus]string{
	db.NotDownloaded: "not_downloaded", db.Downloading: "downloading", db.Downloaded: "downloaded",
	db.Deleted: "skipped_or_deleted", db.Paused: "paused",
}

var (
	descEpisodes = prometheus.NewDesc("briefcast_episodes",
		"Episodes in the library by download state.", []string{"download_state"}, nil)
	descTranscripts = prometheus.NewDesc("briefcast_downloaded_episode_transcripts",
		"Downloaded episodes by transcript state (blocked = failed and not retried automatically).", []string{"state"}, nil)
	descBacklogAge = prometheus.NewDesc("briefcast_transcript_backlog_oldest_seconds",
		"Age of the oldest downloaded episode still waiting for a transcript.", nil, nil)
	descAssetsFree = prometheus.NewDesc("briefcast_assets_free_bytes",
		"Free bytes on the volume holding audio and exports.", nil, nil)
)

type pipelineCollector struct{}

func (pipelineCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- descEpisodes
	ch <- descTranscripts
	ch <- descBacklogAge
	ch <- descAssetsFree
}

func (pipelineCollector) Collect(ch chan<- prometheus.Metric) {
	q, err := loadQueueSnapshot(time.Now().UTC())
	if err == nil {
		for state, n := range q.EpisodesByDownloadState {
			ch <- prometheus.MustNewConstMetric(descEpisodes, prometheus.GaugeValue, float64(n), state)
		}
		for state, n := range q.TranscriptsByState {
			ch <- prometheus.MustNewConstMetric(descTranscripts, prometheus.GaugeValue, float64(n), state)
		}
		ch <- prometheus.MustNewConstMetric(descBacklogAge, prometheus.GaugeValue, q.OldestPendingSeconds)
	}
	if free, ferr := availableBytes(getEnvString("DATA", "/assets")); ferr == nil {
		ch <- prometheus.MustNewConstMetric(descAssetsFree, prometheus.GaugeValue, float64(free))
	}
}

var registerPipelineOnce sync.Once

// RegisterPipelineMetrics adds the database-backed gauges to the app registry.
func RegisterPipelineMetrics() {
	registerPipelineOnce.Do(func() { telemetry.MustRegister(pipelineCollector{}) })
}

// QueueSnapshot is the current state of the download and transcription queues.
type QueueSnapshot struct {
	EpisodesByDownloadState map[string]int64 `json:"episodes_by_download_state"`
	TranscriptsByState      map[string]int64 `json:"transcripts_by_state"`
	OldestPendingSeconds    float64          `json:"oldest_pending_seconds"`
}

func loadQueueSnapshot(now time.Time) (QueueSnapshot, error) {
	out := QueueSnapshot{EpisodesByDownloadState: map[string]int64{}, TranscriptsByState: map[string]int64{}}
	if db.DB == nil {
		return out, errors.New("database unavailable")
	}
	var byDownload []struct {
		DownloadStatus db.DownloadStatus
		N              int64
	}
	if err := db.DB.Model(&db.PodcastItem{}).Where("deleted_at IS NULL").Select("download_status, COUNT(*) AS n").Group("download_status").Scan(&byDownload).Error; err != nil {
		return out, err
	}
	for _, row := range byDownload {
		name, ok := downloadStateNames[row.DownloadStatus]
		if !ok {
			name = "other"
		}
		out.EpisodesByDownloadState[name] += row.N
	}
	var byTranscript []struct {
		TranscriptStatus        string
		TranscriptProgressStage string
		N                       int64
	}
	if err := db.DB.Model(&db.PodcastItem{}).Where("deleted_at IS NULL").Where("download_status = ?", db.Downloaded).
		Select("transcript_status, CASE WHEN transcript_progress_stage = 'blocked' THEN 'blocked' ELSE '' END AS transcript_progress_stage, COUNT(*) AS n").
		Group("transcript_status, CASE WHEN transcript_progress_stage = 'blocked' THEN 'blocked' ELSE '' END").Scan(&byTranscript).Error; err != nil {
		return out, err
	}
	for _, state := range []string{"available", "pending_whisperx", "processing", "failed", "blocked"} {
		out.TranscriptsByState[state] = 0
	}
	for _, row := range byTranscript {
		state := row.TranscriptStatus
		switch {
		case state == "failed" && row.TranscriptProgressStage == "blocked":
			state = "blocked"
		case state == "":
			state = "none"
		case !knownTranscriptStates[state]:
			state = "other"
		}
		out.TranscriptsByState[state] += row.N
	}
	// Read the typed column of the oldest row: SQLite returns MIN(datetime) as text.
	var oldest []db.PodcastItem
	if err := db.DB.Model(&db.PodcastItem{}).Where("deleted_at IS NULL").Select("id, download_date").
		Where("download_status = ? AND transcript_status IN ? AND COALESCE(transcript_json, '') = ''", db.Downloaded, []string{"pending_whisperx", "processing"}).
		Order("download_date ASC").Limit(1).Find(&oldest).Error; err != nil {
		return out, err
	}
	if len(oldest) == 1 && !oldest[0].DownloadDate.IsZero() {
		out.OldestPendingSeconds = max(now.Sub(oldest[0].DownloadDate).Seconds(), 0)
	}
	return out, nil
}

// ---- Structured events -------------------------------------------------------

func downloadOutcome(err error) string {
	switch {
	case err == nil:
		return telemetry.OutcomeSuccess
	case errors.Is(err, ErrDownloadCancelled):
		return telemetry.OutcomeCancelled
	case errors.Is(err, ErrDownloadPaused):
		return telemetry.OutcomePaused
	default:
		return telemetry.OutcomeFailed
	}
}

// recordEpisodeDownload logs and counts one download attempt.
func recordEpisodeDownload(item *db.PodcastItem, outcome string, start time.Time, filePath string) {
	elapsed := time.Since(start)
	var size int64
	if outcome == telemetry.OutcomeSuccess && filePath != "" {
		if info, err := os.Stat(filePath); err == nil {
			size = info.Size()
		}
	}
	telemetry.ObserveDownload(outcome, elapsed, size)
	if outcome != telemetry.OutcomeSuccess {
		return // failures, pauses and cancellations keep their existing log lines
	}
	host := ""
	if u, err := url.Parse(item.FileURL); err == nil {
		host = u.Hostname()
	}
	logging.Sugar().Infow("episode_download_completed", "podcast_item_id", item.ID, "podcast_id", item.PodcastID,
		"bytes", size, "duration_ms", elapsed.Milliseconds(), "host", host)
}

// transcriptionTimings derives worker-side timings from the final job status.
func transcriptionTimings(s transcription.JobStatus) (queue, processing time.Duration) {
	if s.StartedAt != nil && !s.CreatedAt.IsZero() {
		queue = max(s.StartedAt.Sub(s.CreatedAt), 0)
	}
	if s.StartedAt != nil && s.FinishedAt != nil {
		processing = max(s.FinishedAt.Sub(*s.StartedAt), 0)
	}
	return queue, processing
}

func recordTranscriptionStarted(item *db.PodcastItem, e *remoteEnvelope, stage string) {
	logging.Sugar().Infow("transcription_started", "podcast_item_id", item.ID, "job_id", e.JobID,
		"attempt", e.Attempt, "stage", stage, "retry_count", item.TranscriptRetryCount)
}

func recordTranscriptionCompleted(item *db.PodcastItem, e *remoteEnvelope, last transcription.JobStatus, started time.Time) {
	queue, processing := transcriptionTimings(last)
	endToEnd := time.Since(started)
	audio := last.TotalSeconds
	if audio <= 0 && item.Duration > 0 {
		audio = float64(item.Duration)
	}
	telemetry.ObserveTranscriptionCompleted(telemetry.TranscriptionSample{EndToEnd: endToEnd, WorkerQueue: queue, Processing: processing, AudioSeconds: audio})
	speed := 0.0
	if processing > 0 && audio > 0 {
		speed = audio / processing.Seconds()
	}
	logging.Sugar().Infow("transcription_completed", "podcast_item_id", item.ID, "podcast_id", item.PodcastID, "job_id", e.JobID,
		"attempt", e.Attempt, "audio_seconds", audio, "worker_queue_ms", queue.Milliseconds(), "processing_ms", processing.Milliseconds(),
		"end_to_end_ms", endToEnd.Milliseconds(), "speed_x", fmt.Sprintf("%.1f", speed),
		"resumed_from_checkpoint", last.Checkpoint.ResumeFromJobID != nil || last.Checkpoint.CompletedSeconds > 0)
}

func recordTranscriptionProblem(item *db.PodcastItem, e *remoteEnvelope, problem *remoteError, status, stage string, retries int, next *time.Time) {
	outcome := telemetry.OutcomeRetrying
	switch {
	case stage == "blocked":
		outcome = telemetry.OutcomeBlocked
	case status == "failed":
		outcome = telemetry.OutcomeFailed
	}
	telemetry.ObserveTranscriptionProblem(outcome)
	jobID := ""
	if e != nil {
		jobID = e.JobID
	}
	fields := []any{"podcast_item_id", item.ID, "job_id", jobID, "outcome", outcome, "code", problem.problem.Code,
		"retryable", problem.problem.Retryable, "http_status", problem.status, "retry_count", retries}
	if next != nil {
		fields = append(fields, "next_attempt", next.UTC().Format(time.RFC3339))
	}
	if outcome == telemetry.OutcomeRetrying {
		logging.Sugar().Infow("transcription_retry_scheduled", fields...)
		return
	}
	logging.Sugar().Warnw("transcription_failed", fields...)
}

// ---- Worker metrics passthrough ------------------------------------------------

const workerMetricsLimit = 4 << 20

// ErrWorkerMetricsUnavailable means the split WhisperX worker is not configured.
var ErrWorkerMetricsUnavailable = errors.New("whisperx worker metrics unavailable")

// FetchWorkerMetrics returns the worker's Prometheus exposition using the private token.
func FetchWorkerMetrics(ctx context.Context) ([]byte, string, error) {
	cfg := LoadWhisperXConfig()
	if !cfg.Enabled || cfg.Backend != "http" {
		return nil, "", ErrWorkerMetricsUnavailable
	}
	b, err := newHTTPTranscriptionBackend(cfg)
	if err != nil {
		return nil, "", err
	}
	defer b.client.CloseIdleConnections()
	u := *b.base
	u.Path = "/v1/metrics"
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+b.token)
	req.Header.Set("Accept", string(expfmt.NewFormat(expfmt.TypeTextPlain)))
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("worker metrics request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("worker metrics returned HTTP %d", resp.StatusCode)
	}
	raw, err := readRemoteBounded(resp.Body, workerMetricsLimit)
	if err != nil {
		return nil, "", err
	}
	return raw, resp.Header.Get("Content-Type"), nil
}

// ---- Monitoring summary for the Settings panel ---------------------------------

// MonitoringWindow counts completed work in a recent time window.
type MonitoringWindow struct {
	Label                   string  `json:"label"`
	Hours                   int     `json:"hours"`
	Downloads               int64   `json:"downloads"`
	Transcripts             int64   `json:"transcripts"`
	TranscribedAudioSeconds float64 `json:"transcribed_audio_seconds"`
}

// WorkerGPU is one GPU reported by the worker.
type WorkerGPU struct {
	Index            string  `json:"index"`
	MemoryUsedBytes  float64 `json:"memory_used_bytes"`
	MemoryTotalBytes float64 `json:"memory_total_bytes"`
	Utilization      float64 `json:"utilization"`
}

// WorkerSummary is the subset of worker metrics shown in the panel.
type WorkerSummary struct {
	Configured          bool               `json:"configured"`
	Reachable           bool               `json:"reachable"`
	Error               string             `json:"error,omitempty"`
	Healthy             bool               `json:"healthy"`
	Accepting           bool               `json:"accepting"`
	Running             float64            `json:"running"`
	Queued              float64            `json:"queued"`
	MaxRunning          float64            `json:"max_running"`
	EngineProfile       string             `json:"engine_profile,omitempty"`
	Device              string             `json:"device,omitempty"`
	StateFreeBytes      float64            `json:"state_free_bytes"`
	JobsFinished        map[string]float64 `json:"jobs_finished"`
	AvgSpeedRatio       float64            `json:"avg_speed_ratio"`
	AvgStageSeconds     map[string]float64 `json:"avg_stage_seconds"`
	TranscribedAudioSec float64            `json:"transcribed_audio_seconds"`
	GPUs                []WorkerGPU        `json:"gpus"`
}

// MonitoringSnapshot is the payload of GET /settings/monitoring.
type MonitoringSnapshot struct {
	GeneratedAt     time.Time            `json:"generated_at"`
	Queue           QueueSnapshot        `json:"queue"`
	Windows         []MonitoringWindow   `json:"windows"`
	SinceStart      telemetry.SinceStart `json:"since_start"`
	Worker          WorkerSummary        `json:"worker"`
	AssetsFreeBytes *uint64              `json:"assets_free_bytes"`
}

// BuildMonitoringSnapshot gathers queue state, recent throughput and worker health.
func BuildMonitoringSnapshot(ctx context.Context) (MonitoringSnapshot, error) {
	now := time.Now().UTC()
	snap := MonitoringSnapshot{GeneratedAt: now, SinceStart: telemetry.Summary()}
	queue, err := loadQueueSnapshot(now)
	if err != nil {
		return snap, err
	}
	snap.Queue = queue
	for _, w := range []struct {
		label string
		hours int
	}{{"Last 24 hours", 24}, {"Last 7 days", 24 * 7}} {
		window, werr := loadWindow(now, w.label, w.hours)
		if werr != nil {
			return snap, werr
		}
		snap.Windows = append(snap.Windows, window)
	}
	if free, ferr := availableBytes(getEnvString("DATA", "/assets")); ferr == nil {
		snap.AssetsFreeBytes = &free
	}
	snap.Worker = summarizeWorker(ctx)
	return snap, nil
}

func loadWindow(now time.Time, label string, hours int) (MonitoringWindow, error) {
	since := now.Add(-time.Duration(hours) * time.Hour)
	out := MonitoringWindow{Label: label, Hours: hours}
	if err := db.DB.Model(&db.PodcastItem{}).Where("deleted_at IS NULL").Where("download_status = ? AND download_date >= ?", db.Downloaded, since).Count(&out.Downloads).Error; err != nil {
		return out, err
	}
	var agg struct {
		N     int64
		Audio float64
	}
	if err := db.DB.Model(&db.PodcastItem{}).Where("deleted_at IS NULL").Select("COUNT(*) AS n, COALESCE(SUM(duration), 0) AS audio").
		Where("transcript_status = ? AND canonical_updated_at >= ? AND COALESCE(transcript_json, '') <> ''", "available", since).
		Scan(&agg).Error; err != nil {
		return out, err
	}
	out.Transcripts, out.TranscribedAudioSeconds = agg.N, agg.Audio
	return out, nil
}

func summarizeWorker(ctx context.Context) WorkerSummary {
	out := WorkerSummary{JobsFinished: map[string]float64{}, AvgStageSeconds: map[string]float64{}, GPUs: []WorkerGPU{}}
	raw, _, err := FetchWorkerMetrics(ctx)
	if errors.Is(err, ErrWorkerMetricsUnavailable) {
		return out
	}
	out.Configured = true
	if err != nil {
		out.Error = "Worker metrics unavailable"
		return out
	}
	var parser expfmt.TextParser
	families, err := parser.TextToMetricFamilies(strings.NewReader(string(raw)))
	if err != nil {
		out.Error = "Worker metrics could not be parsed"
		return out
	}
	out.Reachable = true
	applyWorkerFamilies(&out, families)
	return out
}

func applyWorkerFamilies(out *WorkerSummary, families map[string]*dto.MetricFamily) {
	gauge := func(name string) []*dto.Metric {
		if f := families[name]; f != nil {
			return f.GetMetric()
		}
		return nil
	}
	for _, m := range gauge("whisperx_worker_healthy") {
		out.Healthy = m.GetGauge().GetValue() == 1
	}
	for _, m := range gauge("whisperx_worker_accepting") {
		out.Accepting = m.GetGauge().GetValue() == 1
	}
	for _, m := range gauge("whisperx_jobs") {
		switch metricLabel(m, "state") {
		case "running":
			out.Running = m.GetGauge().GetValue()
		case "queued":
			out.Queued = m.GetGauge().GetValue()
		}
	}
	for _, m := range gauge("whisperx_worker_capacity") {
		if metricLabel(m, "kind") == "running" {
			out.MaxRunning = m.GetGauge().GetValue()
		}
	}
	for _, m := range gauge("whisperx_worker_info") {
		out.EngineProfile, out.Device = metricLabel(m, "engine_profile"), metricLabel(m, "device")
	}
	for _, m := range gauge("whisperx_state_free_bytes") {
		out.StateFreeBytes = m.GetGauge().GetValue()
	}
	if f := families["whisperx_jobs_finished_total"]; f != nil {
		for _, m := range f.GetMetric() {
			out.JobsFinished[metricLabel(m, "state")] += m.GetCounter().GetValue()
		}
	}
	if f := families["whisperx_transcribed_audio_seconds_total"]; f != nil {
		for _, m := range f.GetMetric() {
			out.TranscribedAudioSec += m.GetCounter().GetValue()
		}
	}
	if f := families["whisperx_job_speed_ratio"]; f != nil {
		for _, m := range f.GetMetric() {
			if c := m.GetHistogram().GetSampleCount(); c > 0 {
				out.AvgSpeedRatio = m.GetHistogram().GetSampleSum() / float64(c)
			}
		}
	}
	if f := families["whisperx_job_stage_seconds"]; f != nil {
		for _, m := range f.GetMetric() {
			if c := m.GetHistogram().GetSampleCount(); c > 0 {
				out.AvgStageSeconds[metricLabel(m, "stage")] = m.GetHistogram().GetSampleSum() / float64(c)
			}
		}
	}
	gpus := map[string]*WorkerGPU{}
	gpuFor := func(m *dto.Metric) *WorkerGPU {
		index := metricLabel(m, "gpu")
		if gpus[index] == nil {
			gpus[index] = &WorkerGPU{Index: index}
		}
		return gpus[index]
	}
	for _, m := range gauge("whisperx_gpu_memory_used_bytes") {
		gpuFor(m).MemoryUsedBytes = m.GetGauge().GetValue()
	}
	for _, m := range gauge("whisperx_gpu_memory_total_bytes") {
		gpuFor(m).MemoryTotalBytes = m.GetGauge().GetValue()
	}
	for _, m := range gauge("whisperx_gpu_utilization_ratio") {
		gpuFor(m).Utilization = m.GetGauge().GetValue()
	}
	for _, g := range gpus {
		out.GPUs = append(out.GPUs, *g)
	}
	sort.Slice(out.GPUs, func(i, j int) bool { return out.GPUs[i].Index < out.GPUs[j].Index })
}

func metricLabel(m *dto.Metric, name string) string {
	for _, l := range m.GetLabel() {
		if l.GetName() == name {
			return l.GetValue()
		}
	}
	return ""
}
