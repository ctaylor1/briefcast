package service

import (
	"context"
	"errors"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/logging"
	"github.com/ctaylor1/briefcast/internal/telemetry"
)

// Summary attempt outcomes, as shown in Monitoring and counted in
// briefcast_summary_attempts_total.
const (
	SummaryOutcomeSucceeded  = "succeeded"
	SummaryOutcomeFailed     = "failed"
	SummaryOutcomeTimedOut   = "timed_out"
	SummaryOutcomeTokenLimit = "token_limit"
	SummaryOutcomePanicked   = "panicked"
	// SummaryOutcomeDiscarded: the transcript changed during the call, so the
	// result was not saved.
	SummaryOutcomeDiscarded = "discarded"
)

// Summary attempt sources.
const (
	SummarySourceAutomatic   = "automatic" // a new transcript or a feed refresh asked for it
	SummarySourceBackfill    = "backfill"  // Summarize waiting episodes, or Repair
	SummarySourceResummarize = "resummarize"
	SummarySourceDirect      = "direct" // SummarizeEpisode called outside a summary job
)

const (
	summaryRecentLimit   = 50
	summaryErrorMaxChars = 300
)

// SummaryAttempt is one LLM summary attempt. It holds identifiers, counts and
// a shortened error only: never transcript, prompt or summary text.
type SummaryAttempt struct {
	EpisodeID        string     `json:"episode_id"`
	Title            string     `json:"title"`
	Source           string     `json:"source"`
	Model            string     `json:"model,omitempty"`
	StartedAt        time.Time  `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
	DurationMS       int64      `json:"duration_ms"`
	Outcome          string     `json:"outcome,omitempty"`
	FinishReason     string     `json:"finish_reason,omitempty"`
	PromptTokens     int        `json:"prompt_tokens,omitempty"`
	CompletionTokens int        `json:"completion_tokens,omitempty"`
	ThinkingChars    int        `json:"thinking_chars,omitempty"`
	Error            string     `json:"error,omitempty"`
}

// llmCallDetails is what the provider reported about one call, whether or not
// it produced a summary.
type llmCallDetails struct {
	FinishReason     string
	PromptTokens     int
	CompletionTokens int
	ThinkingChars    int
	Model            string
}

type summaryActivityState struct {
	mu                  sync.Mutex
	nextID              uint64
	inFlight            map[uint64]SummaryAttempt
	recent              []SummaryAttempt // oldest first, at most summaryRecentLimit
	outcomes            map[string]int
	consecutiveFailures int
	lastSuccessAt       *time.Time
}

var summaryActivity = newSummaryActivityState()

func newSummaryActivityState() *summaryActivityState {
	return &summaryActivityState{inFlight: map[uint64]SummaryAttempt{}, outcomes: map[string]int{}}
}

// summaryAttemptHandle finishes one attempt started by beginSummaryAttempt.
type summaryAttemptHandle struct {
	id    uint64
	state *summaryActivityState
}

// beginSummaryAttempt records an attempt as in flight until finish is called.
func beginSummaryAttempt(item *db.PodcastItem, source, model string) summaryAttemptHandle {
	s := summaryActivity
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	s.inFlight[s.nextID] = SummaryAttempt{
		EpisodeID: item.ID,
		Title:     item.Title,
		Source:    source,
		Model:     model,
		StartedAt: time.Now().UTC(),
	}
	return summaryAttemptHandle{id: s.nextID, state: s}
}

// finish moves the attempt from in flight to the recent results, counts it
// and logs one summary_attempt_finished line.
func (h summaryAttemptHandle) finish(outcome string, details llmCallDetails, err error) {
	s := h.state
	if s == nil {
		return
	}
	now := time.Now().UTC()
	s.mu.Lock()
	attempt, ok := s.inFlight[h.id]
	if !ok {
		s.mu.Unlock()
		return
	}
	delete(s.inFlight, h.id)
	attempt.FinishedAt = &now
	attempt.DurationMS = now.Sub(attempt.StartedAt).Milliseconds()
	attempt.Outcome = outcome
	attempt.FinishReason = truncateForLog(details.FinishReason, 40)
	attempt.PromptTokens = details.PromptTokens
	attempt.CompletionTokens = details.CompletionTokens
	attempt.ThinkingChars = details.ThinkingChars
	if details.Model != "" {
		attempt.Model = details.Model
	}
	if err != nil {
		attempt.Error = trimToLength(err.Error(), summaryErrorMaxChars)
	}
	s.recent = append(s.recent, attempt)
	if len(s.recent) > summaryRecentLimit {
		s.recent = append([]SummaryAttempt(nil), s.recent[len(s.recent)-summaryRecentLimit:]...)
	}
	s.outcomes[outcome]++
	switch outcome {
	case SummaryOutcomeSucceeded:
		s.consecutiveFailures = 0
		s.lastSuccessAt = &now
	case SummaryOutcomeDiscarded:
		// The provider worked; the transcript changed. Neither a success nor a failure.
	default:
		s.consecutiveFailures++
	}
	s.mu.Unlock()

	telemetry.ObserveSummaryAttempt(outcome, now.Sub(attempt.StartedAt))
	fields := []any{"podcast_item_id", attempt.EpisodeID, "source", attempt.Source, "outcome", outcome,
		"duration_ms", attempt.DurationMS, "model", attempt.Model}
	if attempt.FinishReason != "" {
		fields = append(fields, "finish_reason", attempt.FinishReason)
	}
	if attempt.PromptTokens > 0 || attempt.CompletionTokens > 0 {
		fields = append(fields, "prompt_tokens", attempt.PromptTokens, "completion_tokens", attempt.CompletionTokens)
	}
	if attempt.ThinkingChars > 0 {
		fields = append(fields, "thinking_chars", attempt.ThinkingChars)
	}
	if outcome == SummaryOutcomeSucceeded {
		logging.Sugar().Infow("summary_attempt_finished", fields...)
		return
	}
	logging.Sugar().Warnw("summary_attempt_finished", append(fields, "error", attempt.Error)...)
}

// classifySummaryAttempt names the outcome of a finished, non-panicking attempt.
func classifySummaryAttempt(details llmCallDetails, err error) string {
	switch {
	case err == nil:
		return SummaryOutcomeSucceeded
	case errors.Is(err, db.ErrTranscriptChanged):
		return SummaryOutcomeDiscarded
	case details.FinishReason == "length":
		return SummaryOutcomeTokenLimit
	case isTimeoutError(err):
		return SummaryOutcomeTimedOut
	default:
		return SummaryOutcomeFailed
	}
}

func isTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// SummaryActivitySnapshot is the in-memory summary activity since the app started.
type SummaryActivitySnapshot struct {
	InFlight            []SummaryAttempt `json:"in_flight"`
	Recent              []SummaryAttempt `json:"recent"` // newest first
	Outcomes            map[string]int   `json:"outcomes"`
	ConsecutiveFailures int              `json:"consecutive_failures"`
	LastSuccessAt       *time.Time       `json:"last_success_at,omitempty"`
}

// summaryActivitySnapshot copies the tracker state, keeping the newest recent
// results up to recentLimit.
func summaryActivitySnapshot(recentLimit int) SummaryActivitySnapshot {
	s := summaryActivity
	s.mu.Lock()
	defer s.mu.Unlock()
	out := SummaryActivitySnapshot{
		InFlight:            make([]SummaryAttempt, 0, len(s.inFlight)),
		Recent:              make([]SummaryAttempt, 0, min(recentLimit, len(s.recent))),
		Outcomes:            make(map[string]int, len(s.outcomes)),
		ConsecutiveFailures: s.consecutiveFailures,
		LastSuccessAt:       cloneTimePointer(s.lastSuccessAt),
	}
	for _, attempt := range s.inFlight {
		out.InFlight = append(out.InFlight, attempt)
	}
	sort.Slice(out.InFlight, func(i, j int) bool { return out.InFlight[i].StartedAt.Before(out.InFlight[j].StartedAt) })
	for i := len(s.recent) - 1; i >= 0 && len(out.Recent) < recentLimit; i-- {
		out.Recent = append(out.Recent, s.recent[i])
	}
	for outcome, n := range s.outcomes {
		out.Outcomes[outcome] = n
	}
	return out
}

func resetSummaryActivityForTests() {
	s := summaryActivity
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inFlight = map[uint64]SummaryAttempt{}
	s.recent = nil
	s.outcomes = map[string]int{}
	s.consecutiveFailures = 0
	s.lastSuccessAt = nil
}
