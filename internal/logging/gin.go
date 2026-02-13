package logging

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func RequestLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		requestID := strings.TrimSpace(c.GetHeader(RequestIDHeader))
		if requestID == "" {
			requestID = NewRequestID()
		}

		c.Set(RequestIDKey, requestID)
		c.Writer.Header().Set(RequestIDHeader, requestID)

		requestLogger := LoggerWithRequestID(requestID)
		c.Set(RequestLoggerKey, requestLogger)

		c.Next()

		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Int("status", c.Writer.Status()),
			zap.Int64("latency_ms", time.Since(start).Milliseconds()),
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
		}

		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("errors", c.Errors.String()))
			requestLogger.Error("http_request", fields...)
			return
		}

		requestLogger.Info("http_request", fields...)
	}
}

func RequestIDFromGin(c *gin.Context) string {
	if c == nil {
		return ""
	}

	raw, exists := c.Get(RequestIDKey)
	if exists {
		if requestID, ok := raw.(string); ok {
			return strings.TrimSpace(requestID)
		}
	}

	return strings.TrimSpace(c.GetHeader(RequestIDHeader))
}

func LoggerFromGin(c *gin.Context) *zap.Logger {
	if c == nil {
		return Base()
	}

	raw, exists := c.Get(RequestLoggerKey)
	if exists {
		if logger, ok := raw.(*zap.Logger); ok && logger != nil {
			return logger
		}
	}

	return LoggerWithRequestID(RequestIDFromGin(c))
}
