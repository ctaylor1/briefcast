package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ctaylor1/briefcast/internal/logging"
	"github.com/ctaylor1/briefcast/internal/telemetry"
)

// Failure notifications push a short message when part of the pipeline needs
// attention. They are off unless BRIEFCAST_NOTIFY_URL is set, and they read
// only in-memory state (summary attempts, job history, the cached LLM check
// and the worker summary), never transcript, prompt or summary text.

const (
	notifyURLEnv       = "BRIEFCAST_NOTIFY_URL"
	notifyFormatEnv    = "BRIEFCAST_NOTIFY_FORMAT"
	notifyTokenFileEnv = "BRIEFCAST_NOTIFY_TOKEN_FILE"

	notifyFormatNtfy    = "ntfy"
	notifyFormatDiscord = "discord"
	notifyFormatSlack   = "slack"
	notifyFormatJSON    = "json"

	notifyEventFiring   = "firing"
	notifyEventReminder = "reminder"
	notifyEventResolved = "resolved"
	notifyEventTest     = "test"
)

// Tunables; tests shorten them.
var (
	notifyEvaluateEvery    = time.Minute
	notifyReminderEvery    = 6 * time.Hour
	notifyMaxPerHour       = 10
	notifyOutageAfter      = 10 * time.Minute
	notifyJobFailingAfter  = time.Hour
	notifySendTimeout      = 10 * time.Second
	notifyRetryDelays      = []time.Duration{time.Second, 3 * time.Second}
	notifyLowDiskBytes     = uint64(10 * 1024 * 1024 * 1024)
	notifySlowShare        = 0.8
	notifyFailureStreak    = 3
	notifyMessageMaxChars  = 900
	notifyDiscordMaxChars  = 1900
	errNotificationsOff    = errors.New("notifications are off: BRIEFCAST_NOTIFY_URL is not set")
	errNotificationCapped  = errors.New("notification rate limit reached")
	notifyWorkerSummary    = summarizeWorker
	notifyLLMServerCheck   = CheckLLMServer
	notifyAssetsFreeBytes  = func() (uint64, error) { return availableBytes(getEnvString("DATA", "/assets")) }
	notifyNow              = time.Now
	notificationLoopActive sync.Once
)

// ErrNotificationsOff is returned by SendTestNotification when no URL is set.
var ErrNotificationsOff = errNotificationsOff

type notifyConfig struct {
	URL       *url.URL
	Format    string
	TokenFile string
}

// loadNotifyConfig reads the notification settings. ok is false when they
// are off; err reports a set but unusable URL.
func loadNotifyConfig() (notifyConfig, bool, error) {
	raw := strings.TrimSpace(os.Getenv(notifyURLEnv))
	if raw == "" {
		return notifyConfig{}, false, nil
	}
	parsed, err := validateOutboundURL(raw, outboundPurposeNotify)
	if err != nil {
		return notifyConfig{}, true, err
	}
	format := strings.ToLower(strings.TrimSpace(os.Getenv(notifyFormatEnv)))
	if format == "" {
		format = inferNotifyFormat(parsed)
	}
	switch format {
	case notifyFormatNtfy, notifyFormatDiscord, notifyFormatSlack, notifyFormatJSON:
	default:
		return notifyConfig{}, true, fmt.Errorf("%s must be ntfy, discord, slack or json", notifyFormatEnv)
	}
	return notifyConfig{URL: parsed, Format: format, TokenFile: strings.TrimSpace(os.Getenv(notifyTokenFileEnv))}, true, nil
}

func inferNotifyFormat(u *url.URL) string {
	host := strings.ToLower(u.Hostname())
	switch {
	case host == "ntfy.sh" || strings.HasPrefix(host, "ntfy."):
		return notifyFormatNtfy
	case host == "discord.com" || host == "discordapp.com":
		return notifyFormatDiscord
	case host == "hooks.slack.com":
		return notifyFormatSlack
	default:
		return notifyFormatJSON
	}
}

// notification is one message about one condition.
type notification struct {
	Key     string
	Event   string
	Title   string
	Message string
	Since   time.Time
}

