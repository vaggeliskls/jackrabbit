package workers

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	"github.com/runner/server/internal/models"
	"github.com/runner/server/internal/store"
)

// Retention enforces log retention policies by deleting old logs
type Retention struct {
	store    store.Store
	logger   zerolog.Logger
	interval time.Duration
}

func NewRetention(store store.Store, logger zerolog.Logger, interval time.Duration) *Retention {
	return &Retention{
		store:    store,
		logger:   logger.With().Str("worker", "retention").Logger(),
		interval: interval,
	}
}

func (r *Retention) Start(ctx context.Context) {
	r.logger.Info().Dur("interval", r.interval).Msg("retention worker started")
	
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			r.logger.Info().Msg("retention worker stopped")
			return
		case <-ticker.C:
			r.run(ctx)
		}
	}
}

func (r *Retention) run(ctx context.Context) {
	// Get all active retention policies
	policies, err := r.store.GetLogRetentionPolicies(ctx)
	if err != nil {
		r.logger.Error().Err(err).Msg("failed to get retention policies")
		return
	}
	
	for _, policy := range policies {
		if err := r.applyPolicy(ctx, policy); err != nil {
			scopeID := "N/A"
			if policy.ScopeID != nil {
				scopeID = *policy.ScopeID
			}
			r.logger.Error().
				Err(err).
				Str("scope", policy.Scope).
				Str("scope_id", scopeID).
				Msg("failed to apply retention policy")
		}
	}
}

func (r *Retention) applyPolicy(ctx context.Context, policy models.LogRetentionPolicy) error {
	deleted := 0
	
	// Apply age-based retention
	if policy.MaxAgeDays != nil && *policy.MaxAgeDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -*policy.MaxAgeDays)
		scopeID := ""
		if policy.ScopeID != nil {
			scopeID = *policy.ScopeID
		}
		count, err := r.store.DeleteLogsByAge(ctx, policy.Scope, scopeID, cutoff)
		if err != nil {
			return err
		}
		deleted += count
	}
	
	// Apply size-based retention (delete oldest logs first)
	if policy.MaxSizeMB != nil && *policy.MaxSizeMB > 0 {
		scopeID := ""
		if policy.ScopeID != nil {
			scopeID = *policy.ScopeID
		}
		count, err := r.store.DeleteLogsBySize(ctx, policy.Scope, scopeID, int64(*policy.MaxSizeMB)*1024*1024)
		if err != nil {
			return err
		}
		deleted += count
	}
	
	if deleted > 0 {
		scopeID := "N/A"
		if policy.ScopeID != nil {
			scopeID = *policy.ScopeID
		}
		r.logger.Info().
			Int("deleted", deleted).
			Str("scope", policy.Scope).
			Str("scope_id", scopeID).
			Msg("applied retention policy")
	}
	
	return nil
}
