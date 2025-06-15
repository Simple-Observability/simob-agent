package cmd

import (
	"agent/internal/config"
	"agent/internal/logger"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config [key=value]",
	Short: "Manage configuration settings",
	Long: `Manage configuration settings for simob agent.

	Examples:
		simob config                    # Show current config
		simob config api_key=your-key   # Set API key
	`,
	Run: func(cmd *cobra.Command, args []string) {
		runConfig(args)
	},
}

func runConfig(args []string) {
	// Initialize logger
	debug := os.Getenv("DEBUG") == "1"
	logger.Init(debug)
	if debug {
		logger.Log.Debug("DEBUG mode is enabled. Expect verbose logging.")
	}

	if len(args) == 0 {
		// Show current config
		showConfig()
		return
	}

	// Parse key=value pairs
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			fmt.Printf("Invalid format: %s. Use key=value\n", arg)
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if err := setConfigValue(key, value); err != nil {
			fmt.Printf("Error setting %s: %v\n", key, err)
		} else {
			fmt.Printf("Set %s = %s\n", key, value)
		}
	}
}
func showConfig() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("No existing config found, showing defaults:\n")
		cfg = config.NewConfig("")
	}

	fmt.Printf("Current configuration:\n")
	fmt.Printf("  api_key = %s\n", cfg.APIKey)
	fmt.Printf("  api_url = %s\n", cfg.APIUrl)
}

func setConfigValue(key, value string) error {
	// Load existing config or create new one
	cfg, err := config.Load()
	if err != nil {
		cfg = config.NewConfig("")
	}

	// Set the value based on key
	switch strings.ToLower(key) {
	case "api_key":
		cfg.SetAPIKey(value)
	case "api_url":
		cfg.SetAPIUrl(value)
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}

	// Save the updated config
	return cfg.Save()
}