type notifyCondition struct {
	Title    string
	Message  string
	Since    time.Time
	LastSent time.Time
}

// NotificationStatus is the notification part of the Monitoring snapshot.
type NotificationStatus struct {
	Enabled    bool                 `json:"enabled"`
	Host       string               `json:"host,omitempty"`
	Format     string               `json:"format,omitempty"`
	Error      string               `json:"error,omitempty"`
	Active     []ActiveNotification `json:"active"`
	LastSentAt *time.Time           `json:"last_sent_at,omitempty"`
	LastError  string               `json:"last_error,omitempty"`
}

// ActiveNotification is a condition that is currently firing.
type ActiveNotification struct {
	Key   string    `json:"key"`
	Title string    `json:"title"`
	Since time.Time `json:"since"`
}

var notifier = struct {
	mu         sync.Mutex
	active     map[string]*notifyCondition
	pending    map[string]time.Time // outage conditions not yet long enough to fire
	sent       []time.Time          // send times in the last hour, for the cap
	lastSentAt *time.Time
	lastError  string
}{active: map[string]*notifyCondition{}, pending: map[string]time.Time{}}

func resetNotifierForTests() {
	notifier.mu.Lock()
	defer notifier.mu.Unlock()
	notifier.active = map[string]*notifyCondition{}
	notifier.pending = map[string]time.Time{}
	notifier.sent = nil
	notifier.lastSentAt = nil
	notifier.lastError = ""
}

// StartFailureNotifications starts the evaluation loop when notifications are
// configured. With BRIEFCAST_NOTIFY_URL unset it does nothing.
func StartFailureNotifications() {
	cfg, ok, err := loadNotifyConfig()
	if !ok {
		return
	}
	if err != nil {
		logging.Sugar().Warnw("notifications not started", "error", err)
		return
	}
	notificationLoopActive.Do(func() {
		logging.Sugar().Infow("notifications started", "host", cfg.URL.Host, "format", cfg.Format)
		SafeGo("failure notifications", func() {
			ticker := time.NewTicker(notifyEvaluateEvery)
			defer ticker.Stop()
			for range ticker.C {
				evaluateNotifications(context.Background())
			}
		})
	})
}

// evaluateNotifications compares the conditions now with the ones already
// reported and sends what changed.
func evaluateNotifications(ctx context.Context) {
	cfg, ok, err := loadNotifyConfig()
	if !ok || err != nil {
		return
	}
	now := notifyNow().UTC()
	current := currentNotifyConditions(ctx, now)

	var outgoing []notification
	notifier.mu.Lock()
	for key, cond := range current {
		existing, firing := notifier.active[key]
		switch {
		case !firing:
			notifier.active[key] = &notifyCondition{Title: cond.Title, Message: cond.Message, Since: cond.Since}
			outgoing = append(outgoing, notification{Key: key, Event: notifyEventFiring, Title: cond.Title, Message: cond.Message, Since: cond.Since})
		case now.Sub(existing.LastSent) >= notifyReminderEvery:
			existing.Title, existing.Message = cond.Title, cond.Message
			outgoing = append(outgoing, notification{Key: key, Event: notifyEventReminder, Title: cond.Title, Message: cond.Message, Since: existing.Since})
		default:
			existing.Title, existing.Message = cond.Title, cond.Message
		}
	}
	for key, existing := range notifier.active {
		if _, still := current[key]; still {
			continue
		}
		delete(notifier.active, key)
		outgoing = append(outgoing, notification{Key: key, Event: notifyEventResolved, Title: "Resolved: " + existing.Title, Message: "This has recovered.", Since: existing.Since})
	}
	notifier.mu.Unlock()

	sort.Slice(outgoing, func(i, j int) bool { return outgoing[i].Key < outgoing[j].Key })
	for _, n := range outgoing {
		if err := deliverNotification(ctx, cfg, n, true); err == nil && n.Event != notifyEventResolved {
			notifier.mu.Lock()
			if cond, ok := notifier.active[n.Key]; ok {
				cond.LastSent = now
			}
			notifier.mu.Unlock()
		}
	}
}

