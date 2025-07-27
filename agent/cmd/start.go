package cmd

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"agent/internal/api"
	"agent/internal/collection"
	"agent/internal/config"
	"agent/internal/exporter"
	"agent/internal/lifecycle"
	"agent/internal/logger"
	"agent/internal/logs"
	logsRegistry "agent/internal/logs/registry"
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

	// Load config
	cfg, err := config.Load()
	if err != nil {
		logger.Log.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize API client
	client := api.NewClient(*cfg)
	logger.Log.Debug("API client initialized.")

	// Init collection config
	var clcCfg *collection.CollectionConfig
	if dryRun {
		clcCfg = nil
	} else {
		var err error
		clcCfg, err = client.GetCollectionConfig()
		if err != nil {
			logger.Log.Error("failed to fetch collection config", "error", err)
			os.Exit(1)
		}
	}

	// Run init lifecycle
	lifecycle.RunInit("", dryRun)

	// Create a context to signal when to stop the collectors
	var ctx context.Context
	var cancel context.CancelFunc
	if dryRun {
		logger.Log.Info("Running in dry-run mode. Output will be logged to stdout.")
		ctx, cancel = context.WithTimeout(context.Background(), 20*time.Second)

	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()

	// Used to wait for collectors to exit/stop
	var wg sync.WaitGroup

	// Initialize exporter
	exporter, err := exporter.NewExporter(dryRun)
	if err != nil {
		logger.Log.Error("cannot initialize exporter", "error", err)
		cancel()
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

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan
	logger.Log.Info("Termination signal received.")
	cancel()
	wg.Wait()
	logger.Log.Info("Agent and collectors stopped. Exiting.")
}
