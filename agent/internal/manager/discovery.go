package manager

import (
	"context"
	"sync"
	"time"

	"agent/internal/api"
	"agent/internal/hostinfo"
	"agent/internal/logger"
	"agent/internal/logs"
	logsRegistry "agent/internal/logs/registry"
	"agent/internal/metrics"
	metricsRegistry "agent/internal/metrics/registry"
)

const discoveryInterval = time.Hour

type Discovery struct {
	client *api.Client
	wg     *sync.WaitGroup
}

func NewDiscovery(client *api.Client, wg *sync.WaitGroup) *Discovery {
	return &Discovery{
		client: client,
		wg:     wg,
	}
}

func (d *Discovery) Start(ctx context.Context) {
	go d.run(ctx)
}

func (d *Discovery) run(ctx context.Context) {
	defer d.wg.Done()

	d.publish()

	ticker := time.NewTicker(discoveryInterval)
	defer ticker.Stop()

	logger.Log.Info("Running discovery.", "interval", discoveryInterval)

	for {
		select {
		case <-ctx.Done():
			logger.Log.Info("Discovery received shutdown signal.")
			return
		case <-ticker.C:
			d.publish()
		}
	}
}

func (d *Discovery) publish() {
	info, err := hostinfo.Gather()
	if err != nil {
		logger.Log.Error("failed to gather host info", "error", err)
	} else if err := d.client.PostHostInfo(*info); err != nil {
		logger.Log.Error("failed to send host info to backend", "error", err)
	}

	metricsCollectors := metricsRegistry.BuildCollectors(nil)
	discoveredMetrics := metrics.DiscoverAvailableMetrics(metricsCollectors)
	logger.Log.Info("Metrics discovered", "count", len(discoveredMetrics))
	if err := d.client.PostAvailableMetrics(discoveredMetrics); err != nil {
		logger.Log.Error("failed to send discovered metrics to backend", "error", err)
	}

	logsCollectors := logsRegistry.BuildCollectors(nil)
	discoveredLogSources := logs.DiscoverAvailableLogSources(logsCollectors)
	logger.Log.Info("Log sources discovered", "count", len(discoveredLogSources))
	if err := d.client.PostAvailableLogSources(discoveredLogSources); err != nil {
		logger.Log.Error("failed to send discovered log sources to backend", "error", err)
	}
}
