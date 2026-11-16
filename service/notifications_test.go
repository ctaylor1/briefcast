package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"go.uber.org/zap"
)

type notifyReceiver struct {
	server   *httptest.Server
	mu       sync.Mutex
	bodies   []map[string]any
	headers  []http.Header
	status   func(n int) int
	requests atomic.Int32
}

func (r *notifyReceiver) received() []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]map[string]any(nil), r.bodies...)
}

// setupNotifications points notifications at a test receiver with a fake
// clock and stubbed outage sources. Returns the receiver and a clock setter.
func setupNotifications(t *testing.T, format string) (*notifyReceiver, func(time.Time)) {
	t.Helper()
	setupRetentionTestDB(t)
	resetNotifierForTests()
	t.Cleanup(resetNotifierForTests)
	recv := &notifyReceiver{}
	recv.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(recv.requests.Add(1))
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		recv.mu.Lock()
		recv.bodies = append(recv.bodies, body)
		recv.headers = append(recv.headers, r.Header.Clone())
		recv.mu.Unlock()
		if recv.status != nil {
			w.WriteHeader(recv.status(n))
		}
	}))
	t.Cleanup(recv.server.Close)
	t.Setenv(notifyURLEnv, recv.server.URL+"/briefcast-alerts")
	t.Setenv(notifyFormatEnv, format)
	t.Setenv(outboundAllowPrivateNotifyEnv, "true")

	clock := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	origNow, origWorker, origLLM, origDisk, origDelays := notifyNow, notifyWorkerSummary, notifyLLMServerCheck, notifyAssetsFreeBytes, notifyRetryDelays
	notifyNow = func() time.Time { return clock }
	notifyWorkerSummary = func(context.Context) WorkerSummary { return WorkerSummary{} }
	notifyLLMServerCheck = func(context.Context) (LLMServerHealth, bool) { return LLMServerHealth{}, false }
	notifyAssetsFreeBytes = func() (uint64, error) { return 500 << 30, nil }
	notifyRetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() {
		notifyNow, notifyWorkerSummary, notifyLLMServerCheck, notifyAssetsFreeBytes, notifyRetryDelays = origNow, origWorker, origLLM, origDisk, origDelays
	})
	return recv, func(next time.Time) { clock = next }
}

func recordAttempt(t *testing.T, title, outcome string, err error) {
	t.Helper()
	h := beginSummaryAttempt(&db.PodcastItem{Base: db.Base{ID: title}, Title: title}, SummarySourceAutomatic, "m")
	h.finish(outcome, llmCallDetails{}, err)
}

