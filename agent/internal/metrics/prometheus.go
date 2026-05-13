package metrics

import (
	"io"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

// ParsePrometheus parses Prometheus text format data into a map of MetricFamily.
func ParsePrometheus(r io.Reader) (map[string]*dto.MetricFamily, error) {
	parser := expfmt.NewTextParser(model.LegacyValidation)
	return parser.TextToMetricFamilies(r)
}

// GetMetricValue returns the value of a Prometheus metric.
// It handles Counters, Gauges, and Untyped metrics.
func GetMetricValue(m *dto.Metric) float64 {
	if m.Gauge != nil {
		return m.GetGauge().GetValue()
	}
	if m.Counter != nil {
		return m.GetCounter().GetValue()
	}
	if m.Untyped != nil {
		return m.GetUntyped().GetValue()
	}
	if m.Summary != nil {
		return m.GetSummary().GetSampleSum()
	}
	if m.Histogram != nil {
		return m.GetHistogram().GetSampleSum()
	}
	return 0
}