// currentNotifyConditions lists the conditions that hold now, keyed by a
// stable name. Outage conditions must hold for notifyOutageAfter first.
func currentNotifyConditions(ctx context.Context, now time.Time) map[string]notifyCondition {
	out := map[string]notifyCondition{}
	summaryConditions(out, now)
	for _, job := range JobHistorySnapshot() {
		if job.ConsecutiveFailures == 0 || job.FailingSince == nil {
			continue
		}
		if job.ConsecutiveFailures < notifyFailureStreak && now.Sub(*job.FailingSince) < notifyJobFailingAfter {
			continue
		}
		out["job_failing:"+job.Name] = notifyCondition{
			Title:   job.Name + " is failing",
			Message: fmt.Sprintf("%d runs in a row since %s: %s", job.ConsecutiveFailures, job.FailingSince.Local().Format("Jan 2 15:04"), job.LastError),
			Since:   *job.FailingSince,
		}
	}

	outage := map[string]notifyCondition{}
	if summariesActive() {
		if health, checked := notifyLLMServerCheck(ctx); checked && !health.Reachable {
			outage["llm_unreachable"] = notifyCondition{Title: "LLM server unreachable", Message: fmt.Sprintf("%s: %s. Summaries cannot run.", health.Host, health.Error)}
		}
	}
	if worker := notifyWorkerSummary(ctx); worker.Configured && (!worker.Reachable || !worker.Healthy) {
		message := "The WhisperX worker reports a fault; transcription is stopped."
		if !worker.Reachable {
			message = "The app cannot reach the WhisperX worker; transcription is stopped."
		}
		outage["worker_down"] = notifyCondition{Title: "WhisperX worker down", Message: message}
	}
	notifier.mu.Lock()
	for key := range notifier.pending {
		if _, still := outage[key]; !still {
			delete(notifier.pending, key)
		}
	}
	for key, cond := range outage {
		first, seen := notifier.pending[key]
		if !seen {
			notifier.pending[key] = now
			first = now
		}
		if now.Sub(first) >= notifyOutageAfter {
			cond.Since = first
			out[key] = cond
		}
	}
	notifier.mu.Unlock()

	if free, err := notifyAssetsFreeBytes(); err == nil && free < notifyLowDiskBytes {
		out["disk_low"] = notifyCondition{Title: "Low disk space for audio", Message: fmt.Sprintf("%s free on the audio and export volume.", formatNotifyBytes(free)), Since: now}
	}
	return out
}

func summariesActive() bool {
	cfg := LoadLLMConfig()
	if !cfg.Enabled || strings.TrimSpace(cfg.APIKey) == "" {
		return false
	}
	setting := currentSetting()
	return setting != nil && setting.SummarizationEnabled
}

func summaryConditions(out map[string]notifyCondition, now time.Time) {
	snap := summaryActivitySnapshot(summaryRecentLimit)
	if snap.ConsecutiveFailures >= notifyFailureStreak && len(snap.Recent) > 0 {
		last := snap.Recent[0]
		since := last.StartedAt
		for _, attempt := range snap.Recent[:min(snap.ConsecutiveFailures, len(snap.Recent))] {
			since = attempt.StartedAt
		}
		out["summaries_failing"] = notifyCondition{
			Title:   fmt.Sprintf("%d summaries failed in a row", snap.ConsecutiveFailures),
			Message: fmt.Sprintf("Last: %s on %q. %s", summaryOutcomeText(last.Outcome), last.Title, last.Error),
			Since:   since,
		}
	}
	timeout := time.Duration(LoadLLMConfig().TimeoutSecs) * time.Second
	if timeout <= 0 {
		return
	}
	var slow []SummaryAttempt
	for _, attempt := range snap.Recent {
		if attempt.Outcome != SummaryOutcomeSucceeded {
			continue
		}
		if time.Duration(attempt.DurationMS)*time.Millisecond < time.Duration(float64(timeout)*notifySlowShare) {
			break
		}
		slow = append(slow, attempt)
		if len(slow) == notifyFailureStreak {
			out["summaries_slow"] = notifyCondition{
				Title:   "Summaries are close to their time limit",
				Message: fmt.Sprintf("The last %d took over %d%% of the %s limit (LLM_TIMEOUT_SECONDS).", notifyFailureStreak, int(notifySlowShare*100), timeout),
				Since:   slow[len(slow)-1].StartedAt,
			}
			return
		}
	}
}

