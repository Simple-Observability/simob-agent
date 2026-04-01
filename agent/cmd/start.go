package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"agent/internal/common"
	"agent/internal/config"
	"agent/internal/logger"
	"agent/internal/manager"
)

var dryRun bool

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
	// Check if running as Windows service
	if isWindowsService() {
		runAsWindowsService()
		return
	}

	// Create and run the agent
	agent, err := initializeAndLoadAgent()
	if err != nil {
		os.Exit(1)
	}
	agent.Run(dryRun)
}

func initializeAndLoadAgent() (*manager.Agent, error) {
	// Initialize logger
	debug := os.Getenv("DEBUG") == "1"
	logger.Init(debug)
	logger.Log.Info("Starting agent...")
	logger.Log.Debug("DEBUG mode is enabled. Expect verbose logging.")

	// Attempt to acquire a file lock to ensure only one instance is running.
	if err := common.AcquireLock(); err != nil {
		if errors.Is(err, common.ErrAlreadyRunning) {
			logger.Log.Info("Another instance of agent is already running")
			return nil, err
		}
		logger.Log.Error("failed to acquire process lock", "error", err)
		return nil, err
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		logger.Log.Error("failed to load config", "error", err)
		return nil, err
	}
	if cfg.APIKey == "" {
		err = fmt.Errorf("missing API key in config")
		logger.Log.Error("failed to start agent", "error", err)
		return nil, err
	}

	// Create the agent
	agent := manager.NewAgent(cfg)
	return agent, nil
}
