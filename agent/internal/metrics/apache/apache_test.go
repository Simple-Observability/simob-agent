package apache

import (
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"agent/internal/collection"
	"agent/internal/logger"
	"agent/internal/metrics"
)

func init() {
	logger.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
}

type mockPS struct {
	mock.Mock
}

func (m *mockPS) GetStatusPageBody(url string) (string, error) {
	args := m.Called(url)
	return args.String(0), args.Error(1)
}

const apacheStatusBody = `Total Accesses: 129811861
Total kBytes: 5213701865
CPULoad: 6.51929
Uptime: 941553
ReqPerSec: 137.87
BytesPerSec: 5670240
BytesPerReq: 41127.4
BusyWorkers: 270
IdleWorkers: 630
ConnsTotal: 1451
ConnsAsyncWriting: 32
ConnsAsyncKeepAlive: 945
ConnsAsyncClosing: 205
Scoreboard: WW_____W_RW_R_W__RRR____WR_W___WW________W_WW_W_____R__R_WR__WRWR_RRRW___R_RWW__WWWRW__R_RW___RR_RW_R__W__WR_WWW______WWR__R___R_WR_W___RW______RR________________W______R__RR______W________________R____R__________________________RW_W____R_____W_R_________________R____RR__W___R_R____RW______R____W______W_W_R_R______R__R_R__________R____W_______WW____W____RR__W_____W_R_______W__________W___W____________W_______WRR_R_W____W_____R____W_WW_R____RRW__W............................................................................................................................................................................................................................................................................................................WRRWR____WR__RR_R___RWR_________W_R____RWRRR____R_R__RW_R___WWW_RW__WR_RRR____W___R____WW_R__R___RR_W_W_RRRRWR__RRWR__RRW_W_RRRW_R_RR_W__RR_RWRR_R__R___RR_RR______R__RR____R_____W_R_R_R__R__R__________W____WW_R___R_R___R_________RR__RR____RWWWW___W_R________R_R____R_W___W___R___W_WRRWW_______R__W_RW_______R________RR__R________W_______________________W_W______________RW_________WR__R___R__R_______________WR_R_________W___RW_____R____________W____......................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................`

func TestApacheCollector(t *testing.T) {
	var mps mockPS
	defer mps.AssertExpectations(t)

	mps.On("GetStatusPageBody", mock.Anything).Return(apacheStatusBody, nil).Once()

	c := &ApacheCollector{
		ps:  &mps,
		url: "http://localhost/server-status?auto",
	}

	dps, err := c.CollectAll()
	require.NoError(t, err)

	assert.Len(t, dps, 10)
	assertContainsMetric(t, dps, "apache_requests_total", 129811861.0)
	assertContainsMetric(t, dps, "apache_requests_rate", 137.87)
	assertContainsMetric(t, dps, "apache_bytes_total", 5213701865.0*1024.0)
	assertContainsMetric(t, dps, "apache_bytes_bps", 5670240.0)
	assertContainsMetric(t, dps, "apache_workers_busy_total", 270.0)
	assertContainsMetric(t, dps, "apache_workers_idle_total", 630.0)
	assertContainsMetric(t, dps, "apache_connections_total", 1451.0)
	assertContainsMetric(t, dps, "apache_connections_writing_total", 32.0)
	assertContainsMetric(t, dps, "apache_connections_keepalive_total", 945.0)
	assertContainsMetric(t, dps, "apache_connections_closing_total", 205.0)
}

func TestApacheCollector_Discover(t *testing.T) {
	var mps mockPS
	mps.On("GetStatusPageBody", mock.Anything).Return(apacheStatusBody, nil).Once()

	c := &ApacheCollector{
		ps:  &mps,
		url: "http://localhost/server-status?auto",
	}

	discovered, err := c.Discover()
	require.NoError(t, err)
	require.Len(t, discovered, 10)

	assert.Equal(t, "apache_requests_total", discovered[0].Name)
	assert.Equal(t, "apache_requests_rate", discovered[1].Name)
	assert.Equal(t, "apache_bytes_total", discovered[2].Name)
	assert.Equal(t, "apache_bytes_bps", discovered[3].Name)
	assert.Equal(t, "apache_workers_busy_total", discovered[4].Name)
	assert.Equal(t, "apache_workers_idle_total", discovered[5].Name)
	assert.Equal(t, "apache_connections_total", discovered[6].Name)
	assert.Equal(t, "apache_connections_writing_total", discovered[7].Name)
	assert.Equal(t, "apache_connections_keepalive_total", discovered[8].Name)
	assert.Equal(t, "apache_connections_closing_total", discovered[9].Name)
}

func TestApacheCollector_Errors(t *testing.T) {
	t.Run("GetBodyError", func(t *testing.T) {
		var mps mockPS
		mps.On("GetStatusPageBody", mock.Anything).Return("", fmt.Errorf("http error")).Once()

		c := &ApacheCollector{ps: &mps}
		dps, err := c.CollectAll()
		require.NoError(t, err)
		assert.Nil(t, dps)
	})

	t.Run("ParseError", func(t *testing.T) {
		var mps mockPS
		mps.On("GetStatusPageBody", mock.Anything).Return("invalid body", nil).Once()

		c := &ApacheCollector{ps: &mps}
		dps, err := c.CollectAll()
		require.NoError(t, err)
		t.Logf("dps=%+v", dps)
	})
}

func TestApacheCollector_Filtering(t *testing.T) {
	var mps mockPS
	mps.On("GetStatusPageBody", mock.Anything).Return(apacheStatusBody, nil).Once()

	c := &ApacheCollector{
		ps:  &mps,
		url: "http://localhost/server-status?auto",
	}
	c.SetIncludedMetrics([]collection.Metric{
		{Name: "apache_connections_keepalive_total"},
		{Name: "apache_requests_total"},
		{Name: "apache_connections_total"},
	})

	dps, err := c.Collect()
	require.NoError(t, err)
	require.Len(t, dps, 3)
	assertContainsMetric(t, dps, "apache_connections_keepalive_total", 945.0)
	assertContainsMetric(t, dps, "apache_requests_total", 129811861.0)
	assertContainsMetric(t, dps, "apache_connections_total", 1451.0)
}

func assertContainsMetric(t *testing.T, dps []metrics.DataPoint, name string, value float64) {
	for _, dp := range dps {
		if dp.Name == name {
			assert.Equal(t, value, dp.Value, "Metric %s", name)
			return
		}
	}
	var names []string
	for _, dp := range dps {
		names = append(names, fmt.Sprintf("%s=%v", dp.Name, dp.Value))
	}
	assert.Failf(t, "Metric not found", "Could not find metric %q. Got %v", name, names)
}
