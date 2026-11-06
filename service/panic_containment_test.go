package service

import (
	"strings"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func observeServiceErrors(t *testing.T) *observer.ObservedLogs {
	t.Helper()
	core, logs := observer.New(zap.ErrorLevel)
	original := Logger
	Logger = zap.New(core).Sugar()
	t.Cleanup(func() { Logger = original })
	return logs
}

func TestRunSafelyLogsPanicWithNameValueAndStack(t *testing.T) {
	logs := observeServiceErrors(t)

	runSafely("test work", func() { panic("work failed") })

	if logs.Len() != 1 {
		t.Fatalf("expected one error log, got %d", logs.Len())
	}
	fields := logs.All()[0].ContextMap()
	if fields["work"] != "test work" || fields["panic"] != "work failed" {
		t.Fatalf("expected the work name and panic value, got %v", fields)
	}
	if stack, _ := fields["stack"].(string); !strings.Contains(stack, "TestRunSafelyLogsPanicWithNameValueAndStack") {
		t.Fatalf("expected a stack through the panicking function, got %q", stack)
	}
}

func TestRunSafelyLogsNothingWhenWorkSucceeds(t *testing.T) {
	logs := observeServiceErrors(t)
	ran := false

	runSafely("test work", func() { ran = true })

	if !ran || logs.Len() != 0 {
		t.Fatalf("expected the work to run without logging, ran=%v logs=%d", ran, logs.Len())
	}
}

func TestSafeGoKeepsTheProcessAlive(t *testing.T) {
	logs := observeServiceErrors(t)

	SafeGo("test work", func() { panic("work failed") })

	deadline := time.Now().Add(5 * time.Second)
	for logs.Len() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("expected the panic to be logged")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestGetItunesImageURLHandlesFeedsWithoutChannel(t *testing.T) {
	atom := []byte(`<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><title>Atom feed</title></feed>`)
	if got := getItunesImageURL(atom); got != "" {
		t.Fatalf("expected no image for a feed without <channel>, got %q", got)
	}
}

func TestGetItunesImageURLReadsChannelImage(t *testing.T) {
	rss := []byte(`<?xml version="1.0"?><rss xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd"><channel>` +
		`<itunes:image href="https://example.com/cover.jpg"/></channel></rss>`)
	if got := getItunesImageURL(rss); got != "https://example.com/cover.jpg" {
		t.Fatalf("expected the channel image, got %q", got)
	}
}

// withNilDatabase makes the next database call panic on a nil handle, which is a
// real panic in real job code rather than a test seam.
func withNilDatabase(t *testing.T) {
	t.Helper()
	handle := db.DB
	db.DB = nil
	t.Cleanup(func() { db.DB = handle })
}

func enableSummaryJobs(t *testing.T) {
	t.Helper()
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "not-a-real-key")
}

func requireSummaryJobFinishedWithError(t *testing.T) {
	t.Helper()
	status := GetSummaryJobStatus()
	if GetSummaryBackfillRunning() || status.Running {
		t.Fatalf("expected the summary job to stop running, flag=%v status=%+v", GetSummaryBackfillRunning(), status)
	}
	if status.Error == "" {
		t.Fatalf("expected a stopped job to report an error, got %+v", status)
	}
}

func TestSummaryJobsClearRunningFlagAfterPanic(t *testing.T) {
	jobs := map[string]func(){
		"backfill": func() { _, _, _ = BackfillSummaries(nil) },
		"resummarize": func() {
			_, _ = ResummarizeSummaries(ResummarizeFilter{}, nil)
		},
	}
	for name, job := range jobs {
		t.Run(name, func(t *testing.T) {
			setupRetentionTestDB(t)
			enableSummaryJobs(t)
			withNilDatabase(t)

			runSafely("summary job", job)

			requireSummaryJobFinishedWithError(t)
		})
	}
}

func TestStartSummaryBackfillClearsRunningFlagAfterPanic(t *testing.T) {
	setupRetentionTestDB(t)
	enableSummaryJobs(t)
	withNilDatabase(t)

	if _, err := StartSummaryBackfill(); err != nil {
		t.Fatalf("start summary backfill: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for GetSummaryBackfillRunning() {
		if time.Now().After(deadline) {
			t.Fatal("summary job still marked as running after its goroutine panicked")
		}
		time.Sleep(10 * time.Millisecond)
	}
	requireSummaryJobFinishedWithError(t)
}

func TestRepairWorkClearsRunningFlagAfterPanic(t *testing.T) {
	setupRetentionTestDB(t)
	startedAt := time.Now().UTC()
	repairWorkMu.Lock()
	repairWorkRunning = true
	repairWorkStartedAt = &startedAt
	repairWorkMu.Unlock()
	withNilDatabase(t)

	runSafely("repair work", func() { runRepairWork(startedAt) })

	repairWorkMu.Lock()
	defer repairWorkMu.Unlock()
	if repairWorkRunning || repairWorkStartedAt != nil {
		t.Fatalf("expected repair work to stop running, running=%v startedAt=%v", repairWorkRunning, repairWorkStartedAt)
	}
	if repairWorkLastRun == nil || repairWorkLastRun.Error == "" || repairWorkLastRun.FinishedAt == nil {
		t.Fatalf("expected a finished run that reports the failure, got %+v", repairWorkLastRun)
	}
}

func TestJobsWithDeferredFlagsClearThemAfterPanic(t *testing.T) {
	jobs := map[string]struct {
		run     func()
		running func() bool
	}{
		"export all":    {run: func() { ExportAll() }, running: GetExportAllRunning},
		"link backfill": {run: func() { BackfillShowNoteLinks() }, running: GetLinkBackfillRunning},
	}
	for name, job := range jobs {
		t.Run(name, func(t *testing.T) {
			setupRetentionTestDB(t)
			withNilDatabase(t)

			runSafely("flagged job", job.run)

			if job.running() {
				t.Fatal("expected the running flag to clear after a panic")
			}
		})
	}
}
