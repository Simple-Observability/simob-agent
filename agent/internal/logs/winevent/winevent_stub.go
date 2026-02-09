//go:build !windows
// +build !windows

package winevent

import (
	"context"

	"agent/internal/collection"
	"agent/internal/logs"
)

type WinEventCollector struct {
	name string
}

func NewWinEventCollector() *WinEventCollector {
	return &WinEventCollector{name: "winevent"}
}

func (c *WinEventCollector) Name() string {
	return c.name
}

func (c *WinEventCollector) Discover() []collection.LogSource {
	return []collection.LogSource{}
}

func (c *WinEventCollector) Start(ctx context.Context, out chan<- logs.LogEntry) error {
	return nil
}

func (c *WinEventCollector) Stop() error {
	return nil
}
