package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	// No WriteTimeout or ReadTimeout: media and exports are streamed and
	// OPML uploads can be slow. Header and idle timeouts bound idle clients.
	httpReadHeaderTimeout = 10 * time.Second
	httpIdleTimeout       = 120 * time.Second
	httpShutdownTimeout   = 10 * time.Second

	// defaultMaxRequestBodyBytes caps request bodies on every route that does
	// not set its own limit. JSON settings and edits are far smaller.
	defaultMaxRequestBodyBytes int64 = 1 << 20
)

// routesWithOwnBodyLimit read large bodies under their own, larger cap.
var routesWithOwnBodyLimit = map[string]bool{
	"/opml": true,
}

// resolveListenAddr matches gin.Engine.Run: PORT when set, otherwise :8080.
func resolveListenAddr() string {
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		return ":" + port
	}
	return ":8080"
}

func newHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              resolveListenAddr(),
		Handler:           handler,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		IdleTimeout:       httpIdleTimeout,
	}
}

// serveUntilSignal runs srv until it fails or SIGINT/SIGTERM arrives, then
// shuts down, waiting at most httpShutdownTimeout for in-flight requests.
func serveUntilSignal(srv *http.Server, logger *zap.SugaredLogger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	listener, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return err
	}
	return serveUntilDone(ctx, srv, listener, logger)
}

func serveUntilDone(ctx context.Context, srv *http.Server, listener net.Listener, logger *zap.SugaredLogger) error {
	serveErr := make(chan error, 1)
	// Not SafeGo: this is the server itself. A recovered panic here would leave
	// main waiting forever instead of exiting.
	go func() { serveErr <- srv.Serve(listener) }()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	logger.Infow("shutting down http server", "timeout_seconds", int(httpShutdownTimeout.Seconds()))
	shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		// Long streams (media, exports) can outlast the deadline; cut them off.
		logger.Warnw("http server did not finish in-flight requests before the deadline", "error", err)
		if closeErr := srv.Close(); closeErr != nil {
			logger.Warnw("closing http server failed", "error", closeErr)
		}
	}
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// limitRequestBody caps request bodies at max bytes. Requests that declare a
// larger Content-Length get 413 immediately; others fail when the handler
// reads past the cap.
func limitRequestBody(max int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil || c.Request.Body == http.NoBody || routesWithOwnBodyLimit[c.FullPath()] {
			c.Next()
			return
		}
		if c.Request.ContentLength > max {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max)
		c.Next()
	}
}
