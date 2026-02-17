package service

import (
	"context"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestHostKeyUsesHostnameLowercase(t *testing.T) {
	u, err := url.Parse("https://Example.COM:8443/path")
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}

	key := hostKey(u)
	if key != "example.com" {
		t.Fatalf("expected host key example.com, got %q", key)
	}
}

func TestHostLimiterConcurrencyBound(t *testing.T) {
	limiter := &hostRequestLimiter{
		maxConcurrency: 1,
		minInterval:    0,
		hosts:          map[string]*singleHostLimiter{},
	}

	u, err := url.Parse("https://feeds.example.com/rss")
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}

	var active int32
	var maxActive int32
	var wg sync.WaitGroup

	work := func() {
		defer wg.Done()

		release, acquireErr := limiter.acquire(context.Background(), u)
		if acquireErr != nil {
			t.Errorf("acquire failed: %v", acquireErr)
			return
		}
		defer release()

		current := atomic.AddInt32(&active, 1)
		for {
			recorded := atomic.LoadInt32(&maxActive)
			if current <= recorded {
				break
			}
			if atomic.CompareAndSwapInt32(&maxActive, recorded, current) {
				break
			}
		}

		time.Sleep(25 * time.Millisecond)
		atomic.AddInt32(&active, -1)
	}

	for i := 0; i < 6; i++ {
		wg.Add(1)
		go work()
	}

	wg.Wait()

	if maxActive > 1 {
		t.Fatalf("expected max active requests to be 1, got %d", maxActive)
	}
}

func TestHostLimiterRatePacing(t *testing.T) {
	minInterval := 60 * time.Millisecond
	limiter := &hostRequestLimiter{
		maxConcurrency: 5,
		minInterval:    minInterval,
		hosts:          map[string]*singleHostLimiter{},
	}

	u, err := url.Parse("https://api.example.com/search")
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}

	times := make([]time.Time, 0, 3)
	for i := 0; i < 3; i++ {
		release, acquireErr := limiter.acquire(context.Background(), u)
		if acquireErr != nil {
			t.Fatalf("acquire failed: %v", acquireErr)
		}
		times = append(times, time.Now())
		release()
	}

	firstGap := times[1].Sub(times[0])
	secondGap := times[2].Sub(times[1])
	minExpected := minInterval - 15*time.Millisecond

	if firstGap < minExpected {
		t.Fatalf("expected first pacing gap >= %s, got %s", minExpected, firstGap)
	}
	if secondGap < minExpected {
		t.Fatalf("expected second pacing gap >= %s, got %s", minExpected, secondGap)
	}
}

func TestOutboundRequestLimiterCacheReset(t *testing.T) {
	resetOutboundRequestLimiterForTests()
	t.Cleanup(resetOutboundRequestLimiterForTests)

	t.Setenv("PER_HOST_MAX_CONCURRENCY", "2")
	t.Setenv("PER_HOST_RATE_LIMIT_RPS", "5")
	first := getOutboundRequestLimiter()
	if first.maxConcurrency != 2 {
		t.Fatalf("expected first limiter concurrency 2, got %d", first.maxConcurrency)
	}

	t.Setenv("PER_HOST_MAX_CONCURRENCY", "7")
	second := getOutboundRequestLimiter()
	if second != first {
		t.Fatalf("expected cached limiter instance without reset")
	}
	if second.maxConcurrency != 2 {
		t.Fatalf("expected cached limiter to retain old config 2, got %d", second.maxConcurrency)
	}

	resetOutboundRequestLimiterForTests()
	third := getOutboundRequestLimiter()
	if third == first {
		t.Fatalf("expected new limiter instance after reset")
	}
	if third.maxConcurrency != 7 {
		t.Fatalf("expected limiter to pick up new env config 7, got %d", third.maxConcurrency)
	}
}
