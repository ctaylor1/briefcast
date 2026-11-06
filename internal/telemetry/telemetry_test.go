package telemetry

import (
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSummaryReflectsObservations(t *testing.T) {
	before := Summary()

	ObserveDownload(OutcomeSuccess, 4*time.Second, 1000)
	ObserveDownload(OutcomeSuccess, 6*time.Second, 3000)
	ObserveDownload(OutcomeFailed, time.Second, 99)
	ObserveTranscriptionCompleted(TranscriptionSample{EndToEnd: 70 * time.Second, WorkerQueue: 10 * time.Second, Processing: 60 * time.Second, AudioSeconds: 1800})
	ObserveTranscriptionProblem(OutcomeRetrying)
	ObserveScheduledJob("DownloadMissingImages", errors.New("boom"), time.Second)
	ObserveScheduledJob("DownloadMissingImages", nil, time.Second)
	ObserveExport(errors.New("disk full"))
	ObserveExport(nil)
	ObserveFeedRefresh(nil)

	after := Summary()
	if d := after.Downloads[OutcomeSuccess] - before.Downloads[OutcomeSuccess]; d != 2 {
		t.Fatalf("download successes delta=%v", d)
	}
	if d := after.Downloads[OutcomeFailed] - before.Downloads[OutcomeFailed]; d != 1 {
		t.Fatalf("download failures delta=%v", d)
	}
	if d := after.DownloadBytes - before.DownloadBytes; d != 4000 {
		t.Fatalf("failed downloads must not add bytes, delta=%v", d)
	}
	if d := after.TranscribedAudioSeconds - before.TranscribedAudioSeconds; d != 1800 {
		t.Fatalf("audio seconds delta=%v", d)
	}
	if after.Transcriptions[OutcomeSuccess]-before.Transcriptions[OutcomeSuccess] != 1 ||
		after.Transcriptions[OutcomeRetrying]-before.Transcriptions[OutcomeRetrying] != 1 {
		t.Fatalf("unexpected transcription outcome deltas: before=%v after=%v", before.Transcriptions, after.Transcriptions)
	}
	if after.AvgSpeedRatio <= 0 || after.AvgProcessingSeconds <= 0 || after.AvgEndToEndSeconds <= 0 || after.AvgWorkerQueueSeconds <= 0 {
		t.Fatalf("expected histogram means, got %+v", after)
	}
	if after.ScheduledJobFailures["DownloadMissingImages"]-before.ScheduledJobFailures["DownloadMissingImages"] != 1 {
		t.Fatalf("expected one scheduled job failure, got %v", after.ScheduledJobFailures)
	}
	if after.TranscriptExportFailures-before.TranscriptExportFailures != 1 {
		t.Fatalf("expected one export failure")
	}
}

func TestHandlerServesPrometheusText(t *testing.T) {
	SetVersion("test-version")
	ObserveDownload(OutcomeSuccess, time.Second, 10)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body, _ := io.ReadAll(rec.Body)
	text := string(body)
	for _, want := range []string{
		`briefcast_info{version="test-version"} 1`,
		`briefcast_episode_downloads_total{outcome="success"}`,
		"briefcast_episode_download_duration_seconds_bucket",
		"go_goroutines",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("metrics output missing %q", want)
		}
	}
	for _, forbidden := range []string{"podcast_item_id", "job_id", "episode="} {
		if strings.Contains(text, forbidden) {
			t.Errorf("high-cardinality label %q must not be exported", forbidden)
		}
	}
}
