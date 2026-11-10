package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestServeExitsOnStateFault(t *testing.T) {
	faulted := make(chan struct{})
	close(faulted)
	err := serveUntilStopped(context.Background(), make(chan error), faulted, nil, func() error { return nil })
	if !errors.Is(err, errWorkerFault) {
		t.Fatalf("serve did not exit for recovery: %v", err)
	}
}

func TestServeReturnsServerErrorAndStopsOnSignal(t *testing.T) {
	stopped := make(chan error, 1)
	boom := errors.New("listen failed")
	stopped <- boom
	if err := serveUntilStopped(context.Background(), stopped, make(chan struct{}), nil, nil); !errors.Is(err, boom) {
		t.Fatalf("server error lost: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := serveUntilStopped(ctx, make(chan error), make(chan struct{}), nil, nil); err != nil {
		t.Fatalf("signal stop returned %v", err)
	}
}

func TestServeLogsGCFailureCauseAndKeepsServing(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	gc := make(chan time.Time, 1)
	gc <- time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := serveUntilStopped(ctx, make(chan error), make(chan struct{}), gc, func() error {
		calls++
		cancel() // stop after the failed GC tick has been handled
		return errors.New("prune disk error")
	})
	if err != nil || calls != 1 {
		t.Fatalf("gc failure stopped serving: err=%v calls=%d", err, calls)
	}
	if !strings.Contains(logs.String(), "worker_gc_failed") || !strings.Contains(logs.String(), "prune disk error") {
		t.Fatalf("gc failure cause not logged: %s", logs.String())
	}
}
