package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/runner/server/internal/api/middleware"
	"github.com/runner/server/internal/models"
	"github.com/runner/server/internal/store"
	"github.com/runner/server/internal/store/postgres"
)

type RunnerHandlers struct {
	store *postgres.Store
}

func NewRunnerHandlers(store *postgres.Store) *RunnerHandlers {
	return &RunnerHandlers{store: store}
}

type RegisterRequest struct {
	Slug             string   `json:"slug"`
	Name             string   `json:"name"`
	Tags             []string `json:"tags"`
	ConcurrencyLimit int      `json:"concurrency_limit"`
	GPUCapable       bool     `json:"gpu_capable"`
}

func (h *RunnerHandlers) Register(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := middleware.GetLogger(ctx)

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Error().Err(err).Msg("failed to decode request")
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Slug == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "slug and name are required")
		return
	}

	if req.ConcurrencyLimit == 0 {
		req.ConcurrencyLimit = 4
	}

	runner := models.Runner{
		Slug:             req.Slug,
		Name:             req.Name,
		Tags:             req.Tags,
		Status:           "online",
		ConcurrencyLimit: req.ConcurrencyLimit,
		GPUCapable:       req.GPUCapable,
		ActiveCount:      0,
	}

	if err := h.store.UpsertRunner(ctx, runner); err != nil {
		logger.Error().Err(err).Msg("failed to upsert runner")
		writeError(w, http.StatusInternalServerError, "failed to register runner")
		return
	}

	registered, err := h.store.GetRunner(ctx, req.Slug)
	if err != nil {
		logger.Error().Err(err).Msg("failed to get runner")
		writeError(w, http.StatusInternalServerError, "failed to retrieve runner")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(registered)

	logger.Info().Str("slug", req.Slug).Msg("runner registered")
}

func (h *RunnerHandlers) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := middleware.GetLogger(ctx)
	slug := chi.URLParam(r, "slug")

	runner, err := h.store.GetRunner(ctx, slug)
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			writeError(w, http.StatusNotFound, "runner not found")
			return
		}
		logger.Error().Err(err).Msg("failed to get runner")
		writeError(w, http.StatusInternalServerError, "failed to retrieve runner")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runner)
}

func (h *RunnerHandlers) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := middleware.GetLogger(ctx)

	status := r.URL.Query().Get("status")
	tags := r.URL.Query()["tags"]

	runners, err := h.store.ListRunners(ctx, tags, status)
	if err != nil {
		logger.Error().Err(err).Msg("failed to list runners")
		writeError(w, http.StatusInternalServerError, "failed to list runners")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runners)
}

func (h *RunnerHandlers) Deregister(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := middleware.GetLogger(ctx)
	slug := chi.URLParam(r, "slug")

	if err := h.store.UpdateRunnerStatus(ctx, slug, "offline"); err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			writeError(w, http.StatusNotFound, "runner not found")
			return
		}
		logger.Error().Err(err).Msg("failed to update runner status")
		writeError(w, http.StatusInternalServerError, "failed to deregister runner")
		return
	}

	w.WriteHeader(http.StatusNoContent)
	logger.Info().Str("slug", slug).Msg("runner deregistered")
}

func (h *RunnerHandlers) Heartbeat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := middleware.GetLogger(ctx)
	slug := chi.URLParam(r, "slug")

	if err := h.store.UpdateRunnerHeartbeat(ctx, slug); err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			writeError(w, http.StatusNotFound, "runner not found")
			return
		}
		logger.Error().Err(err).Msg("failed to update heartbeat")
		writeError(w, http.StatusInternalServerError, "failed to update heartbeat")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type BatchLogsRequest struct {
	Logs []LogEntry `json:"logs"`
}

type LogEntry struct {
	CommandID *string    `json:"command_id,omitempty"`
	Source    string     `json:"source"`
	Level     *string    `json:"level,omitempty"`
	Line      string     `json:"line"`
	Seq       int64      `json:"seq"`
	Ts        time.Time  `json:"ts"`
}

