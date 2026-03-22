package exporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"agent/internal/authguard"
	"agent/internal/config"
	"agent/internal/logger"
)

const (
	flushInterval = 5 * time.Second
)

type flusher struct {
	apiKey     string
	metricsURL string
	logsURL    string
	httpClient *http.Client
	stopChans  []chan struct{}
	ctx        context.Context
	cancel     context.CancelFunc
	spool      *spool
	dryRun     bool
}

type payloadConfig struct {
	name      string
	url       string
	unmarshal func([]byte) (Payload, error)
}

func newFlusher(spool *spool, dryRun bool) (*flusher, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load exporter configuration: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &flusher{
		apiKey:     cfg.APIKey,
		metricsURL: cfg.MetricsExportUrl,
		logsURL:    cfg.LogsExportUrl,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		ctx:        ctx,
		cancel:     cancel,
		spool:      spool,
		dryRun:     dryRun,
	}, nil
}

// start launches the background flusher goroutines
func (f *flusher) start() {
	streams := []payloadConfig{
		{name: "metrics", url: f.metricsURL, unmarshal: unmarshalMetric},
		{name: "logs", url: f.logsURL, unmarshal: unmarshalLog},
	}
	for _, config := range streams {
		done := make(chan struct{})
		f.stopChans = append(f.stopChans, done)
		go f.runFlusherLoop(config, done)
	}
}

func (f *flusher) stop() {
	if f.cancel != nil {
		logger.Log.Debug("Exporter received shutdown signal")
		f.cancel()
		for _, done := range f.stopChans {
			<-done
		}
		logger.Log.Debug("Exporter shutdown complete")
	}
}

// runFlusherLoop runs the periodic flush loop
func (f *flusher) runFlusherLoop(cfg payloadConfig, done chan struct{}) {
	defer close(done)

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-f.ctx.Done():
			// Final flush before shutdown
			f.flushAll(cfg)
			return
		case <-ticker.C:
			f.flushAll(cfg)
		}
	}
}

// flushAll processes all entries in the spool, sending them in batches
// until the file is empty or context is cancelled
func (f *flusher) flushAll(cfg payloadConfig) {
	for {
		select {
		case <-f.ctx.Done():
			return
		default:
		}

		hasMoreEntries, err := f.flushOnce(cfg)
		if err != nil {
			logger.Log.Error("error during flush", "error", err)
			return
		}
		if !hasMoreEntries {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// flushOnce processed and sends a batch from the spool file
func (f *flusher) flushOnce(cfg payloadConfig) (bool, error) {
	toSend, hasMore, err := f.spool.getBatch(cfg.name, cfg.unmarshal)
	if err != nil {
		return false, fmt.Errorf("failed to get payloads from spool: %w", err)
	}

	// Send batch if we have valid entries
	if len(toSend) > 0 {
		if err := f.sendPayload(cfg.url, toSend); err != nil {
			// When sending fails, put back into the spool
			for _, p := range toSend {
				_ = f.spool.append(p)
			}
			return false, fmt.Errorf("failed to send batch: %w", err)
		}
		logger.Log.Debug("successfully sent batch", "url", cfg.url, "count", len(toSend))
	}
	return hasMore, nil
}

// sendPayload is a private helper function to send JSON data to a given URL.
func (f *flusher) sendPayload(url string, payload []Payload) error {
	// Dry run. Print payload without actually sending the request
	if f.dryRun {
		prettyPayload, err := json.MarshalIndent(payload, "", " ")
		if err != nil {
			logger.Log.Error("failed to pretty-print payload for dry-run", "error", err)
			return nil
		}
		fmt.Printf("[dry-run] Would send payload: %v\n", string(prettyPayload))
		return nil
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", f.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send data to %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		authguard.Get().HandleUnauthorized()
	}

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("data export to %s failed with status code: %d", url, resp.StatusCode)
	}
	return nil
}
