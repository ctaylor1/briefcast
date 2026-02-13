package logging

import (
	"os"
	"strings"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	RequestIDKey     = "request_id"
	RequestIDHeader  = "X-Request-ID"
	RequestLoggerKey = "request_logger"
)

var (
	baseLogger     *zap.Logger
	baseLoggerOnce sync.Once
)

func Base() *zap.Logger {
	baseLoggerOnce.Do(func() {
		baseLogger = newLogger()
	})
	return baseLogger
}

func Sugar() *zap.SugaredLogger {
	return Base().Sugar()
}

func LoggerWithRequestID(requestID string) *zap.Logger {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return Base()
	}
	return Base().With(zap.String("request_id", requestID))
}

func NewJobLogger(jobName string) (*zap.Logger, string) {
	jobID := uuid.NewString()
	return Base().With(
		zap.String("job_name", strings.TrimSpace(jobName)),
		zap.String("job_id", jobID),
	), jobID
}

func NewJobSugar(jobName string) (*zap.SugaredLogger, string) {
	logger, jobID := NewJobLogger(jobName)
	return logger.Sugar(), jobID
}

func NewRequestID() string {
	return uuid.NewString()
}

func Sync() {
	if baseLogger != nil {
		_ = baseLogger.Sync()
	}
}

func newLogger() *zap.Logger {
	cfg := zap.NewProductionConfig()
	if level, ok := parseLogLevel(os.Getenv("LOG_LEVEL")); ok {
		cfg.Level = zap.NewAtomicLevelAt(level)
	}

	logger, err := cfg.Build()
	if err != nil {
		fallback, fallbackErr := zap.NewProduction()
		if fallbackErr == nil {
			return fallback.With(zap.String("service", "briefcast"))
		}
		return zap.NewNop()
	}
	return logger.With(zap.String("service", "briefcast"))
}

func parseLogLevel(input string) (zapcore.Level, bool) {
	raw := strings.TrimSpace(strings.ToLower(input))
	if raw == "" {
		return zapcore.InfoLevel, false
	}

	var level zapcore.Level
	if err := level.UnmarshalText([]byte(raw)); err != nil {
		return zapcore.InfoLevel, false
	}
	return level, true
}
