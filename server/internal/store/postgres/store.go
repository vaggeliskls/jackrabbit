package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/runner/server/internal/models"
	"github.com/runner/server/internal/store"
)

var (
	ErrNotFound      = errors.New("resource not found")
	ErrAlreadyExists = errors.New("resource already exists")
	ErrInvalidInput  = errors.New("invalid input")
)

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string, maxConns int) (*Store, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}

	config.MaxConns = int32(maxConns)

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) UpsertRunner(ctx context.Context, r models.Runner) error {
	query := `
		INSERT INTO runners (slug, name, tags, status, concurrency_limit, gpu_capable, active_count, last_seen, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (slug) DO UPDATE SET
			name = EXCLUDED.name,
			tags = EXCLUDED.tags,
			status = EXCLUDED.status,
			concurrency_limit = EXCLUDED.concurrency_limit,
			gpu_capable = EXCLUDED.gpu_capable,
			last_seen = EXCLUDED.last_seen,
			updated_at = EXCLUDED.updated_at
	`

	now := time.Now()
	_, err := s.pool.Exec(ctx, query,
		r.Slug, r.Name, r.Tags, r.Status, r.ConcurrencyLimit, r.GPUCapable,
		r.ActiveCount, now, now,
	)
	if err != nil {
		return fmt.Errorf("upsert runner: %w", err)
	}

	return nil
}

func (s *Store) GetRunner(ctx context.Context, slug string) (*models.Runner, error) {
	query := `
		SELECT slug, name, tags, status, concurrency_limit, gpu_capable, active_count,
		       last_seen, orphaned_at, created_at, updated_at
		FROM runners
		WHERE slug = $1
	`

	var r models.Runner
	err := s.pool.QueryRow(ctx, query, slug).Scan(
		&r.Slug, &r.Name, &r.Tags, &r.Status, &r.ConcurrencyLimit, &r.GPUCapable,
		&r.ActiveCount, &r.LastSeen, &r.OrphanedAt, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get runner: %w", err)
	}

	return &r, nil
}

