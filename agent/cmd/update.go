package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"agent/internal/updater"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update simob agent",
	Run: func(cmd *cobra.Command, args []string) {
		error := updater.Update()
		if error != nil {
			fmt.Printf("Update failed: %v\n", error)
			os.Exit(1)
		}
	},
}
