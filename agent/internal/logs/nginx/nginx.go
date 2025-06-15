package nginx

import (
	"context"
	"path/filepath"

	"agent/internal/logs"
)

type NginxLogCollector struct {
	name    string
	pattern string
	runner  *logs.TailRunner
}

func NewNginxLogCollector() *NginxLogCollector {
	return &NginxLogCollector{
		name:    "nginx",
		pattern: "/var/log/nginx/*.log",
	}
}

func (c *NginxLogCollector) Name() string {
	return c.name
}

func (c *NginxLogCollector) Discover() []logs.LogSource {
	sources := []logs.LogSource{}
	files, _ := filepath.Glob(c.pattern)
	if len(files) > 0 {
		sources = append(sources, logs.LogSource{Name: c.name, Path: c.pattern})
	}
	return sources
}

func (c *NginxLogCollector) Start(ctx context.Context, out chan<- logs.LogEntry) error {
	// Initialize the runner on the first start
	if c.runner == nil {
		regexString := `\[(?P<timestamp>\d{2}/\w{3}/\d{4}:\d{2}:\d{2}:\d{2} [+-]\d{4})\]`
		runner, err := logs.NewTailRunner(c.pattern, regexString, c.name)
		if err != nil {
			return err
		}
		c.runner = runner
	}
	return c.runner.Start(ctx, out)
}

func (c *NginxLogCollector) Stop() error {
	return c.runner.Stop()
}
