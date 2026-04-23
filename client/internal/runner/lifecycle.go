package runner

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/runner/client/internal/config"
	"github.com/runner/client/internal/shipper"
	"github.com/runner/sdk"
)

// Runner orchestrates all client components
type Runner struct {
	config         *config.Config
	sdk            *sdk.Client
	executor       *Executor
	sseListener    *SSEListener
	logShipper     *shipper.LogShipper
	metricShipper  *shipper.MetricShipper
	logger         zerolog.Logger
	
	// State tracking
	mu             sync.Mutex
	activeCommand  string
	logSeq         map[string]int64
}

func New(cfg *config.Config, logger zerolog.Logger) (*Runner, error) {
	// Create SDK client
	sdkClient := sdk.New(cfg.ServerURL, sdk.WithToken(cfg.APIToken))

	// Create components
	executor := NewExecutor(logger)
	sseListener := NewSSEListener(cfg.ServerURL, cfg.RunnerSlug, cfg.APIToken, logger)
	logShipper := shipper.NewLogShipper(sdkClient, cfg.RunnerSlug, cfg.LogBatchInterval, logger)
	metricShipper := shipper.NewMetricShipper(sdkClient, cfg.RunnerSlug, cfg.MetricSampleInterval, cfg.MetricBatchSize, logger)

	return &Runner{
		config:        cfg,
		sdk:           sdkClient,
		executor:      executor,
		sseListener:   sseListener,
		logShipper:    logShipper,
		metricShipper: metricShipper,
		logger:        logger.With().Str("component", "runner").Logger(),
		logSeq:        make(map[string]int64),
	}, nil
}

// Start begins the runner lifecycle
func (r *Runner) Start(ctx context.Context) error {
	// Register with server
	if err := r.register(ctx); err != nil {
		return fmt.Errorf("failed to register: %w", err)
	}

	// Start background components
	go r.logShipper.Start(ctx)
	go r.metricShipper.Start(ctx, r.getActiveCommandInfo)
	go r.heartbeat(ctx)

	// Start SSE listener and handle commands
	eventCh := r.sseListener.Start(ctx)
	
	r.logger.Info().Msg("runner started successfully")

	// Main event loop
	for {
		select {
		case <-ctx.Done():
			r.logger.Info().Msg("shutting down runner")
			r.deregister(context.Background())
			return ctx.Err()
		case event, ok := <-eventCh:
			if !ok {
				r.logger.Warn().Msg("SSE channel closed")
				return fmt.Errorf("SSE connection lost")
			}
			r.handleCommandEvent(ctx, event)
		}
	}
}

func (r *Runner) register(ctx context.Context) error {
	r.logger.Info().Msg("registering runner with server")

	runner, err := r.sdk.RegisterRunner(ctx, sdk.RunnerConfig{
		Slug:             r.config.RunnerSlug,
		Name:             r.config.RunnerName,
		Tags:             r.config.RunnerTags,
		ConcurrencyLimit: r.config.MaxConcurrency,
		GPUCapable:       r.config.GPUCapable,
	})

	if err != nil {
		return err
	}

	r.logger.Info().
		Str("slug", r.config.RunnerSlug).
		Interface("runner", runner).
		Msg("runner registered")

	return nil
}

func (r *Runner) deregister(ctx context.Context) {
	r.logger.Info().Msg("deregistering runner")
	
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := r.sdk.DeregisterRunner(ctx, r.config.RunnerSlug); err != nil {
		r.logger.Error().Err(err).Msg("failed to deregister")
	}
}

func (r *Runner) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(r.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := r.sdk.Heartbeat(ctx2, r.config.RunnerSlug)
			cancel()
			if err != nil {
				r.logger.Warn().Err(err).Msg("heartbeat failed")
			} else {
				r.logger.Debug().Msg("heartbeat")
			}
		}
	}
}

func (r *Runner) handleCommandEvent(ctx context.Context, event CommandEvent) {
	r.logger.Info().
		Str("command_id", event.CommandID).
		Msg("handling command dispatch")

	// Check concurrency limit
	if r.executor.ActiveCount() >= r.config.MaxConcurrency {
		r.logger.Warn().
			Str("command_id", event.CommandID).
			Int("active", r.executor.ActiveCount()).
			Msg("at max concurrency, cannot accept command")
		return
	}

	// Extract command from payload
	cmdStr, ok := event.Payload["cmd"].(string)
	if !ok {
		r.logger.Error().
			Str("command_id", event.CommandID).
			Msg("payload missing 'cmd' field")
		return
	}

	// Set active command
	r.mu.Lock()
	r.activeCommand = event.CommandID
	r.logSeq[event.CommandID] = 0
	r.mu.Unlock()

	// Execute command
	timeout := time.Duration(event.TimeoutSecs) * time.Second
	if timeout == 0 {
		timeout = 300 * time.Second // default 5 minutes
	}

	logCh, resultCh, err := r.executor.Execute(ctx, event.CommandID, cmdStr, timeout)
	if err != nil {
		r.logger.Error().
			Err(err).
			Str("command_id", event.CommandID).
			Msg("failed to execute command")
		return
	}

	// Stream logs
	go r.streamLogs(event.CommandID, logCh)

	// Wait for result
	go r.handleResult(event.CommandID, resultCh)
}

func (r *Runner) streamLogs(commandID string, logCh <-chan LogLine) {
	for logLine := range logCh {
		r.mu.Lock()
		seq := r.logSeq[commandID]
		r.logSeq[commandID]++
		r.mu.Unlock()

		r.logShipper.Add(commandID, logLine.Source, logLine.Line, seq, logLine.Time)
	}
}

func (r *Runner) handleResult(commandID string, resultCh <-chan Result) {
	result := <-resultCh

	r.mu.Lock()
	if r.activeCommand == commandID {
		r.activeCommand = ""
	}
	delete(r.logSeq, commandID)
	r.mu.Unlock()

	r.logger.Info().
		Str("command_id", commandID).
		Int("exit_code", result.ExitCode).
		Msg("command result ready")

	// Report result to server via SDK
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	if err := r.sdk.ReportResult(ctx, r.config.RunnerSlug, commandID, result.ExitCode, result.ErrorMessage); err != nil {
		r.logger.Error().Err(err).Str("command_id", commandID).Msg("failed to report result")
	} else {
		r.logger.Info().Str("command_id", commandID).Msg("result reported to server")
	}
}

func (r *Runner) getActiveCommandInfo() (string, int32) {
	r.mu.Lock()
	commandID := r.activeCommand
	r.mu.Unlock()
	if commandID == "" {
		return "", 0
	}
	pid, ok := r.executor.GetPID(commandID)
	if !ok {
		return commandID, 0
	}
	return commandID, pid
}

