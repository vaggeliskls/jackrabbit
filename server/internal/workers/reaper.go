package workers

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	"github.com/runner/server/internal/store"
)

// Reaper handles cleanup of:
// 1. Commands past deadline → failed
// 2. Commands stuck in 'running' with inactive runners → failed
// 3. Runners offline for too long → orphaned status
type Reaper struct {
	store    store.Store
	logger   zerolog.Logger
	interval time.Duration
	
	// Thresholds
	runnerOrphanAge time.Duration // How long runner can be offline before marked orphaned
}

func NewReaper(store store.Store, logger zerolog.Logger, interval time.Duration) *Reaper {
	return &Reaper{
		store:           store,
		logger:          logger.With().Str("worker", "reaper").Logger(),
		interval:        interval,
		runnerOrphanAge: 5 * time.Minute,
	}
}

func (r *Reaper) Start(ctx context.Context) {
	r.logger.Info().Dur("interval", r.interval).Msg("reaper started")
	
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	
	// Run immediately on start
	r.run(ctx)
	
	for {
		select {
		case <-ctx.Done():
			r.logger.Info().Msg("reaper stopped")
			return
		case <-ticker.C:
			r.run(ctx)
		}
	}
}

func (r *Reaper) run(ctx context.Context) {
	r.reapExpiredCommands(ctx)
	r.reapOrphanedRunners(ctx)
	r.reapStuckCommands(ctx)
}

// reapExpiredCommands marks commands past their deadline as failed
func (r *Reaper) reapExpiredCommands(ctx context.Context) {
	count, err := r.store.FailExpiredCommands(ctx, time.Now())
	if err != nil {
		r.logger.Error().Err(err).Msg("failed to reap expired commands")
		return
	}
	
	if count > 0 {
		r.logger.Info().Int("count", count).Msg("reaped expired commands")
	}
}

// reapOrphanedRunners marks runners that have been offline for too long as orphaned
func (r *Reaper) reapOrphanedRunners(ctx context.Context) {
	cutoff := time.Now().Add(-r.runnerOrphanAge)
	count, err := r.store.MarkOrphanedRunners(ctx, cutoff)
	if err != nil {
		r.logger.Error().Err(err).Msg("failed to mark orphaned runners")
		return
	}
	
	if count > 0 {
		r.logger.Warn().Int("count", count).Msg("marked runners as orphaned")
	}
}

// reapStuckCommands finds commands in 'running' state assigned to offline/orphaned runners
func (r *Reaper) reapStuckCommands(ctx context.Context) {
	count, err := r.store.FailStuckCommands(ctx)
	if err != nil {
		r.logger.Error().Err(err).Msg("failed to reap stuck commands")
		return
	}
	
	if count > 0 {
		r.logger.Warn().Int("count", count).Msg("reaped stuck commands")
	}
}
