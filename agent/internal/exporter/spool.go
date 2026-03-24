package exporter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/nsqio/go-diskqueue"

	"agent/internal/common"
	"agent/internal/logger"
)

const (
	metricsQueueName = "metrics"
	logsQueueName    = "logs"
	maxBatchSize     = 100
	maxAge           = 24 * time.Hour
	maxBytesPerFile  = int64(1e8)
	minMsgSize       = int32(1)
	maxMsgSize       = int32(1e7)
	syncEvery        = int64(100)
	syncTimeout      = 2 * time.Second
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
	metricsQueue diskqueue.Interface
	logsQueue    diskqueue.Interface
}

type spoolOption func(*spoolParams)
type spoolParams struct {
	directory string
	syncEvery int64
}

func withDirectory(dir string) spoolOption {
	return func(p *spoolParams) { p.directory = dir }
}
func withSyncEvery(every int64) spoolOption {
	return func(p *spoolParams) { p.syncEvery = every }
}
func newSpool(opts ...spoolOption) (*spool, error) {
	// Defaults
	params := &spoolParams{
		syncEvery: syncEvery,
	}

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

	dummyLogger := func(lvl diskqueue.LogLevel, f string, args ...interface{}) {}
	metricsQueue := diskqueue.New(
		metricsQueueName, params.directory, maxBytesPerFile,
		minMsgSize, maxMsgSize,
		params.syncEvery, syncTimeout,
		dummyLogger,
	)
	logsQueue := diskqueue.New(
		logsQueueName, params.directory, maxBytesPerFile,
		minMsgSize, maxMsgSize,
		params.syncEvery, syncTimeout,
		dummyLogger,
	)

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
		return s.metricsQueue.Put(payloadBytes)
	case *LogPayload, LogPayload:
		return s.logsQueue.Put(payloadBytes)
	default:
		return fmt.Errorf("unsupported payload type: %T", payload)
	}
}

func (s *spool) getBatch(fromQueue string, unmarshal func([]byte) (Payload, error)) ([]Payload, bool, error) {
	queue := s.logsQueue
	if fromQueue == metricsQueueName {
		queue = s.metricsQueue
	}

	if queue.Depth() == 0 {
		return nil, false, nil
	}

	var toSend []Payload
	cutoff := time.Now().Add(-maxAge).UnixMilli()
	for len(toSend) < maxBatchSize {
		select {
		case data := <-queue.ReadChan():
			// Skip corrupted entries
			obj, err := unmarshal(data)
			if err != nil {
				logger.Log.Error("failed to unmarshal spool entry", "line", data, "error", err)
				continue
			}
			// Skip stale (old) entries
			if t, err := strconv.ParseInt(obj.GetTimestamp(), 10, 64); err == nil && t < cutoff {
				logger.Log.Debug("skipping stale entry", "timestamp", obj.GetTimestamp())
				continue
			}
			toSend = append(toSend, obj)
		case <-time.After(50 * time.Millisecond):
			// Timeout waiting for next item, return what we have
			return toSend, queue.Depth() > 0, nil
		}
	}
	return toSend, queue.Depth() > 0, nil
}

func (s *spool) close() {
	s.metricsQueue.Close()
	s.logsQueue.Close()
}
