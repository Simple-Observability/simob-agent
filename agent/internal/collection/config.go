package collection

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
)

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

func (c CollectionConfig) Hash() (string, error) {
	// Copy to avoid mutating objects
	metricsCopy := make([]Metric, len(c.Metrics))
	copy(metricsCopy, c.Metrics)
	logSourcesCopy := make([]LogSource, len(c.LogSources))
	copy(logSourcesCopy, c.LogSources)

	// Normalize
	sort.Slice(metricsCopy, func(i, j int) bool {
		bI, _ := json.Marshal(metricsCopy[i])
		bJ, _ := json.Marshal(metricsCopy[j])
		return string(bI) < string(bJ)
	})
	sort.Slice(logSourcesCopy, func(i, j int) bool {
		bI, _ := json.Marshal(logSourcesCopy[i])
		bJ, _ := json.Marshal(logSourcesCopy[j])
		return string(bI) < string(bJ)
	})
	normalized := CollectionConfig{Metrics: metricsCopy, LogSources: logSourcesCopy}

	data, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum), nil
}
