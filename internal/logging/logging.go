package logging

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	RequestIDKey     = "request_id"
	RequestIDHeader  = "X-Request-ID"
	RequestLoggerKey = "request_logger"
	LogRunTimestamp  = "LOG_RUN_TIMESTAMP"
)

var (
	baseLogger          *zap.Logger
	baseLoggerOnce      sync.Once
	logRunTimestamp     string
	logRunTimestampOnce sync.Once
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
	level := zapcore.InfoLevel
	if parsed, ok := parseLogLevel(os.Getenv("LOG_LEVEL")); ok {
		level = parsed
	}

	logFormat := parseLogFormat(os.Getenv("LOG_FORMAT"))
	encoder := buildEncoder(logFormat)

	outputs := resolveLogOutputs()
	if len(outputs) == 0 {
		outputs = []zapcore.WriteSyncer{zapcore.AddSync(os.Stdout)}
	}

	core := zapcore.NewCore(
		encoder,
		zapcore.NewMultiWriteSyncer(outputs...),
		level,
	)

	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	return logger.With(zap.String("service", "briefcast"))
}

func buildEncoder(logFormat string) zapcore.Encoder {
	if logFormat == "text" {
		encoderCfg := zap.NewDevelopmentEncoderConfig()
		encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		return zapcore.NewConsoleEncoder(encoderCfg)
	}

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	return zapcore.NewJSONEncoder(encoderCfg)
}

func resolveLogOutputs() []zapcore.WriteSyncer {
	raw := strings.TrimSpace(os.Getenv("LOG_OUTPUT"))
	if raw == "" {
		return []zapcore.WriteSyncer{zapcore.AddSync(os.Stdout)}
	}

	parts := strings.Split(raw, ",")
	outputs := make([]zapcore.WriteSyncer, 0, len(parts))
	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			continue
		}
		switch strings.ToLower(token) {
		case "stdout":
			outputs = append(outputs, zapcore.AddSync(os.Stdout))
		case "stderr":
			outputs = append(outputs, zapcore.AddSync(os.Stderr))
		default:
			path := strings.TrimPrefix(token, "file:")
			if path == "" {
				continue
			}
			path = resolveLogFilePath(path)
			if dir := filepath.Dir(path); dir != "." && dir != "" {
				_ = os.MkdirAll(dir, 0o755)
			}
			outputs = append(outputs, zapcore.AddSync(&lumberjack.Logger{
				Filename:   path,
				MaxSize:    getEnvInt("LOG_FILE_MAX_SIZE_MB", 50),
				MaxBackups: getEnvInt("LOG_FILE_MAX_BACKUPS", 7),
				MaxAge:     getEnvInt("LOG_FILE_MAX_AGE_DAYS", 14),
				Compress:   getEnvBool("LOG_FILE_COMPRESS", true),
			}))
		}
	}

	return outputs
}

func resolveLogFilePath(path string) string {
	resolved := strings.TrimSpace(path)
	if resolved == "" {
		return ""
	}

	timestamp := logRunTimestampValue()
	replacer := strings.NewReplacer(
		"{startup_ts}", timestamp,
		"{timestamp}", timestamp,
		"{run_ts}", timestamp,
	)
	return replacer.Replace(resolved)
}

func logRunTimestampValue() string {
	logRunTimestampOnce.Do(func() {
		value := strings.TrimSpace(os.Getenv(LogRunTimestamp))
		if value == "" {
			value = time.Now().UTC().Format("20060102-150405")
		}
		value = sanitizeLogRunTimestamp(value)
		if value == "" {
			value = time.Now().UTC().Format("20060102-150405")
		}

		logRunTimestamp = value
		_ = os.Setenv(LogRunTimestamp, value)
	})
	return logRunTimestamp
}

func sanitizeLogRunTimestamp(raw string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(raw) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func getEnvInt(name string, fallback int) int {
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

func getEnvBool(name string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
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

func parseLogFormat(input string) string {
	switch strings.TrimSpace(strings.ToLower(input)) {
	case "text", "console":
		return "text"
	default:
		return "json"
	}
}
