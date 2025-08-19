package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"agent/internal/common"
)

// ANSI escape codes for colors
const (
	ColorReset = "\033[0m"
	ColorRed   = "\033[31m"
	ColorGreen = "\033[32m"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check if the simob agent is already running",
	Run: func(cmd *cobra.Command, args []string) {
		isLocked, err := common.IsLockAcquired()
		if err != nil {
			fmt.Printf("Error checking agent status: %v\n", err)
			return
		}

		if isLocked {
			// Prettier print for 'running' status
			fmt.Printf("%s[✔]%s simob is running.\n", ColorGreen, ColorReset)
		} else {
			// Prettier print for 'not running' status
			fmt.Printf("%s[✖]%s simob is not running.\n", ColorRed, ColorReset)
		}
	},
}
