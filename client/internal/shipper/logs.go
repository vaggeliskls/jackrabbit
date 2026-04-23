package shipper

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/runner/sdk"
)

// LogShipper batches and ships logs to the server
type LogShipper struct {
	client         *sdk.Client
	runnerSlug     string
	batchInterval  time.Duration
	logger         zerolog.Logger
	mu             sync.Mutex
	buffer         []LogEntry
	maxBatchSize   int
}

type LogEntry struct {
	CommandID string
	Source    string
	Level     string
	Line      string
	Seq       int64
	Timestamp time.Time
}

func NewLogShipper(client *sdk.Client, runnerSlug string, batchInterval time.Duration, logger zerolog.Logger) *LogShipper {
	return &LogShipper{
		client:        client,
		runnerSlug:    runnerSlug,
		batchInterval: batchInterval,
		logger:        logger.With().Str("component", "log-shipper").Logger(),
		buffer:        make([]LogEntry, 0, 100),
		maxBatchSize:  100,
	}
}

// Add adds a log entry to the batch
func (ls *LogShipper) Add(commandID, source, line string, seq int64, timestamp time.Time) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	level := "info"
	if source == "stderr" {
		level = "error"
	}

	ls.buffer = append(ls.buffer, LogEntry{
		CommandID: commandID,
		Source:    source,
		Level:     level,
		Line:      line,
		Seq:       seq,
		Timestamp: timestamp,
	})

	// Flush if buffer is full
	if len(ls.buffer) >= ls.maxBatchSize {
		go ls.flush()
	}
}

// Start begins the batch shipping loop
func (ls *LogShipper) Start(ctx context.Context) {
	ticker := time.NewTicker(ls.batchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Final flush before shutdown
			ls.flush()
			return
		case <-ticker.C:
			ls.flush()
		}
	}
}

func (ls *LogShipper) flush() {
	ls.mu.Lock()
	if len(ls.buffer) == 0 {
		ls.mu.Unlock()
		return
	}

	// Copy buffer for shipping
	batch := make([]LogEntry, len(ls.buffer))
	copy(batch, ls.buffer)
	ls.buffer = ls.buffer[:0]
	ls.mu.Unlock()

	if err := ls.ship(batch); err != nil {
		ls.logger.Error().Err(err).Int("count", len(batch)).Msg("failed to ship logs")
	} else {
		ls.logger.Debug().Int("count", len(batch)).Msg("shipped logs")
	}
}

func (ls *LogShipper) ship(batch []LogEntry) error {
	if len(batch) == 0 {
		return nil
	}

	logs := make([]map[string]interface{}, len(batch))
	for i, entry := range batch {
		logs[i] = map[string]interface{}{
			"command_id": entry.CommandID,
			"source":     entry.Source,
			"level":      entry.Level,
			"line":       entry.Line,
			"seq":        entry.Seq,
			"ts":         entry.Timestamp.Format(time.RFC3339Nano),
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return ls.client.BatchInsertLogs(ctx, ls.runnerSlug, logs)
}
