package metrics

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsePrometheus(t *testing.T) {
	input := `
# HELP caddy_http_requests_total Total number of HTTP requests
# TYPE caddy_http_requests_total counter
caddy_http_requests_total{handler="reverse_proxy",server="srv0"} 10
caddy_http_requests_total{handler="file_server",server="srv0"} 5
# HELP process_resident_memory_bytes Resident memory size in bytes.
# TYPE process_resident_memory_bytes gauge
process_resident_memory_bytes 1.048576e+08
`
	families, err := ParsePrometheus(strings.NewReader(input))
	assert.NoError(t, err)
	assert.Len(t, families, 2)

	reqFamily, ok := families["caddy_http_requests_total"]
	assert.True(t, ok)
	assert.Len(t, reqFamily.Metric, 2)

	memFamily, ok := families["process_resident_memory_bytes"]
	assert.True(t, ok)
	assert.Len(t, memFamily.Metric, 1)
	assert.Equal(t, 104857600.0, memFamily.Metric[0].Gauge.GetValue())
}

func TestGetMetricValue(t *testing.T) {
	families, _ := ParsePrometheus(strings.NewReader("metric_counter 10\nmetric_gauge 20\n"))
	
	counterVal := GetMetricValue(families["metric_counter"].Metric[0])
	assert.Equal(t, 10.0, counterVal)

	gaugeVal := GetMetricValue(families["metric_gauge"].Metric[0])
	assert.Equal(t, 20.0, gaugeVal)
}
