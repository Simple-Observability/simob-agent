package logger

import (
	"log/slog"

	"golang.org/x/sys/windows/svc"
)

func getServiceHandler(opts *slog.HandlerOptions) slog.Handler {
	isService, err := svc.IsWindowsService()
	if err != nil || !isService {
		return nil
	}

	h, err := NewEventLogHandler("simob", opts)
	if err != nil {
		return nil
	}
	return h
}
