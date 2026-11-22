package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"agent/internal/authguard"
	"agent/internal/collection"
	"agent/internal/config"
	"agent/internal/hardware"
	"agent/internal/hostinfo"
	"agent/internal/logger"
	"agent/internal/packages"
)

type Client struct {
	apiKey  string
	baseURL string
	client  *http.Client
	dryRun  bool
}

func NewClient(cfg config.Config, dryRun bool) *Client {
	return &Client{
		apiKey:  cfg.APIKey,
		baseURL: cfg.APIUrl,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		dryRun: dryRun,
	}
}

// CheckAPIKeyValidity checks if the API key is still valid.
func (c *Client) CheckAPIKeyValidity() (bool, error) {
	if c.dryRun {
		return true, nil
	}

	_, err := c.post("/check-key/", struct{}{})
	if err != nil {
		return false, err
	}
	return true, nil
}

func (c *Client) GetCollectionConfig() (*collection.CollectionConfig, error) {
	if c.dryRun {
		return nil, nil
	}

	// Add cache buster param with current timestamp (ms)
	cb := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
	path := "/configs/?cb=" + cb

	res, err := c.get(path)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	var cfg collection.CollectionConfig
	if err := json.NewDecoder(res.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	return &cfg, nil
}

func (c *Client) PostAvailableMetrics(metrics []collection.Metric) error {
	if c.dryRun {
		return nil
	}

	res, err := c.post("/metrics/", metrics)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	return nil
}

func (c *Client) PostAvailableLogSources(log []collection.LogSource) error {
	if c.dryRun {
		return nil
	}

	res, err := c.post("/logs/", log)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	return nil
}

func (c *Client) PostHostInfo(info hostinfo.HostInfo) error {
	if c.dryRun {
		return nil
	}

	res, err := c.post("/servers/info/", info)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	return nil
}

func (c *Client) PostPhysicalDisks(disks []hardware.Disk) error {
	if c.dryRun {
		return nil
	}

	res, err := c.post("/servers/disks/", disks)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	return nil
}

func (c *Client) PostNetworkInterfaces(interfaces []hardware.Interface) error {
	if c.dryRun {
		return nil
	}

	res, err := c.post("/servers/interfaces/", interfaces)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	return nil
}

// PostPackageInventory reports whether the backend acknowledged the inventory observation.
func (c *Client) PostPackageInventory(ctx context.Context, report packages.Report) (bool, error) {
	if c.dryRun {
		return false, nil
	}

	const path = "/servers/packages/"
	var res *http.Response
	var err error
	if snapshot := report.Snapshot; snapshot != nil {
		res, err = c.postBody(ctx, path, snapshot.ContentType, snapshot.Body, 60*time.Second)
	} else {
		res, err = c.postJSON(ctx, path, report.Observation)
	}
	if err != nil {
		return false, err
	}
	defer res.Body.Close()

	var ack packages.Acknowledgement
	if err := json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&ack); err != nil {
		return false, fmt.Errorf("decode package inventory acknowledgement: %w", err)
	}
	err = report.ValidateAcknowledgement(ack)
	return err == nil, err
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
			"GET %s failed: %s (status %d)",
			path,
			string(buf[:n]),
			res.StatusCode,
		)
	}

	logger.Log.Debug("API GET successful", "path", path, "status", res.StatusCode)
	return res, nil
}

func (c *Client) post(path string, payload interface{}) (*http.Response, error) {
	return c.postJSON(context.Background(), path, payload)
}

func (c *Client) postJSON(ctx context.Context, path string, payload interface{}) (*http.Response, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return c.postBody(ctx, path, "application/json", jsonData, 0)
}

func (c *Client) postBody(ctx context.Context, path, contentType string, data []byte, timeout time.Duration) (*http.Response, error) {
	httpClient := c.client
	if timeout > 0 {
		client := *c.client
		client.Timeout = timeout
		httpClient = &client
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Api-Key "+c.apiKey)
	req.Header.Set("Content-Type", contentType)

	res, err := httpClient.Do(req)
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
