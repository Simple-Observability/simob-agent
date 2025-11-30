package initializer

import (
	"log/slog"
	"os"

	"agent/internal/api"
	"agent/internal/config"
	"agent/internal/hostinfo"
	"agent/internal/logger"
	"agent/internal/logs"
	logsRegistry "agent/internal/logs/registry"
	"agent/internal/metrics"
	metricsRegistry "agent/internal/metrics/registry"
)

func Run(apiKey string, dryRun bool) {
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
		err = client.PostHostInfo(*info)
		if err != nil {
			logger.Log.Error("Failed to send host info to backend. Not critical.", slog.Any("error", err))
		}
	}

	// Discover metrics
	logger.Log.Info("Detecting available metrics ... ")
	metricsCollectors := metricsRegistry.BuildCollectors(nil)
	discoveredMetrics := metrics.DiscoverAvailableMetrics(metricsCollectors)
	logger.Log.Info("Metrics discovered", slog.Int("count", len(discoveredMetrics)))

	// Send discovered metrics to API
	if len(discoveredMetrics) == 0 {
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

	// Discover log sources
	logger.Log.Info("Detecting available log sources ... ")
	logsCollectors := logsRegistry.BuildCollectors(nil)
	discoveredLogSources := logs.DiscoverAvailableLogSources(logsCollectors)
	logger.Log.Info("Log sources discovered", slog.Int("count", len(discoveredLogSources)))

	// Send discovered log sources to API
	if len(discoveredLogSources) == 0 {
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
