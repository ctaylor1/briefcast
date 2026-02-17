package logging

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func resetBaseLogger() {
	baseLogger = nil
	baseLoggerOnce = sync.Once{}
}

func TestParseLogLevel(t *testing.T) {
	if level, ok := parseLogLevel("debug"); !ok || level != zapcore.DebugLevel {
		t.Fatalf("expected debug level with ok=true, got %v, %v", level, ok)
	}
	if level, ok := parseLogLevel("invalid"); ok || level != zapcore.InfoLevel {
		t.Fatalf("expected fallback info level with ok=false, got %v, %v", level, ok)
	}
}

func TestParseLogFormat(t *testing.T) {
	if format := parseLogFormat("text"); format != "text" {
		t.Fatalf("expected text format, got %q", format)
	}
	if format := parseLogFormat("console"); format != "text" {
		t.Fatalf("expected console alias to map to text, got %q", format)
	}
	if format := parseLogFormat("json"); format != "json" {
		t.Fatalf("expected json format, got %q", format)
	}
	if format := parseLogFormat("invalid"); format != "json" {
		t.Fatalf("expected fallback json format, got %q", format)
	}
}

func TestEnvParsers(t *testing.T) {
	t.Setenv("LOG_TEST_INT", "42")
	if value := getEnvInt("LOG_TEST_INT", 7); value != 42 {
		t.Fatalf("expected int 42, got %d", value)
	}
	t.Setenv("LOG_TEST_INT", "bad")
	if value := getEnvInt("LOG_TEST_INT", 7); value != 7 {
		t.Fatalf("expected fallback int 7, got %d", value)
	}

	t.Setenv("LOG_TEST_BOOL", "yes")
	if value := getEnvBool("LOG_TEST_BOOL", false); !value {
		t.Fatalf("expected bool true")
	}
	t.Setenv("LOG_TEST_BOOL", "???")
	if value := getEnvBool("LOG_TEST_BOOL", true); !value {
		t.Fatalf("expected fallback true")
	}
}

func TestResolveLogOutputsDefaultAndFile(t *testing.T) {
	t.Setenv("LOG_OUTPUT", "")
	defaultOutputs := resolveLogOutputs()
	if len(defaultOutputs) != 1 {
		t.Fatalf("expected 1 default output, got %d", len(defaultOutputs))
	}

	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "logs", "app.log")
	t.Setenv("LOG_OUTPUT", "stdout,stderr,file:"+logFile)
	outputs := resolveLogOutputs()
	if len(outputs) != 3 {
		t.Fatalf("expected 3 outputs, got %d", len(outputs))
	}
	if _, err := filepath.Abs(logFile); err != nil {
		t.Fatalf("expected valid log path: %v", err)
	}
}

func TestLoggerWithRequestIDAndJobLogger(t *testing.T) {
	resetBaseLogger()
	t.Setenv("LOG_OUTPUT", "stdout")
	logger := LoggerWithRequestID(" req-123 ")
	if logger == nil {
		t.Fatalf("expected non-nil logger")
	}

	jobLogger, jobID := NewJobLogger("cleanup")
	if jobLogger == nil {
		t.Fatalf("expected non-nil job logger")
	}
	if strings.TrimSpace(jobID) == "" {
		t.Fatalf("expected non-empty job id")
	}
}

func TestRequestHelpers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	if RequestIDFromGin(nil) != "" {
		t.Fatalf("expected empty request id for nil context")
	}
	if LoggerFromGin(nil) == nil {
		t.Fatalf("expected base logger for nil context")
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(RequestIDHeader, "header-id")
	c.Request = req

	if got := RequestIDFromGin(c); got != "header-id" {
		t.Fatalf("expected request id from header, got %q", got)
	}

	custom := zap.NewNop()
	c.Set(RequestLoggerKey, custom)
	if LoggerFromGin(c) != custom {
		t.Fatalf("expected logger from gin context")
	}
}

func TestRequestLoggerMiddlewareSetsRequestIDHeader(t *testing.T) {
	resetBaseLogger()
	t.Setenv("LOG_OUTPUT", "stdout")
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RequestLoggerMiddleware())
	router.GET("/ok", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	requestID := strings.TrimSpace(resp.Header().Get(RequestIDHeader))
	if requestID == "" {
		t.Fatalf("expected response request id header")
	}
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
}
