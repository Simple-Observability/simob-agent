package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"agent/internal/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display simob agent version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("simob agent v%s\n", version.Version)
	},
}
