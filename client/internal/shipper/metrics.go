package shipper

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/runner/sdk"
	"github.com/shirou/gopsutil/v3/process"
)

type MetricShipper struct {
	client         *sdk.Client
	runnerSlug     string
	sampleInterval time.Duration
	batchSize      int
	logger         zerolog.Logger
	mu             sync.Mutex
	buffer         []MetricEntry
}

type MetricEntry struct {
	CommandID  string
	CPUPercent float64
	MemMB      float64
	GPUPercent float64
	SampleTime time.Time
}

func NewMetricShipper(client *sdk.Client, runnerSlug string, sampleInterval time.Duration, batchSize int, logger zerolog.Logger) *MetricShipper {
	return &MetricShipper{
		client:         client,
		runnerSlug:     runnerSlug,
		sampleInterval: sampleInterval,
		batchSize:      batchSize,
		logger:         logger.With().Str("component", "metric-shipper").Logger(),
		buffer:         make([]MetricEntry, 0, batchSize),
	}
}

// Start begins the metric collection and shipping loop.
// getActiveInfo returns the current command ID and its OS PID (0 if not yet started).
func (ms *MetricShipper) Start(ctx context.Context, getActiveInfo func() (string, int32)) {
	ticker := time.NewTicker(ms.sampleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			ms.flush()
			return
		case <-ticker.C:
			commandID, pid := getActiveInfo()
			if commandID != "" && pid > 0 {
				ms.collect(commandID, pid)
			}
			ms.flush()
		}
	}
}

func (ms *MetricShipper) collect(commandID string, pid int32) {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return
	}

	cpuPercent, err := proc.CPUPercent()
	if err != nil {
		cpuPercent = 0
	}

	var memMB float64
	if memInfo, err := proc.MemoryInfo(); err == nil {
		memMB = float64(memInfo.RSS) / (1024 * 1024)
	}

	ms.mu.Lock()
	ms.buffer = append(ms.buffer, MetricEntry{
		CommandID:  commandID,
		CPUPercent: cpuPercent,
		MemMB:      memMB,
		GPUPercent: 0,
		SampleTime: time.Now(),
	})
	ms.mu.Unlock()
}

func (ms *MetricShipper) flush() {
	ms.mu.Lock()
	if len(ms.buffer) == 0 {
		ms.mu.Unlock()
		return
	}
	batch := make([]MetricEntry, len(ms.buffer))
	copy(batch, ms.buffer)
	ms.buffer = ms.buffer[:0]
	ms.mu.Unlock()

	if err := ms.ship(batch); err != nil {
		ms.logger.Error().Err(err).Int("count", len(batch)).Msg("failed to ship metrics")
	} else {
		ms.logger.Debug().Int("count", len(batch)).Msg("shipped metrics")
	}
}

func (ms *MetricShipper) ship(batch []MetricEntry) error {
	if len(batch) == 0 {
		return nil
	}

	metrics := make([]map[string]interface{}, len(batch))
	for i, entry := range batch {
		metrics[i] = map[string]interface{}{
			"command_id":  entry.CommandID,
			"cpu_percent": entry.CPUPercent,
			"mem_mb":      entry.MemMB,
			"gpu_percent": entry.GPUPercent,
			"sample_ts":   entry.SampleTime.Format(time.RFC3339Nano),
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return ms.client.BatchInsertMetrics(ctx, ms.runnerSlug, metrics)
}
