package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"

	"github.com/branchd-dev/branchd/internal/config"
	"github.com/branchd-dev/branchd/internal/logger"
	"github.com/branchd-dev/branchd/internal/server"
	"github.com/branchd-dev/branchd/internal/tasks"
	"github.com/branchd-dev/branchd/internal/workers"
)

var version = "dev" // Will be set during build with -ldflags

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger.Init(cfg.Logging.Level, cfg.Logging.Format)
	log := logger.GetLogger()

	log.Info().Str("version", version).Msg("Starting Branchd Asynq worker")

	// Initialize database (reuse server's database initialization)
	srv, err := server.New(cfg, log, version)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize server (needed for DB)")
	}
	db := srv.GetDB()

	// Initialize Asynq client (for enqueueing next tasks in chain)
	asynqClient := asynq.NewClient(asynq.RedisClientOpt{
		Addr: cfg.Redis.Address,
	})
	defer asynqClient.Close()

	// Initialize Asynq server
	asynqServer := asynq.NewServer(
		asynq.RedisClientOpt{
			Addr: cfg.Redis.Address,
		},
		asynq.Config{
			Concurrency: 10, // Number of concurrent workers
			Queues: map[string]int{
				"critical": 6, // 60% of workers for critical tasks
				"default":  3, // 30% of workers for default queue
				"low":      1, // 10% of workers for low priority
			},
			// Logging
			Logger: &asynqLogger{log: log},
		},
	)

	// Register task handlers
	mux := asynq.NewServeMux()

	// Restore workflow tasks
	mux.HandleFunc(tasks.TypeTriggerRestore, func(ctx context.Context, t *asynq.Task) error {
		return workers.HandleTriggerRestore(ctx, t, asynqClient, db, cfg, log)
	})
	mux.HandleFunc(tasks.TypeRestoreWaitComplete, func(ctx context.Context, t *asynq.Task) error {
		return workers.HandleRestoreWaitComplete(ctx, t, asynqClient, db, log)
	})

	// Start refresh scheduler goroutine (checks every hour for instances needing refresh)
	go workers.StartRefreshScheduler(asynqClient, db, log)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		log.Info().Msg("Starting Asynq worker server...")
		if err := asynqServer.Run(mux); err != nil {
			log.Fatal().Err(err).Msg("Asynq worker server failed")
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	log.Info().Msg("Received shutdown signal, shutting down gracefully...")

	// Shutdown Asynq server gracefully
	log.Info().Msg("Stopping Asynq worker - waiting for tasks to finish (30s timeout)...")
	asynqServer.Shutdown()

	log.Info().Msg("Worker shutdown complete")
}

// asynqLogger is a wrapper to make zerolog compatible with Asynq's logger interface
type asynqLogger struct {
	log zerolog.Logger
}

func (l *asynqLogger) Debug(args ...interface{}) {
	l.log.Debug().Msg(fmt.Sprint(args...))
}

func (l *asynqLogger) Info(args ...interface{}) {
	l.log.Info().Msg(fmt.Sprint(args...))
}

func (l *asynqLogger) Warn(args ...interface{}) {
	l.log.Warn().Msg(fmt.Sprint(args...))
}

func (l *asynqLogger) Error(args ...interface{}) {
	l.log.Error().Msg(fmt.Sprint(args...))
}

func (l *asynqLogger) Fatal(args ...interface{}) {
	l.log.Fatal().Msg(fmt.Sprint(args...))
}
