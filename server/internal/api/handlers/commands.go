package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/runner/server/internal/api/middleware"
	"github.com/runner/server/internal/models"
	"github.com/runner/server/internal/store/postgres"
)

type CommandHandlers struct {
	store *postgres.Store
}

func NewCommandHandlers(store *postgres.Store) *CommandHandlers {
	return &CommandHandlers{store: store}
}

type CommandRequest struct {
	TargetType  string         `json:"target_type"`
	TargetValue string         `json:"target_value"`
	Payload     map[string]any `json:"payload"`
	MaxRetries  int            `json:"max_retries"`
	TimeoutSecs int            `json:"timeout_secs"`
	Deadline    *time.Time     `json:"deadline,omitempty"`
}

func (h *CommandHandlers) Send(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := middleware.GetLogger(ctx)

	var req CommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Error().Err(err).Msg("failed to decode request")
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TargetType == "" || req.TargetValue == "" {
		writeError(w, http.StatusBadRequest, "target_type and target_value are required")
		return
	}

	if req.TimeoutSecs == 0 {
		req.TimeoutSecs = 300
	}

	cmd := models.Command{
		TargetType:  req.TargetType,
		TargetValue: req.TargetValue,
		Payload:     req.Payload,
		Status:      "queued",
		MaxRetries:  req.MaxRetries,
		TimeoutSecs: req.TimeoutSecs,
		Deadline:    req.Deadline,
	}

	created, err := h.store.InsertCommand(ctx, cmd)
	if err != nil {
		logger.Error().Err(err).Msg("failed to insert command")
		writeError(w, http.StatusInternalServerError, "failed to create command")
		return
	}

	// Dispatch command to appropriate runner(s)
	if req.TargetType == "slug" || req.TargetType == "runner" {
		// Direct dispatch to specific runner by slug
		if err := h.store.NotifyRunner(ctx, req.TargetValue, created.ID); err != nil {
			logger.Error().Err(err).Msg("failed to notify runner")
		}
	} else if req.TargetType == "tag" {
		// Find least busy runner with matching tag
		runners, err := h.store.ListRunners(ctx, []string{req.TargetValue}, "online")
		if err != nil {
			logger.Error().Err(err).Msg("failed to list runners for tag")
		} else if len(runners) > 0 {
			leastBusy := runners[0]
			for _, r := range runners {
				if r.ActiveCount < leastBusy.ActiveCount {
					leastBusy = r
				}
			}

			if err := h.store.NotifyRunner(ctx, leastBusy.Slug, created.ID); err != nil {
				logger.Error().Err(err).Msg("failed to notify runner")
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(created)

	var cmdIDStr string
	if b, err := created.ID.MarshalJSON(); err == nil {
		cmdIDStr = string(b[1 : len(b)-1])
	}
	logger.Info().
		Str("command_id", cmdIDStr).
		Str("target_type", req.TargetType).
		Str("target_value", req.TargetValue).
		Msg("command created")
}

func (h *CommandHandlers) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := middleware.GetLogger(ctx)
	idStr := chi.URLParam(r, "id")

	var cmdID pgtype.UUID
	if err := cmdID.Scan(idStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid command id")
		return
	}

	cmd, err := h.store.GetCommand(ctx, cmdID)
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			writeError(w, http.StatusNotFound, "command not found")
			return
		}
		logger.Error().Err(err).Msg("failed to get command")
		writeError(w, http.StatusInternalServerError, "failed to retrieve command")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cmd)
}

func (h *CommandHandlers) Kill(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := middleware.GetLogger(ctx)
	idStr := chi.URLParam(r, "id")

	var cmdID pgtype.UUID
	if err := cmdID.Scan(idStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid command id")
		return
	}

	if err := h.store.SetCommandKillRequested(ctx, cmdID); err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			writeError(w, http.StatusNotFound, "command not found or not running")
			return
		}
		logger.Error().Err(err).Msg("failed to set kill requested")
		writeError(w, http.StatusInternalServerError, "failed to kill command")
		return
	}

	w.WriteHeader(http.StatusNoContent)
	logger.Info().Str("command_id", idStr).Msg("kill requested")
}

func (h *CommandHandlers) GetLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := middleware.GetLogger(ctx)
	idStr := chi.URLParam(r, "id")

	var cmdID pgtype.UUID
	if err := cmdID.Scan(idStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid command id")
		return
	}

	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	pageSize := 100
	if sizeStr := r.URL.Query().Get("page_size"); sizeStr != "" {
		if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 && s <= 1000 {
			pageSize = s
		}
	}

	logs, err := h.store.GetLogs(ctx, cmdID, page, pageSize)
	if err != nil {
		logger.Error().Err(err).Msg("failed to get logs")
		writeError(w, http.StatusInternalServerError, "failed to retrieve logs")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

func (h *CommandHandlers) GetMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := middleware.GetLogger(ctx)
	idStr := chi.URLParam(r, "id")

	var cmdID pgtype.UUID
	if err := cmdID.Scan(idStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid command id")
		return
	}

	resolution := r.URL.Query().Get("resolution")
	if resolution == "" {
		resolution = "raw"
	}

	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	pageSize := 100
	if sizeStr := r.URL.Query().Get("page_size"); sizeStr != "" {
		if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 && s <= 1000 {
			pageSize = s
		}
	}

	metrics, err := h.store.GetMetrics(ctx, cmdID, resolution, page, pageSize)
	if err != nil {
		logger.Error().Err(err).Msg("failed to get metrics")
		writeError(w, http.StatusInternalServerError, "failed to retrieve metrics")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}
