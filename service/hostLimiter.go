package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultPerHostMaxConcurrency = 2
	defaultPerHostRateLimitRPS   = 2.0
)

var outboundRequestLimiter *hostRequestLimiter
var outboundRequestLimiterOnce sync.Once

type hostRequestLimiter struct {
	maxConcurrency int
	minInterval    time.Duration

	mu    sync.Mutex
	hosts map[string]*singleHostLimiter
}

type singleHostLimiter struct {
	sem         chan struct{}
	minInterval time.Duration

	mu          sync.Mutex
	nextAllowed time.Time
}

func newHostRequestLimiterFromEnv() *hostRequestLimiter {
	maxConcurrency := envInt("PER_HOST_MAX_CONCURRENCY", defaultPerHostMaxConcurrency)
	if maxConcurrency <= 0 {
		maxConcurrency = defaultPerHostMaxConcurrency
	}

	rps := envFloat("PER_HOST_RATE_LIMIT_RPS", defaultPerHostRateLimitRPS)
	minInterval := time.Duration(0)
	if rps > 0 {
		minInterval = time.Duration(float64(time.Second) / rps)
		if minInterval < time.Millisecond {
			minInterval = time.Millisecond
		}
	}

	return &hostRequestLimiter{
		maxConcurrency: maxConcurrency,
		minInterval:    minInterval,
		hosts:          map[string]*singleHostLimiter{},
	}
}

// doRequestWithHostLimit applies per-host pacing and concurrency caps before
// issuing outbound requests. This keeps fetch-heavy jobs stable when repeated
// at scale and prevents a single host from being overwhelmed.
func doRequestWithHostLimit(client *http.Client, req *http.Request) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	return getOutboundRequestLimiter().Do(req.Context(), client, req)
}

func getOutboundRequestLimiter() *hostRequestLimiter {
	outboundRequestLimiterOnce.Do(func() {
		outboundRequestLimiter = newHostRequestLimiterFromEnv()
	})
	return outboundRequestLimiter
}

func resetOutboundRequestLimiterForTests() {
	outboundRequestLimiter = nil
	outboundRequestLimiterOnce = sync.Once{}
}

func (limiter *hostRequestLimiter) Do(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
	release, err := limiter.acquire(ctx, req.URL)
	if err != nil {
		return nil, err
	}
	defer release()

	return client.Do(req)
}

func (limiter *hostRequestLimiter) acquire(ctx context.Context, requestURL *url.URL) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}

	hostLimiter := limiter.getHostLimiter(hostKey(requestURL))
	if hostLimiter.minInterval > 0 {
		delay := hostLimiter.reserveDelay()
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}

	select {
	case hostLimiter.sem <- struct{}{}:
		return func() {
			<-hostLimiter.sem
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (limiter *hostRequestLimiter) getHostLimiter(host string) *singleHostLimiter {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	entry, exists := limiter.hosts[host]
	if exists {
		return entry
	}

	entry = &singleHostLimiter{
		sem:         make(chan struct{}, limiter.maxConcurrency),
		minInterval: limiter.minInterval,
	}
	limiter.hosts[host] = entry
	return entry
}

func (limiter *singleHostLimiter) reserveDelay() time.Duration {
	now := time.Now()

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	if limiter.nextAllowed.IsZero() || !limiter.nextAllowed.After(now) {
		limiter.nextAllowed = now.Add(limiter.minInterval)
		return 0
	}

	wait := limiter.nextAllowed.Sub(now)
	limiter.nextAllowed = limiter.nextAllowed.Add(limiter.minInterval)
	return wait
}

func hostKey(requestURL *url.URL) string {
	if requestURL == nil {
		return "unknown"
	}

	host := strings.TrimSpace(strings.ToLower(requestURL.Hostname()))
	if host == "" {
		host = strings.TrimSpace(strings.ToLower(requestURL.Host))
	}
	if host == "" {
		return "unknown"
	}
	return host
}

func envInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func envFloat(name string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}

	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return value
}
