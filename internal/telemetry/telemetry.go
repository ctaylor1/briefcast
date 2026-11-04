// Package telemetry holds the app's Prometheus metrics. It uses a private
// registry and low-cardinality labels only (outcome, job name); episode and job
// identifiers belong in structured logs, never in metric labels. Nothing here
// sends data anywhere: metrics are only read by scraping /metrics.
package telemetry

import (
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
)

// Outcome label values.
const (
	OutcomeSuccess   = "success"
	OutcomeFailed    = "failed"
	OutcomeCancelled = "cancelled"
	OutcomePaused    = "paused"
	OutcomeRetrying  = "retrying"
	OutcomeBlocked   = "blocked"
)

var durationBuckets = []float64{1, 5, 15, 30, 60, 120, 300, 600, 1200, 1800, 3600, 7200, 14400}

var (
	registry  = prometheus.NewRegistry()
	startedAt = time.Now().UTC()

	info = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "briefcast_info", Help: "Running app version.",
	}, []string{"version"})

	jobRuns = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "briefcast_scheduled_job_runs_total", Help: "Scheduled job runs by job and outcome.",
	}, []string{"job", "outcome"})
	jobDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "briefcast_scheduled_job_duration_seconds", Help: "Scheduled job run time.", Buckets: durationBuckets,
	}, []string{"job"})

	downloads = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "briefcast_episode_downloads_total", Help: "Episode audio downloads by outcome.",
	}, []string{"outcome"})
	downloadDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "briefcast_episode_download_duration_seconds", Help: "Time to download one episode successfully.", Buckets: durationBuckets,
	})
	downloadBytes = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "briefcast_episode_download_bytes_total", Help: "Bytes of episode audio downloaded successfully.",
	})

	transcriptions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "briefcast_transcriptions_total", Help: "Transcription attempts by outcome (success, failed, blocked, retrying).",
	}, []string{"outcome"})
	transcriptionEndToEnd = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "briefcast_transcription_end_to_end_seconds", Help: "App hand-off to committed transcript.", Buckets: durationBuckets,
	})
	transcriptionQueue = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "briefcast_transcription_worker_queue_seconds", Help: "Time a job waited in the worker queue before starting.", Buckets: durationBuckets,
	})
	transcriptionProcessing = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "briefcast_transcription_processing_seconds", Help: "Worker processing time from first start to finish.", Buckets: durationBuckets,
	})
	transcriptionAudio = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "briefcast_transcription_audio_seconds_total", Help: "Audio seconds in committed transcripts.",
	})
	transcriptionSpeed = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "briefcast_transcription_speed_ratio", Help: "Audio seconds per processing second for committed transcripts.",
		Buckets: []float64{0.5, 1, 2, 5, 10, 15, 20, 30, 40, 50, 75, 100},
	})

	exports = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "briefcast_transcript_exports_total", Help: "Transcript export writes by outcome.",
	}, []string{"outcome"})

	feedRefreshes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "briefcast_feed_refreshes_total", Help: "Per-podcast feed refreshes by outcome.",
	}, []string{"outcome"})

	registerOnce sync.Once
)

