package lifecycle

import (
	"log/slog"
	"os"

	"agent/internal/api"
	"agent/internal/config"
	"agent/internal/hostinfo"
	"agent/internal/logger"
	"agent/internal/logs"
	"agent/internal/logs/nginx"
	"agent/internal/metrics"
	"agent/internal/metrics/cpu"
	"agent/internal/metrics/disk"
	"agent/internal/metrics/memory"
	"agent/internal/metrics/network"
)

func RunInit(apiKey string, dryRun bool) {
	// Initialize logger
	debug := os.Getenv("DEBUG") == "1"
	logger.Init(debug)
	logger.Log.Info("Initializing agent...")
	if debug {
		logger.Log.Debug("DEBUG mode is enabled. Expect verbose logging.")
	}
	if dryRun {
		logger.Log.Info("Dry run mode enabled. No data will be sent to the API.")
	}

	// Create and save config
	cfg := config.NewConfig(apiKey)
	err := cfg.Save()
	if err != nil {
		logger.Log.Error("Failed to save configuration. This might be due to insufficient permissions or an invalid configuration path.", slog.Any("error", err))
		os.Exit(1)
	} else {
		logger.Log.Info("API token saved to configuration successfully.")
	}

	// Init API client
	client := api.NewClient(*cfg)
	logger.Log.Debug("API client initialized.")

	// Gather host info
	info, err := hostinfo.Gather()
	logger.Log.Debug("Sending host info...")
	if err != nil {
		logger.Log.Error("Failed to gather info about host. Not critical.", slog.Any("error", err))
	} else {
		if dryRun {
			logger.Log.Info("Skipping sending host info to API due to dry run mode.")
		} else {
			err = client.PostHostInfo(*info)
			if err != nil {
				logger.Log.Error("Failed to send host info to backend. Not critical.", slog.Any("error", err))
			}
		}
	}

	// List all metrics collectors
	var metricsCollectors []metrics.MetricCollector
	logger.Log.Info("Attempting to initialize metrics collectors...")
	metricsCollectors = append(metricsCollectors,
		cpu.NewCPUCollector(),
		memory.NewMemoryCollector(),
		disk.NewDiskCollector(),
		network.NewNetworkCollector(),
	)
	// Discover metrics
	logger.Log.Info("Detecting available metrics ... ")
	discoveredMetrics := metrics.DiscoverAvailableMetrics(metricsCollectors)
	logger.Log.Info("Metrics discovered", slog.Int("count", len(discoveredMetrics)))
	// Send discovered metrics to API
	if dryRun {
		logger.Log.Info("Skipping sending discovered metrics to API due to dry run mode.")
	} else if len(discoveredMetrics) == 0 {
		logger.Log.Info("No metrics found.")
	} else {
		err = client.PostAvailableMetrics(discoveredMetrics)
		if err != nil {
			logger.Log.Error("Failed to send discovered metrics to API", slog.Any("error", err))
			logger.Log.Error("Exiting due to critical error during API communication.")
			os.Exit(1)
		} else {
			logger.Log.Info("Successfully sent discovered metrics to API.")
		}
	}

	// List all logs collectors
	logger.Log.Info("Attempting to initialize log collectors...")
	var logsCollectors []logs.LogCollector
	logsCollectors = append(logsCollectors,
		nginx.NewNginxLogCollector(),
	)
	// Discover log sources
	logger.Log.Info("Detecting available log sources ... ")
	discoveredLogSources := logs.DiscoverAvailableLogSources(logsCollectors)
	logger.Log.Info("Log sources discovered", slog.Int("count", len(discoveredLogSources)))
	// Send discovered log sources to API
	if dryRun {
		logger.Log.Info("Skipping sending discovered log sources to API due to dry run mode.")
	} else if len(discoveredLogSources) == 0 {
		logger.Log.Info("No log source found.")
	} else {
		err = client.PostAvailableLogSources(discoveredLogSources)
		if err != nil {
			logger.Log.Error("Failed to send discovered log sources to API", slog.Any("error", err))
			logger.Log.Error("Exiting due to critical error during API communication.")
			os.Exit(1)
		} else {
			logger.Log.Info("Successfully sent discovered log sources to API.")
		}
	}

	logger.Log.Info("Agent initialization completed.")
}
