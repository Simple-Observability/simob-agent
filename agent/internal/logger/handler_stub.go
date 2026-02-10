//go:build !windows

package logger

import "log/slog"

func getServiceHandler(opts *slog.HandlerOptions) slog.Handler {
	return nil
}
