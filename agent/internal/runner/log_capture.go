package runner

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"agent/internal/exporter"
	"agent/internal/logger"
)

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
