package logger

import (
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func New(level, format string, development bool) (*zap.Logger, error) {
	var cfg zap.Config
	if development || format == "console" {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		cfg = zap.NewProductionConfig()
	}

	cfg.Level = zap.NewAtomicLevelAt(parseLevel(level))
	cfg.Encoding = normalizeFormat(format, development)

	return cfg.Build(zap.AddCallerSkip(0))
}

func parseLevel(level string) zapcore.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zapcore.DebugLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

func normalizeFormat(format string, development bool) string {
	switch strings.ToLower(format) {
	case "json":
		return "json"
	case "console":
		return "console"
	default:
		if development {
			return "console"
		}
		return "json"
	}
}
