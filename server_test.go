package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestNewHTTPServerSetsTimeoutsAndAddress(t *testing.T) {
	t.Setenv("PORT", "")
	srv := newHTTPServer(http.NewServeMux())
	if srv.Addr != ":8080" {
		t.Fatalf("default addr: got %q", srv.Addr)
	}
	if srv.ReadHeaderTimeout != httpReadHeaderTimeout || srv.IdleTimeout != httpIdleTimeout {
		t.Fatalf("timeouts: header=%v idle=%v", srv.ReadHeaderTimeout, srv.IdleTimeout)
	}
	if srv.WriteTimeout != 0 || srv.ReadTimeout != 0 {
		t.Fatal("streamed media needs no read or write timeout")
	}
	t.Setenv("PORT", "9191")
	if got := newHTTPServer(http.NewServeMux()).Addr; got != ":9191" {
		t.Fatalf("PORT addr: got %q", got)
	}
}

func bodyLimitEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(limitRequestBody(defaultMaxRequestBodyBytes))
	read := func(c *gin.Context) {
		n, err := io.Copy(io.Discard, c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"read": n})
	}
	engine.PATCH("/settings", read)
	engine.POST("/opml", read)
	return engine
}

func TestLimitRequestBodyRejectsDeclaredOversizeBodies(t *testing.T) {
	engine := bodyLimitEngine()
	body := bytes.Repeat([]byte("a"), 2<<20)
	req := httptest.NewRequest(http.MethodPatch, "/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	if resp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", resp.Code)
	}
}

func TestLimitRequestBodyStopsUndeclaredOversizeBodies(t *testing.T) {
	engine := bodyLimitEngine()
	body := bytes.Repeat([]byte("a"), 2<<20)
	req := httptest.NewRequest(http.MethodPatch, "/settings", io.NopCloser(bytes.NewReader(body)))
	req.ContentLength = -1 // chunked: size unknown up front
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest || !strings.Contains(resp.Body.String(), "too large") {
		t.Fatalf("expected the read to stop at the cap, got %d %s", resp.Code, resp.Body.String())
	}
}

func TestLimitRequestBodyAllowsSmallBodiesAndOwnLimitRoutes(t *testing.T) {
	engine := bodyLimitEngine()
	small := httptest.NewRequest(http.MethodPatch, "/settings", strings.NewReader(`{"a":1}`))
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, small)
	if resp.Code != http.StatusOK {
		t.Fatalf("small body: got %d", resp.Code)
	}

	big := bytes.Repeat([]byte("a"), 2<<20)
	opml := httptest.NewRequest(http.MethodPost, "/opml", bytes.NewReader(big))
	resp = httptest.NewRecorder()
	engine.ServeHTTP(resp, opml)
	if resp.Code != http.StatusOK {
		t.Fatalf("OPML keeps its own larger limit, got %d", resp.Code)
	}
}

// setupSettings reads the database; with no database it panics. Recovery must
// sit outside it so the client gets a 500 instead of a dropped connection.
func TestBuildRouterRecoversPanicsInMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := db.DB
	t.Cleanup(func() { db.DB = previous })
	db.DB = nil

	engine := buildRouter()
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/podcasts", nil))
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 from Recovery, got %d", resp.Code)
	}
}

func TestServeUntilDoneFinishesInFlightRequestsOnShutdown(t *testing.T) {
	started := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		close(started)
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte("done"))
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: httpReadHeaderTimeout}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- serveUntilDone(ctx, srv, listener, zap.NewNop().Sugar()) }()

	type result struct {
		body string
		err  error
	}
	got := make(chan result, 1)
	go func() {
		resp, err := http.Get("http://" + listener.Addr().String() + "/slow")
		if err != nil {
			got <- result{err: err}
			return
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		got <- result{body: string(raw), err: err}
	}()

	<-started
	cancel()
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("serveUntilDone: %v", err)
		}
	case <-time.After(httpShutdownTimeout + 5*time.Second):
		t.Fatal("shutdown did not return")
	}
	r := <-got
	if r.err != nil || r.body != "done" {
		t.Fatalf("in-flight request was not completed: %q %v", r.body, r.err)
	}
}
