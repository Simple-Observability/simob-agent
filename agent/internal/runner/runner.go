package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"agent/internal/config"
	"agent/internal/logger"
)

var statusHTTPClient = &http.Client{Timeout: 10 * time.Second}

type statusReportResponse struct {
	ExecutionID string `json:"execution"`
}

func Run(jobKey string, commandArgs []string, captureOutput bool) int {
	logger.Init(false)

	if len(commandArgs) == 0 {
		fmt.Println("Error: No command provided to run. See 'simob run --help' for usage.")
		return 1
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Log.Error("failed to load config", "error", err)
		return 1
	}

	command := exec.Command(commandArgs[0], commandArgs[1:]...)
	configureRunCommand(command)

	logCapture, err := newCommandLogCapture(jobKey, captureOutput)
	if err != nil {
		logger.Log.Error("failed to initialize command log capture", "error", err)
	}
	defer logCapture.Close()
	logCapture.attach(command)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, runCommandSignals()...)
	defer signal.Stop(sigChan)
	go func() {
		sig := <-sigChan
		if err := forwardRunSignal(command, sig); err != nil {
			logger.Log.Error("failed to forward signal to process", "signal", sig, "error", err)
		}
	}()

	monitorURL := fmt.Sprintf("%s/jobs/p/%s", cfg.APIUrl, jobKey)
	executionID := reportStatus(monitorURL, "run")
	logCapture.setExecutionID(executionID)

	err = command.Start()
	if err != nil {
		reportStatus(monitorURL, "fail")
		logger.Log.Error("failed to start command", "error", err)
		return 1
	}

	err = command.Wait()
	logCapture.closeNow()

	if err != nil {
		reportStatus(monitorURL, "fail")
	} else {
		reportStatus(monitorURL, "complete")
	}

	if exitError, ok := err.(*exec.ExitError); ok {
		return exitError.ExitCode()
	}
	if err != nil {
		return 1
	}

	return 0
}

func reportStatus(monitorURL, state string) string {
	url := fmt.Sprintf("%s?state=%s&v=1", monitorURL, state)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logger.Log.Error("failed to create request for status reporting", "error", err)
		return ""
	}

	resp, err := statusHTTPClient.Do(req)
	if err != nil {
		logger.Log.Error("failed to report status", "error", err, "state", state)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Log.Error("status reporting failed", "status_code", resp.StatusCode, "state", state)
		return ""
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Log.Error("failed to read verbose status response", "error", err, "state", state)
		return ""
	}
	if len(body) == 0 {
		return ""
	}
	var payload statusReportResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		logger.Log.Error("failed to decode verbose status response", "error", err, "state", state)
		return ""
	}
	return payload.ExecutionID
}
