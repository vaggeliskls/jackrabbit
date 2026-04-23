package models

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type Runner struct {
	Slug             string             `json:"slug" db:"slug"`
	Name             string             `json:"name" db:"name"`
	Tags             []string           `json:"tags" db:"tags"`
	Status           string             `json:"status" db:"status"`
	ConcurrencyLimit int                `json:"concurrency_limit" db:"concurrency_limit"`
	GPUCapable       bool               `json:"gpu_capable" db:"gpu_capable"`
	ActiveCount      int                `json:"active_count" db:"active_count"`
	LastSeen         *time.Time         `json:"last_seen,omitempty" db:"last_seen"`
	OrphanedAt       *time.Time         `json:"orphaned_at,omitempty" db:"orphaned_at"`
	CreatedAt        time.Time          `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at" db:"updated_at"`
}

type Command struct {
	ID              pgtype.UUID        `json:"id" db:"id"`
	TargetType      string             `json:"target_type" db:"target_type"`
	TargetValue     string             `json:"target_value" db:"target_value"`
	Payload         map[string]any     `json:"payload" db:"payload"`
	Status          string             `json:"status" db:"status"`
	AssignedRunner  *string            `json:"assigned_runner,omitempty" db:"assigned_runner"`
	RetryCount      int                `json:"retry_count" db:"retry_count"`
	MaxRetries      int                `json:"max_retries" db:"max_retries"`
	TimeoutSecs     int                `json:"timeout_secs" db:"timeout_secs"`
	ExitCode        *int               `json:"exit_code,omitempty" db:"exit_code"`
	ErrorMessage    *string            `json:"error_message,omitempty" db:"error_message"`
	KillRequestedAt *time.Time         `json:"kill_requested_at,omitempty" db:"kill_requested_at"`
	Deadline        *time.Time         `json:"deadline,omitempty" db:"deadline"`
	CreatedAt       time.Time          `json:"created_at" db:"created_at"`
	ClaimedAt       *time.Time         `json:"claimed_at,omitempty" db:"claimed_at"`
	StartedAt       *time.Time         `json:"started_at,omitempty" db:"started_at"`
	FinishedAt      *time.Time         `json:"finished_at,omitempty" db:"finished_at"`
}

type Log struct {
	ID         int64           `json:"id" db:"id"`
	CommandID  *pgtype.UUID    `json:"command_id,omitempty" db:"command_id"`
	RunnerSlug *string         `json:"runner_slug,omitempty" db:"runner_slug"`
	Source     string          `json:"source" db:"source"`
	Level      *string         `json:"level,omitempty" db:"level"`
	Line       string          `json:"line" db:"line"`
	Seq        int64           `json:"seq" db:"seq"`
	Ts         time.Time       `json:"ts" db:"ts"`
}

type Metric struct {
	ID          int64           `json:"id" db:"id"`
	CommandID   *pgtype.UUID    `json:"command_id,omitempty" db:"command_id"`
	RunnerSlug  *string         `json:"runner_slug,omitempty" db:"runner_slug"`
	CPUPercent  *float64        `json:"cpu_percent,omitempty" db:"cpu_percent"`
	MemMB       *float64        `json:"mem_mb,omitempty" db:"mem_mb"`
	GPUPercent  *float64        `json:"gpu_percent,omitempty" db:"gpu_percent"`
	GPUMemMB    *float64        `json:"gpu_mem_mb,omitempty" db:"gpu_mem_mb"`
	RolledUp    bool            `json:"rolled_up" db:"rolled_up"`
	SampleTs    time.Time       `json:"sample_ts" db:"sample_ts"`
}

type MetricRollup struct {
	ID          int64           `json:"id" db:"id"`
	CommandID   *pgtype.UUID    `json:"command_id,omitempty" db:"command_id"`
	RunnerSlug  *string         `json:"runner_slug,omitempty" db:"runner_slug"`
	Resolution  string          `json:"resolution" db:"resolution"`
	BucketTs    time.Time       `json:"bucket_ts" db:"bucket_ts"`
	AvgCPU      *float64        `json:"avg_cpu,omitempty" db:"avg_cpu"`
	AvgMem      *float64        `json:"avg_mem,omitempty" db:"avg_mem"`
	AvgGPU      *float64        `json:"avg_gpu,omitempty" db:"avg_gpu"`
	AvgGPUMem   *float64        `json:"avg_gpu_mem,omitempty" db:"avg_gpu_mem"`
}

type LogRetentionPolicy struct {
	ID         int        `json:"id" db:"id"`
	Scope      string     `json:"scope" db:"scope"`
	ScopeID    *string    `json:"scope_id,omitempty" db:"scope_id"`
	MaxAgeDays *int       `json:"max_age_days,omitempty" db:"max_age_days"`
	MaxSizeMB  *int       `json:"max_size_mb,omitempty" db:"max_size_mb"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at" db:"updated_at"`
}
