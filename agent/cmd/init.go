package cmd

import (
	"github.com/spf13/cobra"

	"agent/internal/initializer"
)

var dryRun bool

var initCmd = &cobra.Command{
	Use:   "init [API_KEY]",
	Short: "Initialize agent with optional API key and perform discovery",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		apiKey := ""
		if len(args) > 0 {
			apiKey = args[0]
		}
		initializer.Run(apiKey, dryRun)
	},
}

func init() {
	initCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Don't communicate with the API")
}
