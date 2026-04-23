package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"agent/internal/logger"
	"agent/internal/logs"
	logsRegistry "agent/internal/logs/registry"
	metricsRegistry "agent/internal/metrics/registry"
)

var inspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "Inspect collectors",
}

var inspectMetricsCmd = &cobra.Command{
	Use:   "metrics <collector_name>",
	Short: "Inspect a specific metrics collector and print its output",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		collectorName := args[0]
		logger.Init(os.Getenv("DEBUG") == "1")

		metricsCollectors := metricsRegistry.BuildCollectors(nil)
		for _, c := range metricsCollectors {
			if c.Name() == collectorName {
				// First collection to init state
				_, _ = c.CollectAll()
				time.Sleep(1 * time.Second)

				data, err := c.CollectAll()
				if err != nil {
					return fmt.Errorf("failed to collect metrics: %w", err)
				}
				prettyJSON, _ := json.MarshalIndent(data, "", "  ")
				fmt.Println(string(prettyJSON))
				return nil
			}
		}
		return fmt.Errorf("metrics collector '%s' not found", collectorName)
	},
}

var inspectLogsCmd = &cobra.Command{
	Use:   "logs <collector_name>",
	Short: "Inspect a specific logs collector and print its output",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		collectorName := args[0]
		logger.Init(os.Getenv("DEBUG") == "1")

		logsCollectors := logsRegistry.BuildCollectors(nil)
		for _, c := range logsCollectors {
			if c.Name() == collectorName {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				sigs := make(chan os.Signal, 1)
				signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
				go func() {
					<-sigs
					cancel()
				}()

				logsChan := make(chan logs.LogEntry, 100)
				err := c.Start(ctx, logsChan)
				if err != nil {
					return fmt.Errorf("failed to start log collector: %w", err)
				}

				fmt.Fprintf(os.Stderr, "Waiting for first log entry from %s...\n", collectorName)
				select {
				case entry := <-logsChan:
					prettyJSON, _ := json.MarshalIndent(entry, "", "  ")
					fmt.Println(string(prettyJSON))
					_ = c.Stop()
					return nil
				case <-ctx.Done():
					_ = c.Stop()
					return nil
				}
			}
		}
		return fmt.Errorf("log collector '%s' not found", collectorName)
	},
}

func init() {
	inspectCmd.AddCommand(inspectMetricsCmd)
	inspectCmd.AddCommand(inspectLogsCmd)
	rootCmd.AddCommand(inspectCmd)
}
