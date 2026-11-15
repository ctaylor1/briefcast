package service

import (
	"sort"
	"sync"
	"time"
)

// Job run outcomes recorded by runJobWith.
const (
	JobOutcomeSucceeded = "succeeded"
	JobOutcomeFailed    = "failed"
	JobOutcomePanicked  = "panicked"
)

// JobRunSummary is the in-memory history of one background job name since
// the app started.
type JobRunSummary struct {
	Name                string     `json:"name"`
	Running             bool       `json:"running"`
	LastStartedAt       *time.Time `json:"last_started_at,omitempty"`
	LastFinishedAt      *time.Time `json:"last_finished_at,omitempty"`
	LastDurationMS      int64      `json:"last_duration_ms"`
	LastOutcome         string     `json:"last_outcome,omitempty"`
	LastError           string     `json:"last_error,omitempty"`
	LastSuccessAt       *time.Time `json:"last_success_at,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	// FailingSince is when the current run of failures began.
	FailingSince *time.Time `json:"failing_since,omitempty"`
	Runs         int        `json:"runs"`
	Failures     int        `json:"failures"`
}

var jobHistory = struct {
	mu      sync.Mutex
	byName  map[string]*JobRunSummary
	running map[string]int
}{byName: map[string]*JobRunSummary{}, running: map[string]int{}}

func recordJobStarted(name string, at time.Time) {
	jobHistory.mu.Lock()
	defer jobHistory.mu.Unlock()
	entry := jobHistoryEntry(name)
	started := at.UTC()
	entry.LastStartedAt = &started
	jobHistory.running[name]++
	entry.Running = true
}

func recordJobFinished(name string, started time.Time, outcome string, err error) {
	jobHistory.mu.Lock()
	defer jobHistory.mu.Unlock()
	entry := jobHistoryEntry(name)
	finished := time.Now().UTC()
	entry.LastFinishedAt = &finished
	entry.LastDurationMS = finished.Sub(started).Milliseconds()
	entry.LastOutcome = outcome
	entry.Runs++
	if jobHistory.running[name] > 0 {
		jobHistory.running[name]--
	}
	entry.Running = jobHistory.running[name] > 0
	if outcome == JobOutcomeSucceeded {
		entry.LastError = ""
		entry.LastSuccessAt = &finished
		entry.ConsecutiveFailures = 0
		entry.FailingSince = nil
		return
	}
	entry.Failures++
	entry.ConsecutiveFailures++
	if entry.FailingSince == nil {
		since := started.UTC()
		entry.FailingSince = &since
	}
	if err != nil {
		entry.LastError = trimToLength(err.Error(), summaryErrorMaxChars)
	}
}

func jobHistoryEntry(name string) *JobRunSummary {
	entry, ok := jobHistory.byName[name]
	if !ok {
		entry = &JobRunSummary{Name: name}
		jobHistory.byName[name] = entry
	}
	return entry
}

// JobHistorySnapshot returns each job's history, sorted by name.
func JobHistorySnapshot() []JobRunSummary {
	jobHistory.mu.Lock()
	defer jobHistory.mu.Unlock()
	out := make([]JobRunSummary, 0, len(jobHistory.byName))
	for _, entry := range jobHistory.byName {
		copied := *entry
		copied.LastStartedAt = cloneTimePointer(entry.LastStartedAt)
		copied.LastFinishedAt = cloneTimePointer(entry.LastFinishedAt)
		copied.LastSuccessAt = cloneTimePointer(entry.LastSuccessAt)
		copied.FailingSince = cloneTimePointer(entry.FailingSince)
		out = append(out, copied)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func resetJobHistoryForTests() {
	jobHistory.mu.Lock()
	defer jobHistory.mu.Unlock()
	jobHistory.byName = map[string]*JobRunSummary{}
	jobHistory.running = map[string]int{}
}
