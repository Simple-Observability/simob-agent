package manager

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"agent/internal/api"
	"agent/internal/collection"
	"agent/internal/common"
	"agent/internal/config"
	"agent/internal/exporter"
	"agent/internal/logger"
	"agent/internal/logs"
	logsRegistry "agent/internal/logs/registry"
	"agent/internal/metrics"
	metricsRegistry "agent/internal/metrics/registry"
)

type Agent struct {
	config   *config.Config
	client   *api.Client
	reloadCh chan bool
	wg       *sync.WaitGroup
}

func NewAgent(cfg *config.Config) *Agent {
	return &Agent{
		config:   cfg,
		client:   api.NewClient(*cfg),
		reloadCh: make(chan bool, 1),
		wg:       &sync.WaitGroup{},
	}
}

func (a *Agent) Run(dryRun bool) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	for {
		// Create a context to signal when exit
		var ctx context.Context
		var cancel context.CancelFunc
		if dryRun {
			logger.Log.Info("Running in dry-run mode. Output will be logged to stdout.")
			ctx, cancel = context.WithTimeout(context.Background(), 20*time.Second)
		} else {
			ctx, cancel = context.WithCancel(context.Background())
		}

		a.startServices(ctx, dryRun)

		// Wait for custom restart signal
		restartCh := common.RestartSignal(ctx.Done())

		select {
		case sig := <-signalChan:
			logger.Log.Info("Termination signal received.", "signal", sig)
			cancel()
			a.wg.Wait()
			common.ReleaseLock()
			logger.Log.Info("Collectors stopped. Exiting.")
			return
		case <-ctx.Done():
			if dryRun {
				cancel()
				a.wg.Wait()
				common.ReleaseLock()
				logger.Log.Info("Dry run finished. Exiting agent.")
				return
			}
		case <-restartCh:
			logger.Log.Info("Restart requested. Shutting down gracefully for updater.")
			cancel()
			a.wg.Wait()
			common.ReleaseLock()
			logger.Log.Info("Agent stopped for restart. Automatic restart will only happen if running under systemd.")
			os.Exit(1)
		case <-a.reloadCh:
			logger.Log.Info("Configuration change detected. Reloading collectors.")
			cancel()
			a.wg.Wait()
			logger.Log.Info("Collectors stopped. Restarting with new configuration.")
		}
	}
}

func (a *Agent) startServices(ctx context.Context, dryRun bool) {
	var clcCfg *collection.CollectionConfig
	if !dryRun {
		var err error
		clcCfg, err = a.client.GetCollectionConfig()
		if err != nil {
			logger.Log.Error("exiting due to error when fetching config", "error", err)
			os.Exit(1)
		}
	}

	if !dryRun && clcCfg != nil {
		configReloader := NewConfigWatcher(a.client, a.reloadCh)
		configReloader.Start(ctx, clcCfg)
	}

	exporter, err := exporter.NewExporter(dryRun)
	if err != nil {
		logger.Log.Error("cannot initialize exporter", "error", err)
		os.Exit(1)
	}

	logsCollectors := logsRegistry.BuildCollectors(clcCfg)
	logger.Log.Info("Starting log collectors", "count", len(logsCollectors))
	a.wg.Add(1)
	go logs.StartCollection(logsCollectors, ctx, a.wg, exporter)

	metricsCollectors := metricsRegistry.BuildCollectors(clcCfg)
	collectionInterval := 60 * time.Second
	if dryRun {
		collectionInterval = 3 * time.Second
	}
	logger.Log.Info("Starting metric collectors", "count", len(metricsCollectors))
	a.wg.Add(1)
	go metrics.StartCollection(metricsCollectors, collectionInterval, ctx, a.wg, exporter)
}
