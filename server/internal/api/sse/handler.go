package sse

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/runner/server/internal/api/middleware"
	"github.com/runner/server/internal/store/postgres"
)

type SSEHandler struct {
	hub   *Hub
	store *postgres.Store
}

func NewSSEHandler(hub *Hub, store *postgres.Store) *SSEHandler {
	return &SSEHandler{
		hub:   hub,
		store: store,
	}
}

func (h *SSEHandler) Stream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := middleware.GetLogger(ctx)
	slug := chi.URLParam(r, "slug")

	if _, err := h.store.GetRunner(ctx, slug); err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			http.Error(w, "runner not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Msg("failed to get runner")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		logger.Error().Msg("streaming not supported")
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	eventCh := h.hub.Register(slug)
	defer h.hub.Unregister(slug, eventCh)

	pingTicker := time.NewTicker(15 * time.Second)
	defer pingTicker.Stop()

	go h.flushQueuedCommands(ctx, slug)

	logger.Info().Str("slug", slug).Msg("SSE stream started")

	for {
		select {
		case <-ctx.Done():
			logger.Info().Str("slug", slug).Msg("SSE stream closed by client")
			return

		case event := <-eventCh:
			data, err := FormatSSE(event)
			if err != nil {
				logger.Error().Err(err).Msg("failed to format SSE event")
				continue
			}

			if _, err := w.Write(data); err != nil {
				logger.Error().Err(err).Msg("failed to write SSE event")
				return
			}
			flusher.Flush()

		case <-pingTicker.C:
			pingEvent := Event{Type: "ping", Data: map[string]interface{}{}}
			data, _ := FormatSSE(pingEvent)
			if _, err := w.Write(data); err != nil {
				logger.Error().Err(err).Msg("failed to write ping")
				return
			}
			flusher.Flush()
		}
	}
}

func (h *SSEHandler) flushQueuedCommands(ctx context.Context, slug string) {
	logger := middleware.GetLogger(ctx)

	runner, err := h.store.GetRunner(ctx, slug)
	if err != nil {
		logger.Error().Err(err).Msg("failed to get runner for flush")
		return
	}

	cmd, err := h.store.ClaimNextCommand(ctx, slug, runner.Tags)
	if err != nil {
		if !errors.Is(err, postgres.ErrNotFound) {
			logger.Error().Err(err).Msg("failed to claim command")
		}
		return
	}

	if err := h.store.IncrementRunnerActiveCount(ctx, slug, 1); err != nil {
		logger.Error().Err(err).Msg("failed to increment active count")
	}

	event := Event{
		Type: "command_dispatch",
		Data: map[string]interface{}{
			"command_id":   cmd.ID,
			"payload":      cmd.Payload,
			"timeout_secs": cmd.TimeoutSecs,
		},
	}

	h.hub.Send(slug, event)
	
	var cmdIDStr string
	if b, err := cmd.ID.MarshalJSON(); err == nil {
		cmdIDStr = string(b[1 : len(b)-1])
	}
	logger.Info().Str("slug", slug).Str("command_id", cmdIDStr).Msg("flushed queued command")
}
