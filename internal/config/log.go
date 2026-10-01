package config

import (
	"log/slog"
	"os"
)

// NewLogger returns an application wide logger.
func NewLogger() *slog.Logger {
	opts := &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}
	jsonLogger := slog.New(slog.NewJSONHandler(os.Stdout, opts))
	logger := jsonLogger.With("service", "checkregress")
	return logger
}