func init() {
	registry.MustRegister(info, jobRuns, jobDuration, downloads, downloadDuration, downloadBytes,
		transcriptions, transcriptionEndToEnd, transcriptionQueue, transcriptionProcessing,
		transcriptionAudio, transcriptionSpeed, exports, feedRefreshes,
		collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
}

// SetVersion records the running app version once.
func SetVersion(version string) {
	registerOnce.Do(func() { info.WithLabelValues(version).Set(1) })
}

// MustRegister adds a collector (for example database-backed gauges) to the app registry.
func MustRegister(c prometheus.Collector) { registry.MustRegister(c) }

// Handler serves the app registry in Prometheus exposition format.
func Handler() http.Handler { return promhttp.HandlerFor(registry, promhttp.HandlerOpts{}) }

// Gather returns the current metric families (used by the monitoring summary and tests).
func Gather() ([]*dto.MetricFamily, error) { return registry.Gather() }

// StartedAt is when this process began recording.
func StartedAt() time.Time { return startedAt }

// ObserveScheduledJob records one scheduled job run.
func ObserveScheduledJob(job string, err error, d time.Duration) {
	outcome := OutcomeSuccess
	if err != nil {
		outcome = OutcomeFailed
	}
	jobRuns.WithLabelValues(job, outcome).Inc()
	jobDuration.WithLabelValues(job).Observe(d.Seconds())
}

// ObserveDownload records an episode download outcome; d and bytes apply to successes.
func ObserveDownload(outcome string, d time.Duration, bytes int64) {
	downloads.WithLabelValues(outcome).Inc()
	if outcome == OutcomeSuccess {
		downloadDuration.Observe(d.Seconds())
		if bytes > 0 {
			downloadBytes.Add(float64(bytes))
		}
	}
}

// TranscriptionSample describes a committed transcript. Zero durations are skipped.
type TranscriptionSample struct {
	EndToEnd, WorkerQueue, Processing time.Duration
	AudioSeconds                      float64
}

// ObserveTranscriptionCompleted records a committed transcript.
func ObserveTranscriptionCompleted(s TranscriptionSample) {
	transcriptions.WithLabelValues(OutcomeSuccess).Inc()
	if s.EndToEnd > 0 {
		transcriptionEndToEnd.Observe(s.EndToEnd.Seconds())
	}
	if s.WorkerQueue > 0 {
		transcriptionQueue.Observe(s.WorkerQueue.Seconds())
	}
	if s.Processing > 0 {
		transcriptionProcessing.Observe(s.Processing.Seconds())
	}
	if s.AudioSeconds > 0 {
		transcriptionAudio.Add(s.AudioSeconds)
		if s.Processing > 0 {
			transcriptionSpeed.Observe(s.AudioSeconds / s.Processing.Seconds())
		}
	}
}

// ObserveTranscriptionProblem records a failed, blocked or retrying attempt.
func ObserveTranscriptionProblem(outcome string) { transcriptions.WithLabelValues(outcome).Inc() }

// ObserveFeedRefresh records one podcast feed refresh.
func ObserveFeedRefresh(err error) {
	if err != nil {
		feedRefreshes.WithLabelValues(OutcomeFailed).Inc()
		return
	}
	feedRefreshes.WithLabelValues(OutcomeSuccess).Inc()
}

// ObserveExport records a transcript export write.
func ObserveExport(err error) {
	if err != nil {
		exports.WithLabelValues(OutcomeFailed).Inc()
		return
	}
	exports.WithLabelValues(OutcomeSuccess).Inc()
}

// SinceStart summarizes counters and histogram means since the process started.
type SinceStart struct {
	StartedAt                time.Time          `json:"started_at"`
	Downloads                map[string]float64 `json:"downloads"`
	DownloadBytes            float64            `json:"download_bytes"`
	AvgDownloadSeconds       float64            `json:"avg_download_seconds"`
	Transcriptions           map[string]float64 `json:"transcriptions"`
	TranscribedAudioSeconds  float64            `json:"transcribed_audio_seconds"`
	AvgProcessingSeconds     float64            `json:"avg_processing_seconds"`
	AvgEndToEndSeconds       float64            `json:"avg_end_to_end_seconds"`
	AvgWorkerQueueSeconds    float64            `json:"avg_worker_queue_seconds"`
	AvgSpeedRatio            float64            `json:"avg_speed_ratio"`
	ScheduledJobFailures     map[string]float64 `json:"scheduled_job_failures"`
	TranscriptExportFailures float64            `json:"transcript_export_failures"`
}

// Summary computes SinceStart from the registry.
func Summary() SinceStart {
	out := SinceStart{StartedAt: startedAt, Downloads: map[string]float64{}, Transcriptions: map[string]float64{}, ScheduledJobFailures: map[string]float64{}}
	families, err := registry.Gather()
	if err != nil {
		return out
	}
	for _, f := range families {
		switch f.GetName() {
		case "briefcast_episode_downloads_total":
			sumByLabel(f, "outcome", out.Downloads)
		case "briefcast_episode_download_bytes_total":
			out.DownloadBytes = counterValue(f)
		case "briefcast_episode_download_duration_seconds":
			out.AvgDownloadSeconds = histogramMean(f)
		case "briefcast_transcriptions_total":
			sumByLabel(f, "outcome", out.Transcriptions)
		case "briefcast_transcription_audio_seconds_total":
			out.TranscribedAudioSeconds = counterValue(f)
		case "briefcast_transcription_processing_seconds":
			out.AvgProcessingSeconds = histogramMean(f)
		case "briefcast_transcription_end_to_end_seconds":
			out.AvgEndToEndSeconds = histogramMean(f)
		case "briefcast_transcription_worker_queue_seconds":
			out.AvgWorkerQueueSeconds = histogramMean(f)
		case "briefcast_transcription_speed_ratio":
			out.AvgSpeedRatio = histogramMean(f)
		case "briefcast_scheduled_job_runs_total":
			for _, m := range f.GetMetric() {
				if labelValue(m, "outcome") == OutcomeFailed {
					out.ScheduledJobFailures[labelValue(m, "job")] += m.GetCounter().GetValue()
				}
			}
		case "briefcast_transcript_exports_total":
			for _, m := range f.GetMetric() {
				if labelValue(m, "outcome") == OutcomeFailed {
					out.TranscriptExportFailures += m.GetCounter().GetValue()
				}
			}
		}
	}
	return out
}

func labelValue(m *dto.Metric, name string) string {
	for _, l := range m.GetLabel() {
		if l.GetName() == name {
			return l.GetValue()
		}
	}
	return ""
}

func sumByLabel(f *dto.MetricFamily, label string, into map[string]float64) {
	for _, m := range f.GetMetric() {
		into[labelValue(m, label)] += m.GetCounter().GetValue()
	}
}

func counterValue(f *dto.MetricFamily) float64 {
	total := 0.0
	for _, m := range f.GetMetric() {
		total += m.GetCounter().GetValue()
	}
	return total
}

func histogramMean(f *dto.MetricFamily) float64 {
	var sum float64
	var count uint64
	for _, m := range f.GetMetric() {
		sum += m.GetHistogram().GetSampleSum()
		count += m.GetHistogram().GetSampleCount()
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}
