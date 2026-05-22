package processes

import (
	"context"
	"encoding/json"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"

	"agent/internal/exporter"
	"agent/internal/logger"
)

const (
	// maxNumWorkers limits the number of concurrent goroutines fetching process info.
	// 8 is chosen as a balance between speed and system call overhead.
	maxNumWorkers = 8
)

type perProcessResult struct {
	info  ProcessInfo
	stats *processCPUStats
}

// processStats stores historical data for computing interval-average CPU usage
type processCPUStats struct {
	CPUTime   float64
	Timestamp int64
}

// Collector collects system process information
type Collector struct {
	mu        sync.RWMutex
	prevStats map[int32]processCPUStats
}

// NewCollector creates a new Collector instance
func NewCollector() *Collector {
	return &Collector{
		prevStats: make(map[int32]processCPUStats),
	}
}

// StartCollection begins the process collection loop. It ticks at the specified interval
// and runs until the context is cancelled.
func StartCollection(
	ctx context.Context,
	wg *sync.WaitGroup,
	exporter *exporter.Exporter,
	interval time.Duration,
) {
	// Signal completion on exit
	defer wg.Done()

	collector := NewCollector()
	collectAndExport := func() {
		now := time.Now().UnixMilli()
		processTable, err := collector.Collect()
		if err != nil {
			logger.Log.Error("failed to collect process table snapshot", "error", err)
			return
		}
		telemetryPayload := convertProcessTableToPayload(processTable, now)
		if telemetryPayload == nil {
			return
		}
		err = exporter.ExportTelemetry(telemetryPayload)
		if err != nil {
			logger.Log.Error("failed to export process table snapshot", "error", err)
		} else {
			logger.Log.Debug("Process snapshot collected and exported", "count", len(processTable))
		}
	}

	// Perform initial collection immediately
	collectAndExport()

	// Create ticker and ensure is stopped when function exits
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Infinite loop
	for {
		select {
		// Perform collection when the ticker fires
		case <-ticker.C:
			collectAndExport()
		// Exit loop when stop signal fires
		case <-ctx.Done():
			logger.Log.Info("Process collection loop received stop signal.")
			return
		}
	}
}

// Collect queries all active system processes and maps them to ProcessInfo structs
func (c *Collector) Collect() ([]ProcessInfo, error) {
	processes, err := process.Processes()
	if err != nil {
		return nil, err
	}

	totalMem := uint64(0)
	vm, err := mem.VirtualMemory()
	if err == nil {
		totalMem = vm.Total
	}

	numWorkers := max(1, min(len(processes), maxNumWorkers, runtime.NumCPU()))
	jobs := make(chan *process.Process, numWorkers*2)
	results := make(chan perProcessResult, numWorkers*2)

	var wg sync.WaitGroup
	for range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				results <- c.getProcessInfo(p, totalMem)
			}
		}()
	}

	go func() {
		for _, p := range processes {
			jobs <- p
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	nextStats := make(map[int32]processCPUStats)
	payload := make([]ProcessInfo, 0, len(processes))

	for res := range results {
		payload = append(payload, res.info)
		if res.stats != nil {
			nextStats[res.info.Pid] = *res.stats
		}
	}

	c.mu.Lock()
	c.prevStats = nextStats
	c.mu.Unlock()

	return payload, nil
}

func (c *Collector) getProcessInfo(p *process.Process, totalMem uint64) perProcessResult {
	pid := p.Pid
	ppid, _ := p.Ppid()
	name, _ := p.Name()
	user, _ := p.Username()
	threads, _ := p.NumThreads()

	var rss uint64
	memInfo, err := p.MemoryInfo()
	if err == nil && memInfo != nil {
		rss = memInfo.RSS
	}

	var memPercent float64
	if totalMem > 0 {
		memPercent = float64(rss) / float64(totalMem)
	}

	var cpuPercent float64
	var cpuStats *processCPUStats
	times, err := p.Times()
	now := time.Now().UnixMilli()
	if err == nil && times != nil {
		currCPUTime := times.User + times.System
		c.mu.RLock()
		prev, ok := c.prevStats[pid]
		c.mu.RUnlock()

		numCPU := float64(runtime.NumCPU())
		if ok {
			timeDelta := float64(now-prev.Timestamp) / 1000.0
			if timeDelta > 0 {
				cpuDelta := currCPUTime - prev.CPUTime
				cpuPercent = max(0.0, min(1.0, cpuDelta/(timeDelta*numCPU)))
			}
		}

		cpuStats = &processCPUStats{
			CPUTime:   currCPUTime,
			Timestamp: now,
		}
	}

	return perProcessResult{
		info: ProcessInfo{
			Pid:     pid,
			Ppid:    ppid,
			Name:    name,
			User:    user,
			CPU:     cpuPercent,
			Mem:     memPercent,
			RSS:     rss,
			Threads: threads,
		},
		stats: cpuStats,
	}
}

func convertProcessTableToPayload(table []ProcessInfo, timestamp int64) []exporter.TelemetryPayload {
	data, err := json.Marshal(table)
	if err != nil {
		logger.Log.Error("failed to marshal process snapshot", "error", err)
		return nil
	}
	return []exporter.TelemetryPayload{
		{
			Timestamp: strconv.FormatInt(timestamp, 10),
			Type:      "processes",
			Data:      data,
		},
	}
}
