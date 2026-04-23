package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/runner/server/internal/models"
)

// Store defines the data access interface for the runner system
type Store interface {
	// Runners
	UpsertRunner(ctx context.Context, r models.Runner) error
	GetRunner(ctx context.Context, slug string) (*models.Runner, error)
	ListRunners(ctx context.Context, tags []string, status string) ([]models.Runner, error)
	UpdateRunnerHeartbeat(ctx context.Context, slug string) error
	UpdateRunnerStatus(ctx context.Context, slug string, status string) error
	IncrementRunnerActiveCount(ctx context.Context, slug string, delta int) error
	MarkOrphanedRunners(ctx context.Context, cutoff time.Time) (int, error)

	// Commands
	InsertCommand(ctx context.Context, cmd models.Command) (*models.Command, error)
	ClaimNextCommand(ctx context.Context, runnerSlug string, runnerTags []string) (*models.Command, error)
	UpdateCommandStatus(ctx context.Context, id pgtype.UUID, status string, opts UpdateCommandOpts) error
	GetCommand(ctx context.Context, id pgtype.UUID) (*models.Command, error)
	SetCommandKillRequested(ctx context.Context, id pgtype.UUID) error
	FailExpiredCommands(ctx context.Context, now time.Time) (int, error)
	FailStuckCommands(ctx context.Context) (int, error)

	// Logs
	BatchInsertLogs(ctx context.Context, logs []models.Log) error
	GetLogs(ctx context.Context, commandID pgtype.UUID, page, pageSize int) ([]models.Log, error)
	DeleteLogsByAge(ctx context.Context, scope, scopeID string, cutoff time.Time) (int, error)
	DeleteLogsBySize(ctx context.Context, scope, scopeID string, maxBytes int64) (int, error)

	// Metrics
	BatchInsertMetrics(ctx context.Context, metrics []models.Metric) error
	GetMetrics(ctx context.Context, commandID pgtype.UUID, resolution string, page, pageSize int) ([]models.Metric, error)
	RollupMetrics(ctx context.Context, resolution string, start, end time.Time) (int, error)

	// Retention Policies
	GetRetentionPolicy(ctx context.Context, scope string, scopeID *string) (*models.LogRetentionPolicy, error)
	GetLogRetentionPolicies(ctx context.Context) ([]models.LogRetentionPolicy, error)

	// Notifications
	NotifyRunner(ctx context.Context, slug string, commandID pgtype.UUID) error

	// Lifecycle
	Close()
}

// UpdateCommandOpts contains optional fields for updating command status
type UpdateCommandOpts struct {
	ExitCode     *int
	ErrorMessage *string
	StartedAt    *time.Time
	FinishedAt   *time.Time
}
