package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	applogger "github.com/vic4code/system-design-lab/beatstream/internal/logger"
)

// ZapLogger replaces gin.Logger() with structured zap JSON output.
//
// Each request log line includes:
//   - timestamp, level
//   - method, path, query
//   - status, latency_ms
//   - client_ip, user_agent
//   - request_id (from X-Request-ID / RequestID middleware)
//   - user_id (from JWT middleware, if authenticated)
//
// In production the JSON lines flow to CloudWatch Logs.
// CloudWatch Metric Filters can turn log fields into metrics
// (e.g. p99 latency, 5xx rate) without a separate APM agent.
func ZapLogger() gin.HandlerFunc {
	log := applogger.L()

	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		// Collect errors from gin context
		errMsg := c.Errors.ByType(gin.ErrorTypePrivate).String()

		level := zapcore.InfoLevel
		switch {
		case status >= 500:
			level = zapcore.ErrorLevel
		case status >= 400:
			level = zapcore.WarnLevel
		}

		fields := []zap.Field{
			zap.Int("status", status),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Duration("latency", latency),
			zap.Int64("latency_ms", latency.Milliseconds()),
			zap.String("ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
		}

		if query != "" {
			fields = append(fields, zap.String("query", query))
		}
		if rid := c.GetString("request_id"); rid != "" {
			fields = append(fields, zap.String("request_id", rid))
		}
		if uid, exists := c.Get("user_id"); exists && uid != "" {
			fields = append(fields, zap.String("user_id", uid.(string)))
		}
		if errMsg != "" {
			fields = append(fields, zap.String("error", errMsg))
		}

		// Correlate log line with the OTel trace span.
		// This is the "three pillars" correlation:
		//   trace_id in logs → find the full trace in Jaeger / X-Ray
		//   request_id in logs → find all log lines for this request
		if span := trace.SpanFromContext(c.Request.Context()); span.SpanContext().IsValid() {
			sc := span.SpanContext()
			fields = append(fields,
				zap.String("trace_id", sc.TraceID().String()),
				zap.String("span_id", sc.SpanID().String()),
			)
		}

		log.Log(level, "request", fields...)
	}
}