func TestNotificationsFireOnceRemindAndResolve(t *testing.T) {
	recv, setClock := setupNotifications(t, "json")
	start := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	for _, title := range []string{"one", "two", "three"} {
		recordAttempt(t, title, SummaryOutcomeTokenLimit, errors.New("LLM response reached the output token limit (max_tokens=2048)"))
	}

	evaluateNotifications(context.Background())
	evaluateNotifications(context.Background())
	recordAttempt(t, "four", SummaryOutcomeTimedOut, errors.New("deadline"))
	setClock(start.Add(time.Minute))
	evaluateNotifications(context.Background())
	got := recv.received()
	if len(got) != 1 || got[0]["event"] != notifyEventFiring || got[0]["condition"] != "summaries_failing" {
		t.Fatalf("expected one firing message, got %v", got)
	}
	if !strings.Contains(got[0]["message"].(string), "timed out") && !strings.Contains(got[0]["message"].(string), "token limit") {
		t.Fatalf("message does not give the reason: %v", got[0]["message"])
	}

	setClock(start.Add(6*time.Hour + time.Minute))
	evaluateNotifications(context.Background())
	if got := recv.received(); len(got) != 2 || got[1]["event"] != notifyEventReminder {
		t.Fatalf("expected a reminder after 6 h, got %v", got)
	}

	recordAttempt(t, "five", SummaryOutcomeSucceeded, nil)
	evaluateNotifications(context.Background())
	got = recv.received()
	if len(got) != 3 || got[2]["event"] != notifyEventResolved || !strings.HasPrefix(got[2]["title"].(string), "Resolved: ") {
		t.Fatalf("expected a resolved message, got %v", got)
	}
	evaluateNotifications(context.Background())
	if len(recv.received()) != 3 {
		t.Fatal("a resolved condition was reported again")
	}
	if status := NotificationStatusSnapshot(); !status.Enabled || status.Format != "json" || len(status.Active) != 0 || status.LastSentAt == nil {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestNotificationsReportFailingJobsWithTheirReason(t *testing.T) {
	recv, _ := setupNotifications(t, "json")
	resetJobHistoryForTests()
	t.Cleanup(resetJobHistoryForTests)
	logger := zap.NewNop().Sugar()
	for i := 0; i < 3; i++ {
		_ = runJobWith("TranscribePendingEpisodes", logger, func(*zap.SugaredLogger) error {
			return errors.New("INVALID_CONFIG: Worker token file unavailable")
		})
	}
	evaluateNotifications(context.Background())
	got := recv.received()
	if len(got) != 1 || got[0]["condition"] != "job_failing:TranscribePendingEpisodes" || !strings.Contains(got[0]["message"].(string), "Worker token file unavailable") {
		t.Fatalf("expected the failing job with its reason, got %v", got)
	}
	if active := NotificationStatusSnapshot().Active; len(active) != 1 || active[0].Title != "TranscribePendingEpisodes is failing" {
		t.Fatalf("active conditions: %+v", active)
	}
}

func TestNotificationsWaitTenMinutesBeforeReportingAnOutage(t *testing.T) {
	recv, setClock := setupNotifications(t, "json")
	t.Setenv("LLM_ENABLED", "true")
	t.Setenv("LLM_API_KEY", "k")
	setting := db.GetOrCreateSetting()
	setting.SummarizationEnabled = true
	if err := db.UpdateSettings(setting); err != nil {
		t.Fatal(err)
	}
	reachable := false
	notifyLLMServerCheck = func(context.Context) (LLMServerHealth, bool) {
		return LLMServerHealth{Host: "192.168.1.50:11434", Reachable: reachable, Error: "connection refused"}, true
	}
	start := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	for _, offset := range []time.Duration{0, 5 * time.Minute, 9 * time.Minute} {
		setClock(start.Add(offset))
		evaluateNotifications(context.Background())
	}
	if n := len(recv.received()); n != 0 {
		t.Fatalf("an outage under 10 minutes was reported (%d messages)", n)
	}
	setClock(start.Add(10 * time.Minute))
	evaluateNotifications(context.Background())
	got := recv.received()
	if len(got) != 1 || got[0]["condition"] != "llm_unreachable" || !strings.Contains(got[0]["message"].(string), "192.168.1.50:11434: connection refused") {
		t.Fatalf("expected the LLM outage, got %v", got)
	}
	reachable = true
	setClock(start.Add(11 * time.Minute))
	evaluateNotifications(context.Background())
	if got := recv.received(); len(got) != 2 || got[1]["event"] != notifyEventResolved {
		t.Fatalf("expected the outage resolved, got %v", got)
	}
}

func TestNotificationsAreCappedPerHour(t *testing.T) {
	recv, setClock := setupNotifications(t, "json")
	resetJobHistoryForTests()
	t.Cleanup(resetJobHistoryForTests)
	original := notifyMaxPerHour
	notifyMaxPerHour = 2
	t.Cleanup(func() { notifyMaxPerHour = original })
	for _, name := range []string{"JobA", "JobB", "JobC"} {
		for i := 0; i < 3; i++ {
			_ = runJobWith(name, zap.NewNop().Sugar(), func(*zap.SugaredLogger) error { return errors.New("boom") })
		}
	}
	start := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	evaluateNotifications(context.Background())
	if n := len(recv.received()); n != 2 {
		t.Fatalf("expected 2 messages under the cap, got %d", n)
	}
	setClock(start.Add(time.Hour + time.Minute))
	evaluateNotifications(context.Background())
	got := recv.received()
	if len(got) != 3 || got[2]["condition"] != "job_failing:JobC" {
		t.Fatalf("the capped message was not sent later: %v", got)
	}
}

func TestNotificationsOffMakeNoRequests(t *testing.T) {
	recv, _ := setupNotifications(t, "json")
	t.Setenv(notifyURLEnv, "")
	for i := 0; i < 3; i++ {
		recordAttempt(t, "x", SummaryOutcomeFailed, errors.New("x"))
	}
	evaluateNotifications(context.Background())
	if err := SendTestNotification(context.Background()); !errors.Is(err, ErrNotificationsOff) {
		t.Fatalf("expected notifications off, got %v", err)
	}
	if recv.requests.Load() != 0 || NotificationStatusSnapshot().Enabled {
		t.Fatal("notifications made a request or reported on while off")
	}
}

func TestNotificationPrivateHostIsRejectedUnlessAllowed(t *testing.T) {
	recv, _ := setupNotifications(t, "json")
	t.Setenv(outboundAllowPrivateNotifyEnv, "")
	err := SendTestNotification(context.Background())
	if err == nil || !strings.Contains(err.Error(), "private or loopback") || recv.requests.Load() != 0 {
		t.Fatalf("a LAN destination must be refused by default: err=%v requests=%d", err, recv.requests.Load())
	}
	if status := NotificationStatusSnapshot(); status.Enabled || status.Error == "" {
		t.Fatalf("status should report the unusable URL: %+v", status)
	}
}

func TestNotificationDeliveryRetriesAndNeverShowsTheURL(t *testing.T) {
	recv, _ := setupNotifications(t, "json")
	recv.status = func(n int) int {
		if n <= 2 {
			return http.StatusServiceUnavailable
		}
		return http.StatusOK
	}
	if err := SendTestNotification(context.Background()); err != nil || recv.requests.Load() != 3 {
		t.Fatalf("expected success on the third try: err=%v requests=%d", err, recv.requests.Load())
	}

	recv.status = func(int) int { return http.StatusBadRequest }
	before := recv.requests.Load()
	err := SendTestNotification(context.Background())
	if err == nil || recv.requests.Load()-before != 1 || !strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("a 400 must not be retried: err=%v requests=%d", err, recv.requests.Load()-before)
	}
	if strings.Contains(err.Error(), recv.server.URL) || strings.Contains(err.Error(), "briefcast-alerts") {
		t.Fatalf("error exposed the destination: %v", err)
	}

	listener, lerr := net.Listen("tcp", "127.0.0.1:0")
	if lerr != nil {
		t.Fatal(lerr)
	}
	closed := listener.Addr().String()
	_ = listener.Close()
	t.Setenv(notifyURLEnv, "http://"+closed+"/secret-topic")
	err = SendTestNotification(context.Background())
	if err == nil || strings.Contains(err.Error(), closed) || strings.Contains(err.Error(), "secret-topic") {
		t.Fatalf("unreachable error must not show the URL: %v", err)
	}
}

func TestNotificationTokenFileSendsBearerAuth(t *testing.T) {
	recv, _ := setupNotifications(t, "json")
	token := filepath.Join(t.TempDir(), "ntfy-token")
	if err := os.WriteFile(token, []byte("tk_secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(notifyTokenFileEnv, token)
	if err := SendTestNotification(context.Background()); err != nil {
		t.Fatal(err)
	}
	recv.mu.Lock()
	auth := recv.headers[0].Get("Authorization")
	recv.mu.Unlock()
	if auth != "Bearer tk_secret" {
		t.Fatalf("unexpected Authorization header %q", auth)
	}
	t.Setenv(notifyTokenFileEnv, filepath.Join(t.TempDir(), "missing"))
	if err := SendTestNotification(context.Background()); err == nil || !strings.Contains(err.Error(), notifyTokenFileEnv) {
		t.Fatalf("a missing token file must fail clearly: %v", err)
	}
}

func TestNotificationsNeverCarryFullURLs(t *testing.T) {
	recv, _ := setupNotifications(t, "json")
	resetJobHistoryForTests()
	t.Cleanup(resetJobHistoryForTests)
	for i := 0; i < 3; i++ {
		_ = runJobWith("RefreshEpisodes", zap.NewNop().Sugar(), func(*zap.SugaredLogger) error {
			return errors.New(`Get "https://www.patreon.com/rss/private-show?auth=SECRET-TOKEN": dial tcp: no such host`)
		})
	}
	recordAttempt(t, "a", SummaryOutcomeTimedOut, errors.New(`LLM request failed: Post "http://user:pw@192.168.1.50:11434/v1/chat/completions": context deadline exceeded`))
	recordAttempt(t, "b", SummaryOutcomeTimedOut, errors.New("deadline"))
	recordAttempt(t, "c", SummaryOutcomeTimedOut, errors.New(`LLM request failed: Post "http://user:pw@192.168.1.50:11434/v1/chat/completions": context deadline exceeded`))
	evaluateNotifications(context.Background())

	got := recv.received()
	if len(got) != 2 {
		t.Fatalf("expected the job and summary alerts, got %v", got)
	}
	raw, _ := json.Marshal(got)
	for _, secret := range []string{"SECRET-TOKEN", "private-show", "/rss/", "pw@", "user:", "/v1/chat"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("notification carried %q: %s", secret, raw)
		}
	}
	for _, kept := range []string{"https://www.patreon.com/…", "http://192.168.1.50:11434/…", "no such host"} {
		if !strings.Contains(string(raw), kept) {
			t.Fatalf("notification lost %q: %s", kept, raw)
		}
	}
	if got := redactURLs("plain text, 3 runs"); got != "plain text, 3 runs" {
		t.Fatalf("text without URLs changed: %q", got)
	}
	if got := redactURLs("see https://ntfy.sh now"); got != "see https://ntfy.sh now" {
		t.Fatalf("a bare host must stay as it is: %q", got)
	}
}

func TestNotificationBodiesPerFormat(t *testing.T) {
	n := notification{Key: "summaries_failing", Event: notifyEventFiring, Title: "3 summaries failed in a row", Message: "Last: timed out on “Friday Wrap-Up”.", Since: time.Unix(0, 0)}
	parse := func(raw []byte) map[string]any {
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	mustURL := func(raw string) notifyConfig {
		u, err := validateOutboundURLWithPolicy(raw, outboundURLPolicy{purpose: outboundPurposeNotify, allowPrivateHosts: true})
		if err != nil {
			t.Fatal(err)
		}
		return notifyConfig{URL: u, Format: inferNotifyFormat(u)}
	}

	ntfy := mustURL("https://ntfy.sh/my-alerts")
	target, raw, _, err := notificationRequest(ntfy, n)
	body := parse(raw)
	if err != nil || ntfy.Format != notifyFormatNtfy || target != "https://ntfy.sh/" || body["topic"] != "my-alerts" || body["priority"] != float64(4) || !strings.Contains(body["message"].(string), "“Friday Wrap-Up”") {
		t.Fatalf("ntfy request: target=%s body=%v err=%v", target, body, err)
	}
	if _, _, _, err := notificationRequest(mustURL("https://ntfy.sh/"), n); err == nil {
		t.Fatal("an ntfy URL without a topic must be refused")
	}

	discord := mustURL("https://discord.com/api/webhooks/1/abc")
	_, raw, _, _ = notificationRequest(discord, n)
	if discord.Format != notifyFormatDiscord || !strings.HasPrefix(parse(raw)["content"].(string), "**3 summaries failed in a row**\n") {
		t.Fatalf("discord body: %s", raw)
	}
	slack := mustURL("https://hooks.slack.com/services/T/B/x")
	_, raw, _, _ = notificationRequest(slack, n)
	if slack.Format != notifyFormatSlack || !strings.HasPrefix(parse(raw)["text"].(string), "*3 summaries failed in a row*\n") {
		t.Fatalf("slack body: %s", raw)
	}
	generic := mustURL("https://example.com/hook")
	target, raw, _, _ = notificationRequest(generic, n)
	body = parse(raw)
	if generic.Format != notifyFormatJSON || target != "https://example.com/hook" || body["event"] != "firing" || body["condition"] != "summaries_failing" || body["app"] != "briefcast" {
		t.Fatalf("json body: %v", body)
	}
	t.Setenv(notifyURLEnv, "https://example.com/hook")
	t.Setenv(notifyFormatEnv, "email")
	if _, _, err := loadNotifyConfig(); err == nil {
		t.Fatal("an unknown format must be refused")
	}
}
