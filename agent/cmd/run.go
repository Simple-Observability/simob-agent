package cmd

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
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
	os.Exit(runCommandWithExitCode(cmd, args))
}

func runCommandWithExitCode(cmd *cobra.Command, args []string) int {
	logger.Init(false)

	jobKey := args[0]

	commandToRunArgs := []string{}
	dashdash := cmd.ArgsLenAtDash()
	if dashdash != -1 {
		commandToRunArgs = args[dashdash:]
	}
	if len(commandToRunArgs) == 0 {
		fmt.Println("Error: No command provided to run. See 'simob run --help' for usage.")
		return 1
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Log.Error("failed to load config", "error", err)
		return 1
	}
	monitorURL := fmt.Sprintf("%s/jobs/p/%s", cfg.APIUrl, jobKey)

	command := exec.Command(commandToRunArgs[0], commandToRunArgs[1:]...)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	logCapture, err := newCommandLogCapture(jobKey, captureOutput)
	if err != nil {
		logger.Log.Error("failed to initialize command log capture", "error", err)
	}
	defer logCapture.Close()
	logCapture.attach(command)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)
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
		return 1
	}
	err = command.Wait()

	logCapture.closeNow()

	if err != nil {
		reportStatus(monitorURL, "fail")
	} else {
		reportStatus(monitorURL, "complete")
	}

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return exitError.ExitCode()
		} else {
			return 1
		}
	}
	return 0
}

var statusHTTPClient = &http.Client{Timeout: 10 * time.Second}

func reportStatus(monitorURL, state string) {
	url := fmt.Sprintf("%s?state=%s", monitorURL, state)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logger.Log.Error("failed to create request for status reporting", "error", err)
		return
	}
	resp, err := statusHTTPClient.Do(req)
	if err != nil {
		logger.Log.Error("failed to report status", "error", err, "state", state)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		logger.Log.Error("status reporting failed", "status_code", resp.StatusCode, "state", state)
	}
}

type commandLogCapture struct {
	jobKey string
	exp    *exporter.Exporter
	stdout *logCaptureWriter
	stderr *logCaptureWriter
}

func newCommandLogCapture(jobKey string, enabled bool) (*commandLogCapture, error) {
	capture := &commandLogCapture{jobKey: jobKey}
	if !enabled {
		return capture, nil
	}

	exp, err := exporter.NewExporterWithoutFlusher()
	if err != nil {
		return capture, err
	}

	capture.exp = exp
	capture.stdout = newLogCaptureWriter(jobKey, "stdout", exp)
	capture.stderr = newLogCaptureWriter(jobKey, "stderr", exp)
	return capture, nil
}

func (c *commandLogCapture) attach(command *exec.Cmd) {
	if c.stdout != nil {
		command.Stdout = io.MultiWriter(os.Stdout, c.stdout)
	} else {
		command.Stdout = os.Stdout
	}

	if c.stderr != nil {
		command.Stderr = io.MultiWriter(os.Stderr, c.stderr)
	} else {
		command.Stderr = os.Stderr
	}
}

func (c *commandLogCapture) Close() {
	if c == nil {
		return
	}
	c.closeNow()
}

func (c *commandLogCapture) closeNow() {
	if c == nil {
		return
	}

	c.closeWriter("stderr", &c.stderr)
	c.closeWriter("stdout", &c.stdout)

	if c.exp != nil {
		c.exp.Close()
		c.exp = nil
	}
}

func (c *commandLogCapture) closeWriter(streamName string, writer **logCaptureWriter) {
	if *writer == nil {
		return
	}

	if err := (*writer).Close(); err != nil {
		logger.Log.Error("failed to flush command output capture", "error", err, "job_key", c.jobKey, "stream", streamName)
	}
	*writer = nil
}

type logCaptureWriter struct {
	jobKey     string
	streamName string
	exp        *exporter.Exporter
	buf        bytes.Buffer
}

func newLogCaptureWriter(jobKey, streamName string, exp *exporter.Exporter) *logCaptureWriter {
	return &logCaptureWriter{
		jobKey:     jobKey,
		streamName: streamName,
		exp:        exp,
	}
}

func (w *logCaptureWriter) Write(p []byte) (int, error) {
	if _, err := w.buf.Write(p); err != nil {
		return 0, err
	}
	for {
		line, ok := w.nextLine()
		if !ok {
			break
		}
		w.exportLine(line)
	}
	return len(p), nil
}

func (w *logCaptureWriter) Close() error {
	if w.buf.Len() == 0 {
		return nil
	}
	line := strings.TrimRight(w.buf.String(), "\r\n")
	w.buf.Reset()
	if line != "" {
		w.exportLine(line)
	}
	return nil
}

func (w *logCaptureWriter) nextLine() (string, bool) {
	data := w.buf.Bytes()
	idx := bytes.IndexByte(data, '\n')
	if idx == -1 {
		return "", false
	}
	line := string(data[:idx])
	if strings.HasSuffix(line, "\r") {
		line = strings.TrimSuffix(line, "\r")
	}
	w.buf.Next(idx + 1)
	return line, true
}

func (w *logCaptureWriter) exportLine(line string) {
	logPayload := exporter.LogPayload{
		Timestamp: fmt.Sprintf("%d", time.Now().UnixMilli()),
		Labels:    map[string]string{"source": "cron"},
		Metadata:  map[string]string{"job": w.jobKey, "stream": w.streamName},
		Message:   line,
	}
	if err := w.exp.ExportLog([]exporter.LogPayload{logPayload}); err != nil {
		logger.Log.Error("failed to export log line", "error", err, "job_key", w.jobKey)
	}
}
