//go:build windows

package logger

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"golang.org/x/sys/windows/svc/eventlog"
)

type EventLogHandler struct {
	source string
	el     *eventlog.Log
	opts   slog.HandlerOptions
}

func NewEventLogHandler(source string, opts *slog.HandlerOptions) (*EventLogHandler, error) {
	el, err := eventlog.Open(source)
	if err != nil {
		return nil, err
	}

	h := &EventLogHandler{
		source: source,
		el:     el,
	}
	if opts != nil {
		h.opts = *opts
	}
	return h, nil
}

func (h *EventLogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.opts.Level.Level()
}

func (h *EventLogHandler) Handle(_ context.Context, r slog.Record) error {
	var sb strings.Builder
	sb.WriteString(r.Message)

	r.Attrs(func(a slog.Attr) bool {
		sb.WriteString(fmt.Sprintf("\n%s=%v", a.Key, a.Value.Any()))
		return true
	})

	msg := sb.String()

	switch {
	case r.Level >= slog.LevelError:
		return h.el.Error(1, msg)
	case r.Level >= slog.LevelWarn:
		return h.el.Warning(1, msg)
	default:
		return h.el.Info(1, msg)
	}
}

func (h *EventLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *EventLogHandler) WithGroup(name string) slog.Handler {
	return h
}

func (h *EventLogHandler) Close() error {
	return h.el.Close()
}