func (h *RunnerHandlers) BatchInsertLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := middleware.GetLogger(ctx)
	slug := chi.URLParam(r, "slug")

	var req BatchLogsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Error().Err(err).Msg("failed to decode logs request")
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	logs := make([]models.Log, len(req.Logs))
	for i, entry := range req.Logs {
		var cmdID *pgtype.UUID
		if entry.CommandID != nil {
			var uuid pgtype.UUID
			if err := uuid.Scan(*entry.CommandID); err != nil {
				logger.Error().Err(err).Msg("invalid command_id")
				writeError(w, http.StatusBadRequest, "invalid command_id")
				return
			}
			cmdID = &uuid
		}

		logs[i] = models.Log{
			CommandID:  cmdID,
			RunnerSlug: &slug,
			Source:     entry.Source,
			Level:      entry.Level,
			Line:       entry.Line,
			Seq:        entry.Seq,
			Ts:         entry.Ts,
		}
	}

	if err := h.store.BatchInsertLogs(ctx, logs); err != nil {
		logger.Error().Err(err).Msg("failed to insert logs")
		writeError(w, http.StatusInternalServerError, "failed to insert logs")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type BatchMetricsRequest struct {
	Metrics []MetricSample `json:"metrics"`
}

type MetricSample struct {
	CommandID  *string   `json:"command_id,omitempty"`
	CPUPercent *float64  `json:"cpu_percent,omitempty"`
	MemMB      *float64  `json:"mem_mb,omitempty"`
	GPUPercent *float64  `json:"gpu_percent,omitempty"`
	GPUMemMB   *float64  `json:"gpu_mem_mb,omitempty"`
	SampleTs   time.Time `json:"sample_ts"`
}

func (h *RunnerHandlers) BatchInsertMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := middleware.GetLogger(ctx)
	slug := chi.URLParam(r, "slug")

	var req BatchMetricsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Error().Err(err).Msg("failed to decode metrics request")
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	metrics := make([]models.Metric, len(req.Metrics))
	for i, sample := range req.Metrics {
		var cmdID *pgtype.UUID
		if sample.CommandID != nil {
			var uuid pgtype.UUID
			if err := uuid.Scan(*sample.CommandID); err != nil {
				logger.Error().Err(err).Msg("invalid command_id")
				writeError(w, http.StatusBadRequest, "invalid command_id")
				return
			}
			cmdID = &uuid
		}

		metrics[i] = models.Metric{
			CommandID:  cmdID,
			RunnerSlug: &slug,
			CPUPercent: sample.CPUPercent,
			MemMB:      sample.MemMB,
			GPUPercent: sample.GPUPercent,
			GPUMemMB:   sample.GPUMemMB,
			RolledUp:   false,
			SampleTs:   sample.SampleTs,
		}
	}

	if err := h.store.BatchInsertMetrics(ctx, metrics); err != nil {
		logger.Error().Err(err).Msg("failed to insert metrics")
		writeError(w, http.StatusInternalServerError, "failed to insert metrics")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type ResultRequest struct {
	CommandID    string  `json:"command_id"`
	Status       string  `json:"status"`
	ExitCode     *int    `json:"exit_code,omitempty"`
	ErrorMessage *string `json:"error_message,omitempty"`
}

func (h *RunnerHandlers) ReportResult(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := middleware.GetLogger(ctx)
	slug := chi.URLParam(r, "slug")

	var req ResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Error().Err(err).Msg("failed to decode result request")
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var cmdID pgtype.UUID
	if err := cmdID.Scan(req.CommandID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid command_id")
		return
	}

	now := time.Now()
	opts := store.UpdateCommandOpts{
		ExitCode:     req.ExitCode,
		ErrorMessage: req.ErrorMessage,
		FinishedAt:   &now,
	}

	if err := h.store.UpdateCommandStatus(ctx, cmdID, req.Status, opts); err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			writeError(w, http.StatusNotFound, "command not found")
			return
		}
		logger.Error().Err(err).Msg("failed to update command status")
		writeError(w, http.StatusInternalServerError, "failed to update command status")
		return
	}

	if err := h.store.IncrementRunnerActiveCount(ctx, slug, -1); err != nil {
		logger.Error().Err(err).Msg("failed to decrement active count")
	}

	w.WriteHeader(http.StatusNoContent)
	logger.Info().Str("command_id", req.CommandID).Str("status", req.Status).Msg("result reported")
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
