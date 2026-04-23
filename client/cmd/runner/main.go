package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/runner/client/internal/config"
	"github.com/runner/client/internal/runner"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "runner",
	Short: "Remote command execution runner client",
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the runner client",
	RunE:  runRunner,
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func main() {
	// Setup logging
	if os.Getenv("ENV") == "production" {
		log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	} else {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	if err := rootCmd.Execute(); err != nil {
		log.Fatal().Err(err).Msg("command failed")
	}
}

func runRunner(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log.Info().
		Str("server_url", cfg.ServerURL).
		Str("runner_slug", cfg.RunnerSlug).
		Int("max_concurrency", cfg.MaxConcurrency).
		Msg("starting runner")

	// Create runner
	rn, err := runner.New(cfg, log.Logger)
	if err != nil {
		return err
	}

	// Setup context with cancellation on signal
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Start runner in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- rn.Start(ctx)
	}()

	// Wait for shutdown signal or error
	select {
	case sig := <-sigCh:
		log.Info().Str("signal", sig.String()).Msg("received shutdown signal")
		cancel()
		// Give runner time to cleanup
		select {
		case <-errCh:
		case <-ctx.Done():
		}
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			log.Error().Err(err).Msg("runner error")
			return err
		}
	}

	log.Info().Msg("runner stopped")
	return nil
}
