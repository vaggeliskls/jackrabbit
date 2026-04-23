package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/runner/server/internal/api/handlers"
	"github.com/runner/server/internal/api/middleware"
	"github.com/runner/server/internal/api/sse"
	"github.com/runner/server/internal/config"
	"github.com/runner/server/internal/store/postgres"
	"github.com/runner/server/internal/workers"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	if cfg.Server.Env == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := postgres.New(ctx, cfg.Postgres.DSN(), cfg.Postgres.MaxConns)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer store.Close()

	log.Info().Msg("database connected")

	hub := sse.NewHub()

	// Start background workers
	workerCtx, workerCancel := context.WithCancel(ctx)
	defer workerCancel()

	reaper := workers.NewReaper(store, log.Logger, 30*time.Second)
	go reaper.Start(workerCtx)

	retention := workers.NewRetention(store, log.Logger, 1*time.Hour)
	go retention.Start(workerCtx)

	rollup := workers.NewRollup(store, log.Logger, 1*time.Minute)
	go rollup.Start(workerCtx)

	log.Info().Msg("background workers started")

	go startListenNotify(ctx, cfg, hub, store)

	r := chi.NewRouter()
	// Apply base middleware without timeout
	r.Use(
		chimiddleware.RequestID,
		middleware.RequestLogger,
		chimiddleware.Recoverer,
	)

	runnerHandlers := handlers.NewRunnerHandlers(store)
	commandHandlers := handlers.NewCommandHandlers(store)
	sseHandler := sse.NewSSEHandler(hub, store)
	authHandler := handlers.NewAuthHandler(log.Logger, cfg.Traefik.APIToken)

	r.Get("/health", handlers.Health)

	// Auth verification endpoint for Traefik ForwardAuth
	authHandler.RegisterRoutes(r)

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/runners", func(r chi.Router) {
			// Apply timeout to non-SSE endpoints
			r.With(chimiddleware.Timeout(30 * time.Second)).Post("/register", runnerHandlers.Register)
			r.With(chimiddleware.Timeout(30 * time.Second)).Get("/", runnerHandlers.List)

			r.Route("/{slug}", func(r chi.Router) {
				r.With(chimiddleware.Timeout(30 * time.Second)).Get("/", runnerHandlers.Get)
				r.With(chimiddleware.Timeout(30 * time.Second)).Delete("/", runnerHandlers.Deregister)
				r.With(chimiddleware.Timeout(30 * time.Second)).Post("/heartbeat", runnerHandlers.Heartbeat)
				// SSE endpoint with NO timeout
				r.Get("/sse", sseHandler.Stream)
				r.With(chimiddleware.Timeout(30 * time.Second)).Post("/logs", runnerHandlers.BatchInsertLogs)
				r.With(chimiddleware.Timeout(30 * time.Second)).Post("/metrics", runnerHandlers.BatchInsertMetrics)
				r.With(chimiddleware.Timeout(30 * time.Second)).Post("/result", runnerHandlers.ReportResult)
			})
		})

		r.Route("/commands", func(r chi.Router) {
			r.With(chimiddleware.Timeout(30 * time.Second)).Post("/", commandHandlers.Send)

			r.Route("/{id}", func(r chi.Router) {
				r.With(chimiddleware.Timeout(30 * time.Second)).Get("/", commandHandlers.Get)
				r.With(chimiddleware.Timeout(30 * time.Second)).Post("/kill", commandHandlers.Kill)
				r.With(chimiddleware.Timeout(30 * time.Second)).Get("/logs", commandHandlers.GetLogs)
				r.With(chimiddleware.Timeout(30 * time.Second)).Get("/metrics", commandHandlers.GetMetrics)
			})
		})
	})

	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // No write timeout for SSE streams
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Info().Str("port", cfg.Server.Port).Msg("server starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Info().Msg("shutdown signal received")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("server shutdown failed")
	}

	cancel()

	log.Info().Msg("server stopped")
}

func startListenNotify(ctx context.Context, cfg *config.Config, hub *sse.Hub, store *postgres.Store) {
	dsn := cfg.Postgres.DSN()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("LISTEN/NOTIFY listener stopped")
			return
		default:
		}

		conn, err := pgx.Connect(ctx, dsn)
		if err != nil {
			log.Error().Err(err).Msg("failed to connect for LISTEN/NOTIFY")
			time.Sleep(5 * time.Second)
			continue
		}

		log.Info().Msg("LISTEN/NOTIFY listener started")

		if _, err := conn.Exec(ctx, "LISTEN runner_notifications"); err != nil {
			log.Error().Err(err).Msg("failed to LISTEN")
			conn.Close(ctx)
			time.Sleep(5 * time.Second)
			continue
		}

		for {
			notification, err := conn.WaitForNotification(ctx)
			if err != nil {
				if ctx.Err() != nil {
					conn.Close(ctx)
					return
				}
				log.Error().Err(err).Msg("notification error")
				break
			}

			go handleNotification(ctx, notification.Payload, hub, store)
		}

		conn.Close(ctx)
		time.Sleep(2 * time.Second)
	}
}

func handleNotification(ctx context.Context, payload string, hub *sse.Hub, store *postgres.Store) {
	parts := splitNotificationPayload(payload)
	if len(parts) != 2 {
		log.Error().Str("payload", payload).Msg("invalid notification payload format")
		return
	}

	slug := parts[0]
	cmdIDStr := parts[1]

	var cmdID pgtype.UUID
	if err := cmdID.Scan(cmdIDStr); err != nil {
		log.Error().Err(err).Str("command_id", cmdIDStr).Msg("invalid command UUID")
		return
	}

	cmd, err := store.GetCommand(ctx, cmdID)
	if err != nil {
		log.Error().Err(err).Msg("failed to get command for notification")
		return
	}

	event := sse.Event{
		Type: "command_dispatch",
		Data: map[string]interface{}{
			"command_id":   cmd.ID,
			"payload":      cmd.Payload,
			"timeout_secs": cmd.TimeoutSecs,
		},
	}

	if hub.Send(slug, event) {
		if err := store.IncrementRunnerActiveCount(ctx, slug, 1); err != nil {
			log.Error().Err(err).Msg("failed to increment active count")
		}
		log.Info().Str("command_id", cmdIDStr).Str("runner", slug).Msg("command dispatched via notification")
	} else {
		log.Debug().Str("command_id", cmdIDStr).Str("runner", slug).Msg("runner offline, command queued")
	}
}

func splitNotificationPayload(payload string) []string {
	for i := 0; i < len(payload); i++ {
		if payload[i] == ':' {
			return []string{payload[:i], payload[i+1:]}
		}
	}
	return []string{payload}
}
