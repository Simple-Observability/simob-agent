package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"agent/internal/config"
	"agent/internal/hostinfo"
	"agent/internal/logger"
	"agent/internal/logs"
	"agent/internal/metrics"
)

type Client struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewClient(cfg config.Config) *Client {
	return &Client{
		apiKey:  cfg.APIKey,
		baseURL: cfg.APIUrl,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) PostAvailableMetrics(metrics []metrics.Metric) error {
	res, err := c.post("/metrics/", metrics)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}
	return nil
}

func (c *Client) PostAvailableLogSources(log []logs.LogSource) error {
	res, err := c.post("/logs/", log)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}
	return nil
}

func (c *Client) PostHostInfo(info hostinfo.HostInfo) error {
	res, err := c.post("/servers/info/", info)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}
	return nil
}

func (c *Client) post(path string, payload interface{}) (*http.Response, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.baseURL+path, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Api-Key "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var buf [512]byte
		n, _ := res.Body.Read(buf[:])
		res.Body.Close()
		return nil, fmt.Errorf(
			"POST %s failed: %s (status %d)",
			path,
			string(buf[:n]),
			res.StatusCode,
		)
	}

	logger.Log.Debug("API POST successful", "path", path, "status", res.StatusCode)
	return res, nil
}
