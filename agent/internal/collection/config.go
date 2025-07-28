package collection

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// Metric represents a type of measurement collected by a metric collector.
type Metric struct {
	Name   string            `json:"name"`
	Type   string            `json:"type"`
	Value  float64           `json:"value"`
	Unit   string            `json:"unit"`
	Labels map[string]string `json:"labels"`
}

type LogSource struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type CollectionConfig struct {
	Metrics    []Metric    `json:"metrics"`
	LogSources []LogSource `json:"log_sources"`
}

func (c *CollectionConfig) Hash() (string, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum), nil
}
