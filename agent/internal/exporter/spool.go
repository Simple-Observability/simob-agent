package exporter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"agent/internal/common"
	"agent/internal/logger"
)

const (
	metricsQueueName = "metrics"
	logsQueueName    = "logs"
	maxBatchSize     = 100
	maxAge           = 24 * time.Hour
)

// unmarshalMetric unmarshals a metric payload from JSON
func unmarshalMetric(data []byte) (Payload, error) {
	var metric MetricPayload
	if err := json.Unmarshal(data, &metric); err != nil {
		return nil, err
	}
	return metric, nil
}

// unmarshalLog unmarshals a log payload from JSON
func unmarshalLog(data []byte) (Payload, error) {
	var log LogPayload
	if err := json.Unmarshal(data, &log); err != nil {
		return nil, err
	}
	return log, nil
}

type spool struct {
	metricsQueue *jsonlQueue
	logsQueue    *jsonlQueue
}

type spoolOption func(*spoolParams)
type spoolParams struct {
	directory string
}

func withDirectory(dir string) spoolOption {
	return func(p *spoolParams) { p.directory = dir }
}

func newSpool(opts ...spoolOption) (*spool, error) {
	params := &spoolParams{}

	for _, opt := range opts {
		opt(params)
	}

	if params.directory == "" {
		programDirectory, err := common.GetProgramDirectory()
		if err != nil {
			return nil, fmt.Errorf("can't create spool directory. failed to get program directory: %w", err)
		}
		params.directory = filepath.Join(programDirectory, "spool")
	}

	err := os.MkdirAll(params.directory, 0o770)
	if err != nil {
		return nil, fmt.Errorf("failed to create spool directory: %w", err)
	}
	info, err := os.Stat(params.directory)
	if err == nil {
		if info.Mode().Perm() != 0o770 {
			err = os.Chmod(params.directory, 0o770)
			if err != nil {
				logger.Log.Debug("Could not set directory permissions", "error", err)
			}
		}
	}

	metricsQueue := newJSONLQueue(metricsQueueName, params.directory)
	logsQueue := newJSONLQueue(logsQueueName, params.directory)

	return &spool{metricsQueue, logsQueue}, nil
}

// appendToSpool appends a single payload to the specified spool file
func (s *spool) append(payload Payload) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	switch payload.(type) {
	case *MetricPayload, MetricPayload:
		return s.metricsQueue.Append(payloadBytes)
	case *LogPayload, LogPayload:
		return s.logsQueue.Append(payloadBytes)
	default:
		return fmt.Errorf("unsupported payload type: %T", payload)
	}
}

func (s *spool) getBatch(fromQueue string, unmarshal func([]byte) (Payload, error)) ([]Payload, bool, error) {
	queue := s.logsQueue
	if fromQueue == metricsQueueName {
		queue = s.metricsQueue
	}

	lines, hasMore, err := queue.PopBatch(maxBatchSize)
	if err != nil {
		return nil, false, err
	}

	var toSend []Payload
	cutoff := time.Now().Add(-maxAge).UnixMilli()
	for _, data := range lines {
		obj, err := unmarshal(data)
		if err != nil {
			logger.Log.Error("failed to unmarshal spool entry", "line", string(data), "error", err)
			continue
		}
		if t, err := strconv.ParseInt(obj.GetTimestamp(), 10, 64); err == nil && t < cutoff {
			logger.Log.Debug("skipping stale entry", "timestamp", obj.GetTimestamp())
			continue
		}
		toSend = append(toSend, obj)
	}
	return toSend, hasMore, nil
}

func (s *spool) close() {
	if err := s.metricsQueue.Close(); err != nil {
		logger.Log.Error("failed to close metrics queue", "error", err)
	}
	if err := s.logsQueue.Close(); err != nil {
		logger.Log.Error("failed to close logs queue", "error", err)
	}
}