func summaryOutcomeText(outcome string) string {
	switch outcome {
	case SummaryOutcomeTokenLimit:
		return "output token limit"
	case SummaryOutcomeTimedOut:
		return "timed out"
	case SummaryOutcomePanicked:
		return "crashed"
	default:
		return "failed"
	}
}

func formatNotifyBytes(b uint64) string {
	return fmt.Sprintf("%.1f GB", float64(b)/(1024*1024*1024))
}

// SendTestNotification sends one test message now, outside the rate cap.
func SendTestNotification(ctx context.Context) error {
	cfg, ok, err := loadNotifyConfig()
	if !ok {
		return ErrNotificationsOff
	}
	if err != nil {
		return err
	}
	return deliverNotification(ctx, cfg, notification{Key: "test", Event: notifyEventTest, Title: "Briefcast test notification", Message: "Notifications from Briefcast reach this channel.", Since: notifyNow().UTC()}, false)
}

// deliverNotification sends n with retries. capped applies the hourly limit.
func deliverNotification(ctx context.Context, cfg notifyConfig, n notification, capped bool) error {
	if capped && !takeNotifySlot(notifyNow()) {
		telemetry.ObserveNotification("suppressed")
		logging.Sugar().Warnw("notification_suppressed", "condition", n.Key, "event", n.Event, "reason", "rate limit")
		return errNotificationCapped
	}
	var err error
	for attempt := 0; ; attempt++ {
		var retry bool
		retry, err = sendNotification(ctx, cfg, n)
		if err == nil || !retry || attempt >= len(notifyRetryDelays) {
			break
		}
		select {
		case <-ctx.Done():
			err = ctx.Err()
		case <-time.After(notifyRetryDelays[attempt]):
			continue
		}
		break
	}
	now := notifyNow().UTC()
	notifier.mu.Lock()
	if err != nil {
		notifier.lastError = err.Error()
	} else {
		notifier.lastSentAt, notifier.lastError = &now, ""
	}
	notifier.mu.Unlock()
	if err != nil {
		telemetry.ObserveNotification("failed")
		logging.Sugar().Warnw("notification_failed", "condition", n.Key, "event", n.Event, "format", cfg.Format, "error", err)
		return err
	}
	telemetry.ObserveNotification("sent")
	logging.Sugar().Infow("notification_sent", "condition", n.Key, "event", n.Event, "format", cfg.Format)
	return nil
}

func takeNotifySlot(now time.Time) bool {
	notifier.mu.Lock()
	defer notifier.mu.Unlock()
	kept := notifier.sent[:0]
	for _, at := range notifier.sent {
		if now.Sub(at) < time.Hour {
			kept = append(kept, at)
		}
	}
	notifier.sent = kept
	if len(notifier.sent) >= notifyMaxPerHour {
		return false
	}
	notifier.sent = append(notifier.sent, now)
	return true
}

// sendNotification makes one delivery attempt. retry reports whether a
// later attempt might succeed. Errors never include the destination URL.
func sendNotification(ctx context.Context, cfg notifyConfig, n notification) (retry bool, err error) {
	target, body, contentType, err := notificationRequest(cfg, n)
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(ctx, notifySendTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return false, errors.New("could not build the notification request")
	}
	req.Header.Set("Content-Type", contentType)
	userAgent, _ := CurrentOutboundUserAgent()
	req.Header.Set("User-Agent", userAgent)
	if cfg.TokenFile != "" {
		token, readErr := os.ReadFile(cfg.TokenFile)
		if readErr != nil || strings.TrimSpace(string(token)) == "" {
			return false, fmt.Errorf("%s is unreadable or empty", notifyTokenFileEnv)
		}
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	}
	client := &http.Client{
		Transport:     outboundHTTPTransport(outboundPurposeNotify),
		CheckRedirect: checkOutboundRedirect(outboundPurposeNotify),
		Timeout:       notifySendTimeout,
	}
	resp, err := client.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		if isTimeoutError(err) {
			return true, errors.New("the notification service did not answer in time")
		}
		if strings.Contains(err.Error(), "outbound URL rejected") {
			return false, errors.New("the notification URL is not allowed by the outbound URL policy")
		}
		return true, errors.New("the notification service could not be reached")
	}
	defer resp.Body.Close()
	_, _ = readBoundedOutboundBody(resp.Body, resp.ContentLength)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return false, nil
	}
	retry = resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
	return retry, fmt.Errorf("the notification service answered HTTP %d", resp.StatusCode)
}

