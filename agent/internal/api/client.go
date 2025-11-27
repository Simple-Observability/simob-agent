package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"agent/internal/authguard"
	"agent/internal/collection"
	"agent/internal/config"
	"agent/internal/hostinfo"
	"agent/internal/logger"
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

// CheckAPIKeyValidity checks if the API key is still valid.
func (c *Client) CheckAPIKeyValidity() (bool, error) {
	_, err := c.post("/check-key/", struct{}{})
	if err != nil {
		return false, err
	}
	return true, nil
}

func (c *Client) GetCollectionConfig() (*collection.CollectionConfig, error) {
	// Add cache buster param with current timestamp (ms)
	cb := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
	path := "/configs/?cb=" + cb

	res, err := c.get(path)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}

	var cfg collection.CollectionConfig
	if err := json.NewDecoder(res.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	return &cfg, nil
}

func (c *Client) PostAvailableMetrics(metrics []collection.Metric) error {
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

func (c *Client) PostAvailableLogSources(log []collection.LogSource) error {
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

func (c *Client) get(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Api-Key "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		authguard.Get().HandleUnauthorized()
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

	logger.Log.Debug("API GET successful", "path", path, "status", res.StatusCode)
	return res, nil
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

	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		authguard.Get().HandleUnauthorized()
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