func (s *Store) ListRunners(ctx context.Context, tags []string, status string) ([]models.Runner, error) {
	query := `
		SELECT slug, name, tags, status, concurrency_limit, gpu_capable, active_count,
		       last_seen, orphaned_at, created_at, updated_at
		FROM runners
		WHERE ($1::text IS NULL OR status = $1)
		  AND ($2::text[] IS NULL OR tags && $2)
		ORDER BY created_at DESC
	`

	var statusPtr *string
	if status != "" {
		statusPtr = &status
	}

	var tagsParam []string
	if len(tags) > 0 {
		tagsParam = tags
	}

	rows, err := s.pool.Query(ctx, query, statusPtr, tagsParam)
	if err != nil {
		return nil, fmt.Errorf("list runners: %w", err)
	}
	defer rows.Close()

	var runners []models.Runner
	for rows.Next() {
		var r models.Runner
		err := rows.Scan(
			&r.Slug, &r.Name, &r.Tags, &r.Status, &r.ConcurrencyLimit, &r.GPUCapable,
			&r.ActiveCount, &r.LastSeen, &r.OrphanedAt, &r.CreatedAt, &r.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan runner: %w", err)
		}
		runners = append(runners, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runners: %w", err)
	}

	return runners, nil
}

func (s *Store) UpdateRunnerHeartbeat(ctx context.Context, slug string) error {
	query := `
		UPDATE runners
		SET last_seen = $1, status = 'online', updated_at = $1
		WHERE slug = $2
	`

	now := time.Now()
	result, err := s.pool.Exec(ctx, query, now, slug)
	if err != nil {
		return fmt.Errorf("update heartbeat: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (s *Store) UpdateRunnerStatus(ctx context.Context, slug string, status string) error {
	query := `
		UPDATE runners
		SET status = $1, updated_at = $2
		WHERE slug = $3
	`

	now := time.Now()
	result, err := s.pool.Exec(ctx, query, status, now, slug)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (s *Store) InsertCommand(ctx context.Context, cmd models.Command) (*models.Command, error) {
	payloadJSON, err := json.Marshal(cmd.Payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	query := `
		INSERT INTO commands (target_type, target_value, payload, status, max_retries, timeout_secs, deadline)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`

	var result models.Command
	err = s.pool.QueryRow(ctx, query,
		cmd.TargetType, cmd.TargetValue, payloadJSON, cmd.Status,
		cmd.MaxRetries, cmd.TimeoutSecs, cmd.Deadline,
	).Scan(&result.ID, &result.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert command: %w", err)
	}

	result.TargetType = cmd.TargetType
	result.TargetValue = cmd.TargetValue
	result.Payload = cmd.Payload
	result.Status = cmd.Status
	result.MaxRetries = cmd.MaxRetries
	result.TimeoutSecs = cmd.TimeoutSecs
	result.Deadline = cmd.Deadline

	return &result, nil
}

func (s *Store) ClaimNextCommand(ctx context.Context, runnerSlug string, runnerTags []string) (*models.Command, error) {
	query := `
		WITH claimed AS (
			SELECT id FROM commands
			WHERE status = 'queued'
			  AND (
				(target_type = 'slug' AND target_value = $1)
				OR
				(target_type = 'tag' AND target_value = ANY($2))
			  )
			  AND (deadline IS NULL OR deadline > now())
			ORDER BY created_at
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE commands SET
			status = 'claimed',
			assigned_runner = $1,
			claimed_at = now()
		FROM claimed
		WHERE commands.id = claimed.id
		RETURNING commands.id, commands.target_type, commands.target_value, commands.payload,
		          commands.status, commands.assigned_runner, commands.retry_count,
		          commands.max_retries, commands.timeout_secs, commands.exit_code,
		          commands.error_message, commands.kill_requested_at, commands.deadline,
		          commands.created_at, commands.claimed_at, commands.started_at, commands.finished_at
	`

	var cmd models.Command
	var payloadJSON []byte
	err := s.pool.QueryRow(ctx, query, runnerSlug, runnerTags).Scan(
		&cmd.ID, &cmd.TargetType, &cmd.TargetValue, &payloadJSON,
		&cmd.Status, &cmd.AssignedRunner, &cmd.RetryCount,
		&cmd.MaxRetries, &cmd.TimeoutSecs, &cmd.ExitCode,
		&cmd.ErrorMessage, &cmd.KillRequestedAt, &cmd.Deadline,
		&cmd.CreatedAt, &cmd.ClaimedAt, &cmd.StartedAt, &cmd.FinishedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("claim command: %w", err)
	}

	if err := json.Unmarshal(payloadJSON, &cmd.Payload); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}

	return &cmd, nil
}

func (s *Store) UpdateCommandStatus(ctx context.Context, id pgtype.UUID, status string, opts store.UpdateCommandOpts) error {
	query := `
		UPDATE commands
		SET status = $1,
		    exit_code = COALESCE($2, exit_code),
		    error_message = COALESCE($3, error_message),
		    started_at = COALESCE($4, started_at),
		    finished_at = COALESCE($5, finished_at)
		WHERE id = $6
	`

	result, err := s.pool.Exec(ctx, query,
		status, opts.ExitCode, opts.ErrorMessage,
		opts.StartedAt, opts.FinishedAt, id,
	)
	if err != nil {
		return fmt.Errorf("update command status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (s *Store) GetCommand(ctx context.Context, id pgtype.UUID) (*models.Command, error) {
	query := `
		SELECT id, target_type, target_value, payload, status, assigned_runner,
		       retry_count, max_retries, timeout_secs, exit_code, error_message,
		       kill_requested_at, deadline, created_at, claimed_at, started_at, finished_at
		FROM commands
		WHERE id = $1
	`

	var cmd models.Command
	var payloadJSON []byte
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&cmd.ID, &cmd.TargetType, &cmd.TargetValue, &payloadJSON,
		&cmd.Status, &cmd.AssignedRunner, &cmd.RetryCount,
		&cmd.MaxRetries, &cmd.TimeoutSecs, &cmd.ExitCode,
		&cmd.ErrorMessage, &cmd.KillRequestedAt, &cmd.Deadline,
		&cmd.CreatedAt, &cmd.ClaimedAt, &cmd.StartedAt, &cmd.FinishedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get command: %w", err)
	}

	if err := json.Unmarshal(payloadJSON, &cmd.Payload); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}

	return &cmd, nil
}

func (s *Store) SetCommandKillRequested(ctx context.Context, id pgtype.UUID) error {
	query := `
		UPDATE commands
		SET kill_requested_at = now()
		WHERE id = $1 AND status IN ('claimed', 'running')
	`

	result, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("set kill requested: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (s *Store) BatchInsertLogs(ctx context.Context, logs []models.Log) error {
	if len(logs) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO logs (command_id, runner_slug, source, level, line, seq, ts)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	for _, log := range logs {
		batch.Queue(query,
			log.CommandID, log.RunnerSlug, log.Source,
			log.Level, log.Line, log.Seq, log.Ts,
		)
	}

	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()

	for i := 0; i < len(logs); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert log %d: %w", i, err)
		}
	}

	return nil
}

func (s *Store) GetLogs(ctx context.Context, commandID pgtype.UUID, page, pageSize int) ([]models.Log, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 1000 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize

	query := `
		SELECT id, command_id, runner_slug, source, level, line, seq, ts
		FROM logs
		WHERE command_id = $1
		ORDER BY seq ASC
		LIMIT $2 OFFSET $3
	`

	rows, err := s.pool.Query(ctx, query, commandID, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("get logs: %w", err)
	}
	defer rows.Close()

	logs := make([]models.Log, 0)
	for rows.Next() {
		var log models.Log
		err := rows.Scan(
			&log.ID, &log.CommandID, &log.RunnerSlug, &log.Source,
			&log.Level, &log.Line, &log.Seq, &log.Ts,
		)
		if err != nil {
			return nil, fmt.Errorf("scan log: %w", err)
		}
		logs = append(logs, log)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate logs: %w", err)
	}

	return logs, nil
}

func (s *Store) BatchInsertMetrics(ctx context.Context, metrics []models.Metric) error {
	if len(metrics) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO metrics (command_id, runner_slug, cpu_percent, mem_mb, gpu_percent, gpu_mem_mb, rolled_up, sample_ts)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	for _, m := range metrics {
		batch.Queue(query,
			m.CommandID, m.RunnerSlug, m.CPUPercent, m.MemMB,
			m.GPUPercent, m.GPUMemMB, m.RolledUp, m.SampleTs,
		)
	}

	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()

	for i := 0; i < len(metrics); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert metric %d: %w", i, err)
		}
	}

	return nil
}

func (s *Store) GetMetrics(ctx context.Context, commandID pgtype.UUID, resolution string, page, pageSize int) ([]models.Metric, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 1000 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize

	if resolution != "" && resolution != "raw" {
		return s.getMetricRollups(ctx, commandID, resolution, page, pageSize)
	}

	query := `
		SELECT id, command_id, runner_slug, cpu_percent, mem_mb, gpu_percent, gpu_mem_mb, rolled_up, sample_ts
		FROM metrics
		WHERE command_id = $1
		ORDER BY sample_ts ASC
		LIMIT $2 OFFSET $3
	`

	rows, err := s.pool.Query(ctx, query, commandID, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("get metrics: %w", err)
	}
	defer rows.Close()

	metrics := make([]models.Metric, 0)
	for rows.Next() {
		var m models.Metric
		err := rows.Scan(
			&m.ID, &m.CommandID, &m.RunnerSlug, &m.CPUPercent, &m.MemMB,
			&m.GPUPercent, &m.GPUMemMB, &m.RolledUp, &m.SampleTs,
		)
		if err != nil {
			return nil, fmt.Errorf("scan metric: %w", err)
		}
		metrics = append(metrics, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate metrics: %w", err)
	}

	return metrics, nil
}

func (s *Store) getMetricRollups(ctx context.Context, commandID pgtype.UUID, resolution string, page, pageSize int) ([]models.Metric, error) {
	offset := (page - 1) * pageSize

	query := `
		SELECT id, command_id, runner_slug, avg_cpu, avg_mem, avg_gpu, avg_gpu_mem, bucket_ts
		FROM metric_rollups
		WHERE command_id = $1 AND resolution = $2
		ORDER BY bucket_ts ASC
		LIMIT $3 OFFSET $4
	`

	rows, err := s.pool.Query(ctx, query, commandID, resolution, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("get metric rollups: %w", err)
	}
	defer rows.Close()

	metrics := make([]models.Metric, 0)
	for rows.Next() {
		var m models.Metric
		err := rows.Scan(
			&m.ID, &m.CommandID, &m.RunnerSlug, &m.CPUPercent, &m.MemMB,
			&m.GPUPercent, &m.GPUMemMB, &m.SampleTs,
		)
		if err != nil {
			return nil, fmt.Errorf("scan rollup: %w", err)
		}
		metrics = append(metrics, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rollups: %w", err)
	}

	return metrics, nil
}

func (s *Store) GetRetentionPolicy(ctx context.Context, scope string, scopeID *string) (*models.LogRetentionPolicy, error) {
	query := `
		SELECT id, scope, scope_id, max_age_days, max_size_mb, created_at, updated_at
		FROM log_retention_policies
		WHERE scope = $1 AND ($2::text IS NULL OR scope_id = $2)
		ORDER BY created_at DESC
		LIMIT 1
	`

	var p models.LogRetentionPolicy
	err := s.pool.QueryRow(ctx, query, scope, scopeID).Scan(
		&p.ID, &p.Scope, &p.ScopeID, &p.MaxAgeDays, &p.MaxSizeMB,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get retention policy: %w", err)
	}

	return &p, nil
}

func (s *Store) NotifyRunner(ctx context.Context, slug string, commandID pgtype.UUID) error {
	channel := "runner_notifications"
	
	var cmdIDStr string
	cmdIDBytes, err := commandID.MarshalJSON()
	if err != nil {
		return fmt.Errorf("marshal command id: %w", err)
	}
	cmdIDStr = string(cmdIDBytes)
	cmdIDStr = cmdIDStr[1 : len(cmdIDStr)-1]

	payload := fmt.Sprintf("%s:%s", slug, cmdIDStr)
	query := fmt.Sprintf("SELECT pg_notify('%s', '%s')", channel, payload)
	
	_, err = s.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("notify runner: %w", err)
	}

	return nil
}

func (s *Store) IncrementRunnerActiveCount(ctx context.Context, slug string, delta int) error {
	query := `
		UPDATE runners
		SET active_count = active_count + $1, updated_at = now()
		WHERE slug = $2
	`

	result, err := s.pool.Exec(ctx, query, delta, slug)
	if err != nil {
		return fmt.Errorf("increment active count: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// FailExpiredCommands marks commands past their deadline as failed
func (s *Store) FailExpiredCommands(ctx context.Context, now time.Time) (int, error) {
	query := `
		UPDATE commands
		SET status = 'failed',
		    error_message = 'deadline exceeded',
		    finished_at = now()
		WHERE status IN ('pending', 'running')
		  AND deadline IS NOT NULL
		  AND deadline < $1
	`

	result, err := s.pool.Exec(ctx, query, now)
	if err != nil {
		return 0, fmt.Errorf("fail expired commands: %w", err)
	}

	return int(result.RowsAffected()), nil
}

// MarkOrphanedRunners marks runners that have been offline for too long
func (s *Store) MarkOrphanedRunners(ctx context.Context, cutoff time.Time) (int, error) {
	query := `
		UPDATE runners
		SET status = 'orphaned',
		    orphaned_at = now(),
		    updated_at = now()
		WHERE status = 'offline'
		  AND last_seen < $1
		  AND orphaned_at IS NULL
	`

	result, err := s.pool.Exec(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("mark orphaned runners: %w", err)
	}

	return int(result.RowsAffected()), nil
}

// FailStuckCommands marks commands in 'running' state with offline/orphaned runners as failed
func (s *Store) FailStuckCommands(ctx context.Context) (int, error) {
	query := `
		UPDATE commands
		SET status = 'failed',
		    error_message = 'runner became unavailable',
		    finished_at = now()
		WHERE status = 'running'
		  AND assigned_runner IN (
		    SELECT slug FROM runners WHERE status IN ('offline', 'orphaned')
		  )
	`

	result, err := s.pool.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("fail stuck commands: %w", err)
	}

	return int(result.RowsAffected()), nil
}

// GetLogRetentionPolicies retrieves all active retention policies
func (s *Store) GetLogRetentionPolicies(ctx context.Context) ([]models.LogRetentionPolicy, error) {
	query := `
		SELECT scope, scope_id, max_age_days, max_size_mb, created_at
		FROM log_retention_policies
		ORDER BY scope, scope_id
	`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("get retention policies: %w", err)
	}
	defer rows.Close()

	var policies []models.LogRetentionPolicy
	for rows.Next() {
		var p models.LogRetentionPolicy
		err := rows.Scan(&p.Scope, &p.ScopeID, &p.MaxAgeDays, &p.MaxSizeMB, &p.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan retention policy: %w", err)
		}
		policies = append(policies, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate retention policies: %w", err)
	}

	return policies, nil
}

// DeleteLogsByAge deletes logs older than the given cutoff time for a specific scope
func (s *Store) DeleteLogsByAge(ctx context.Context, scope, scopeID string, cutoff time.Time) (int, error) {
	var query string
	var args []interface{}

	if scope == "global" {
		query = `DELETE FROM logs WHERE ts < $1`
		args = []interface{}{cutoff}
	} else if scope == "runner" {
		query = `DELETE FROM logs WHERE runner_slug = $1 AND ts < $2`
		args = []interface{}{scopeID, cutoff}
	} else if scope == "command" {
		query = `DELETE FROM logs WHERE command_id = $1 AND ts < $2`
		var cmdUUID pgtype.UUID
		if err := cmdUUID.Scan(scopeID); err != nil {
			return 0, fmt.Errorf("parse command UUID: %w", err)
		}
		args = []interface{}{cmdUUID, cutoff}
	} else {
		return 0, fmt.Errorf("unsupported scope: %s", scope)
	}

	result, err := s.pool.Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("delete logs by age: %w", err)
	}

	return int(result.RowsAffected()), nil
}

// DeleteLogsBySize deletes oldest logs to stay within size limit for a specific scope
func (s *Store) DeleteLogsBySize(ctx context.Context, scope, scopeID string, maxBytes int64) (int, error) {
	// This is a simplified implementation that estimates log size
	// In production, you might want to track actual sizes in a separate table
	var query string
	var args []interface{}

	if scope == "global" {
		query = `
			DELETE FROM logs
			WHERE id IN (
				SELECT id FROM logs
				ORDER BY ts ASC
				LIMIT (
					SELECT GREATEST(0, COUNT(*) - $1)
					FROM logs
				)
			)
		`
		// Rough estimate: 100 bytes per log entry
		maxLogs := maxBytes / 100
		args = []interface{}{maxLogs}
	} else if scope == "runner" {
		query = `
			DELETE FROM logs
			WHERE id IN (
				SELECT id FROM logs
				WHERE runner_slug = $1
				ORDER BY ts ASC
				LIMIT (
					SELECT GREATEST(0, COUNT(*) - $2)
					FROM logs
					WHERE runner_slug = $1
				)
			)
		`
		maxLogs := maxBytes / 100
		args = []interface{}{scopeID, maxLogs}
	} else if scope == "command" {
		query = `
			DELETE FROM logs
			WHERE id IN (
				SELECT id FROM logs
				WHERE command_id = $1
				ORDER BY ts ASC
				LIMIT (
					SELECT GREATEST(0, COUNT(*) - $2)
					FROM logs
					WHERE command_id = $1
				)
			)
		`
		var cmdUUID pgtype.UUID
		if err := cmdUUID.Scan(scopeID); err != nil {
			return 0, fmt.Errorf("parse command UUID: %w", err)
		}
		maxLogs := maxBytes / 100
		args = []interface{}{cmdUUID, maxLogs}
	} else {
		return 0, fmt.Errorf("unsupported scope: %s", scope)
	}

	result, err := s.pool.Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("delete logs by size: %w", err)
	}

	return int(result.RowsAffected()), nil
}

// RollupMetrics aggregates raw metrics into time buckets
func (s *Store) RollupMetrics(ctx context.Context, resolution string, start, end time.Time) (int, error) {
	var bucketInterval string
	switch resolution {
	case "1m":
		bucketInterval = "1 minute"
	case "5m":
		bucketInterval = "5 minutes"
	default:
		return 0, fmt.Errorf("unsupported resolution: %s", resolution)
	}

	query := fmt.Sprintf(`
		INSERT INTO metric_rollups (
			command_id, runner_slug, resolution, bucket_ts,
			avg_cpu, avg_mem, avg_gpu, avg_gpu_mem
		)
		SELECT
			command_id,
			runner_slug,
			$1 as resolution,
			date_trunc('%s', sample_ts) as bucket_ts,
			AVG(cpu_percent) as avg_cpu,
			AVG(mem_mb) as avg_mem,
			AVG(gpu_percent) as avg_gpu,
			AVG(gpu_mem_mb) as avg_gpu_mem
		FROM metrics
		WHERE sample_ts >= $2
		  AND sample_ts < $3
		  AND rolled_up = false
		GROUP BY command_id, runner_slug, bucket_ts
		ON CONFLICT (command_id, runner_slug, resolution, bucket_ts) DO UPDATE SET
			avg_cpu = EXCLUDED.avg_cpu,
			avg_mem = EXCLUDED.avg_mem,
			avg_gpu = EXCLUDED.avg_gpu,
			avg_gpu_mem = EXCLUDED.avg_gpu_mem
	`, bucketInterval)

	_, err := s.pool.Exec(ctx, query, resolution, start, end)
	if err != nil {
		return 0, fmt.Errorf("rollup metrics: %w", err)
	}

	// Mark raw metrics as rolled up
	markQuery := `
		UPDATE metrics
		SET rolled_up = true
		WHERE sample_ts >= $1
		  AND sample_ts < $2
		  AND rolled_up = false
	`

	result, err := s.pool.Exec(ctx, markQuery, start, end)
	if err != nil {
		return 0, fmt.Errorf("mark metrics as rolled up: %w", err)
	}

	return int(result.RowsAffected()), nil
}
