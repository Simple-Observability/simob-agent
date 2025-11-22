package cmd

import (
	"errors"
	"os"

	"github.com/spf13/cobra"

	"agent/internal/common"
	"agent/internal/config"
	"agent/internal/initializer"
	"agent/internal/logger"
	"agent/internal/manager"
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
	if err := common.AcquireLock(); err != nil {
		if errors.Is(err, common.ErrAlreadyRunning) {
			// Exit if another instance is detected.
			logger.Log.Info("Another instance of agent is already running. Exiting")
			os.Exit(0)
		}
		logger.Log.Error("failed to acquire process lock", "error", err)
		os.Exit(1)
	}

	// Run init lifecycle
	initializer.Run("", dryRun)

	// Load config
	cfg, err := config.Load()
	if err != nil {
		logger.Log.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Create and run the agent
	agent := manager.NewAgent(cfg)
	agent.Run(dryRun)
}
