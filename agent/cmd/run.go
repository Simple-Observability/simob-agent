package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"agent/internal/runner"
)

var runCmd = &cobra.Command{
	Use:   "run <job-key> -- <command>",
	Short: "Run a command and report its execution status to Simple Observability.",
	Long: `Wraps a command and reports its execution lifecycle to Simple Observability
(start, success, failure).

The command's stdout and stderr are passed through unchanged.

Example:
simob run Vp4s8S0SsnMo -- /usr/local/bin/backup.sh | gzip > /backups/backup.gz
`,
	Run:  runCommand,
	Args: cobra.MinimumNArgs(1),
}

var captureOutput bool

func init() {
	runCmd.Flags().BoolVar(
		&captureOutput,
		"capture-output",
		false,
		"Capture stdout and stderr and send them to Simple Observability",
	)
}

func runCommand(cmd *cobra.Command, args []string) {
	os.Exit(runCommandWithExitCode(cmd, args))
}

func runCommandWithExitCode(cmd *cobra.Command, args []string) int {
	jobKey := args[0]
	dashdash := cmd.ArgsLenAtDash()
	if dashdash == -1 || len(args[dashdash:]) == 0 {
		fmt.Println("Error: No command provided to run. See 'simob run --help' for usage.")
		return 1
	}
	commandToRunArgs := args[dashdash:]
	return runner.Run(jobKey, commandToRunArgs, captureOutput)
}
