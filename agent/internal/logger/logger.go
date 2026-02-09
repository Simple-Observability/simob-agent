package logger

import (
	"log/slog"
	"os"
)

var Log *slog.Logger

func Init(debug bool) {
	// Set level
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{Level: level}
	// getServiceHandler will return a platform-specific handler if running as a Windows service
	handler := getServiceHandler(opts)
	if handler == nil {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	Log = slog.New(handler)
	slog.SetDefault(Log)
}
