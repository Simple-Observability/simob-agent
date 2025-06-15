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

	// Set handler
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	Log = slog.New(handler)
	slog.SetDefault(Log)
}
