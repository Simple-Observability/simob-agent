package cmd

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"agent/internal/api"
	"agent/internal/collection"
	"agent/internal/common"
	"agent/internal/config"
	"agent/internal/exporter"
	"agent/internal/lifecycle"
	"agent/internal/logger"
	"agent/internal/logs"
	logsRegistry "agent/internal/logs/registry"
	"agent/internal/manager"
	"agent/internal/metrics"
	metricsRegistry "agent/internal/metrics/registry"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start metrics and logs collection agent",
	Run: func(cmd *cobra.Command, args []string) {
		Start()
	},
}

func init() {
	startCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Start a short dry run where collected data is redirected to stdout")
}

func Start() {
	// Initialize logger
	debug := os.Getenv("DEBUG") == "1"
	logger.Init(debug)
	logger.Log.Info("Starting agent...")
	logger.Log.Debug("DEBUG mode is enabled. Expect verbose logging.")

	// Attempt to acquire a file lock to ensure only one instance is running.
	err := common.AcquireLock()
	if err != nil {
		if errors.Is(err, common.ErrAlreadyRunning) {
			// Exit if another instance is detected.
			logger.Log.Info("Another instance of agent is already running. Exiting")
			os.Exit(0)
		}
		logger.Log.Error("failed to acquire process lock", "error", err)
		os.Exit(1)
	}
	defer common.ReleaseLock()

	// Load config
	cfg, err := config.Load()
	if err != nil {
		logger.Log.Error("failed to load config", "error", err)
		common.ReleaseLock()
		os.Exit(1)
	}

	// Create a context to signal when exit
	var ctx context.Context
	var cancel context.CancelFunc
	if dryRun {
		logger.Log.Info("Running in dry-run mode. Output will be logged to stdout.")
		ctx, cancel = context.WithTimeout(context.Background(), 20*time.Second)

	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()

	// Initialize API client
	client := api.NewClient(*cfg)
	logger.Log.Debug("API client initialized.")

	// Run init lifecycle
	lifecycle.RunInit("", dryRun)

	// Init collection config
	var clcCfg *collection.CollectionConfig
	if dryRun {
		clcCfg = nil
	} else {
		clcCfg, err = client.GetCollectionConfig()
		if err != nil {
			logger.Log.Error("exiting due to error when fetching config", "error", err)
			common.ReleaseLock()
			os.Exit(1)
		}
	}

	// Start config watcher
	if !dryRun && clcCfg != nil {
		configRealoder := manager.NewConfigWatcher(client)
		configRealoder.Run(ctx, clcCfg)

	}

	// Used to wait for collectors to exit/stop
	var wg sync.WaitGroup

	// Initialize exporter
	exporter, err := exporter.NewExporter(dryRun)
	if err != nil {
		logger.Log.Error("cannot initialize exporter", "error", err)
		cancel()
		common.ReleaseLock()
		os.Exit(1)
	}

	// Initialize log collectors
	logsCollectors := logsRegistry.BuildCollectors(clcCfg)
	logger.Log.Info("Starting log collectors", "count", len(logsCollectors))
	wg.Add(1)
	go logs.StartCollection(logsCollectors, ctx, &wg, exporter)

	// Initialize metrics collectors
	metricsCollectors := metricsRegistry.BuildCollectors(clcCfg)
	// Set metrics collection interval
	collectionInterval := 60 * time.Second
	if dryRun {
		collectionInterval = 3 * time.Second
	}
	logger.Log.Info("Starting metric collectors", "count", len(metricsCollectors))
	wg.Add(1)
	go metrics.StartCollection(metricsCollectors, collectionInterval, ctx, &wg, exporter)

	if dryRun {
		wg.Wait()
		logger.Log.Info("Dry run completed. Exiting.")
		return
	}

	// Wait for custom restart signal
	restartCh := common.RestartSignal(ctx.Done())

	// Wait for OS term/exit signals
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-signalChan:
		logger.Log.Info("Termination signal received.", "signal", sig)
		cancel()
		wg.Wait()
		logger.Log.Info("Collectors stopped. Exiting.")
	case <-restartCh:
		logger.Log.Info("Restart requested. Shutting down gracefully for updater.")
		cancel()
		wg.Wait()
		common.ReleaseLock()
		logger.Log.Info("Agent stopped for restart. Automatic restart will only happen if running under systemd.")
		os.Exit(1)
	}
}
