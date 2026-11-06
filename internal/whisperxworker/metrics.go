package whisperxworker

import (
	"context"
	"log/slog"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/ctaylor1/briefcast/internal/transcription"
)

// Worker telemetry. Metrics use a private registry and low-cardinality labels
// only (state, stage, error code); job IDs appear in logs, never in labels.

var knownStages = map[string]bool{
	"queued": true, "starting": true, "model_loaded": true, "audio_loaded": true, "resuming": true,
	"transcribing": true, "aligning": true, "diarizing": true, "complete": true, "failed": true,
	"interrupted": true, "restarting_checkpoint_invalid": true, "succeeded": true, "cancelled": true, "cancelling": true,
}

var errorCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,39}$`)

func stageLabel(stage string) string {
	if knownStages[stage] {
		return stage
	}
	return "other"
}

func errorCodeLabel(err *transcription.WorkerError) string {
	if err == nil {
		return "none"
	}
	if errorCodePattern.MatchString(err.Code) {
		return err.Code
	}
	return "OTHER"
}

// Durations of a single run span seconds (short clips) to hours (long episodes).
var durationBuckets = []float64{1, 5, 15, 30, 60, 120, 300, 600, 1200, 1800, 3600, 7200, 14400}

type jobClock struct {
	stage        string
	stageSince   time.Time
	runStart     time.Time
	resumeOffset float64 // audio seconds already covered by a checkpoint when the run began
}

type workerMetrics struct {
	registry     *prometheus.Registry
	accepted     prometheus.Counter
	started      prometheus.Counter
	finished     *prometheus.CounterVec
	interrupted  prometheus.Counter
	queueWait    prometheus.Histogram
	runSeconds   *prometheus.HistogramVec
	stageSeconds *prometheus.HistogramVec
	audioSeconds prometheus.Counter
	speed        prometheus.Histogram

	mu     sync.Mutex
	clocks map[string]*jobClock
	now    func() time.Time
}

func newWorkerMetrics(w *Worker) *workerMetrics {
	m := &workerMetrics{
		registry: prometheus.NewRegistry(),
		accepted: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "whisperx_jobs_accepted_total", Help: "Transcription jobs accepted into the worker journal.",
		}),
		started: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "whisperx_job_runs_started_total", Help: "Job runs started (a resumed job starts a new run).",
		}),
		finished: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "whisperx_jobs_finished_total", Help: "Job runs that reached a terminal state.",
		}, []string{"state", "error_code"}),
		interrupted: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "whisperx_job_runs_interrupted_total", Help: "Runs stopped by worker shutdown and requeued for resume.",
		}),
		queueWait: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "whisperx_job_queue_wait_seconds", Help: "Time from acceptance to the first run start.", Buckets: durationBuckets,
		}),
		runSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "whisperx_job_run_seconds", Help: "Wall time of a job run, by outcome.", Buckets: durationBuckets,
		}, []string{"state"}),
		stageSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "whisperx_job_stage_seconds", Help: "Time spent in each pipeline stage.", Buckets: durationBuckets,
		}, []string{"stage"}),
		audioSeconds: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "whisperx_transcribed_audio_seconds_total", Help: "Audio seconds transcribed by successful runs (excluding resumed portions).",
		}),
		speed: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "whisperx_job_speed_ratio", Help: "Audio seconds processed per wall-clock second for successful runs.",
			Buckets: []float64{0.5, 1, 2, 5, 10, 15, 20, 30, 40, 50, 75, 100},
		}),
		clocks: map[string]*jobClock{},
		now:    time.Now,
	}
	info := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "whisperx_worker_info", Help: "Static worker build and engine configuration.",
	}, []string{"worker_version", "engine_profile", "pipeline_version", "device", "compute_type"})
	info.WithLabelValues(w.cfg.WorkerVersion, w.cfg.EngineProfile, transcription.PipelineVersion, w.cfg.Device, w.cfg.ComputeType).Set(1)

	m.registry.MustRegister(m.accepted, m.started, m.finished, m.interrupted, m.queueWait, m.runSeconds,
		m.stageSeconds, m.audioSeconds, m.speed, info, &workerStateCollector{w: w},
		collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return m
}

func (m *workerMetrics) handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *workerMetrics) jobAccepted(r *record) {
	m.accepted.Inc()
	slog.Info("worker_job_accepted", "job_id", r.Status.JobID, "attempt", r.Status.Attempt,
		"input_bytes", r.Request.Input.SizeBytes, "resume_from_checkpoint", r.Checkpoint != nil)
}

func (m *workerMetrics) runStarted(r *record, firstStart bool) {
	now := m.now()
	clock := &jobClock{stage: stageLabel(r.Status.Stage), stageSince: now, runStart: now}
	if r.Checkpoint != nil {
		clock.resumeOffset = r.Checkpoint.CompletedSeconds
	}
	m.mu.Lock()
	m.clocks[r.Status.JobID] = clock
	m.mu.Unlock()
	m.started.Inc()
	queueWaitMS := int64(-1)
	if firstStart {
		wait := max(now.Sub(r.Status.CreatedAt), 0)
		m.queueWait.Observe(wait.Seconds())
		queueWaitMS = wait.Milliseconds()
	}
	slog.Info("worker_job_started", "job_id", r.Status.JobID, "attempt", r.Status.Attempt, "first_run", firstStart,
		"queue_wait_ms", queueWaitMS, "resume_offset_seconds", clock.resumeOffset)
}

// stageChanged closes the previous stage and logs the transition.
func (m *workerMetrics) stageChanged(id, stage string, percent int, totalSeconds float64) {
	label := stageLabel(stage)
	m.mu.Lock()
	clock := m.clocks[id]
	if clock == nil || clock.stage == label {
		m.mu.Unlock()
		return
	}
	now := m.now()
	previous, elapsed := clock.stage, max(now.Sub(clock.stageSince), 0)
	clock.stage, clock.stageSince = label, now
	m.mu.Unlock()
	m.stageSeconds.WithLabelValues(previous).Observe(elapsed.Seconds())
	slog.Info("worker_job_stage", "job_id", id, "stage", label, "previous_stage", previous,
		"previous_stage_ms", elapsed.Milliseconds(), "percent", percent, "total_seconds", totalSeconds)
}

// runEnded records the outcome of a run; requeued is true when shutdown interrupted it.
func (m *workerMetrics) runEnded(r *record, requeued bool) {
	id := r.Status.JobID
	now := m.now()
	m.mu.Lock()
	clock := m.clocks[id]
	delete(m.clocks, id)
	m.mu.Unlock()
	var run time.Duration
	resumeOffset := 0.0
	if clock != nil {
		run = max(now.Sub(clock.runStart), 0)
		resumeOffset = clock.resumeOffset
		m.stageSeconds.WithLabelValues(clock.stage).Observe(max(now.Sub(clock.stageSince), 0).Seconds())
	}
	state := r.Status.State
	if requeued {
		m.interrupted.Inc()
		slog.Info("worker_job_interrupted", "job_id", id, "run_ms", run.Milliseconds(), "stage", r.Status.Stage)
		return
	}
	code := errorCodeLabel(r.Status.Error)
	m.finished.WithLabelValues(state, code).Inc()
	m.runSeconds.WithLabelValues(state).Observe(run.Seconds())
	processed := max(r.Status.TotalSeconds-resumeOffset, 0)
	speed := 0.0
	if state == "succeeded" {
		m.audioSeconds.Add(processed)
		if run > 0 && processed > 0 {
			speed = processed / run.Seconds()
			m.speed.Observe(speed)
		}
	}
	attrs := []any{"job_id", id, "state", state, "error_code", code, "attempt", r.Status.Attempt,
		"run_ms", run.Milliseconds(), "budget_used_ms", r.BudgetUsed.Milliseconds(),
		"audio_seconds", r.Status.TotalSeconds, "processed_audio_seconds", processed,
		"speed_x", strconv.FormatFloat(speed, 'f', 1, 64), "resumed", resumeOffset > 0}
	if state == "succeeded" {
		slog.Info("worker_job_finished", attrs...)
	} else {
		slog.Warn("worker_job_finished", attrs...)
	}
}

// workerStateCollector reports queue, health and device gauges at scrape time.
type workerStateCollector struct {
	w *Worker

	gpuMu      sync.Mutex
	gpuAt      time.Time
	gpuReading *gpuReading
}

var (
	descJobs      = prometheus.NewDesc("whisperx_jobs", "Jobs in the journal by state.", []string{"state"}, nil)
	descAccepting = prometheus.NewDesc("whisperx_worker_accepting", "1 when the worker accepts new jobs.", nil, nil)
	descHealthy   = prometheus.NewDesc("whisperx_worker_healthy", "1 when the worker has no fault and is not stopping.", nil, nil)
	descCapacity  = prometheus.NewDesc("whisperx_worker_capacity", "Configured job slots.", []string{"kind"}, nil)
	descFreeBytes = prometheus.NewDesc("whisperx_state_free_bytes", "Free bytes on the worker state volume.", nil, nil)
	descGPUUsed   = prometheus.NewDesc("whisperx_gpu_memory_used_bytes", "GPU memory in use (whole device).", []string{"gpu"}, nil)
	descGPUTotal  = prometheus.NewDesc("whisperx_gpu_memory_total_bytes", "GPU memory capacity.", []string{"gpu"}, nil)
	descGPUUtil   = prometheus.NewDesc("whisperx_gpu_utilization_ratio", "GPU compute utilization, 0-1.", []string{"gpu"}, nil)
)

func (c *workerStateCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{descJobs, descAccepting, descHealthy, descCapacity, descFreeBytes, descGPUUsed, descGPUTotal, descGPUUtil} {
		ch <- d
	}
}

func (c *workerStateCollector) Collect(ch chan<- prometheus.Metric) {
	w := c.w
	w.mu.Lock()
	counts, err := w.store.counts()
	healthy := w.fault == nil && !w.stopping
	w.mu.Unlock()
	if err == nil {
		ch <- prometheus.MustNewConstMetric(descJobs, prometheus.GaugeValue, float64(counts.Queued), "queued")
		ch <- prometheus.MustNewConstMetric(descJobs, prometheus.GaugeValue, float64(counts.Running), "running")
		ch <- prometheus.MustNewConstMetric(descJobs, prometheus.GaugeValue, float64(counts.UnacknowledgedResults), "unacknowledged")
		ch <- prometheus.MustNewConstMetric(descAccepting, prometheus.GaugeValue, boolFloat(counts.Accepting))
	}
	ch <- prometheus.MustNewConstMetric(descHealthy, prometheus.GaugeValue, boolFloat(healthy && err == nil))
	ch <- prometheus.MustNewConstMetric(descCapacity, prometheus.GaugeValue, float64(w.cfg.MaxRunning), "running")
	ch <- prometheus.MustNewConstMetric(descCapacity, prometheus.GaugeValue, float64(w.cfg.MaxWaiting), "waiting")
	if free, ferr := freeBytes(w.store.dir); ferr == nil {
		ch <- prometheus.MustNewConstMetric(descFreeBytes, prometheus.GaugeValue, float64(free))
	}
	if w.cfg.Device == "cuda" {
		if g := c.gpu(); g != nil {
			for _, d := range g.devices {
				ch <- prometheus.MustNewConstMetric(descGPUUsed, prometheus.GaugeValue, d.usedBytes, d.index)
				ch <- prometheus.MustNewConstMetric(descGPUTotal, prometheus.GaugeValue, d.totalBytes, d.index)
				ch <- prometheus.MustNewConstMetric(descGPUUtil, prometheus.GaugeValue, d.utilization, d.index)
			}
		}
	}
}

type gpuDevice struct {
	index                 string
	usedBytes, totalBytes float64
	utilization           float64
}
type gpuReading struct{ devices []gpuDevice }

var queryGPU = func(ctx context.Context) ([]byte, error) {
	return exec.CommandContext(ctx, "nvidia-smi", "--query-gpu=index,memory.used,memory.total,utilization.gpu",
		"--format=csv,noheader,nounits").Output()
}

// gpu returns a reading cached for 5 s so frequent scrapes do not spawn nvidia-smi each time.
func (c *workerStateCollector) gpu() *gpuReading {
	c.gpuMu.Lock()
	defer c.gpuMu.Unlock()
	if !c.gpuAt.IsZero() && time.Since(c.gpuAt) < 5*time.Second {
		return c.gpuReading
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := queryGPU(ctx)
	c.gpuAt = time.Now()
	if err != nil {
		c.gpuReading = nil
		return nil
	}
	c.gpuReading = parseGPUQuery(string(out))
	return c.gpuReading
}

func parseGPUQuery(out string) *gpuReading {
	reading := &gpuReading{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Split(line, ",")
		if len(fields) != 4 {
			continue
		}
		var nums [3]float64
		ok := true
		for i := 0; i < 3; i++ {
			v, err := strconv.ParseFloat(strings.TrimSpace(fields[i+1]), 64)
			if err != nil {
				ok = false
				break
			}
			nums[i] = v
		}
		index := strings.TrimSpace(fields[0])
		if !ok || index == "" || len(index) > 3 {
			continue
		}
		reading.devices = append(reading.devices, gpuDevice{index: index, usedBytes: nums[0] * 1024 * 1024,
			totalBytes: nums[1] * 1024 * 1024, utilization: nums[2] / 100})
	}
	if len(reading.devices) == 0 {
		return nil
	}
	return reading
}

func boolFloat(v bool) float64 {
	if v {
		return 1
	}
	return 0
}
