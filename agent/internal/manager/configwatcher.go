package manager

import (
	"context"
	"os"
	"time"

	"agent/internal/api"
	"agent/internal/collection"
	"agent/internal/common"
	"agent/internal/logger"
)

// ConfigWatcher manages the background process of checking for config changes.
type ConfigWatcher struct {
	client      *api.Client
	initialHash string
}

// NewConfigWatcher creates a new instance of the ConfigWatcher.
func NewConfigWatcher(client *api.Client) *ConfigWatcher {
	return &ConfigWatcher{
		client: client,
	}
}

// Start launches the background goroutine to watch for config changes.
func (r *ConfigWatcher) Start(ctx context.Context, initialCfg *collection.CollectionConfig) {
	hash, err := initialCfg.Hash()
	if err != nil {
		// Critical error. Hashing should not fail on valid config
		logger.Log.Error("Failed to hash initial config, exiting", "error", err)
		os.Exit(1)
	}
	r.initialHash = hash

	go r.run(ctx, initialCfg)
}

// Run is the main loop for checking config changes with dynamic intervals.
func (r *ConfigWatcher) run(ctx context.Context, initialCfg *collection.CollectionConfig) {
	currentTickDuration := determineTickDuration(initialCfg)

	// Create the initial ticker
	ticker := time.NewTicker(currentTickDuration)
	defer ticker.Stop()

	logger.Log.Info("Running config reloader.", "interval", currentTickDuration)

	for {
		select {
		case <-ctx.Done():
			logger.Log.Info("Config reloader received shutdown signal. Exiting.")
			return

		case <-ticker.C:
			newCfg, shouldRestart := r.checkConfigForChange()
			// checkConfigForChange will called os.Exit(1) if hash changed.
			if shouldRestart {
				return
			}

			// Determine the next desired tick duration.
			if newCfg != nil {
				nextTickDuration := determineTickDuration(newCfg)
				// Check if the duration needs to change
				if nextTickDuration != currentTickDuration {
					logger.Log.Debug("Changing tick interval in config watcher.",
						"old", currentTickDuration,
						"new", nextTickDuration,
					)
					// Re-initialize the ticker with the new duration
					ticker.Stop()
					ticker = time.NewTicker(nextTickDuration)
					currentTickDuration = nextTickDuration
				}
			}
		}
	}
}

// determineTickDuration checks the config and returns the appropriate tick duration.
// Given an empty config it'll return a fast tick duration.
func determineTickDuration(cfg *collection.CollectionConfig) time.Duration {
	const (
		fast = 5 * time.Second
		slow = 5 * time.Minute
	)
	// Check for empty lists in the config
	if len(cfg.LogSources) == 0 && len(cfg.Metrics) == 0 {
		return fast
	}
	return slow
}

// checkConfigForChange fetches the new config, compares the hash, and triggers os.Exit(1) on change.
// Returns the fetched config and a boolean indicating if an exit was triggered.
func (r *ConfigWatcher) checkConfigForChange() (*collection.CollectionConfig, bool) {
	newCfg, err := r.client.GetCollectionConfig()
	if err != nil {
		logger.Log.Warn("Failed to fetch config for change detection", "error", err)
		return nil, false
	}

	// Hash check
	newHash, err := newCfg.Hash()
	if err != nil {
		logger.Log.Warn("Failed to hash new config. Skipping this check cycle", "error", err)
		return nil, false
	}

	if newHash != r.initialHash {
		logger.Log.Info("Configuration has changed. Exiting for auto-restart.")
		common.ReleaseLock()
		os.Exit(1)
		return newCfg, true
	}
	return newCfg, false
}