// notificationRequest builds the URL and body for the configured format.
func notificationRequest(cfg notifyConfig, n notification) (string, []byte, string, error) {
	title := truncateRunes(redactURLs(n.Title), 200)
	message := truncateRunes(redactURLs(n.Message), notifyMessageMaxChars)
	var payload any
	target := cfg.URL.String()
	switch cfg.Format {
	case notifyFormatNtfy:
		// JSON publishing keeps UTF-8 titles intact (ntfy headers must be ASCII).
		topic := strings.Trim(cfg.URL.Path, "/")
		if topic == "" || strings.Contains(topic, "/") {
			return "", nil, "", errors.New("the ntfy URL must end with the topic, e.g. https://ntfy.sh/my-topic")
		}
		root := *cfg.URL
		root.Path, root.RawQuery = "/", ""
		target = root.String()
		priority, tags := 4, []string{"warning"}
		switch n.Event {
		case notifyEventResolved:
			priority, tags = 3, []string{"white_check_mark"}
		case notifyEventTest:
			priority, tags = 3, []string{"bell"}
		}
		payload = map[string]any{"topic": topic, "title": title, "message": message, "priority": priority, "tags": tags}
	case notifyFormatDiscord:
		payload = map[string]any{"content": truncateRunes("**"+title+"**\n"+message, notifyDiscordMaxChars)}
	case notifyFormatSlack:
		payload = map[string]any{"text": "*" + title + "*\n" + message}
	default:
		payload = map[string]any{"app": "briefcast", "event": n.Event, "condition": n.Key, "title": title, "message": message, "since": n.Since.UTC().Format(time.RFC3339)}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", nil, "", err
	}
	return target, body, "application/json", nil
}

var notifyURLPattern = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s"'<>]+`)

// redactURLs keeps only the scheme and host of every URL in s. Job and summary
// errors can quote a feed URL with an access token in its path or query
// (private feeds) or the LLM URL; a notification leaves the machine, so
// neither is sent.
func redactURLs(s string) string {
	return notifyURLPattern.ReplaceAllStringFunc(s, func(raw string) string {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return "[link]"
		}
		host := u.Host
		if u.User != nil {
			host = u.Hostname()
			if port := u.Port(); port != "" {
				host += ":" + port
			}
		}
		if u.Path == "" && u.RawQuery == "" && u.Fragment == "" {
			return u.Scheme + "://" + host
		}
		return u.Scheme + "://" + host + "/…"
	})
}

func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit-1]) + "…"
}

// NotificationStatusSnapshot reports the notification setup and the
// conditions currently firing, for Monitoring.
func NotificationStatusSnapshot() NotificationStatus {
	out := NotificationStatus{Active: []ActiveNotification{}}
	cfg, ok, err := loadNotifyConfig()
	if !ok {
		return out
	}
	out.Enabled = err == nil
	if err != nil {
		out.Error = err.Error()
		return out
	}
	out.Host, out.Format = cfg.URL.Host, cfg.Format
	notifier.mu.Lock()
	defer notifier.mu.Unlock()
	for key, cond := range notifier.active {
		out.Active = append(out.Active, ActiveNotification{Key: key, Title: cond.Title, Since: cond.Since})
	}
	sort.Slice(out.Active, func(i, j int) bool { return out.Active[i].Key < out.Active[j].Key })
	out.LastSentAt = cloneTimePointer(notifier.lastSentAt)
	out.LastError = notifier.lastError
	return out
}
