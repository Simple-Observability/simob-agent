//go:build !linux
// +build !linux

package journalctl

import (
	"context"

	"agent/internal/collection"
	"agent/internal/logs"
)

type JournalCTLCollector struct {
	name string
}

func NewJournalCTLCollector() *JournalCTLCollector {
	return &JournalCTLCollector{name: "journalctl"}
}

func (c *JournalCTLCollector) Name() string {
	return c.name
}

func (c *JournalCTLCollector) Discover() []collection.LogSource {
	return []collection.LogSource{}
}

func (c *JournalCTLCollector) Start(ctx context.Context, out chan<- logs.LogEntry) error {
	return nil
}

func (c *JournalCTLCollector) Stop() error {
	return nil
}
