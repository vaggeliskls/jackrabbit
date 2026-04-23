package workers

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	"github.com/runner/server/internal/store"
)

// Rollup aggregates raw metrics into time-bucketed rollups (1m, 5m)
type Rollup struct {
	store    store.Store
	logger   zerolog.Logger
	interval time.Duration
}

func NewRollup(store store.Store, logger zerolog.Logger, interval time.Duration) *Rollup {
	return &Rollup{
		store:    store,
		logger:   logger.With().Str("worker", "rollup").Logger(),
		interval: interval,
	}
}

func (r *Rollup) Start(ctx context.Context) {
	r.logger.Info().Dur("interval", r.interval).Msg("rollup worker started")
	
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			r.logger.Info().Msg("rollup worker stopped")
			return
		case <-ticker.C:
			r.run(ctx)
		}
	}
}

func (r *Rollup) run(ctx context.Context) {
	now := time.Now()
	
	// Rollup 1-minute buckets for data from 2-3 minutes ago
	// (allows stragglers to arrive)
	r.rollup1m(ctx, now.Add(-3*time.Minute), now.Add(-2*time.Minute))
	
	// Rollup 5-minute buckets for data from 6-11 minutes ago
	r.rollup5m(ctx, now.Add(-11*time.Minute), now.Add(-6*time.Minute))
}

func (r *Rollup) rollup1m(ctx context.Context, start, end time.Time) {
	count, err := r.store.RollupMetrics(ctx, "1m", start, end)
	if err != nil {
		r.logger.Error().Err(err).Msg("failed to rollup 1m metrics")
		return
	}
	
	if count > 0 {
		r.logger.Debug().
			Int("rollups", count).
			Time("start", start).
			Time("end", end).
			Msg("created 1m rollups")
	}
}

func (r *Rollup) rollup5m(ctx context.Context, start, end time.Time) {
	count, err := r.store.RollupMetrics(ctx, "5m", start, end)
	if err != nil {
		r.logger.Error().Err(err).Msg("failed to rollup 5m metrics")
		return
	}
	
	if count > 0 {
		r.logger.Debug().
			Int("rollups", count).
			Time("start", start).
			Time("end", end).
			Msg("created 5m rollups")
	}
}
