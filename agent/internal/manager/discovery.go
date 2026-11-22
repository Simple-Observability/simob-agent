package manager

import (
	"context"
	"sync"
	"time"

	"agent/internal/api"
	"agent/internal/hardware"
	"agent/internal/hostinfo"
	"agent/internal/logger"
	"agent/internal/logs"
	logsRegistry "agent/internal/logs/registry"
	"agent/internal/metrics"
	metricsRegistry "agent/internal/metrics/registry"
	"agent/internal/packages"
)

const discoveryInterval = time.Hour

type Discovery struct {
	client                  *api.Client
	wg                      *sync.WaitGroup
	acknowledgedPackageHash string
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

	d.publish(ctx)

	ticker := time.NewTicker(discoveryInterval)
	defer ticker.Stop()

	logger.Log.Info("Running discovery.", "interval", discoveryInterval)

	for {
		select {
		case <-ctx.Done():
			logger.Log.Info("Discovery received shutdown signal.")
			return
		case <-ticker.C:
			d.publish(ctx)
		}
	}
}

func (d *Discovery) publish(ctx context.Context) {
	info, err := hostinfo.Gather()
	if err != nil {
		logger.Log.Error("failed to gather host info", "error", err)
	} else if err := d.client.PostHostInfo(*info); err != nil {
		logger.Log.Error("failed to send host info to backend", "error", err)
	}

	disks, err := hardware.GetPhysicalDisks()
	if err != nil {
		logger.Log.Error("failed to gather disk info", "error", err)
	} else if err := d.client.PostPhysicalDisks(disks); err != nil {
		logger.Log.Error("failed to send disk info to backend", "error", err)
	}

	ifaces, err := hardware.GetNetworkInterfaces()
	if err != nil {
		logger.Log.Error("failed to gather network info", "error", err)
	} else if err := d.client.PostNetworkInterfaces(ifaces); err != nil {
		logger.Log.Error("failed to send network info to backend", "error", err)
	}

	inventory, collectErr := packages.Collect(ctx)
	report, err := packages.NewReport(inventory, collectErr, d.acknowledgedPackageHash)
	if err != nil {
		logger.Log.Error("failed to prepare package inventory report", "error", err)
	} else if acknowledged, err := d.client.PostPackageInventory(ctx, report); err != nil {
		if report.Observation.Status == packages.Unchanged {
			d.acknowledgedPackageHash = ""
		}
		logger.Log.Error("failed to send package inventory to backend", "error", err)
	} else if acknowledged && report.Observation.Status == packages.Snapshot {
		d.acknowledgedPackageHash = report.Observation.InventoryHash
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
