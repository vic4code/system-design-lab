// Package logger provides a global zap logger configured for structured JSON output.
// In production (LOG_FORMAT=json) logs are newline-delimited JSON — ready for CloudWatch.
// In development (default) logs are human-readable console format.
package logger

import (
	"os"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	once   sync.Once
	global *zap.Logger
)

// Init initialises the global logger. Call once from main().
// LOG_FORMAT=json  → newline-delimited JSON (CloudWatch / production)
// LOG_FORMAT=console (default) → human-readable (local dev)
// LOG_LEVEL=debug|info|warn|error (default: info)
func Init() *zap.Logger {
	once.Do(func() {
		level := zapcore.InfoLevel
		if l := os.Getenv("LOG_LEVEL"); l != "" {
			_ = level.UnmarshalText([]byte(strings.ToLower(l)))
		}

		var cfg zap.Config
		if os.Getenv("LOG_FORMAT") == "json" {
			cfg = zap.NewProductionConfig()
		} else {
			cfg = zap.NewDevelopmentConfig()
			cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		}
		cfg.Level = zap.NewAtomicLevelAt(level)

		// Always include these fields in every log line
		cfg.InitialFields = map[string]interface{}{
			"service": "beatstream-api",
		}

		var err error
		global, err = cfg.Build(zap.AddCallerSkip(0))
		if err != nil {
			panic("failed to initialise logger: " + err.Error())
		}
	})
	return global
}

// L returns the global logger (Init must have been called first).
func L() *zap.Logger {
	if global == nil {
		return zap.NewNop()
	}
	return global
}

// S returns the global sugared logger.
func S() *zap.SugaredLogger {
	return L().Sugar()
}

// Sync flushes any buffered log entries — call in a defer from main().
func Sync() {
	if global != nil {
		_ = global.Sync()
	}
}
