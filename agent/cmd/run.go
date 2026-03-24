package cmd

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"agent/internal/config"
	"agent/internal/exporter"
	"agent/internal/logger"
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
	logger.Init(false)

	jobKey := args[0]

	commandToRunArgs := []string{}
	dashdash := cmd.ArgsLenAtDash()
	if dashdash != -1 {
		commandToRunArgs = args[dashdash:]
	}
	if len(commandToRunArgs) == 0 {
		fmt.Println("Error: No command provided to run. See 'simob run --help' for usage.")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Log.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	monitorURL := fmt.Sprintf("%s/jobs/p/%s", cfg.APIUrl, jobKey)

	command := exec.Command(commandToRunArgs[0], commandToRunArgs[1:]...)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var wg sync.WaitGroup

	var exp *exporter.Exporter
	if captureOutput {
		// Initialize exporter early if we are capturing output
		exp, err = exporter.NewExporterWithoutFlusher()
		if err != nil {
			logger.Log.Error("failed to create exporter", "error", err)
		} else {
			defer exp.Close()
			stdoutPipe, _ := command.StdoutPipe()
			stderrPipe, _ := command.StderrPipe()
			wg.Add(2)
			go streamLogs(jobKey, "stdout", stdoutPipe, exp, &wg)
			go streamLogs(jobKey, "stderr", stderrPipe, exp, &wg)
		}
	} else {
		// If not capturing, just link directly to the terminal
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
	}

	// Handle signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		if command.Process != nil {
			if err := syscall.Kill(-command.Process.Pid, sig.(syscall.Signal)); err != nil {
				logger.Log.Error("failed to kill process", "pid", command.Process.Pid, "error", err)
			}
		}
	}()

	reportStatus(monitorURL, "run")
	err = command.Start()
	if err != nil {
		reportStatus(monitorURL, "fail")
		logger.Log.Error("failed to start command", "error", err)
		os.Exit(1)
	}
	wg.Wait()
	err = command.Wait()

	if err != nil {
		reportStatus(monitorURL, "fail")
	} else {
		reportStatus(monitorURL, "complete")
	}

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			os.Exit(exitError.ExitCode())
		} else {
			os.Exit(1)
		}
	}
	os.Exit(0)
}

func reportStatus(monitorURL, state string) {
	url := fmt.Sprintf("%s?state=%s", monitorURL, state)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logger.Log.Error("failed to create request for status reporting", "error", err)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Log.Error("failed to report status", "error", err, "state", state)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		logger.Log.Error("status reporting failed", "status_code", resp.StatusCode, "state", state)
	}
}

func streamLogs(jobKey, streamName string, pipe io.ReadCloser, exp *exporter.Exporter, wg *sync.WaitGroup) {
	defer wg.Done()

	labels := map[string]string{"source": "cron"}

	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := scanner.Text()
		if streamName == "stdout" {
			fmt.Fprintln(os.Stdout, line)
		} else {
			fmt.Fprintln(os.Stderr, line)
		}

		// TODO: add execution id
		logPayload := exporter.LogPayload{
			Timestamp: fmt.Sprintf("%d", time.Now().UnixMilli()),
			Labels:    labels,
			Metadata:  map[string]string{"job": jobKey, "stream": streamName},
			Message:   line,
		}

		if exp != nil {
			if err := exp.ExportLog([]exporter.LogPayload{logPayload}); err != nil {
				logger.Log.Error("failed to export log line", "error", err, "job_key", jobKey)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		logger.Log.Error("error reading stream", "stream", streamName, "error", err)
	}
}
