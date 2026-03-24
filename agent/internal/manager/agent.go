package manager

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"agent/internal/api"
	"agent/internal/authguard"
	"agent/internal/common"
	"agent/internal/config"
	"agent/internal/exporter"
	"agent/internal/logger"
	"agent/internal/logs"
	logsRegistry "agent/internal/logs/registry"
	"agent/internal/metrics"
	metricsRegistry "agent/internal/metrics/registry"
)

type ControlEvent int

const (
	Shutdown ControlEvent = iota
	Reload
	Restart
	Hibernate
)

type Agent struct {
	config     *config.Config
	client     *api.Client
	exporter   *exporter.Exporter
	reloadCh   chan bool
	restartCh  chan bool
	shutdownCh chan bool
	wg         *sync.WaitGroup
}

func NewAgent(cfg *config.Config) *Agent {
	return &Agent{
		config:     cfg,
		reloadCh:   make(chan bool, 1),
		restartCh:  make(chan bool, 1),
		shutdownCh: make(chan bool, 1),
		wg:         &sync.WaitGroup{},
	}
}

func (a *Agent) Run(dryRun bool) {
	ctrl := make(chan ControlEvent, 1)

	// OS signals -> Shutdown event
	go func() {
		s := make(chan os.Signal, 1)
		signal.Notify(s, syscall.SIGINT, syscall.SIGTERM)
		select {
		case <-s:
			ctrl <- Shutdown
		case <-a.shutdownCh:
			ctrl <- Shutdown
		}
	}()

	// Collection config change -> Reload event
	go func() {
		for {
			select {
			case <-a.shutdownCh:
				return
			case <-a.reloadCh:
				ctrl <- Reload
			}
		}
	}()

	// Restart signal -> Restart event
	go func() {
		for {
			select {
			case <-a.shutdownCh:
				return
			case <-a.restartCh:
				ctrl <- Restart
			}
		}
	}()

	// Key check -> Hibernate event
	keyCheckCh := make(chan bool, 1)
	authguard.Get().Subscribe(keyCheckCh)
	go func() {
		for {
			select {
			case <-a.shutdownCh:
				return
			case <-keyCheckCh:
				valid, _ := a.client.CheckAPIKeyValidity()
				if !valid {
					ctrl <- Hibernate
				}
			}
		}
	}()

	// Initialize client
	a.client = api.NewClient(*a.config, dryRun)

	// Initial key validation
	valid, err := a.client.CheckAPIKeyValidity()
	if !valid || err != nil {
		logger.Log.Error("failed to check API key validity", "error", err)
		os.Exit(1)
	}

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

		select {
		case evt := <-ctrl:
			switch evt {
			case Shutdown:
				a.stopServices(cancel)
				common.ReleaseLock()
				logger.Log.Info("Collectors stopped. Exiting.")
				return
			case Restart:
				a.stopServices(cancel)
				common.ReleaseLock()
				logger.Log.Info("Agent stopped for restart. Automatic restart will only happen if running under systemd.")
				os.Exit(1)
			case Reload:
				a.stopServices(cancel)
				logger.Log.Info("Reloading collectors")
				continue
			case Hibernate:
				a.stopServices(cancel)
				if a.hibernate(ctrl) {
					return
				}
				continue
			}
		case <-ctx.Done():
			if dryRun {
				a.stopServices(cancel)
				common.ReleaseLock()
				logger.Log.Info("Dry run finished. Exiting agent.")
				return
			}
		}
	}
}

func (a *Agent) Stop() {
	close(a.shutdownCh)
}

func (a *Agent) startServices(ctx context.Context, dryRun bool) {
	// Start config watcher
	clcCfg, err := a.client.GetCollectionConfig()
	if err != nil {
		logger.Log.Error("exiting due to error when fetching config", "error", err)
		os.Exit(1)
	}
	if !dryRun && clcCfg != nil {
		a.wg.Add(1)
		configWatcher := NewConfigWatcher(a.client, a.reloadCh, a.wg)
		configWatcher.Start(ctx, clcCfg)
	}

	// Start restart watcher
	a.wg.Add(1)
	restartWatcher := NewRestartWatcher(a.restartCh, a.wg)
	restartWatcher.Start(ctx)

	// Start discovery loop
	a.wg.Add(1)
	discovery := NewDiscovery(a.client, a.wg)
	discovery.Start(ctx)

	a.exporter, err = exporter.NewExporter(a.config, dryRun)
	if err != nil {
		logger.Log.Error("cannot initialize exporter", "error", err)
		os.Exit(1)
	}

	logsCollectors := logsRegistry.BuildCollectors(clcCfg)
	logger.Log.Info("Starting log collectors", "count", len(logsCollectors))
	a.wg.Add(1)
	go logs.StartCollection(logsCollectors, ctx, a.wg, a.exporter)

	metricsCollectors := metricsRegistry.BuildCollectors(clcCfg)
	collectionInterval := 60 * time.Second
	if dryRun {
		collectionInterval = 3 * time.Second
	}
	logger.Log.Info("Starting metric collectors", "count", len(metricsCollectors))
	a.wg.Add(1)
	go metrics.StartCollection(metricsCollectors, collectionInterval, ctx, a.wg, a.exporter)
}

func (a *Agent) hibernate(ctrl <-chan ControlEvent) (exit bool) {
	logger.Log.Warn("Hibernating for 1h")
	timer := time.NewTimer(1 * time.Hour)

	for {
		select {
		case <-timer.C:
			logger.Log.Info("Hibernation finished.")
			return false
		case evt := <-ctrl:
			timer.Stop()
			switch evt {
			case Shutdown:
				logger.Log.Info("Shutdown received during hibernation.")
				return true
			case Restart:
				logger.Log.Info("Restart received during hibernation.")
				os.Exit(1)
			case Reload:
				logger.Log.Info("Reload received during hibernation.")
				return false
			}
		}
	}
}

func (a *Agent) stopServices(cancel context.CancelFunc) {
	cancel()
	a.wg.Wait()
	if a.exporter != nil {
		if err := a.exporter.Close(); err != nil {
			logger.Log.Error("failed to close exporter", "error", err)
		}
		a.exporter = nil
	}
}
