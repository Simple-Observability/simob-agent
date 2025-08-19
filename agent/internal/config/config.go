package config

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"

	"agent/internal/common"
	"agent/internal/logger"
)

type Config struct {
	APIKey           string `json:"api_key"`
	APIUrl           string `json:"api_url"`
	LogsExportUrl    string `json:"logs_export_url"`
	MetricsExportUrl string `json:"metrics_export_url"`
}

const ConfigFilename = "config.json"

func NewConfig(apiKey string) *Config {
	// Defaults
	defaultAPIUrl := "https://api.simpleobservability.com"
	defaultLogsExportUrl := "https://logs.simpleobservability.com"
	defaultMetricsExportUrl := "https://metrics.simpleobservability.com"

	// Start with defaults
	cfg := &Config{
		APIKey:           apiKey,
		APIUrl:           defaultAPIUrl,
		LogsExportUrl:    defaultLogsExportUrl,
		MetricsExportUrl: defaultMetricsExportUrl,
	}

	// Try to load existing config file first
	logger.Log.Debug("Trying to load existing config file")
	if existingCfg, err := Load(); err == nil {
		// If config file exists, use its values (override defaults)
		if existingCfg.APIKey != "" {
			cfg.APIKey = existingCfg.APIKey
		}
		if existingCfg.APIUrl != "" {
			cfg.APIUrl = existingCfg.APIUrl
		}
		if existingCfg.LogsExportUrl != "" {
			cfg.LogsExportUrl = existingCfg.LogsExportUrl
		}
		if existingCfg.MetricsExportUrl != "" {
			cfg.MetricsExportUrl = existingCfg.MetricsExportUrl
		}
	} else {
		logger.Log.Debug("Failed to open existing config file")
	}

	// Finally, override with provided apiKey parameter if it's not empty
	if apiKey != "" {
		cfg.APIKey = apiKey
		logger.Log.Debug("Overriding API key")
	} else {
		logger.Log.Debug("apiKey parameter is empty. Leave API key as is.")
	}

	logger.Log.Debug("Config created", slog.Any("cfg", cfg))
	return cfg
}

// Setters
func (c *Config) SetAPIKey(apiKey string)                     { c.APIKey = apiKey }
func (c *Config) SetAPIUrl(apiUrl string)                     { c.APIUrl = apiUrl }
func (c *Config) SetLogsExportUrl(logsExportUrl string)       { c.LogsExportUrl = logsExportUrl }
func (c *Config) SetMetricsExportUrl(metricsExportUrl string) { c.MetricsExportUrl = metricsExportUrl }

func ConfigPath() (string, error) {
	programDirectory, err := common.GetProgramDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(programDirectory, ConfigFilename), nil
}

func (c *Config) Save() error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	logger.Log.Debug("Saving config", slog.Any("cfg", c))
	return encoder.Encode(c)
}

func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
